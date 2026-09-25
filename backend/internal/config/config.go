package config

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the runtime configuration for the nib backend (env vars + YAML file).
type Config struct {
	Server                ServerConfig
	Database              DatabaseConfig
	LLM                   LLMConfig
	Projects              []ProjectConfig
	ConfigPath            string
	DataDir               string
	KnowledgeBaseURI      string
	KnowledgeBaseDir      string
	KnowledgeSettingsFile string
	Agent                 AgentConfig
	Log                   LogConfig
	Skills                SkillsConfig
	Rules                 RulesConfig
	Prompts               PromptsConfig
	IncludedTools         IncludedToolsConfig
	MCP                   MCPConfig
	Metrics               MetricsConfig
	Executor              ExecutorConfig
	SecretsEncryptionKey  []byte
}

const (
	defaultEmbeddingModel    = "text-embedding-3-small"
	defaultLLMTimeoutSeconds = 120
	// maxEmbeddingDimensions is a sanity bound, not a provider limit: it is
	// here to catch a typo before it reaches the API.
	maxEmbeddingDimensions = 4096
)

// LLMConfig holds LLM provider settings. APIKey comes from env; model/baseURL may come from YAML.
type LLMConfig struct {
	APIKey          string
	Model           string
	BaseURL         string
	EmbeddingModel  string
	TimeoutSeconds  int
	HTTPReferer     string
	AppTitle        string
	ReasoningEffort string
	// API selects the completion endpoint: "chat" (/v1/chat/completions) or
	// "responses" (/v1/responses). Only the latter supports tools together
	// with reasoning on gpt-5.6.
	API string
	// Embeddings points the embeddings client somewhere of its own. Every
	// field falls back to the matching llm.* value, so an absent block is
	// exactly the single-endpoint behaviour that came before it.
	Embeddings EmbeddingsConfig
}

// EmbeddingsConfig gives embeddings an endpoint separate from completions.
// The two cannot be assumed to share a host: several providers that serve
// chat completions have no /embeddings route at all, and pointing one base URL
// at them would take knowledge search and the tool catalog down with it.
type EmbeddingsConfig struct {
	BaseURL string
	APIKey  string
	Model   string
	// Dimensions is sent as the request's `dimensions` parameter; zero omits
	// it. It exists because kb_chunks.embedding is vector(1536), so a model
	// whose native width is not 1536 has to be asked to return 1536.
	Dimensions int
}

const (
	// APIChat targets /v1/chat/completions, supported by every
	// OpenAI-compatible gateway (OpenRouter, DO Gradient, Google).
	APIChat = "chat"
	// APIResponses targets /v1/responses, currently OpenAI-only.
	APIResponses = "responses"
)

// AgentConfig controls agent-loop behaviour (parsed from YAML; wiring is future work).
type AgentConfig struct {
	MaxIterations int `yaml:"maxIterations"`
	// PlanFanoutConcurrency caps how many stage subagents plan at once.
	PlanFanoutConcurrency int `yaml:"planFanoutConcurrency"`
	// StageMaxIterations is the completion round budget of a single stage
	// subagent, which researches one stage rather than a whole plan.
	StageMaxIterations int `yaml:"stageMaxIterations"`
	// PlanFanoutTimeoutMinutes budgets the time a fan-out run spends working. It
	// outlives the HTTP request that starts it, so it needs a bound of its own,
	// and it does not run down while a stage waits on the operator's answer.
	PlanFanoutTimeoutMinutes int `yaml:"planFanoutTimeoutMinutes"`
	// ActionExecConcurrency caps how many action subagents execute at once.
	ActionExecConcurrency int `yaml:"actionExecConcurrency"`
	// ActionExecMaxIterations is the completion round budget of a single action
	// subagent, which carries out one action rather than planning a whole stage.
	ActionExecMaxIterations int `yaml:"actionExecMaxIterations"`
	// ActionExecTimeoutMinutes bounds one action run. Like a fan-out it outlives
	// the turn that starts it, so it needs a deadline of its own.
	ActionExecTimeoutMinutes int `yaml:"actionExecTimeoutMinutes"`
}

