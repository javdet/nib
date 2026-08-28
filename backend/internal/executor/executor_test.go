package executor

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     Config
		wantErr error
	}{
		{name: "local", cfg: Config{Type: TypeLocal}},
		{
			name: "remote kubernetes local config",
			cfg: Config{
				Type:               TypeRemote,
				Platform:           PlatformKubernetes,
				KubernetesAuthMode: KubernetesAuthModeLocalConfig,
			},
		},
		{
			name: "remote kubernetes token",
			cfg: Config{
				Type:                      TypeRemote,
				Platform:                  PlatformKubernetes,
				KubernetesAuthMode:        KubernetesAuthModeToken,
				KubernetesHost:            "https://cluster:6443",
				KubernetesTokenSecretName: "EXECUTOR_KUBERNETES_TOKEN",
			},
		},
		{
			name: "remote kubernetes token without host",
			cfg: Config{
				Type:                      TypeRemote,
				Platform:                  PlatformKubernetes,
				KubernetesAuthMode:        KubernetesAuthModeToken,
				KubernetesTokenSecretName: "EXECUTOR_KUBERNETES_TOKEN",
			},
			wantErr: ErrKubernetesHostRequired,
		},
		{
			name: "remote kubernetes token without secret",
			cfg: Config{
				Type:               TypeRemote,
				Platform:           PlatformKubernetes,
				KubernetesAuthMode: KubernetesAuthModeToken,
				KubernetesHost:     "https://cluster:6443",
			},
			wantErr: ErrKubernetesTokenSecretRequired,
		},
		{name: "remote without platform", cfg: Config{Type: TypeRemote}, wantErr: ErrInvalidPlatform},
		{name: "remote unknown platform", cfg: Config{Type: TypeRemote, Platform: "nomad"}, wantErr: ErrInvalidPlatform},
		{name: "remote docker", cfg: Config{Type: TypeRemote, Platform: PlatformDocker}, wantErr: ErrPlatformUnavailable},
		{name: "remote kubefoundry", cfg: Config{Type: TypeRemote, Platform: PlatformKubeFoundry}, wantErr: ErrPlatformUnavailable},
		{name: "legacy kubernetes type", cfg: Config{Type: "kubernetes"}, wantErr: ErrInvalidType},
		{name: "invalid", cfg: Config{Type: "vm"}, wantErr: ErrInvalidType},
		{name: "claude-code api key", cfg: Config{Type: TypeLocal, Agent: AgentClaudeCode, AuthType: AuthTypeAPIKey}},
		{name: "claude-code oauth token", cfg: Config{Type: TypeLocal, Agent: AgentClaudeCode, AuthType: AuthTypeOAuthToken}},
		{name: "invalid agent", cfg: Config{Type: TypeLocal, Agent: "unknown"}, wantErr: ErrInvalidAgent},
		{name: "codex api key", cfg: Config{Type: TypeLocal, Agent: AgentCodex, AuthType: AuthTypeAPIKey}},
		{name: "codex oauth rejected", cfg: Config{Type: TypeLocal, Agent: AgentCodex, AuthType: AuthTypeOAuthToken}, wantErr: ErrInvalidAuthType},
		{name: "invalid auth type", cfg: Config{Type: TypeLocal, Agent: AgentClaudeCode, AuthType: "bearer"}, wantErr: ErrInvalidAuthType},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateConfig(NormalizeConfig(tt.cfg))
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ValidateConfig() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateConfig() unexpected error: %v", err)
			}
		})
	}
}

func TestBuildContainerName(t *testing.T) {
	t.Parallel()

	name, err := buildContainerName("feature/my-branch")
	if err != nil {
		t.Fatalf("buildContainerName() error = %v", err)
	}
	if len(name) < len("nib-executor-feature-my-branch-")+10 {
		t.Fatalf("unexpected container name length: %q", name)
	}
	if name[:len("nib-executor-")] != "nib-executor-" {
		t.Fatalf("unexpected prefix: %q", name)
	}
}

func TestResolveLLMModel(t *testing.T) {
	t.Parallel()

	secrets := Secrets{LLMModel: "env/model"}
	if got := ResolveLLMModel(Config{}, secrets); got != "env/model" {
		t.Fatalf("ResolveLLMModel() = %q, want %q", got, "env/model")
	}
	if got := ResolveLLMModel(Config{LLMModel: "config/model"}, secrets); got != "config/model" {
		t.Fatalf("ResolveLLMModel() = %q, want %q", got, "config/model")
	}
}

func TestValidateRunRequest(t *testing.T) {
	t.Parallel()

	if err := validateRunRequest(RunRequest{
		RepoURL:    "https://github.com/org/repo.git",
		BaseBranch: "main",
		TaskPrompt: "do something",
	}); err != nil {
		t.Fatalf("validateRunRequest() unexpected error: %v", err)
	}

	if err := validateRunRequest(RunRequest{}); err == nil {
		t.Fatal("expected validation error for empty request")
	}
}

func TestResolveSecretValue(t *testing.T) {
	t.Parallel()

	if got := resolveSecretValue("from-request", "from-fallback"); got != "from-request" {
		t.Fatalf("resolveSecretValue() = %q, want from-request", got)
	}
	if got := resolveSecretValue("", "from-fallback"); got != "from-fallback" {
		t.Fatalf("resolveSecretValue() = %q, want from-fallback", got)
	}
}

func TestResolveWorkBranch(t *testing.T) {
	t.Parallel()

	if got := resolveWorkBranch("feature/my-work"); got != "feature/my-work" {
		t.Fatalf("resolveWorkBranch() = %q, want feature/my-work", got)
	}
	if got := resolveWorkBranch(""); !strings.HasPrefix(got, "agent/") {
		t.Fatalf("resolveWorkBranch() = %q, want agent/ prefix", got)
	}
}
