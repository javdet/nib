package executor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigStoreGetSet(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := NewConfigStore(dir, "", "http://localhost:8080")

	cfg, err := store.Get()
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if cfg.Type != TypeDisabled {
		t.Fatalf("default type = %q, want %q", cfg.Type, TypeDisabled)
	}
	if cfg.WebhookBaseURL != "http://localhost:8080" {
		t.Fatalf("default webhookBaseURL = %q, want http://localhost:8080", cfg.WebhookBaseURL)
	}

	want := Config{
		Type:           TypeLocal,
		Agent:          AgentClaudeCode,
		AuthType:       AuthTypeAPIKey,
		Image:          "agent-runner:local",
		LLMModel:       "anthropic/claude-sonnet-4-5",
		WebhookBaseURL: "http://localhost:8080",
	}
	if err := store.Set(want); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	got, err := store.Get()
	if err != nil {
		t.Fatalf("Get() after Set error = %v", err)
	}
	if got != want {
		t.Fatalf("Get() = %+v, want %+v", got, want)
	}

	data, err := os.ReadFile(filepath.Join(dir, "executor.json"))
	if err != nil {
		t.Fatalf("read executor.json: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("executor.json is empty")
	}
}

func TestParseConfigLegacyKubernetesType(t *testing.T) {
	t.Parallel()

	cfg, err := parseConfig(`{"type":"kubernetes","image":"agent-runner:local"}`)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.Type != TypeRemote || cfg.Platform != PlatformKubernetes {
		t.Fatalf("parseConfig() = %q/%q, want %q/%q", cfg.Type, cfg.Platform, TypeRemote, PlatformKubernetes)
	}
}

func TestParseConfigLegacySystemField(t *testing.T) {
	t.Parallel()

	cfg, err := parseConfig(`{"type":"remote","system":"kubernetes","image":"agent-runner:local"}`)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.Type != TypeRemote || cfg.Platform != PlatformKubernetes {
		t.Fatalf("parseConfig() = %q/%q, want %q/%q", cfg.Type, cfg.Platform, TypeRemote, PlatformKubernetes)
	}
}

func TestNormalizeConfigPlatform(t *testing.T) {
	t.Parallel()

	got := NormalizeConfig(Config{Type: TypeLocal, Platform: PlatformKubernetes, KubernetesHost: "https://cluster:6443"})
	if got.Platform != "" {
		t.Fatalf("NormalizeConfig() platform = %q, want empty for local", got.Platform)
	}
	if got.KubernetesHost != "" {
		t.Fatalf("NormalizeConfig() host = %q, want empty for local", got.KubernetesHost)
	}

	got = NormalizeConfig(Config{Type: " Remote ", Platform: " Kubernetes ", KubernetesHost: " https://cluster:6443 "})
	if got.Type != TypeRemote || got.Platform != PlatformKubernetes {
		t.Fatalf("NormalizeConfig() = %q/%q, want %q/%q", got.Type, got.Platform, TypeRemote, PlatformKubernetes)
	}
	if got.KubernetesAuthMode != KubernetesAuthModeLocalConfig {
		t.Fatalf("NormalizeConfig() authMode = %q, want %q", got.KubernetesAuthMode, KubernetesAuthModeLocalConfig)
	}
	if got.ServiceAccount != defaultServiceAccount {
		t.Fatalf("NormalizeConfig() serviceAccount = %q, want %q", got.ServiceAccount, defaultServiceAccount)
	}
	if got.AgentLimitCPU != defaultAgentLimitCPU {
		t.Fatalf("NormalizeConfig() agentLimitCPU = %q, want %q", got.AgentLimitCPU, defaultAgentLimitCPU)
	}
}

func TestNormalizeConfigKubernetesAuthModes(t *testing.T) {
	t.Parallel()

	got := NormalizeConfig(Config{
		Type:                      TypeRemote,
		Platform:                  PlatformKubernetes,
		KubernetesAuthMode:        KubernetesAuthModeToken,
		KubernetesHost:            "https://cluster:6443",
		KubernetesContext:         "my-context",
		KubernetesTokenSecretName: "EXECUTOR_KUBERNETES_TOKEN",
	})
	if got.KubernetesContext != "" {
		t.Fatalf("token mode context = %q, want empty", got.KubernetesContext)
	}
	if got.KubernetesHost != "https://cluster:6443" {
		t.Fatalf("token mode host = %q, want trimmed host", got.KubernetesHost)
	}

	got = NormalizeConfig(Config{
		Type:                      TypeRemote,
		Platform:                  PlatformKubernetes,
		KubernetesAuthMode:        KubernetesAuthModeLocalConfig,
		KubernetesHost:            "https://cluster:6443",
		KubernetesContext:         "my-context",
		KubernetesTokenSecretName: "EXECUTOR_KUBERNETES_TOKEN",
	})
	if got.KubernetesHost != "" {
		t.Fatalf("local_config mode host = %q, want empty", got.KubernetesHost)
	}
	if got.KubernetesTokenSecretName != "" {
		t.Fatalf("local_config mode token secret = %q, want empty", got.KubernetesTokenSecretName)
	}
	if got.KubernetesContext != "my-context" {
		t.Fatalf("local_config mode context = %q, want my-context", got.KubernetesContext)
	}
}

func TestNormalizeConfigAgent(t *testing.T) {
	t.Parallel()

	got := NormalizeConfig(Config{
		Type:     TypeLocal,
		Agent:    AgentClaudeCode,
		AuthType: AuthTypeOAuthToken,
		BaseURL:  "https://api.example.com",
	})
	if got.BaseURL != "" {
		t.Fatalf("NormalizeConfig() baseURL = %q, want empty for oauth", got.BaseURL)
	}

	got = NormalizeConfig(Config{Type: TypeLocal})
	if got.Agent != AgentClaudeCode {
		t.Fatalf("NormalizeConfig() agent = %q, want %q", got.Agent, AgentClaudeCode)
	}
	if got.AuthType != AuthTypeAPIKey {
		t.Fatalf("NormalizeConfig() authType = %q, want %q", got.AuthType, AuthTypeAPIKey)
	}
}

func TestMigrateConfigAgentDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := parseConfig(`{"type":"local","image":"agent-runner:local"}`)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.Agent != AgentClaudeCode {
		t.Fatalf("migrateConfig() agent = %q, want %q", cfg.Agent, AgentClaudeCode)
	}
	if cfg.AuthType != AuthTypeAPIKey {
		t.Fatalf("migrateConfig() authType = %q, want %q", cfg.AuthType, AuthTypeAPIKey)
	}
}
