package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMergeProjects(t *testing.T) {
	t.Parallel()

	arrayOnly := []ProjectConfig{{Name: "myproject2", Description: "from array"}}
	singular := ProjectConfig{Name: "toolchain", Description: "from block"}
	duplicate := ProjectConfig{Name: "myproject2", Description: "from block"}

	tests := []struct {
		name     string
		projects []ProjectConfig
		singular ProjectConfig
		want     []ProjectConfig
	}{
		{
			name:     "array only",
			projects: arrayOnly,
			want:     arrayOnly,
		},
		{
			name:     "singular only",
			singular: singular,
			want:     []ProjectConfig{singular},
		},
		{
			name:     "merge distinct names",
			projects: arrayOnly,
			singular: singular,
			want:     []ProjectConfig{arrayOnly[0], singular},
		},
		{
			name:     "skip duplicate name",
			projects: arrayOnly,
			singular: duplicate,
			want:     arrayOnly,
		},
		{
			name:     "ignore empty singular",
			projects: arrayOnly,
			singular: ProjectConfig{Description: "no name"},
			want:     arrayOnly,
		},
		{
			name:     "trim singular name",
			singular: ProjectConfig{Name: "  toolchain  ", Description: "trimmed"},
			want:     []ProjectConfig{{Name: "toolchain", Description: "trimmed"}},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := MergeProjects(tt.projects, tt.singular)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("MergeProjects() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestLoadParsesStringLocations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	const yamlBody = `
llm:
  baseURL: https://api.example/v1
  model: gpt-test
projects:
  - name: myproject
    description: myproject project
    clouds:
      - name: digitalocean
        description: do cloud
        locations:
          - fra1
          - ams3
      - name: aws
        locations:
          - name: us-east-2
            description: New York region
`
	if err := os.WriteFile(path, []byte(yamlBody), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("OPENAI_API_KEY", "")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(cfg.Projects) != 1 {
		t.Fatalf("Projects len = %d, want 1", len(cfg.Projects))
	}
	clouds := cfg.Projects[0].Clouds
	if len(clouds) != 2 {
		t.Fatalf("Clouds len = %d, want 2", len(clouds))
	}

	doLocs := clouds[0].Locations
	wantDO := []LocationConfig{{Name: "fra1"}, {Name: "ams3"}}
	if !reflect.DeepEqual(doLocs, wantDO) {
		t.Errorf("digitalocean locations = %#v, want %#v", doLocs, wantDO)
	}

	yaLocs := clouds[1].Locations
	wantYA := []LocationConfig{{Name: "us-east-2", Description: "New York region"}}
	if !reflect.DeepEqual(yaLocs, wantYA) {
		t.Errorf("aws locations = %#v, want %#v", yaLocs, wantYA)
	}
}

func TestLoadParsesUnwiredToolchainFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	const toolchainYAML = `
llm:
  model: gpt-test
  embeddingModel: text-embedding-3-large
  baseURL: https://api.example/v1
  timeoutSeconds: 90
  httpReferer: https://example.com
  appTitle: nib-test
  reasoningEffort: none
log:
  enabled: true
  file: /var/log/nib.log
agent:
  maxIterations: 12
knowledge_base:
  uri: postgres://u:p@db:5432/nib?sslmode=disable
skills:
  dir: /opt/skills
rules:
  dir: /opt/rules
prompts:
  dir: /opt/prompts
includedTools:
  dir: /opt/tools
mcp:
  file: /opt/mcp.json
projects: []
`
	if err := os.WriteFile(path, []byte(toolchainYAML), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("OPENAI_API_KEY", "")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Agent.MaxIterations != 12 {
		t.Errorf("Agent.MaxIterations = %d, want 12", cfg.Agent.MaxIterations)
	}
	if !cfg.Log.Enabled || cfg.Log.File != "/var/log/nib.log" {
		t.Errorf("Log = %+v, want enabled=true file=/var/log/nib.log", cfg.Log)
	}
	if cfg.Skills.Dir != "/opt/skills" {
		t.Errorf("Skills.Dir = %q, want /opt/skills", cfg.Skills.Dir)
	}
	if cfg.Rules.Dir != "/opt/rules" {
		t.Errorf("Rules.Dir = %q, want /opt/rules", cfg.Rules.Dir)
	}
	if cfg.Prompts.Dir != "/opt/prompts" {
		t.Errorf("Prompts.Dir = %q, want /opt/prompts", cfg.Prompts.Dir)
	}
	if cfg.IncludedTools.Dir != "/opt/tools" {
		t.Errorf("IncludedTools.Dir = %q, want /opt/tools", cfg.IncludedTools.Dir)
	}
	if cfg.MCP.File != "/opt/mcp.json" {
		t.Errorf("MCP.File = %q, want /opt/mcp.json", cfg.MCP.File)
	}
	if cfg.LLM.EmbeddingModel != "text-embedding-3-large" {
		t.Errorf("LLM.EmbeddingModel = %q, want text-embedding-3-large", cfg.LLM.EmbeddingModel)
	}
	if cfg.LLM.TimeoutSeconds != 90 {
		t.Errorf("LLM.TimeoutSeconds = %d, want 90", cfg.LLM.TimeoutSeconds)
	}
	if cfg.LLM.HTTPReferer != "https://example.com" {
		t.Errorf("LLM.HTTPReferer = %q, want https://example.com", cfg.LLM.HTTPReferer)
	}
	if cfg.LLM.ReasoningEffort != "none" {
		t.Errorf("LLM.ReasoningEffort = %q, want none", cfg.LLM.ReasoningEffort)
	}
	if cfg.LLM.AppTitle != "nib-test" {
		t.Errorf("LLM.AppTitle = %q, want nib-test", cfg.LLM.AppTitle)
	}
}

const minimalValidLLMYAML = `
llm:
  baseURL: https://api.example/v1
  model: gpt-test
`

func TestLoad_Log_errors(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantSub string
	}{
		{
			name: "enabled true missing file",
			body: minimalValidLLMYAML + `
log:
  enabled: true
`,
			wantSub: "log.file is required when log.enabled is true",
		},
		{
			name: "enabled true blank file",
			body: minimalValidLLMYAML + `
log:
  enabled: true
  file: ""
`,
			wantSub: "log.file is required when log.enabled is true",
		},
		{
			name: "enabled true whitespace file",
			body: minimalValidLLMYAML + `
log:
  enabled: true
  file: "   \t  "
`,
			wantSub: "log.file is required when log.enabled is true",
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}
			t.Setenv("LLM_API_KEY", "test-key")
			t.Setenv("OPENAI_API_KEY", "")
			_, err := Load(path)
			if err == nil {
				t.Fatal("Load() expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("Load() error %q does not contain %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestLoad_LLM_envResolution(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(minimalValidLLMYAML), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("LLM_API_KEY", "llm-key")
	t.Setenv("OPENAI_API_KEY", "openai-key")
	t.Setenv("LLM_MODEL", "llm-model")
	t.Setenv("OPENAI_MODEL", "openai-model")
	t.Setenv("LLM_BASE_URL", "https://env.example/v1")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.LLM.APIKey != "llm-key" {
		t.Errorf("LLM.APIKey = %q, want llm-key", cfg.LLM.APIKey)
	}
	// YAML model overlays env.
	if cfg.LLM.Model != "gpt-test" {
		t.Errorf("LLM.Model = %q, want gpt-test from YAML", cfg.LLM.Model)
	}
	// YAML baseURL overlays env.
	if cfg.LLM.BaseURL != "https://api.example/v1" {
		t.Errorf("LLM.BaseURL = %q, want https://api.example/v1 from YAML", cfg.LLM.BaseURL)
	}
	if cfg.LLM.TimeoutSeconds != defaultLLMTimeoutSeconds {
		t.Errorf("LLM.TimeoutSeconds = %d, want default %d", cfg.LLM.TimeoutSeconds, defaultLLMTimeoutSeconds)
	}
}

func TestLoad_LLM_envOnlyRequiredFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	const yamlBody = `
projects: []
`
	if err := os.WriteFile(path, []byte(yamlBody), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("LLM_API_KEY", "env-key")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("LLM_MODEL", "env-model")
	t.Setenv("OPENAI_MODEL", "")
	t.Setenv("LLM_BASE_URL", "https://env-only.example/v1")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.LLM.APIKey != "env-key" {
		t.Errorf("LLM.APIKey = %q, want env-key", cfg.LLM.APIKey)
	}
	if cfg.LLM.Model != "env-model" {
		t.Errorf("LLM.Model = %q, want env-model", cfg.LLM.Model)
	}
	if cfg.LLM.BaseURL != "https://env-only.example/v1" {
		t.Errorf("LLM.BaseURL = %q, want https://env-only.example/v1", cfg.LLM.BaseURL)
	}
}

func TestLoad_LLM_openAIKeyFallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	const yamlBody = `
llm:
  baseURL: https://api.example/v1
`
	if err := os.WriteFile(path, []byte(yamlBody), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("LLM_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "legacy-openai-key")
	t.Setenv("LLM_MODEL", "")
	t.Setenv("OPENAI_MODEL", "legacy-model")
	t.Setenv("LLM_BASE_URL", "")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.LLM.APIKey != "legacy-openai-key" {
		t.Errorf("LLM.APIKey = %q, want legacy-openai-key", cfg.LLM.APIKey)
	}
	if cfg.LLM.Model != "legacy-model" {
		t.Errorf("LLM.Model = %q, want legacy-model", cfg.LLM.Model)
	}
}

func TestLoad_LLM_validationErrors(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		env     map[string]string
		wantSub string
	}{
		{
			name:    "missing baseURL",
			body:    "llm:\n  model: gpt-test\n",
			env:     map[string]string{"LLM_API_KEY": "key"},
			wantSub: "llm.baseURL is required",
		},
		{
			name:    "missing api key",
			body:    minimalValidLLMYAML,
			env:     map[string]string{"LLM_API_KEY": "", "OPENAI_API_KEY": ""},
			wantSub: "llm API key is required",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			_, err := Load(path)
			if err == nil {
				t.Fatal("Load() expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("Load() error %q does not contain %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestParseLogLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		level   string
		want    slog.Level
		wantErr bool
	}{
		{name: "empty defaults to info", level: "", want: slog.LevelInfo},
		{name: "info", level: "info", want: slog.LevelInfo},
		{name: "debug", level: "debug", want: slog.LevelDebug},
		{name: "warn", level: "warn", want: slog.LevelWarn},
		{name: "error", level: "error", want: slog.LevelError},
		{name: "case insensitive", level: "DeBuG", want: slog.LevelDebug},
		{name: "whitespace trimmed", level: "  info  ", want: slog.LevelInfo},
		{name: "invalid", level: "trace", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseLogLevel(tc.level)
			if tc.wantErr {
				if err == nil {
					t.Fatal("ParseLogLevel() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseLogLevel() error = %v", err)
			}
			if got != tc.want {
				t.Errorf("ParseLogLevel() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLoad_LogLevel(t *testing.T) {
	t.Run("yaml debug", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		body := minimalValidLLMYAML + `
log:
  level: debug
`
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		t.Setenv("LLM_API_KEY", "test-key")
		t.Setenv("OPENAI_API_KEY", "")
		t.Setenv("LOG_LEVEL", "")

		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if cfg.Log.Level != "debug" {
			t.Errorf("Log.Level = %q, want debug", cfg.Log.Level)
		}
		level, err := ParseLogLevel(cfg.Log.Level)
		if err != nil {
			t.Fatalf("ParseLogLevel() error = %v", err)
		}
		if level != slog.LevelDebug {
			t.Errorf("parsed level = %v, want %v", level, slog.LevelDebug)
		}
	})

	t.Run("empty defaults to info", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		if err := os.WriteFile(path, []byte(minimalValidLLMYAML), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		t.Setenv("LLM_API_KEY", "test-key")
		t.Setenv("OPENAI_API_KEY", "")
		t.Setenv("LOG_LEVEL", "")

		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		level, err := ParseLogLevel(cfg.Log.Level)
		if err != nil {
			t.Fatalf("ParseLogLevel() error = %v", err)
		}
		if level != slog.LevelInfo {
			t.Errorf("parsed level = %v, want %v", level, slog.LevelInfo)
		}
	})

	t.Run("invalid yaml level", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		body := minimalValidLLMYAML + `
log:
  level: verbose
`
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		t.Setenv("LLM_API_KEY", "test-key")
		t.Setenv("OPENAI_API_KEY", "")
		_, err := Load(path)
		if err == nil {
			t.Fatal("Load() expected error, got nil")
		}
		if !strings.Contains(err.Error(), "invalid log.level") {
			t.Errorf("Load() error %q does not contain invalid log.level", err.Error())
		}
	})

	t.Run("LOG_LEVEL env overrides yaml", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		body := minimalValidLLMYAML + `
log:
  level: info
`
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		t.Setenv("LLM_API_KEY", "test-key")
		t.Setenv("OPENAI_API_KEY", "")
		t.Setenv("LOG_LEVEL", "debug")

		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if cfg.Log.Level != "debug" {
			t.Errorf("Log.Level = %q, want debug from LOG_LEVEL env", cfg.Log.Level)
		}
	})
}

func TestValidateLLMConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		llm     LLMConfig
		wantSub string
	}{
		{
			name: "missing baseURL",
			llm: LLMConfig{
				Model:          "gpt-test",
				APIKey:         "key",
				TimeoutSeconds: defaultLLMTimeoutSeconds,
			},
			wantSub: "llm.baseURL is required",
		},
		{
			name: "missing model",
			llm: LLMConfig{
				BaseURL:        "https://api.example/v1",
				APIKey:         "key",
				TimeoutSeconds: defaultLLMTimeoutSeconds,
			},
			wantSub: "llm.model is required",
		},
		{
			name: "missing api key",
			llm: LLMConfig{
				BaseURL:        "https://api.example/v1",
				Model:          "gpt-test",
				TimeoutSeconds: defaultLLMTimeoutSeconds,
			},
			wantSub: "llm API key is required",
		},
		{
			name: "invalid timeout",
			llm: LLMConfig{
				BaseURL:        "https://api.example/v1",
				Model:          "gpt-test",
				APIKey:         "key",
				TimeoutSeconds: 0,
			},
			wantSub: "llm.timeoutSeconds must be at least 1",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateLLMConfig(tc.llm)
			if err == nil || !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("validateLLMConfig() = %v, want error containing %q", err, tc.wantSub)
			}
		})
	}
}
