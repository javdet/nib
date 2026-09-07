package hostenv

import (
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
)

// probe is every impure dependency of detect, so a test can drive detection off
// a temporary directory instead of the machine running it. Nothing here starts a
// subprocess: a probe runs on the boot path and must not be able to hang.
type probe struct {
	// root prefixes every absolute path the probe reads. "" is the real
	// filesystem root; a test points it at t.TempDir().
	root string
	// goos is runtime.GOOS, injected so the host-versus-container fallback can
	// be tested for a platform other than the one running the test.
	goos     string
	getenv   func(string) string
	lookPath func(string) (string, error)
	hostname func() (string, error)
	getwd    func() (string, error)
	geteuid  func() int
	username func(uid int) (string, error)
}

func systemProbe() probe {
	return probe{
		goos:     runtime.GOOS,
		getenv:   os.Getenv,
		lookPath: exec.LookPath,
		hostname: os.Hostname,
		getwd:    os.Getwd,
		geteuid:  os.Geteuid,
		username: func(uid int) (string, error) {
			u, err := user.LookupId(strconv.Itoa(uid))
			if err != nil {
				return "", err
			}
			return u.Username, nil
		},
	}
}

func (p probe) path(abs string) string {
	if p.root == "" {
		return abs
	}
	return filepath.Join(p.root, abs)
}

// read returns the file contents, or "" for anything that goes wrong. Every
// caller treats an unreadable probe file and an absent one the same way.
func (p probe) read(abs string) string {
	data, err := os.ReadFile(p.path(abs))
	if err != nil {
		return ""
	}
	return string(data)
}

func (p probe) exists(abs string) bool {
	_, err := os.Stat(p.path(abs))
	return err == nil
}
