package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeConfigFile writes a YAML config with the LLM block Load requires.
func writeConfigFile(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	full := `llm:
  baseURL: https://api.openai.com/v1
  model: gpt-5.6
` + body
	if err := os.WriteFile(path, []byte(full), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoad_metricsDefaults(t *testing.T) {
	t.Setenv("LLM_API_KEY", "test-key")
	path := writeConfigFile(t, "")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// A config with no metrics block has to come up enabled: the pointer in
	// fileMetricsConfig exists precisely so "absent" and "false" differ.
	if !cfg.Metrics.Enabled {
		t.Error("Enabled = false, want true when no metrics block is present")
	}
	if got, want := cfg.Metrics.Host, defaultMetricsHost; got != want {
		t.Errorf("Host = %q, want %q", got, want)
	}
	if got, want := cfg.Metrics.Port, defaultMetricsPort; got != want {
		t.Errorf("Port = %d, want %d", got, want)
	}
	if got, want := cfg.Metrics.Path, defaultMetricsPath; got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
	if got, want := cfg.Metrics.RefreshSeconds, defaultMetricsRefreshSeconds; got != want {
		t.Errorf("RefreshSeconds = %d, want %d", got, want)
	}
	if got, want := cfg.Metrics.RefreshInterval().Seconds(), float64(defaultMetricsRefreshSeconds); got != want {
		t.Errorf("RefreshInterval() = %v, want %v", got, want)
	}
}

func TestLoad_metricsFromYAML(t *testing.T) {
	t.Setenv("LLM_API_KEY", "test-key")
	path := writeConfigFile(t, `metrics:
  enabled: false
  host: 127.0.0.1
  port: 9110
  path: /internal/metrics
  refreshSeconds: 15
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Metrics.Enabled {
		t.Error("Enabled = true, want false from YAML")
	}
	if got, want := cfg.Metrics.Host, "127.0.0.1"; got != want {
		t.Errorf("Host = %q, want %q", got, want)
	}
	if got, want := cfg.Metrics.Port, 9110; got != want {
		t.Errorf("Port = %d, want %d", got, want)
	}
	if got, want := cfg.Metrics.Path, "/internal/metrics"; got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
	if got, want := cfg.Metrics.RefreshSeconds, 15; got != want {
		t.Errorf("RefreshSeconds = %d, want %d", got, want)
	}
}

func TestLoad_metricsEnvOverridesYAML(t *testing.T) {
	t.Setenv("LLM_API_KEY", "test-key")
	// Env has to win: the Dockerfile and the Helm deployment both pass
	// METRICS_*, and a config.yaml baked into the image or left on a data
	// volume must not override what the deployment asked for.
	t.Setenv("METRICS_ENABLED", "true")
	t.Setenv("METRICS_PORT", "9200")
	t.Setenv("METRICS_PATH", "/m")
	t.Setenv("METRICS_HOST", "0.0.0.0")
	t.Setenv("METRICS_REFRESH_SECONDS", "60")

	path := writeConfigFile(t, `metrics:
  enabled: false
  host: 127.0.0.1
  port: 9110
  path: /internal/metrics
  refreshSeconds: 15
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !cfg.Metrics.Enabled {
		t.Error("Enabled = false, want the env override to win")
	}
	if got, want := cfg.Metrics.Port, 9200; got != want {
		t.Errorf("Port = %d, want %d", got, want)
	}
	if got, want := cfg.Metrics.Path, "/m"; got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
	if got, want := cfg.Metrics.RefreshSeconds, 60; got != want {
		t.Errorf("RefreshSeconds = %d, want %d", got, want)
	}
}

func TestLoad_metricsEnvCanDisable(t *testing.T) {
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("METRICS_ENABLED", "false")
	path := writeConfigFile(t, "")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Metrics.Enabled {
		t.Error("Enabled = true, want METRICS_ENABLED=false to win")
	}
}

func TestValidateMetricsConfig(t *testing.T) {
	t.Parallel()

	server := ServerConfig{Host: "0.0.0.0", Port: 8080}

	tests := []struct {
		name    string
		metrics MetricsConfig
		wantErr bool
	}{
		{
			name:    "valid",
			metrics: MetricsConfig{Enabled: true, Port: 9090, Path: "/metrics", RefreshSeconds: 30},
		},
		{
			name: "disabled skips every check",
			// A disabled block is never read, so an invalid one must not stop
			// the backend booting.
			metrics: MetricsConfig{Enabled: false, Port: 0, Path: ""},
		},
		{
			name:    "path without a leading slash",
			metrics: MetricsConfig{Enabled: true, Port: 9090, Path: "metrics", RefreshSeconds: 30},
			wantErr: true,
		},
		{
			name:    "empty path",
			metrics: MetricsConfig{Enabled: true, Port: 9090, Path: "", RefreshSeconds: 30},
			wantErr: true,
		},
		{
			name:    "port out of range",
			metrics: MetricsConfig{Enabled: true, Port: 70000, Path: "/metrics", RefreshSeconds: 30},
			wantErr: true,
		},
		{
			name:    "zero port",
			metrics: MetricsConfig{Enabled: true, Port: 0, Path: "/metrics", RefreshSeconds: 30},
			wantErr: true,
		},
		{
			name: "port collides with the API",
			// Sharing the API port would put /metrics behind the ingress, which
			// is the whole thing a separate listener avoids.
			metrics: MetricsConfig{Enabled: true, Port: 8080, Path: "/metrics", RefreshSeconds: 30},
			wantErr: true,
		},
		{
			name:    "refresh interval below one second",
			metrics: MetricsConfig{Enabled: true, Port: 9090, Path: "/metrics", RefreshSeconds: 0},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateMetricsConfig(tt.metrics, server)
			if tt.wantErr && err == nil {
				t.Fatal("validateMetricsConfig returned no error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("validateMetricsConfig: %v", err)
			}
		})
	}
}

func TestEnvOrDefaultBool(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		set      bool
		fallback bool
		want     bool
	}{
		{name: "unset keeps the fallback", fallback: true, want: true},
		{name: "true", value: "true", set: true, want: true},
		{name: "1", value: "1", set: true, want: true},
		{name: "false", value: "false", set: true, fallback: true, want: false},
		{name: "0", value: "0", set: true, fallback: true, want: false},
		// Same shape as envOrDefaultInt: an unparseable value keeps the
		// fallback rather than silently reading as false.
		{name: "garbage keeps the fallback", value: "yes-please", set: true, fallback: true, want: true},
		{name: "empty keeps the fallback", value: "", set: true, fallback: true, want: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			const key = "NIB_TEST_METRICS_BOOL"
			if tt.set {
				t.Setenv(key, tt.value)
			} else {
				os.Unsetenv(key)
			}
			if got := envOrDefaultBool(key, tt.fallback); got != tt.want {
				t.Errorf("envOrDefaultBool(%q, %v) = %v, want %v", tt.value, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestFirstNonEmpty(t *testing.T) {
	t.Parallel()

	if got := firstNonEmpty("", "  ", "value", "other"); got != "value" {
		t.Errorf("firstNonEmpty = %q, want %q", got, "value")
	}
	if got := firstNonEmpty("", ""); got != "" {
		t.Errorf("firstNonEmpty = %q, want empty", got)
	}
	if got := firstNonEmpty("  padded  "); got != "padded" {
		t.Errorf("firstNonEmpty = %q, want %q", got, "padded")
	}
}
