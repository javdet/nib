//go:build !linux

package cmdrun

import (
	"errors"
	"syscall"
	"time"
)

type hostCaps struct {
	canSwitch   bool
	ambientCaps bool
	reason      string
}

// probeHost never isolates off Linux: the backend only ships in Linux images,
// and this file exists so the package still builds on a developer's machine.
func probeHost() hostCaps {
	return hostCaps{reason: "uid isolation is implemented for Linux only"}
}

func sysProcAttr(int) *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

func signalUID(int, syscall.Signal) (int, error) {
	return 0, errors.New("signalling by uid is implemented for Linux only")
}

func killUID(int, time.Duration) {}

func killUIDRange(int, int) {}

func ownerUID(string) int { return -1 }
