package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/javdet/nib/internal/executor"
)

// TestExecutorConfigRoundTrip guards the four separate literal blocks a config
// field has to be threaded through: the request payload, the response struct,
// newExecutorConfigResponse and the Config literal in UpdateConfig. Missing any
// one of them silently drops the field instead of failing to compile.
func TestExecutorConfigRoundTrip(t *testing.T) {
	t.Parallel()

	store := executor.NewConfigStore(t.TempDir(), "executor.json", "")
	h := NewExecutorHandler(executor.NewService(store, executor.Secrets{}))

	body, err := json.Marshal(executorConfigPayload{
		Type:               executor.TypeLocal,
		Agent:              executor.AgentClaudeCode,
		AuthType:           executor.AuthTypeAPIKey,
		TokenSecretName:    "MY_LLM_KEY",
		GitTokenSecretName: "MY_GIT_TOKEN",
		Image:              "agent-runner:local",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	rec := httptest.NewRecorder()
	h.UpdateConfig()(rec, httptest.NewRequest(http.MethodPut, "/api/v1/executor/config", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("UpdateConfig() status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var updated executorConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("unmarshal update response: %v", err)
	}
	if updated.GitTokenSecretName != "MY_GIT_TOKEN" {
		t.Errorf("UpdateConfig() gitTokenSecretName = %q, want MY_GIT_TOKEN", updated.GitTokenSecretName)
	}

	rec = httptest.NewRecorder()
	h.GetConfig()(rec, httptest.NewRequest(http.MethodGet, "/api/v1/executor/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GetConfig() status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var fetched executorConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("unmarshal get response: %v", err)
	}
	if fetched.GitTokenSecretName != "MY_GIT_TOKEN" {
		t.Errorf("GetConfig() gitTokenSecretName = %q, want MY_GIT_TOKEN", fetched.GitTokenSecretName)
	}
	if fetched.TokenSecretName != "MY_LLM_KEY" {
		t.Errorf("GetConfig() tokenSecretName = %q, want MY_LLM_KEY", fetched.TokenSecretName)
	}
}
