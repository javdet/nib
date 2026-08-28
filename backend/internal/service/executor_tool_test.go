package service

import (
	"context"
	"strings"
	"testing"
)

func TestResolveRunExecutorRequest(t *testing.T) {
	t.Parallel()

	repo := newFakeSecretRepo()
	cipher := testCipher(t)
	secretSvc := NewSecretService(repo, cipher)

	if _, err := secretSvc.Create(context.Background(), SecretInput{
		Scope: "global",
		Name:  executorGitTokenSecretName,
		Value: "git-token-value",
	}); err != nil {
		t.Fatalf("Create git token: %v", err)
	}
	if _, err := secretSvc.Create(context.Background(), SecretInput{
		Scope: "global",
		Name:  executorLLMAPIKeySecretName,
		Value: "llm-key-value",
	}); err != nil {
		t.Fatalf("Create llm key: %v", err)
	}

	svc := &ChatService{
		variableRepo: &stubVariableRepo{
			vars: map[string]map[string]any{
				"global": {
					executorVarGitBaseURL:           "https://github.com/org/repo.git",
					executorVarVersionControlSystem: "github",
					executorVarGitUsername:          "agent-runner",
					executorVarGitEmail:             "agent-runner@localhost",
				},
			},
		},
		secretSvc:  secretSvc,
		llmBaseURL: "https://llm.example.com/v1",
	}

	req, err := svc.resolveRunExecutorRequest(context.Background(), map[string]any{
		"task_prompt": "add health check endpoint",
		"base_branch": "main",
		"work_branch": "feature/health-check",
		"pr_title":    "Add health check endpoint",
	})
	if err != nil {
		t.Fatalf("resolveRunExecutorRequest() error = %v", err)
	}

	if req.RepoURL != "https://github.com/org/repo.git" {
		t.Fatalf("RepoURL = %q", req.RepoURL)
	}
	if req.GitProvider != "github" {
		t.Fatalf("GitProvider = %q", req.GitProvider)
	}
	if req.BaseBranch != "main" {
		t.Fatalf("BaseBranch = %q", req.BaseBranch)
	}
	if req.WorkBranch != "feature/health-check" {
		t.Fatalf("WorkBranch = %q", req.WorkBranch)
	}
	if req.TaskPrompt != "add health check endpoint" {
		t.Fatalf("TaskPrompt = %q", req.TaskPrompt)
	}
	if req.PRTitle != "Add health check endpoint" {
		t.Fatalf("PRTitle = %q", req.PRTitle)
	}
	if req.GitToken != "git-token-value" {
		t.Fatalf("GitToken = %q", req.GitToken)
	}
	if req.LLMAPIKey != "llm-key-value" {
		t.Fatalf("LLMAPIKey = %q", req.LLMAPIKey)
	}
	if req.LLMBaseURL != "https://llm.example.com/v1" {
		t.Fatalf("LLMBaseURL = %q", req.LLMBaseURL)
	}
	if req.GitUsername != "agent-runner" {
		t.Fatalf("GitUsername = %q", req.GitUsername)
	}
	if req.GitEmail != "agent-runner@localhost" {
		t.Fatalf("GitEmail = %q", req.GitEmail)
	}
}

func TestResolveRunExecutorRequestMissingGitBaseURL(t *testing.T) {
	t.Parallel()

	svc := &ChatService{
		variableRepo: &stubVariableRepo{vars: map[string]map[string]any{"global": {}}},
		secretSvc:    NewSecretService(newFakeSecretRepo(), testCipher(t)),
	}

	_, err := svc.resolveRunExecutorRequest(context.Background(), map[string]any{
		"task_prompt": "do work",
		"base_branch": "main",
		"pr_title":    "PR title",
	})
	if err == nil || !strings.Contains(err.Error(), "GitBaseURL") {
		t.Fatalf("resolveRunExecutorRequest() error = %v, want GitBaseURL message", err)
	}
}

func TestResolveRunExecutorRequestMissingSecret(t *testing.T) {
	t.Parallel()

	svc := &ChatService{
		variableRepo: &stubVariableRepo{
			vars: map[string]map[string]any{
				"global": {executorVarGitBaseURL: "https://github.com/org/repo.git"},
			},
		},
		secretSvc: NewSecretService(newFakeSecretRepo(), testCipher(t)),
	}

	_, err := svc.resolveRunExecutorRequest(context.Background(), map[string]any{
		"task_prompt": "do work",
		"base_branch": "main",
		"pr_title":    "PR title",
	})
	if err == nil || !strings.Contains(err.Error(), executorGitTokenSecretName) {
		t.Fatalf("resolveRunExecutorRequest() error = %v, want missing secret message", err)
	}
}

func TestRunExecutorToolDef(t *testing.T) {
	t.Parallel()

	def := RunExecutorToolDef()
	if def.Name != RunExecutorToolName {
		t.Fatalf("Name = %q, want %q", def.Name, RunExecutorToolName)
	}
	if def.Description == "" {
		t.Fatal("Description is empty")
	}
	if len(def.Parameters) == 0 {
		t.Fatal("Parameters is empty")
	}
}
