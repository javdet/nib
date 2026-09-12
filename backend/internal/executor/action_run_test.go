package executor

import (
	"errors"
	"regexp"
	"strings"
	"testing"
)

func envMap(t *testing.T, env []string) map[string]string {
	t.Helper()

	out := make(map[string]string, len(env))
	for _, entry := range env {
		name, value, ok := strings.Cut(entry, "=")
		if !ok {
			t.Fatalf("malformed env entry %q", entry)
		}
		if _, dup := out[name]; dup {
			t.Fatalf("duplicate env variable %q", name)
		}
		out[name] = value
	}
	return out
}

func validActionRunRequest() ActionRunRequest {
	return ActionRunRequest{
		Prompt:       "Bump postgres_version to 17",
		ChatID:       "0f2b9f5c-0f0e-4f7b-9d2e-2f2c7c9a1111",
		TaskID:       "DO-236",
		RepoURL:      "https://github.com/org/ansible-roles",
		TargetBranch: "nib/DO-236",
		PRTitle:      "chore: bump postgres_version",
		GitProvider:  "github",
		GitToken:     "ghp_token",
		LLMToken:     "llm-token",
	}
}

func TestValidateActionRunRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*ActionRunRequest)
		wantErr error
	}{
		{name: "complete", mutate: func(*ActionRunRequest) {}},
		{name: "missing prompt", mutate: func(r *ActionRunRequest) { r.Prompt = "  " }, wantErr: ErrPromptRequired},
		{name: "missing repo url", mutate: func(r *ActionRunRequest) { r.RepoURL = "" }, wantErr: ErrRepoURLRequired},
		{name: "missing target branch", mutate: func(r *ActionRunRequest) { r.TargetBranch = "" }, wantErr: ErrTargetBranchRequired},
		{name: "missing git token", mutate: func(r *ActionRunRequest) { r.GitToken = "" }, wantErr: ErrGitTokenRequired},
		{name: "missing llm token", mutate: func(r *ActionRunRequest) { r.LLMToken = "" }, wantErr: ErrLLMTokenRequired},
		{name: "empty task id allowed", mutate: func(r *ActionRunRequest) { r.TaskID = "" }},
		{name: "empty pr title allowed", mutate: func(r *ActionRunRequest) { r.PRTitle = "" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := validActionRunRequest()
			tt.mutate(&req)

			err := validateActionRunRequest(req, Config{Type: TypeLocal})
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("validateActionRunRequest() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateActionRunRequest() unexpected error: %v", err)
			}
		})
	}
}

func TestBuildActionRunEnvCommonVariables(t *testing.T) {
	t.Parallel()

	cfg := Config{
		Type:     TypeLocal,
		Agent:    AgentClaudeCode,
		AuthType: AuthTypeOAuthToken,
		Image:    "javdet/nib-agent:0.2.0",
	}

	req := validActionRunRequest()
	req.GitUsername = "Byvshev Sergey"
	req.GitEmail = "javdetserg@gmail.com"

	env := envMap(t, buildActionRunEnv(cfg, Secrets{}, req, "nib-12345678"))

	want := map[string]string{
		"AGENT_TYPE":       "claude-code",
		"PROMPT":           "Bump postgres_version to 17",
		"CHAT_ID":          "0f2b9f5c-0f0e-4f7b-9d2e-2f2c7c9a1111",
		"TASK_ID":          "DO-236",
		"GIT_PROVIDER":     "github",
		"REPO_URL":         "https://github.com/org/ansible-roles",
		"TARGET_BRANCH":    "nib/DO-236",
		"PR_TITLE":         "chore: bump postgres_version",
		"ALLOWED_TOOLS":    "Read,Write,Edit,Grep,Glob,Bash",
		"PERMISSION_MODE":  "acceptEdits",
		"TIMEOUT_SECONDS":  "1500",
		"JOB_NAME":         "nib-12345678",
		"GITHUB_TOKEN":     "ghp_token",
		"GIT_AUTHOR_NAME":  "Byvshev Sergey",
		"GIT_AUTHOR_EMAIL": "javdetserg@gmail.com",
	}
	for name, value := range want {
		if env[name] != value {
			t.Errorf("env %s = %q, want %q", name, env[name], value)
		}
	}

	// The entrypoint clones the repository default branch when BASE_BRANCH is unset.
	if _, ok := env["BASE_BRANCH"]; ok {
		t.Errorf("BASE_BRANCH should not be set, got %q", env["BASE_BRANCH"])
	}
}

