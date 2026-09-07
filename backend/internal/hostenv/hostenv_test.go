package hostenv

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// fakeRoot builds a filesystem the probe can read: keys are absolute paths, a
// value of "" creates a directory instead of a file.
func fakeRoot(t *testing.T, files map[string]string) string {
	t.Helper()

	root := t.TempDir()
	for abs, content := range files {
		path := filepath.Join(root, abs)
		if content == "" {
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", path, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return root
}

func testProbe(t *testing.T, root string, env map[string]string) probe {
	t.Helper()

	return probe{
		root: root,
		goos: "linux",
		getenv: func(name string) string {
			return env[name]
		},
		lookPath: func(string) (string, error) { return "", errors.New("not found") },
		hostname: func() (string, error) { return "nib-test", nil },
		getwd:    func() (string, error) { return "/app", nil },
		geteuid:  func() int { return 0 },
		username: func(int) (string, error) { return "root", nil },
	}
}

func TestContainerRuntime(t *testing.T) {
	t.Parallel()

	const saDir = "/var/run/secrets/kubernetes.io/serviceaccount"

	tests := []struct {
		name          string
		goos          string
		files         map[string]string
		env           map[string]string
		wantRuntime   Runtime
		wantNamespace string
	}{
		{
			name:        "kubernetes from the service host env var",
			env:         map[string]string{"KUBERNETES_SERVICE_HOST": "10.96.0.1"},
			wantRuntime: RuntimeKubernetes,
		},
		{
			name: "kubernetes from the service account token, with a namespace",
			files: map[string]string{
				saDir + "/token":     "jwt",
				saDir + "/namespace": "nib\n",
			},
			wantRuntime:   RuntimeKubernetes,
			wantNamespace: "nib",
		},
		{
			name:        "kubernetes with an unreadable namespace file",
			files:       map[string]string{saDir + "/token": "jwt"},
			wantRuntime: RuntimeKubernetes,
		},
		{
			name:        "kubernetes wins over the docker marker",
			files:       map[string]string{saDir + "/token": "jwt", "/.dockerenv": "x"},
			wantRuntime: RuntimeKubernetes,
		},
		{
			name:        "docker from the dockerenv marker",
			files:       map[string]string{"/.dockerenv": "x"},
			wantRuntime: RuntimeDocker,
		},
		{
			name:        "podman from the containerenv marker",
			files:       map[string]string{"/run/.containerenv": "x"},
			wantRuntime: RuntimeContainer,
		},
		{
			name:        "kubernetes from the cgroup hierarchy",
			files:       map[string]string{"/proc/1/cgroup": "11:memory:/kubepods/besteffort/pod123\n"},
			wantRuntime: RuntimeKubernetes,
		},
		{
			name:        "docker from the cgroup hierarchy",
			files:       map[string]string{"/proc/1/cgroup": "11:memory:/docker/deadbeef\n"},
			wantRuntime: RuntimeDocker,
		},
		{
			name:        "containerd from the cgroup hierarchy",
			files:       map[string]string{"/proc/1/cgroup": "11:memory:/containerd/deadbeef\n"},
			wantRuntime: RuntimeContainer,
		},
		{
			// A cgroup v2 host answers this for every process, container or not,
			// so it must not be read as a container signal.
			name:        "cgroup v2 falls through to the host",
			files:       map[string]string{"/proc/1/cgroup": "0::/\n"},
			wantRuntime: RuntimeHost,
		},
		{
			name:        "plain linux host",
			files:       map[string]string{"/proc/1": ""},
			wantRuntime: RuntimeHost,
		},
		{
			name:        "darwin is always a host",
			goos:        "darwin",
			wantRuntime: RuntimeHost,
		},
		{
			name:        "nothing to go on",
			wantRuntime: RuntimeUnknown,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := testProbe(t, fakeRoot(t, tt.files), tt.env)
			if tt.goos != "" {
				p.goos = tt.goos
			}

			gotRuntime, gotNamespace := containerRuntime(p)
			if gotRuntime != tt.wantRuntime {
				t.Fatalf("containerRuntime() runtime = %q, want %q", gotRuntime, tt.wantRuntime)
			}
			if gotNamespace != tt.wantNamespace {
				t.Fatalf("containerRuntime() namespace = %q, want %q", gotNamespace, tt.wantNamespace)
			}
		})
	}
}

func TestDistro(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		release string
		want    string
	}{
		{
			name:    "quoted pretty name",
			release: "ID=debian\nPRETTY_NAME=\"Debian GNU/Linux 12 (bookworm)\"\n",
			want:    "Debian GNU/Linux 12 (bookworm)",
		},
		{
			name:    "unquoted pretty name",
			release: "PRETTY_NAME=Alpine Linux v3.20\n",
			want:    "Alpine Linux v3.20",
		},
		{
			name:    "falls back to name and version",
			release: "NAME=\"Ubuntu\"\nVERSION_ID=\"24.04\"\n",
			want:    "Ubuntu 24.04",
		},
		{
			name:    "falls back to name alone",
			release: "NAME=Ubuntu\n",
			want:    "Ubuntu",
		},
		{
			name:    "comments and blank lines are skipped",
			release: "# a comment\n\nPRETTY_NAME='Fedora 40'\n",
			want:    "Fedora 40",
		},
		{
			name:    "garbage yields nothing",
			release: "not-an-assignment\n",
			want:    "",
		},
		{
			name: "missing file yields nothing",
			want: "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			files := map[string]string{}
			if tt.release != "" {
				files["/etc/os-release"] = tt.release
			}

			if got := distro(testProbe(t, fakeRoot(t, files), nil)); got != tt.want {
				t.Fatalf("distro() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestShell(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		env   map[string]string
		files map[string]string
		want  string
	}{
		{
			name: "SHELL wins",
			env:  map[string]string{"SHELL": "/usr/bin/zsh"},
			want: "/usr/bin/zsh",
		},
		{
			name:  "falls back to bash on disk",
			files: map[string]string{"/bin/bash": "elf", "/bin/sh": "elf"},
			want:  "/bin/bash",
		},
		{
			name:  "falls back to sh when bash is absent",
			files: map[string]string{"/bin/sh": "elf"},
			want:  "/bin/sh",
		},
		{
			name: "no shell at all",
			want: "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := shell(testProbe(t, fakeRoot(t, tt.files), tt.env)); got != tt.want {
				t.Fatalf("shell() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOnPathKeepsOnlyWhatResolves(t *testing.T) {
	t.Parallel()

	present := map[string]struct{}{"jq": {}, "curl": {}, "bash": {}}
	p := testProbe(t, t.TempDir(), nil)
	p.lookPath = func(name string) (string, error) {
		if _, ok := present[name]; ok {
			return "/usr/bin/" + name, nil
		}
		return "", errors.New("not found")
	}

	want := []string{"bash", "curl", "jq"}
	if got := onPath(p); !reflect.DeepEqual(got, want) {
		t.Fatalf("onPath() = %v, want %v", got, want)
	}
}

// A probe where every call fails is the shape of a hostile container: the
// snapshot must still come back usable rather than panic on the boot path.
func TestDetectSurvivesAFailingProbe(t *testing.T) {
	t.Parallel()

	failing := probe{
		root:     t.TempDir(),
		goos:     "linux",
		getenv:   func(string) string { return "" },
		lookPath: func(string) (string, error) { return "", errors.New("no PATH") },
		hostname: func() (string, error) { return "", errors.New("no hostname") },
		getwd:    func() (string, error) { return "", errors.New("no cwd") },
		geteuid:  func() int { return 1000 },
		username: func(int) (string, error) { return "", errors.New("no passwd") },
	}

	info := detect(failing)

	if info.Hostname != "" || info.WorkDir != "" || info.User != "" {
		t.Fatalf("failing probe leaked values: %+v", info)
	}
	if info.Runtime != RuntimeUnknown {
		t.Fatalf("Runtime = %q, want %q", info.Runtime, RuntimeUnknown)
	}
	if len(info.OnPath) != 0 {
		t.Fatalf("OnPath = %v, want empty", info.OnPath)
	}
	if got := info.DescribeUser(); got != "uid 1000" {
		t.Fatalf("DescribeUser() = %q, want %q", got, "uid 1000")
	}
	if got := info.DescribeOS(); got != "linux/"+runtime.GOARCH {
		t.Fatalf("DescribeOS() = %q, want the platform alone", got)
	}
	if got := info.DescribeCommands(); !strings.Contains(got, "none") {
		t.Fatalf("DescribeCommands() = %q, want it to say none", got)
	}
}

func TestDescribe(t *testing.T) {
	t.Parallel()

	full := Info{
		GOOS: "linux", GOARCH: "arm64",
		Distro: "Debian GNU/Linux 12 (bookworm)",
		Kernel: "6.10.14-linuxkit",
		User:   "root", UID: 0,
		Runtime: RuntimeKubernetes, KubeNamespace: "nib",
		OnPath: []string{"bash", "curl"},
	}

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"os", full.DescribeOS(), "Debian GNU/Linux 12 (bookworm), linux/arm64, kernel 6.10.14-linuxkit"},
		{"runtime", full.DescribeRuntime(), "Kubernetes pod (namespace nib)"},
		{"user", full.DescribeUser(), "root (uid 0)"},
		{"commands", full.DescribeCommands(), "bash, curl"},
		{"kubernetes without a namespace", Info{Runtime: RuntimeKubernetes}.DescribeRuntime(), "Kubernetes pod"},
		{"docker", Info{Runtime: RuntimeDocker}.DescribeRuntime(), "Docker container"},
		{"bare host", Info{Runtime: RuntimeHost}.DescribeRuntime(), "bare host, not containerised"},
		{"unknown runtime", Info{}.DescribeRuntime(), "unknown"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.got != tt.want {
				t.Fatalf("= %q, want %q", tt.got, tt.want)
			}
		})
	}
}

// Detect runs against the real machine, so it can only assert the invariants
// that hold everywhere: the platform is known and nothing panics.
func TestDetectOnThisMachine(t *testing.T) {
	t.Parallel()

	info := Detect()
	if info.GOOS != runtime.GOOS || info.GOARCH != runtime.GOARCH {
		t.Fatalf("Detect() platform = %s/%s, want %s/%s", info.GOOS, info.GOARCH, runtime.GOOS, runtime.GOARCH)
	}
	if info.Runtime == "" {
		t.Fatal("Detect() left Runtime empty")
	}
	if info.DescribeOS() == "" {
		t.Fatal("DescribeOS() is empty")
	}
}