// LogConfig controls optional mirroring of stderr to a file.
// When Enabled is false, File is ignored. Zero value means logging to stderr only.
// Level defaults to info when empty; LOG_LEVEL env overrides the YAML value when set.
type LogConfig struct {
	Enabled bool   `yaml:"enabled"`
	File    string `yaml:"file"`
	Level   string `yaml:"level"`
}

// MetricsConfig controls the Prometheus endpoint.
//
// The endpoint gets a listener of its own rather than a route on the API
// server: both the Helm ingress and the frontend nginx route only the /api
// prefix to the backend, so a separate port is never reachable from the public
// host while an in-cluster scraper on the pod port still gets it.
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
	Path    string `yaml:"path"`
	// RefreshSeconds is how often plans-by-status and the database health probe
	// are recomputed. They are not done at scrape time: counting plans is one
	// Postgres query plus one plan_state file read per plan.
	RefreshSeconds int `yaml:"refreshSeconds"`
}

// fileMetricsConfig mirrors MetricsConfig for YAML.
//
// Enabled is a pointer because the default is true, which a bool zero value
// cannot distinguish from an explicit "enabled: false".
type fileMetricsConfig struct {
	Enabled        *bool  `yaml:"enabled"`
	Host           string `yaml:"host"`
	Port           int    `yaml:"port"`
	Path           string `yaml:"path"`
	RefreshSeconds int    `yaml:"refreshSeconds"`
}

const (
	defaultMetricsHost           = "0.0.0.0"
	defaultMetricsPort           = 9090
	defaultMetricsPath           = "/metrics"
	defaultMetricsRefreshSeconds = 30
)

// RefreshInterval is the metrics refresh interval as a duration.
func (m MetricsConfig) RefreshInterval() time.Duration {
	return time.Duration(m.RefreshSeconds) * time.Second
}

// SkillsConfig holds the skills directory path from YAML.
type SkillsConfig struct {
	Dir string `yaml:"dir"`
}

// RulesConfig holds the rules directory path from YAML.
type RulesConfig struct {
	Dir string `yaml:"dir"`
}

// PromptsConfig holds the system-prompt presets directory path from YAML.
type PromptsConfig struct {
	Dir string `yaml:"dir"`
}

// IncludedToolsConfig holds the per-mode tools directory path from YAML.
// System tool allow lists ({mode}.json) and mcp-included.json live in this directory.
type IncludedToolsConfig struct {
	Dir string `yaml:"dir"`
}

// MCPConfig holds the Cursor-style mcp.json file path from YAML.
// Empty File defaults to {DATA_DIR}/mcp.json at runtime (see mcpconfig.ResolveFile).
type MCPConfig struct {
	File string `yaml:"file"`
}

// ExecutorConfig holds agent-runner executor settings.
// LLMAPIKey, LLMModel, and WebhookToken come from env; File may come from YAML.
type ExecutorConfig struct {
	LLMAPIKey    string
	LLMModel     string
	WebhookToken string
	File         string `yaml:"file"`
}

// KnowledgeBaseConfig holds knowledge-base settings from YAML.
type KnowledgeBaseConfig struct {
	URI string `yaml:"uri"`
	// Dir holds the last uploaded source document per collection
	// ({dir}/{collection}.md). Empty resolves to {DATA_DIR}/knowledgebase.
	Dir string `yaml:"dir"`
	// SettingsFile holds the operator switches (auto-update). Empty resolves to
	// {DATA_DIR}/knowledge.json.
	SettingsFile string `yaml:"settingsFile"`
}

type ServerConfig struct {
	Host string
	Port int
}

