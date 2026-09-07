// Package hostenv snapshots the facts about the process the backend runs in, so
// a system prompt can tell the model what it may actually execute here.
package hostenv

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
)

// Runtime is where the backend process itself runs, as detected at startup. It
// is deliberately a distinct type from executor.Platform: that one is the
// operator's choice of where planned actions are executed, which is a different
// machine with a different toolchain, and conflating the two would tell the
// model it can run things it cannot.
type Runtime string

const (
	RuntimeKubernetes Runtime = "kubernetes"
	RuntimeDocker     Runtime = "docker"
	// RuntimeContainer is a container whose flavour we could not name (podman,
	// plain OCI, LXC).
	RuntimeContainer Runtime = "container"
	RuntimeHost      Runtime = "host"
	RuntimeUnknown   Runtime = "unknown"
)

// probeBinaries is what PATH is checked for. It is a fixed list rather than a
// config knob: the point is to answer "can I run this here?" for the handful of
// tools an infrastructure agent reaches for, and the runtime image deliberately
// ships almost none of them.
var probeBinaries = []string{
	"ansible", "aws", "az", "bash", "curl", "dig", "docker", "gcloud", "git",
	"helm", "jq", "kubectl", "nc", "openssl", "psql", "python3", "sh", "ssh",
	"terraform",
}

// Info is a snapshot of the process environment. Every field degrades to its
// zero value when the probe behind it fails, and the Describe* helpers omit an
// empty field rather than print "unknown".
type Info struct {
	GOOS   string
	GOARCH string
	// Distro is /etc/os-release PRETTY_NAME. Empty off Linux.
	Distro string
	// Kernel is /proc/sys/kernel/osrelease. Empty off Linux.
	Kernel        string
	Hostname      string
	Shell         string
	WorkDir       string
	User          string
	UID           int
	Runtime       Runtime
	KubeNamespace string
	// OnPath is the sorted subset of probeBinaries found on PATH.
	OnPath []string
}

// Detect builds a snapshot of the current process environment. It never returns
// an error: a prompt one line shorter beats a chat turn that fails, so a probe
// that cannot answer leaves its field empty.
func Detect() Info {
	return detect(systemProbe())
}

func detect(p probe) Info {
	info := Info{
		GOOS:   p.goos,
		GOARCH: runtime.GOARCH,
		UID:    p.geteuid(),
	}
	info.Hostname, _ = p.hostname()
	info.WorkDir, _ = p.getwd()
	info.Distro = distro(p)
	info.Kernel = firstLine(p.read("/proc/sys/kernel/osrelease"))
	info.Shell = shell(p)
	info.User, _ = p.username(info.UID)
	info.Runtime, info.KubeNamespace = containerRuntime(p)
	info.OnPath = onPath(p)
	return info
}

// DescribeOS reads as an operator would write it: distribution first, then the
// platform the binary was built for, then the kernel.
func (i Info) DescribeOS() string {
	parts := make([]string, 0, 3)
	if i.Distro != "" {
		parts = append(parts, i.Distro)
	}
	if i.GOOS != "" {
		platform := i.GOOS
		if i.GOARCH != "" {
			platform += "/" + i.GOARCH
		}
		parts = append(parts, platform)
	}
	if i.Kernel != "" {
		parts = append(parts, "kernel "+i.Kernel)
	}
	return strings.Join(parts, ", ")
}

// DescribeRuntime names the container the process sits in, because that decides
// what a local command can reach.
func (i Info) DescribeRuntime() string {
	switch i.Runtime {
	case RuntimeKubernetes:
		if i.KubeNamespace != "" {
			return fmt.Sprintf("Kubernetes pod (namespace %s)", i.KubeNamespace)
		}
		return "Kubernetes pod"
	case RuntimeDocker:
		return "Docker container"
	case RuntimeContainer:
		return "container (not Docker or Kubernetes)"
	case RuntimeHost:
		return "bare host, not containerised"
	default:
		return "unknown"
	}
}

