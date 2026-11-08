//go:build linux

package cmdrun

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	capKill   = 5
	capSetgid = 6
	capSetuid = 7
)

type hostCaps struct {
	canSwitch   bool
	ambientCaps bool
	reason      string
}

// probeHost reads the backend's own capability sets. Switching a child's uid
// needs CAP_SETUID and CAP_SETGID, and signalling it afterwards needs
// CAP_KILL, since the job then belongs to someone else.
func probeHost() hostCaps {
	eff, amb, ok := readCaps("/proc/self/status")
	if !ok {
		return hostCaps{reason: "cannot read /proc/self/status"}
	}
	h := hostCaps{ambientCaps: amb != 0}
	if os.Geteuid() == 0 {
		h.canSwitch = true
		return h
	}
	var missing []string
	for _, c := range []struct {
		bit  uint
		name string
	}{{capSetuid, "CAP_SETUID"}, {capSetgid, "CAP_SETGID"}, {capKill, "CAP_KILL"}} {
		if eff&(1<<c.bit) == 0 {
			missing = append(missing, c.name)
		}
	}
	if len(missing) > 0 {
		h.reason = "the backend is not root and lacks " + strings.Join(missing, ", ")
		return h
	}
	h.canSwitch = true
	return h
}

func readCaps(path string) (eff, amb uint64, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()
	var sawEff bool
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		key, val, found := strings.Cut(sc.Text(), ":")
		if !found {
			continue
		}
		val = strings.TrimSpace(val)
		switch key {
		case "CapEff":
			if v, err := strconv.ParseUint(val, 16, 64); err == nil {
				eff, sawEff = v, true
			}
		case "CapAmb":
			if v, err := strconv.ParseUint(val, 16, 64); err == nil {
				amb = v
			}
		}
	}
	return eff, amb, sawEff
}

// sysProcAttr puts the child in a process group of its own and, for an
// isolated job, under its uid with no supplementary groups -- which is what
// drops the Docker socket's group the backend itself holds.
func sysProcAttr(uid int) *syscall.SysProcAttr {
	attr := &syscall.SysProcAttr{Setpgid: true}
	if uid >= 0 {
		attr.Credential = &syscall.Credential{Uid: uint32(uid), Gid: uint32(uid), Groups: []uint32{}}
	}
	return attr
}

// uidProcs lists the live processes whose real uid is in [lo, hi]. Zombies are
// left out: they are already dead, and only their parent can clear them.
func uidProcs(lo, hi int) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	self := os.Getpid()
	var pids []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == self {
			continue
		}
		uid, zombie, ok := procUID(filepath.Join("/proc", e.Name(), "status"))
		if !ok || zombie || uid < lo || uid > hi {
			continue
		}
		pids = append(pids, pid)
	}
	return pids
}

func procUID(path string) (uid int, zombie, ok bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, val, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		switch key {
		case "State":
			zombie = strings.HasPrefix(strings.TrimSpace(val), "Z")
		case "Uid":
			fields := strings.Fields(val)
			if len(fields) == 0 {
				return 0, false, false
			}
			uid, err = strconv.Atoi(fields[0])
			if err != nil {
				return 0, false, false
			}
			ok = true
		}
	}
	return uid, zombie, ok
}

// signalUID sends sig to every live process running as uid.
func signalUID(uid int, sig syscall.Signal) (int, error) {
	n := 0
	var firstErr error
	for _, pid := range uidProcs(uid, uid) {
		if err := syscall.Kill(pid, sig); err != nil {
			if err != syscall.ESRCH && firstErr == nil {
				firstErr = err
			}
			continue
		}
		n++
	}
	return n, firstErr
}

// killUID SIGKILLs uid's processes until none is left or the timeout passes. A
// fork racing the scan is caught on the next pass.
func killUID(uid int, timeout time.Duration) {
	killUIDRangeUntil(uid, uid, timeout)
}

func killUIDRange(lo, hi int) {
	killUIDRangeUntil(lo, hi, 5*time.Second)
}

func killUIDRangeUntil(lo, hi int, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for {
		pids := uidProcs(lo, hi)
		if len(pids) == 0 || time.Now().After(deadline) {
			return
		}
		for _, pid := range pids {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func ownerUID(path string) int {
	fi, err := os.Lstat(path)
	if err != nil {
		return -1
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return int(st.Uid)
	}
	return -1
}
