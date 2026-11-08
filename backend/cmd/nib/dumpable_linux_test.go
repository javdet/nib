//go:build linux

package main

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
)

func TestDisableDumpable(t *testing.T) {
	if err := disableDumpable(); err != nil {
		t.Fatalf("disableDumpable() = %v", err)
	}

	got, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, syscall.PR_GET_DUMPABLE, 0, 0)
	if errno != 0 {
		t.Fatalf("prctl(PR_GET_DUMPABLE) = %v", errno)
	}
	if got != 0 {
		t.Fatalf("PR_GET_DUMPABLE = %d, want 0", got)
	}

	// The point of the flag: a child under the same uid must not read our environment.
	environ := "/proc/" + strconv.Itoa(os.Getpid()) + "/environ"
	out, err := exec.Command("cat", environ).CombinedOutput()
	if err == nil {
		if os.Geteuid() == 0 {
			t.Skip("running as root, possibly with CAP_SYS_PTRACE, which bypasses the flag")
		}
		t.Fatalf("child read %s (%d bytes), want permission denied", environ, len(out))
	}
}
