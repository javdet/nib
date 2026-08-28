package version

import (
	"os"
	"testing"
)

func TestVersion_Default(t *testing.T) {
	t.Setenv("APP_VERSION", "")
	if got := Version(); got != "dev" {
		t.Fatalf("Version() = %q, want dev", got)
	}
}

func TestVersion_FromEnv(t *testing.T) {
	t.Setenv("APP_VERSION", "v0.7.5")
	if got := Version(); got != "v0.7.5" {
		t.Fatalf("Version() = %q, want v0.7.5", got)
	}
}

func TestVersion_EnvOverridesInjectedValue(t *testing.T) {
	t.Setenv("APP_VERSION", "v9.9.9")
	_ = os.Getenv("APP_VERSION")
	if got := Version(); got != "v9.9.9" {
		t.Fatalf("Version() = %q, want v9.9.9", got)
	}
}