func TestBuildActionRunEnvOmitsBlankPRTitle(t *testing.T) {
	t.Parallel()

	req := validActionRunRequest()
	req.PRTitle = "  "

	// The entrypoint falls back to the first line of PROMPT, so an empty
	// PR_TITLE must not be exported at all.
	env := envMap(t, buildActionRunEnv(Config{AuthType: AuthTypeAPIKey}, Secrets{}, req, "nib-12345678"))
	if _, ok := env["PR_TITLE"]; ok {
		t.Fatalf("PR_TITLE should not be set, got %q", env["PR_TITLE"])
	}
}

func TestBuildActionRunEnvAuth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		cfg       Config
		wantSet   map[string]string
		wantUnset []string
	}{
		{
			name: "oauth token",
			cfg:  Config{AuthType: AuthTypeOAuthToken},
			wantSet: map[string]string{
				"CLAUDE_CODE_OAUTH_TOKEN": "llm-token",
			},
			wantUnset: []string{"ANTHROPIC_API_KEY", "ANTHROPIC_BASE_URL", "ANTHROPIC_MODEL"},
		},
		{
			name: "api key without base url",
			cfg:  Config{AuthType: AuthTypeAPIKey},
			wantSet: map[string]string{
				"ANTHROPIC_API_KEY": "llm-token",
			},
			wantUnset: []string{"CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_BASE_URL"},
		},
		{
			name: "api key with base url and model",
			cfg:  Config{AuthType: AuthTypeAPIKey, BaseURL: "https://openrouter.ai/api", LLMModel: "anthropic/claude-opus-4"},
			wantSet: map[string]string{
				"ANTHROPIC_API_KEY":  "llm-token",
				"ANTHROPIC_BASE_URL": "https://openrouter.ai/api",
				"ANTHROPIC_MODEL":    "anthropic/claude-opus-4",
			},
			wantUnset: []string{"CLAUDE_CODE_OAUTH_TOKEN"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			env := envMap(t, buildActionRunEnv(tt.cfg, Secrets{}, validActionRunRequest(), "nib-12345678"))
			for name, value := range tt.wantSet {
				if env[name] != value {
					t.Errorf("env %s = %q, want %q", name, env[name], value)
				}
			}
			for _, name := range tt.wantUnset {
				if _, ok := env[name]; ok {
					t.Errorf("env %s should not be set, got %q", name, env[name])
				}
			}
		})
	}
}

func TestBuildActionRunEnvNormalizesGitProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider string
		want     string
	}{
		{name: "blank", provider: "", want: "github"},
		{name: "github", provider: "GitHub", want: "github"},
		{name: "unknown", provider: "bitbucket", want: "github"},
		{name: "gitlab", provider: "GitLab", want: "gitlab"},
		{name: "self-hosted gitlab", provider: "self-hosted gitlab", want: "gitlab"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := validActionRunRequest()
			req.GitProvider = tt.provider

			env := envMap(t, buildActionRunEnv(Config{AuthType: AuthTypeAPIKey}, Secrets{}, req, "nib-12345678"))
			if env["GIT_PROVIDER"] != tt.want {
				t.Errorf("GIT_PROVIDER = %q, want %q", env["GIT_PROVIDER"], tt.want)
			}
			// The container derives GITLAB_TOKEN/GL_TOKEN from this one, so the
			// wire name stays GITHUB_TOKEN whatever the provider is.
			if env["GITHUB_TOKEN"] != req.GitToken {
				t.Errorf("GITHUB_TOKEN = %q, want %q", env["GITHUB_TOKEN"], req.GitToken)
			}
		})
	}
}

