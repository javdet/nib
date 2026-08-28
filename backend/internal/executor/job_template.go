package executor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	batchv1 "k8s.io/api/batch/v1"
	"sigs.k8s.io/yaml"

	"github.com/javdet/nib/internal/executor/templates"
)

const jobTemplateFileName = "job.yaml.tmpl"

var k8sLabelSanitizer = regexp.MustCompile(`[^a-z0-9.-]+`)

// actionRunValues holds the shared parameters for Docker env and Kubernetes Job
// template rendering.
type actionRunValues struct {
	JobName            string
	Namespace          string
	TaskID             string
	ServiceAccount     string
	AgentImage         string
	Prompt             string
	RepoURL            string
	BaseBranch         string
	TargetBranch       string
	PRTitle            string
	GitAuthorName      string
	GitAuthorEmail     string
	ChatID             string
	WebhookURL         string
	AgentSecret        string
	AgentLimitCPU      string
	AgentLimitMemory   string
	AgentRequestCPU    string
	AgentRequestMemory string
	AgentMCPConfig     string
	TaskTypeLabel      string
	ThreadRootLabel    string
	AllowedTools       string
	JobTTLSeconds      int
	AgentType          string
	GitProvider        string
	AnthropicModel     string
	AnthropicBaseURL   string
	OpenAIModel        string
	OpenAIBaseURL      string
}

func buildActionRunValues(cfg Config, req ActionRunRequest, jobName string) actionRunValues {
	agent := cfg.Agent
	if agent == "" {
		agent = AgentClaudeCode
	}

	gitProvider := strings.TrimSpace(req.GitProvider)
	if gitProvider == "" {
		gitProvider = "github"
	}

	threadRoot := strings.TrimSpace(req.TaskID)
	if threadRoot == "" {
		threadRoot = strings.TrimSpace(req.ChatID)
	}

	values := actionRunValues{
		JobName:            jobName,
		Namespace:          cfg.Namespace,
		TaskID:             strings.TrimSpace(req.TaskID),
		ServiceAccount:     cfg.ServiceAccount,
		AgentImage:         cfg.Image,
		Prompt:             strings.TrimSpace(req.Prompt),
		RepoURL:            strings.TrimSpace(req.RepoURL),
		TargetBranch:       strings.TrimSpace(req.TargetBranch),
		PRTitle:            strings.TrimSpace(req.PRTitle),
		GitAuthorName:      strings.TrimSpace(req.GitUsername),
		GitAuthorEmail:     strings.TrimSpace(req.GitEmail),
		ChatID:             strings.TrimSpace(req.ChatID),
		WebhookURL:         resolveAgentWebhookURL(cfg.WebhookBaseURL),
		AgentSecret:        cfg.AgentSecretName,
		AgentLimitCPU:      cfg.AgentLimitCPU,
		AgentLimitMemory:   cfg.AgentLimitMemory,
		AgentRequestCPU:    cfg.AgentRequestCPU,
		AgentRequestMemory: cfg.AgentRequestMemory,
		AgentMCPConfig:     cfg.AgentMCPConfig,
		TaskTypeLabel:      sanitizeK8sLabel("code"),
		ThreadRootLabel:    sanitizeK8sLabel(threadRoot),
		AllowedTools:       actionRunAllowedTools,
		JobTTLSeconds:      cfg.JobTTLSeconds,
		AgentType:          string(agent),
		GitProvider:        gitProvider,
	}

	switch agent {
	case AgentCodex:
		if v := strings.TrimSpace(cfg.LLMModel); v != "" {
			values.OpenAIModel = v
		}
		if v := strings.TrimSpace(cfg.BaseURL); v != "" {
			values.OpenAIBaseURL = v
		}
	default:
		if v := strings.TrimSpace(cfg.LLMModel); v != "" {
			values.AnthropicModel = v
		}
		if cfg.AuthType == AuthTypeAPIKey {
			if v := strings.TrimSpace(cfg.BaseURL); v != "" {
				values.AnthropicBaseURL = v
			}
		}
	}

	return values
}

func sanitizeK8sLabel(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = k8sLabelSanitizer.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-.")
	if len(value) > 63 {
		value = strings.Trim(value[:63], "-.")
	}
	if value == "" {
		return "unknown"
	}
	return value
}

func loadJobTemplate(dataDir string) (string, error) {
	if dataDir != "" {
		override := filepath.Join(dataDir, jobTemplateFileName)
		if data, err := os.ReadFile(override); err == nil {
			return string(data), nil
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("read job template override: %w", err)
		}
	}
	return templates.JobYAML, nil
}

func renderJobManifest(dataDir string, values actionRunValues) (*batchv1.Job, error) {
	source, err := loadJobTemplate(dataDir)
	if err != nil {
		return nil, err
	}

	tmpl, err := template.New("job").Funcs(template.FuncMap{
		"quote": quoteYAMLString,
	}).Parse(source)
	if err != nil {
		return nil, fmt.Errorf("%w: parse template: %v", ErrKubernetesJobRender, err)
	}

	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, values); err != nil {
		return nil, fmt.Errorf("%w: execute template: %v", ErrKubernetesJobRender, err)
	}

	var job batchv1.Job
	if err := yaml.UnmarshalStrict(rendered.Bytes(), &job); err != nil {
		return nil, fmt.Errorf("%w: decode yaml: %v", ErrKubernetesJobRender, err)
	}
	return &job, nil
}

func quoteYAMLString(value string) (string, error) {
	quoted, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(quoted), nil
}

func jobDataDir(store *ConfigStore) string {
	if store == nil {
		return ""
	}
	return filepath.Dir(store.Path())
}