type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		d.User, d.Password, d.Host, d.Port, d.DBName, d.SSLMode,
	)
}

// DatabaseDSN returns knowledge_base.uri when set, otherwise the env-built Database DSN.
func (c Config) DatabaseDSN() string {
	if c.KnowledgeBaseURI != "" {
		return c.KnowledgeBaseURI
	}
	return c.Database.DSN()
}

// DatabaseDSNSource reports where DatabaseDSN comes from ("knowledge_base.uri" or "env").
func (c Config) DatabaseDSNSource() string {
	if c.KnowledgeBaseURI != "" {
		return "knowledge_base.uri"
	}
	return "env"
}

type ProjectConfig struct {
	Name         string              `yaml:"name"`
	Description  string              `yaml:"description"`
	Environments []EnvironmentConfig `yaml:"environments"`
	Clouds       []CloudConfig       `yaml:"clouds"`
}

type EnvironmentConfig struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

type CloudConfig struct {
	Name        string           `yaml:"name"`
	Description string           `yaml:"description"`
	Locations   []LocationConfig `yaml:"locations"`
}

type LocationConfig struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// UnmarshalYAML accepts either a plain location name ("fra1") or a mapping
// with name and optional description.
func (lc *LocationConfig) UnmarshalYAML(unmarshal func(any) error) error {
	var name string
	if err := unmarshal(&name); err == nil {
		lc.Name = strings.TrimSpace(name)
		return nil
	}

	type plain LocationConfig
	var aux plain
	if err := unmarshal(&aux); err != nil {
		return err
	}
	*lc = LocationConfig(aux)
	return nil
}

// fileConfig mirrors the toolchain YAML schema for unmarshaling.
type fileConfig struct {
	LLM           fileLLMConfig       `yaml:"llm"`
	Log           LogConfig           `yaml:"log"`
	Agent         AgentConfig         `yaml:"agent"`
	KnowledgeBase KnowledgeBaseConfig `yaml:"knowledge_base"`
	Skills        SkillsConfig        `yaml:"skills"`
	Rules         RulesConfig         `yaml:"rules"`
	Prompts       PromptsConfig       `yaml:"prompts"`
	IncludedTools IncludedToolsConfig `yaml:"includedTools"`
	MCP           MCPConfig           `yaml:"mcp"`
	Metrics       fileMetricsConfig   `yaml:"metrics"`
	Executor      ExecutorConfig      `yaml:"executor"`
	Project       ProjectConfig       `yaml:"project"`
	Projects      []ProjectConfig     `yaml:"projects"`
}

type fileLLMConfig struct {
	BaseURL        string `yaml:"baseURL"`
	Model          string `yaml:"model"`
	EmbeddingModel string `yaml:"embeddingModel"`
	TimeoutSeconds int    `yaml:"timeoutSeconds"`
	HTTPReferer    string `yaml:"httpReferer"`
	AppTitle       string `yaml:"appTitle"`
	// A pointer, not a string: `reasoningEffort: ""` in the file has to be
	// able to clear a value that came from LLM_REASONING_EFFORT, because a
	// model that rejects the parameter needs exactly that. Only an absent key
	// should defer to the environment.
	ReasoningEffort *string              `yaml:"reasoningEffort"`
	API             string               `yaml:"api"`
	Embeddings      fileEmbeddingsConfig `yaml:"embeddings"`
}

// fileEmbeddingsConfig has no apiKey: keys never come from the file.
type fileEmbeddingsConfig struct {
	BaseURL    string `yaml:"baseURL"`
	Model      string `yaml:"model"`
	Dimensions int    `yaml:"dimensions"`
}

