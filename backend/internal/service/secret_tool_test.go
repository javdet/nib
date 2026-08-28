package service

import (
	"context"
	"strings"
	"testing"

	"github.com/javdet/nib/internal/domain"
	"github.com/google/uuid"
)

func TestExecuteGetSecretsReturnsNameAndDescription(t *testing.T) {
	t.Parallel()

	repo := newFakeSecretRepo()
	repo.secrets[uuid.New()] = domain.PromptSecret{
		Name:        "ApiToken",
		Description: "API token for external service",
	}
	repo.secrets[uuid.New()] = domain.PromptSecret{
		Name:        "DbPassword",
		Description: "Database password",
	}

	svc := NewSecretService(repo, nil)
	out, err := svc.ExecuteGetSecrets(context.Background(), nil)
	if err != nil {
		t.Fatalf("ExecuteGetSecrets() error = %v", err)
	}
	if !strings.Contains(out, `"name": "ApiToken"`) {
		t.Fatalf("output missing ApiToken: %s", out)
	}
	if !strings.Contains(out, `"description": "API token for external service"`) {
		t.Fatalf("output missing ApiToken description: %s", out)
	}
	if !strings.Contains(out, `"name": "DbPassword"`) {
		t.Fatalf("output missing DbPassword: %s", out)
	}
	if strings.Contains(out, "value") {
		t.Fatalf("output must not contain secret values: %s", out)
	}
}

func TestGetSecretsToolDef(t *testing.T) {
	t.Parallel()

	def := GetSecretsToolDef()
	if def.Name != GetSecretsToolName {
		t.Fatalf("Name = %q, want %q", def.Name, GetSecretsToolName)
	}
	if def.Description == "" {
		t.Fatal("Description is empty")
	}
	if len(def.Parameters) == 0 {
		t.Fatal("Parameters is empty")
	}
}
