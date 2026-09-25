package config

import (
	"strings"
	"testing"
)

const testAPIToken = "0123456789abcdef0123456789abcdef"

func TestLoad_auth(t *testing.T) {
	tests := []struct {
		name         string
		token        string
		insecure     string
		wantErr      string
		wantInsecure bool
	}{
		{name: "token set", token: testAPIToken},
		{name: "no token refuses to boot", wantErr: "NIB_API_TOKEN is required"},
		{name: "no token with explicit opt-out", insecure: "true", wantInsecure: true},
		{name: "opt-out set to false still refuses", insecure: "false", wantErr: "NIB_API_TOKEN is required"},
		{name: "typo in opt-out is an error", insecure: "ture", wantErr: "NIB_INSECURE_NO_AUTH must be true or false"},
		{name: "short token", token: "short", wantErr: "at least 32 characters"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LLM_API_KEY", "test-key")
			t.Setenv("NIB_API_TOKEN", tt.token)
			t.Setenv("NIB_INSECURE_NO_AUTH", tt.insecure)
			path := writeConfigFile(t, "")

			cfg, err := Load(path)
			if err == nil {
				err = cfg.Auth.ValidateServing()
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Load error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.Auth.APIToken != tt.token {
				t.Errorf("APIToken = %q, want %q", cfg.Auth.APIToken, tt.token)
			}
			if cfg.Auth.InsecureNoAuth != tt.wantInsecure {
				t.Errorf("InsecureNoAuth = %v, want %v", cfg.Auth.InsecureNoAuth, tt.wantInsecure)
			}
		})
	}
}

func TestLoad_authOriginsAndHosts(t *testing.T) {
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("NIB_ALLOWED_ORIGINS", "")
	t.Setenv("NIB_ALLOWED_HOSTS", "")
	path := writeConfigFile(t, "")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := strings.Join(cfg.Auth.AllowedOrigins, ","); got != defaultAllowedOrigin {
		t.Errorf("default AllowedOrigins = %q, want %q", got, defaultAllowedOrigin)
	}
	if len(cfg.Auth.AllowedHosts) != 0 {
		t.Errorf("default AllowedHosts = %v, want empty", cfg.Auth.AllowedHosts)
	}

	t.Setenv("NIB_ALLOWED_ORIGINS", " https://nib.example.com/ , ,http://localhost:5173")
	t.Setenv("NIB_ALLOWED_HOSTS", "nib.example.com, localhost:8080")
	cfg, err = Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := strings.Join(cfg.Auth.AllowedOrigins, ","), "https://nib.example.com,http://localhost:5173"; got != want {
		t.Errorf("AllowedOrigins = %q, want %q", got, want)
	}
	if got, want := strings.Join(cfg.Auth.AllowedHosts, ","), "nib.example.com,localhost:8080"; got != want {
		t.Errorf("AllowedHosts = %q, want %q", got, want)
	}

	// http.CrossOriginProtection refuses anything but scheme://host[:port],
	// so a malformed entry has to stop the boot rather than be dropped later.
	t.Setenv("NIB_ALLOWED_ORIGINS", "nib.example.com/app")
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "NIB_ALLOWED_ORIGINS") {
		t.Fatalf("Load error = %v, want one naming NIB_ALLOWED_ORIGINS", err)
	}
}

// The toolcatalog CLI loads the same config and serves nothing, so Load itself
// must not demand a token.
func TestLoad_noTokenLoadsForNonServingCallers(t *testing.T) {
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("NIB_API_TOKEN", "")
	t.Setenv("NIB_INSECURE_NO_AUTH", "")

	cfg, err := Load(writeConfigFile(t, ""))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := cfg.Auth.ValidateServing(); err == nil {
		t.Fatal("ValidateServing = nil with no token and no opt-out, want an error")
	}
}