// Load reads environment variables and the YAML file at path.
func Load(path string) (Config, error) {
	cfg := Config{
		Server: ServerConfig{
			Host: envOrDefault("SERVER_HOST", "0.0.0.0"),
			Port: envOrDefaultInt("SERVER_PORT", 8080),
		},
		Database: DatabaseConfig{
			Host:     envOrDefault("DB_HOST", "localhost"),
			Port:     envOrDefaultInt("DB_PORT", 5432),
			User:     envOrDefault("DB_USER", "nib"),
			Password: envOrDefault("DB_PASSWORD", "nib"),
			DBName:   envOrDefault("DB_NAME", "nib"),
			SSLMode:  envOrDefault("DB_SSLMODE", "disable"),
		},
	}

	cfg.LLM = LLMConfig{
		APIKey:          resolveLLMAPIKey(),
		Model:           resolveLLMModel(),
		BaseURL:         resolveLLMBaseURL(),
		EmbeddingModel:  envOrDefault("OPENAI_EMBEDDING_MODEL", defaultEmbeddingModel),
		ReasoningEffort: strings.TrimSpace(os.Getenv("LLM_REASONING_EFFORT")),
		API:             envOrDefault("LLM_API", APIChat),
		Embeddings: EmbeddingsConfig{
			BaseURL:    strings.TrimSpace(os.Getenv("LLM_EMBEDDINGS_BASE_URL")),
			APIKey:     strings.TrimSpace(os.Getenv("LLM_EMBEDDINGS_API_KEY")),
			Model:      strings.TrimSpace(os.Getenv("LLM_EMBEDDINGS_MODEL")),
			Dimensions: envOrDefaultInt("LLM_EMBEDDINGS_DIMENSIONS", 0),
		},
	}

	cfg.Executor = ExecutorConfig{
		LLMAPIKey:    strings.TrimSpace(os.Getenv("EXECUTOR_LLM_API_KEY")),
		LLMModel:     strings.TrimSpace(os.Getenv("EXECUTOR_LLM_MODEL")),
		WebhookToken: strings.TrimSpace(os.Getenv("AGENT_WEBHOOK_TOKEN")),
	}

	secretsKey, err := parseSecretsEncryptionKey()
	if err != nil {
		return Config{}, err
	}
	cfg.SecretsEncryptionKey = secretsKey

	fc, err := readFileConfig(path)
	if err != nil {
		return Config{}, fmt.Errorf("load config file: %w", err)
	}
	cfg.Projects = MergeProjects(fc.Projects, fc.Project)
	cfg.ConfigPath = path
	cfg.DataDir = envOrDefault("DATA_DIR", "data")
	cfg.KnowledgeBaseURI = strings.TrimSpace(fc.KnowledgeBase.URI)
	cfg.KnowledgeBaseDir = strings.TrimSpace(fc.KnowledgeBase.Dir)
	cfg.KnowledgeSettingsFile = strings.TrimSpace(fc.KnowledgeBase.SettingsFile)
	applyFileLLMConfig(&cfg.LLM, fc.LLM)
	applyLLMDefaults(&cfg.LLM)
	applyFileAppConfig(&cfg, fc)
	applyFileMetricsConfig(&cfg.Metrics, fc.Metrics)

	if err := validateAppConfig(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func validateAppConfig(cfg Config) error {
	if err := validateLLMConfig(cfg.LLM); err != nil {
		return err
	}
	if cfg.Log.Enabled && strings.TrimSpace(cfg.Log.File) == "" {
		return fmt.Errorf("config: log.file is required when log.enabled is true")
	}
	if _, err := ParseLogLevel(cfg.Log.Level); err != nil {
		return err
	}
	if err := validateMetricsConfig(cfg.Metrics, cfg.Server); err != nil {
		return err
	}
	return nil
}

func validateMetricsConfig(m MetricsConfig, server ServerConfig) error {
	if !m.Enabled {
		return nil
	}
	if !strings.HasPrefix(m.Path, "/") {
		return fmt.Errorf("config: metrics.path must start with / (got %q)", m.Path)
	}
	if m.Port < 1 || m.Port > 65535 {
		return fmt.Errorf("config: metrics.port must be between 1 and 65535 (got %d)", m.Port)
	}
	if m.Port == server.Port {
		return fmt.Errorf("config: metrics.port %d is already the API port; metrics need a listener of their own", m.Port)
	}
	if m.RefreshSeconds < 1 {
		return fmt.Errorf("config: metrics.refreshSeconds must be at least 1 (got %d)", m.RefreshSeconds)
	}
	return nil
}

// ParseLogLevel maps a config string to slog.Level.
// Empty level defaults to info. Accepted values: debug, info, warn, error (case-insensitive).
func ParseLogLevel(level string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("config: invalid log.level %q (allowed: debug, info, warn, error)", strings.TrimSpace(level))
	}
}

func validateLLMConfig(llm LLMConfig) error {
	if strings.TrimSpace(llm.BaseURL) == "" {
		return fmt.Errorf("config: llm.baseURL is required (set llm.baseURL in YAML or LLM_BASE_URL)")
	}
	if strings.TrimSpace(llm.Model) == "" {
		return fmt.Errorf("config: llm.model is required (set llm.model in YAML or LLM_MODEL)")
	}
	if strings.TrimSpace(llm.APIKey) == "" {
		return fmt.Errorf("config: llm API key is required (set LLM_API_KEY or OPENAI_API_KEY)")
	}
	if llm.TimeoutSeconds < 1 {
		return fmt.Errorf("config: llm.timeoutSeconds must be at least 1")
	}
	if !KnownReasoningEffort(llm.ReasoningEffort) {
		return fmt.Errorf("config: invalid llm.reasoningEffort %q (allowed: %s)",
			strings.TrimSpace(llm.ReasoningEffort), strings.Join(reasoningEfforts, ", "))
	}
	// Range-checked but deliberately not pinned to 1536 here: config must not
	// import kb, and a width the provider ignores is only detectable from the
	// response, so the real enforcement is in the embeddings client.
	if llm.Embeddings.Dimensions < 0 || llm.Embeddings.Dimensions > maxEmbeddingDimensions {
		return fmt.Errorf("config: llm.embeddings.dimensions must be between 1 and %d when set (got %d)",
			maxEmbeddingDimensions, llm.Embeddings.Dimensions)
	}
	if strings.TrimSpace(llm.Embeddings.BaseURL) == "" {
		return fmt.Errorf("config: llm.embeddings.baseURL is required (set llm.embeddings.baseURL, LLM_EMBEDDINGS_BASE_URL, or leave it to fall back to llm.baseURL)")
	}
	if strings.TrimSpace(llm.Embeddings.Model) == "" {
		return fmt.Errorf("config: llm.embeddings.model is required (set llm.embeddings.model, LLM_EMBEDDINGS_MODEL, or llm.embeddingModel)")
	}
	if strings.TrimSpace(llm.Embeddings.APIKey) == "" {
		return fmt.Errorf("config: llm.embeddings API key is required (set LLM_EMBEDDINGS_API_KEY, or LLM_API_KEY to share the completion key)")
	}
	switch strings.ToLower(strings.TrimSpace(llm.API)) {
	case "", APIChat, APIResponses:
	default:
		return fmt.Errorf("config: invalid llm.api %q (allowed: %s, %s)",
			strings.TrimSpace(llm.API), APIChat, APIResponses)
	}
	return nil
}

// reasoningEfforts lists the reasoning_effort values accepted by OpenAI reasoning models.
var reasoningEfforts = []string{"none", "minimal", "low", "medium", "high", "xhigh"}

// KnownReasoningEffort reports whether effort is empty (provider default) or a
// supported reasoning_effort value.
func KnownReasoningEffort(effort string) bool {
	effort = strings.ToLower(strings.TrimSpace(effort))
	if effort == "" {
		return true
	}
	for _, e := range reasoningEfforts {
		if e == effort {
			return true
		}
	}
	return false
}

func applyLLMDefaults(llm *LLMConfig) {
	if llm.TimeoutSeconds == 0 {
		llm.TimeoutSeconds = defaultLLMTimeoutSeconds
	}
	ResolveEmbeddings(llm)
}

// ResolveEmbeddings collapses the embeddings block onto the llm.* values it
// falls back to. It is idempotent, and exported so that a caller assembling an
// LLMConfig without going through Load still gets a usable embeddings
// endpoint rather than an empty one.
//
// The model is written back into EmbeddingModel rather than only read from it:
// knowledge search compares that field against kb_collections.embedding_model
// as an exact string, so letting the two spellings drift would return no
// results and no error.
func ResolveEmbeddings(llm *LLMConfig) {
	if llm.Embeddings.BaseURL == "" {
		llm.Embeddings.BaseURL = llm.BaseURL
	}
	if llm.Embeddings.APIKey == "" {
		llm.Embeddings.APIKey = llm.APIKey
	}
	if llm.Embeddings.Model == "" {
		llm.Embeddings.Model = llm.EmbeddingModel
	} else {
		llm.EmbeddingModel = llm.Embeddings.Model
	}
}

// resolveLLMAPIKey prefers LLM_API_KEY and falls back to OPENAI_API_KEY.
func resolveLLMAPIKey() string {
	if v := strings.TrimSpace(os.Getenv("LLM_API_KEY")); v != "" {
		return v
	}
	return strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
}

// resolveLLMModel prefers LLM_MODEL and falls back to OPENAI_MODEL.
func resolveLLMModel() string {
	if v := strings.TrimSpace(os.Getenv("LLM_MODEL")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("OPENAI_MODEL")); v != "" {
		return v
	}
	return "gpt-4o-mini"
}

func resolveLLMBaseURL() string {
	return strings.TrimSpace(os.Getenv("LLM_BASE_URL"))
}

// applyFileAppConfig copies toolchain YAML blocks that are parsed but not yet
// consumed by the nib backend (agent loop, log mirror, dir-based loaders).
func applyFileAppConfig(dst *Config, fc fileConfig) {
	dst.Agent = fc.Agent
	dst.Log = fc.Log
	if v := strings.TrimSpace(os.Getenv("LOG_LEVEL")); v != "" {
		dst.Log.Level = v
	}
	dst.Skills = SkillsConfig{Dir: strings.TrimSpace(fc.Skills.Dir)}
	dst.Rules = RulesConfig{Dir: strings.TrimSpace(fc.Rules.Dir)}
	dst.Prompts = PromptsConfig{Dir: strings.TrimSpace(fc.Prompts.Dir)}
	dst.IncludedTools = IncludedToolsConfig{Dir: strings.TrimSpace(fc.IncludedTools.Dir)}
	dst.MCP = MCPConfig{File: strings.TrimSpace(fc.MCP.File)}
	dst.Executor.File = strings.TrimSpace(fc.Executor.File)
}

// applyFileLLMConfig overlays toolchain YAML llm settings. APIKey stays from env.
func applyFileLLMConfig(dst *LLMConfig, src fileLLMConfig) {
	if v := strings.TrimSpace(src.Model); v != "" {
		dst.Model = v
	}
	if v := strings.TrimSpace(src.EmbeddingModel); v != "" {
		dst.EmbeddingModel = v
	}
	if v := strings.TrimSpace(src.BaseURL); v != "" {
		dst.BaseURL = v
	}
	if src.TimeoutSeconds > 0 {
		dst.TimeoutSeconds = src.TimeoutSeconds
	}
	if v := strings.TrimSpace(src.HTTPReferer); v != "" {
		dst.HTTPReferer = v
	}
	if v := strings.TrimSpace(src.AppTitle); v != "" {
		dst.AppTitle = v
	}
	// Assigned whenever the key is present, including when it is empty: that
	// is how a config clears an effort the environment set.
	if src.ReasoningEffort != nil {
		dst.ReasoningEffort = strings.TrimSpace(*src.ReasoningEffort)
	}
	if v := strings.TrimSpace(src.API); v != "" {
		dst.API = v
	}
	if v := strings.TrimSpace(src.Embeddings.BaseURL); v != "" {
		dst.Embeddings.BaseURL = v
	}
	if v := strings.TrimSpace(src.Embeddings.Model); v != "" {
		dst.Embeddings.Model = v
	}
	if src.Embeddings.Dimensions > 0 {
		dst.Embeddings.Dimensions = src.Embeddings.Dimensions
	}
}

// MergeProjects seeds runtime state from the projects array plus the optional
// singular project block. The singular project is appended when it has a
// non-empty name and no project in the array already uses that name.
func MergeProjects(projects []ProjectConfig, singular ProjectConfig) []ProjectConfig {
	name := strings.TrimSpace(singular.Name)
	if name == "" {
		if len(projects) == 0 {
			return nil
		}
		out := make([]ProjectConfig, len(projects))
		copy(out, projects)
		return out
	}
	singular.Name = name
	for _, p := range projects {
		if p.Name == name {
			out := make([]ProjectConfig, len(projects))
			copy(out, projects)
			return out
		}
	}
	out := make([]ProjectConfig, len(projects), len(projects)+1)
	copy(out, projects)
	return append(out, singular)
}

func readFileConfig(path string) (fileConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return fileConfig{}, fmt.Errorf("read config file %s: %w", path, err)
	}

	var fc fileConfig
	if err := yaml.Unmarshal(data, &fc); err != nil {
		return fileConfig{}, fmt.Errorf("parse config file %s: %w", path, err)
	}

	return fc, nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseSecretsEncryptionKey() ([]byte, error) {
	raw := strings.TrimSpace(os.Getenv("SECRETS_ENCRYPTION_KEY"))
	if raw == "" {
		return nil, nil
	}

	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("config: SECRETS_ENCRYPTION_KEY must be base64-encoded: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("config: SECRETS_ENCRYPTION_KEY must decode to 32 bytes, got %d", len(key))
	}
	return key, nil
}

func envOrDefaultInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envOrDefaultBool(key string, fallback bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

// applyFileMetricsConfig resolves metrics settings as env, then YAML, then default.
//
// Env has to win: the Dockerfile and the Helm deployment both pass METRICS_*,
// and a config.yaml baked into the image or left behind on a data volume must
// not override what the deployment asked for. That is why each field checks
// LookupEnv rather than following the log block, which overwrites itself from
// YAML wholesale and then patches LOG_LEVEL back on top.
func applyFileMetricsConfig(dst *MetricsConfig, fc fileMetricsConfig) {
	dst.Enabled = true
	if fc.Enabled != nil {
		dst.Enabled = *fc.Enabled
	}
	if _, ok := os.LookupEnv("METRICS_ENABLED"); ok {
		dst.Enabled = envOrDefaultBool("METRICS_ENABLED", dst.Enabled)
	}

	dst.Host = firstNonEmpty(os.Getenv("METRICS_HOST"), fc.Host, defaultMetricsHost)
	dst.Path = firstNonEmpty(os.Getenv("METRICS_PATH"), fc.Path, defaultMetricsPath)

	dst.Port = fc.Port
	if dst.Port == 0 {
		dst.Port = defaultMetricsPort
	}
	dst.Port = envOrDefaultInt("METRICS_PORT", dst.Port)

	dst.RefreshSeconds = fc.RefreshSeconds
	if dst.RefreshSeconds == 0 {
		dst.RefreshSeconds = defaultMetricsRefreshSeconds
	}
	dst.RefreshSeconds = envOrDefaultInt("METRICS_REFRESH_SECONDS", dst.RefreshSeconds)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