// DescribeUser carries the uid as well as the name: root is the fact that
// decides whether a command will be permitted, and the name alone can hide it.
func (i Info) DescribeUser() string {
	if i.User == "" {
		return fmt.Sprintf("uid %d", i.UID)
	}
	return fmt.Sprintf("%s (uid %d)", i.User, i.UID)
}

// DescribeCommands lists what is on PATH. An empty list is said out loud rather
// than rendered as a blank, so the model does not read it as "unknown".
func (i Info) DescribeCommands() string {
	if len(i.OnPath) == 0 {
		return "none of the usual infrastructure CLIs"
	}
	return strings.Join(i.OnPath, ", ")
}

// distro prefers PRETTY_NAME and falls back to NAME plus VERSION_ID, which is
// what os-release guarantees when PRETTY_NAME is absent.
func distro(p probe) string {
	fields := parseOSRelease(p.read("/etc/os-release"))
	if pretty := fields["PRETTY_NAME"]; pretty != "" {
		return pretty
	}
	name := fields["NAME"]
	if name == "" {
		return ""
	}
	if version := fields["VERSION_ID"]; version != "" {
		return name + " " + version
	}
	return name
}

func parseOSRelease(content string) map[string]string {
	fields := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		fields[strings.TrimSpace(key)] = unquote(strings.TrimSpace(value))
	}
	return fields
}

func unquote(value string) string {
	if len(value) < 2 {
		return value
	}
	first, last := value[0], value[len(value)-1]
	if first == last && (first == '"' || first == '\'') {
		return value[1 : len(value)-1]
	}
	return value
}

// shell reports the login shell. $SHELL is what the operator's own tooling
// would use; the fallbacks matter because a slim image sets no SHELL at all.
func shell(p probe) string {
	if s := strings.TrimSpace(p.getenv("SHELL")); s != "" {
		return s
	}
	for _, candidate := range []string{"/bin/bash", "/bin/sh", "/bin/ash"} {
		if p.exists(candidate) {
			return candidate
		}
	}
	return ""
}

// containerRuntime answers where the process runs, first match winning.
func containerRuntime(p probe) (Runtime, string) {
	// The same signal rest.InClusterConfig() uses, so the snapshot never claims
	// Kubernetes where in-cluster auth would fail, or the reverse.
	const saDir = "/var/run/secrets/kubernetes.io/serviceaccount"
	if strings.TrimSpace(p.getenv("KUBERNETES_SERVICE_HOST")) != "" || p.exists(saDir+"/token") {
		return RuntimeKubernetes, strings.TrimSpace(p.read(saDir + "/namespace"))
	}
	if p.exists("/.dockerenv") {
		return RuntimeDocker, ""
	}
	if p.exists("/run/.containerenv") {
		return RuntimeContainer, ""
	}
	// cgroup v1 names the runtime in the hierarchy paths. A cgroup v2 host
	// answers a bare "0::/", which matches nothing here and correctly falls
	// through to the host check below.
	if cgroup := p.read("/proc/1/cgroup"); cgroup != "" {
		switch {
		case strings.Contains(cgroup, "kubepods"):
			return RuntimeKubernetes, ""
		case strings.Contains(cgroup, "docker"):
			return RuntimeDocker, ""
		case strings.Contains(cgroup, "containerd"),
			strings.Contains(cgroup, "libpod"),
			strings.Contains(cgroup, "lxc"):
			return RuntimeContainer, ""
		}
	}
	if p.goos == "darwin" || p.exists("/proc/1") {
		return RuntimeHost, ""
	}
	return RuntimeUnknown, ""
}

func onPath(p probe) []string {
	found := make([]string, 0, len(probeBinaries))
	for _, name := range probeBinaries {
		if _, err := p.lookPath(name); err == nil {
			found = append(found, name)
		}
	}
	sort.Strings(found)
	return found
}

func firstLine(content string) string {
	line, _, _ := strings.Cut(content, "\n")
	return strings.TrimSpace(line)
}
