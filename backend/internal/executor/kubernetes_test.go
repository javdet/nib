package executor

import (
	"strings"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

func TestNormalizeConfigKubernetesDefaults(t *testing.T) {
	t.Parallel()

	got := NormalizeConfig(Config{
		Type:     TypeRemote,
		Platform: PlatformKubernetes,
	})
	if got.Namespace != defaultNamespace {
		t.Fatalf("namespace = %q, want %q", got.Namespace, defaultNamespace)
	}
	if got.JobTTLSeconds != defaultJobTTLSeconds {
		t.Fatalf("jobTTLSeconds = %d, want %d", got.JobTTLSeconds, defaultJobTTLSeconds)
	}
}

func TestNormalizeConfigClearsKubernetesFieldsForLocal(t *testing.T) {
	t.Parallel()

	got := NormalizeConfig(Config{
		Type:                            TypeLocal,
		Namespace:                       "production",
		KubernetesCACert:                "ca-data",
		KubernetesInsecureSkipTLSVerify: true,
		JobTTLSeconds:                   7200,
	})
	if got.Namespace != "" {
		t.Fatalf("namespace = %q, want empty for local", got.Namespace)
	}
	if got.KubernetesCACert != "" {
		t.Fatalf("kubernetesCACert = %q, want empty for local", got.KubernetesCACert)
	}
	if got.KubernetesInsecureSkipTLSVerify {
		t.Fatal("kubernetesInsecureSkipTLSVerify should be false for local")
	}
	if got.JobTTLSeconds != 0 {
		t.Fatalf("jobTTLSeconds = %d, want 0 for local", got.JobTTLSeconds)
	}
}

func TestRenderJobManifestQuotesPrompt(t *testing.T) {
	t.Parallel()

	values := actionRunValues{
		JobName:            "nib-12345678",
		Namespace:          "default",
		TaskID:             "DO-236",
		ServiceAccount:     "default",
		AgentImage:         "javdet/nib-agent:0.2.0",
		Prompt:             "line one\n\"quoted\" value: ok",
		RepoURL:            "https://github.com/org/repo",
		TargetBranch:       "nib/DO-236",
		PRTitle:            "chore: update",
		GitAuthorName:      "Agent",
		GitAuthorEmail:     "agent@example.com",
		ChatID:             "0f2b9f5c-0f0e-4f7b-9d2e-2f2c7c9a1111",
		WebhookURL:         "http://nib.example.com/api/v1/agent-runner/webhook",
		AgentSecret:        "nib-agent-secret",
		AgentLimitCPU:      "1",
		AgentLimitMemory:   "1Gi",
		AgentRequestCPU:    "1",
		AgentRequestMemory: "256Mi",
		AgentMCPConfig:     "nib-mcp-config",
		TaskTypeLabel:      "code",
		ThreadRootLabel:      "do-236",
		AllowedTools:       actionRunAllowedTools,
		JobTTLSeconds:      3600,
		AgentType:          "claude-code",
		GitProvider:        "github",
		AnthropicModel:     "anthropic/claude-sonnet-4-5",
	}

	job, err := renderJobManifest("", values)
	if err != nil {
		t.Fatalf("renderJobManifest() error = %v", err)
	}

	if job.Namespace != "default" {
		t.Fatalf("job namespace = %q, want default", job.Namespace)
	}
	if job.Spec.TTLSecondsAfterFinished == nil || *job.Spec.TTLSecondsAfterFinished != 3600 {
		t.Fatalf("ttlSecondsAfterFinished = %v, want 3600", job.Spec.TTLSecondsAfterFinished)
	}
	if job.Spec.Template.Spec.ServiceAccountName != "default" {
		t.Fatalf("serviceAccount = %q, want default", job.Spec.Template.Spec.ServiceAccountName)
	}

	container := job.Spec.Template.Spec.Containers[0]
	if container.Image != values.AgentImage {
		t.Fatalf("image = %q, want %q", container.Image, values.AgentImage)
	}
	if len(container.EnvFrom) != 1 || container.EnvFrom[0].SecretRef == nil {
		t.Fatal("expected envFrom secretRef")
	}
	if container.EnvFrom[0].SecretRef.Name != values.AgentSecret {
		t.Fatalf("agent secret = %q, want %q", container.EnvFrom[0].SecretRef.Name, values.AgentSecret)
	}
	if container.EnvFrom[0].SecretRef.Optional != nil && *container.EnvFrom[0].SecretRef.Optional {
		t.Fatal("agent secretRef must not be optional")
	}

	rendered, err := yaml.Marshal(job)
	if err != nil {
		t.Fatalf("marshal job: %v", err)
	}
	body := string(rendered)
	for _, forbidden := range []string{"ghp_token", "llm-token", "Authorization: Bearer"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("rendered job contains credential fragment %q", forbidden)
		}
	}

	promptEnv := envValue(container.Env, "PROMPT")
	if promptEnv != values.Prompt {
		t.Fatalf("PROMPT env = %q, want %q", promptEnv, values.Prompt)
	}
}

func TestValidateActionRunRequestSkipsTokensForKubernetes(t *testing.T) {
	t.Parallel()

	req := validActionRunRequest()
	req.GitToken = ""
	req.LLMToken = ""

	cfg := Config{
		Type:     TypeRemote,
		Platform: PlatformKubernetes,
	}
	if err := validateActionRunRequest(req, cfg); err != nil {
		t.Fatalf("validateActionRunRequest() error = %v, want nil", err)
	}

	cfg.Type = TypeLocal
	if err := validateActionRunRequest(req, cfg); err == nil {
		t.Fatal("validateActionRunRequest() expected error for local executor without tokens")
	}
}

func envValue(env []corev1.EnvVar, name string) string {
	for _, item := range env {
		if item.Name == name {
			return item.Value
		}
	}
	return ""
}

var _ = batchv1.SchemeGroupVersion
