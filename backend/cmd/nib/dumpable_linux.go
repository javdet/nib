//go:build linux

package main

import (
	"fmt"
	"syscall"
)

// disableDumpable clears the process's dumpable flag. execute_command and api_call run their
// children under the backend's own uid with a scrubbed environment, but a same-uid child could
// still read the backend's /proc/<pid>/environ (LLM key, encryption key, DB password, API token)
// and ptrace it. A non-dumpable process hands its /proc entries to root and refuses both to anyone
// without CAP_SYS_PTRACE, which the entrypoint drops. Children are unaffected: execve resets the
// flag. It closes the environment leak only; docker.sock and DATA_DIR stay reachable.
func disableDumpable() error {
	if _, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, syscall.PR_SET_DUMPABLE, 0, 0); errno != 0 {
		return fmt.Errorf("prctl(PR_SET_DUMPABLE, 0): %w", errno)
	}
	return nil
}