func TestBuildActionRunEnvWebhook(t *testing.T) {
	t.Parallel()

	cfg := Config{
		AuthType:       AuthTypeAPIKey,
		WebhookBaseURL: "http://localhost:8080",
	}
	secrets := Secrets{WebhookToken: "secret-token"}

	env := envMap(t, buildActionRunEnv(cfg, secrets, validActionRunRequest(), "nib-12345678"))

	if env["WEBHOOK_URL"] != "http://localhost:8080/api/v1/agent-runner/webhook" {
		t.Fatalf("WEBHOOK_URL = %q", env["WEBHOOK_URL"])
	}
	if env["WEBHOOK_AUTH_HEADER"] != "Authorization: Bearer secret-token" {
		t.Fatalf("WEBHOOK_AUTH_HEADER = %q", env["WEBHOOK_AUTH_HEADER"])
	}
}

func TestResolveAgentWebhookURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		base string
		want string
	}{
		{
			base: "http://localhost:8080",
			want: "http://localhost:8080/api/v1/agent-runner/webhook",
		},
		{
			base: "http://localhost:8080/",
			want: "http://localhost:8080/api/v1/agent-runner/webhook",
		},
		{
			base: "http://localhost:8080/api/v1/agent-runner/webhook",
			want: "http://localhost:8080/api/v1/agent-runner/webhook",
		},
		{
			base: "http://localhost:8080/api/v1/agent-runner/webhook/",
			want: "http://localhost:8080/api/v1/agent-runner/webhook",
		},
		{base: "", want: ""},
	}
	for _, tt := range tests {
		got := resolveAgentWebhookURL(tt.base)
		if got != tt.want {
			t.Errorf("resolveAgentWebhookURL(%q) = %q, want %q", tt.base, got, tt.want)
		}
	}
}

func TestBuildActionRunEnvWebhookFullPathBase(t *testing.T) {
	t.Parallel()

	cfg := Config{
		AuthType:       AuthTypeAPIKey,
		WebhookBaseURL: "http://localhost:8080/api/v1/agent-runner/webhook",
	}
	env := envMap(t, buildActionRunEnv(cfg, Secrets{}, validActionRunRequest(), "nib-12345678"))
	if env["WEBHOOK_URL"] != "http://localhost:8080/api/v1/agent-runner/webhook" {
		t.Fatalf("WEBHOOK_URL = %q", env["WEBHOOK_URL"])
	}
}

func TestBuildActionRunEnvCodexAuth(t *testing.T) {
	t.Parallel()

	cfg := Config{
		Agent:    AgentCodex,
		AuthType: AuthTypeAPIKey,
		BaseURL:  "https://openrouter.ai/api/v1",
		LLMModel: "openai/gpt-5.3-codex",
	}

	env := envMap(t, buildActionRunEnv(cfg, Secrets{}, validActionRunRequest(), "nib-12345678"))

	wantSet := map[string]string{
		"AGENT_TYPE":       "codex",
		"OPENAI_API_KEY":   "llm-token",
		"OPENAI_BASE_URL":  "https://openrouter.ai/api/v1",
		"OPENAI_MODEL":     "openai/gpt-5.3-codex",
	}
	for name, value := range wantSet {
		if env[name] != value {
			t.Errorf("env %s = %q, want %q", name, env[name], value)
		}
	}

	wantUnset := []string{
		"CLAUDE_CODE_OAUTH_TOKEN",
		"ANTHROPIC_API_KEY",
		"ANTHROPIC_BASE_URL",
		"ANTHROPIC_MODEL",
	}
	for _, name := range wantUnset {
		if _, ok := env[name]; ok {
			t.Errorf("env %s should not be set, got %q", name, env[name])
		}
	}
}

func TestBuildJobName(t *testing.T) {
	t.Parallel()

	pattern := regexp.MustCompile(`^nib-\d{8}$`)
	for i := 0; i < 20; i++ {
		name, err := buildJobName()
		if err != nil {
			t.Fatalf("buildJobName() error = %v", err)
		}
		if !pattern.MatchString(name) {
			t.Fatalf("buildJobName() = %q, want nib-<8 digits>", name)
		}
	}
}
