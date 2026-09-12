package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/javdet/nib/internal/executor"
	"github.com/javdet/nib/internal/kb"
	"github.com/javdet/nib/internal/kbdoc"
	"github.com/javdet/nib/internal/llm"
	"github.com/javdet/nib/internal/mcpconfig"
	"github.com/javdet/nib/internal/repository"
	"github.com/javdet/nib/internal/rules"
	"github.com/javdet/nib/internal/service"
	"github.com/javdet/nib/internal/skills"
	"github.com/javdet/nib/internal/systemprompts"
)

type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		writeError(w, http.StatusNotFound, "resource not found")
	case errors.Is(err, repository.ErrAlreadyExists):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, systemprompts.ErrInvalidName):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, systemprompts.ErrNotEditable):
		// The prompt exists and reads fine; the write is refused by policy.
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, rules.ErrInvalidName):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, kbdoc.ErrInvalidName):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, skills.ErrInvalidName):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, skills.ErrSystemSkill):
		// Same shape as systemprompts.ErrNotEditable: the skill exists, but the
		// image owns it and the write is refused by policy.
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, mcpconfig.ErrInvalidName):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, mcpconfig.ErrInvalidJSON):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, mcpconfig.ErrUnresolvedVariable):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, mcpconfig.ErrStdioNotSupported):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, mcpconfig.ErrInvalidHeader):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, mcpconfig.ErrDiscoveryFailed):
		// The MCP server, not this backend, is what failed, and the message is
		// already redacted — pass it through so the UI can show the cause.
		writeError(w, http.StatusBadGateway, err.Error())
	case errors.Is(err, executor.ErrExecutorDisabled),
		errors.Is(err, executor.ErrInvalidType),
		errors.Is(err, executor.ErrInvalidPlatform),
		errors.Is(err, executor.ErrPlatformUnavailable),
		errors.Is(err, executor.ErrInvalidKubernetesAuthMode),
		errors.Is(err, executor.ErrKubernetesHostRequired),
		errors.Is(err, executor.ErrKubernetesTokenSecretRequired),
		errors.Is(err, executor.ErrInvalidJSON),
		errors.Is(err, executor.ErrImageRequired),
		errors.Is(err, executor.ErrRepoURLRequired),
		errors.Is(err, executor.ErrBranchRequired),
		errors.Is(err, executor.ErrTaskPromptRequired),
		errors.Is(err, executor.ErrSecretsIncomplete),
		errors.Is(err, executor.ErrPromptRequired),
		errors.Is(err, executor.ErrTargetBranchRequired),
		errors.Is(err, executor.ErrGitTokenRequired),
		errors.Is(err, executor.ErrLLMTokenRequired):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, executor.ErrNotImplemented):
		writeError(w, http.StatusNotImplemented, err.Error())
	case errors.Is(err, kb.ErrCollectionMismatch):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, kb.ErrDimensionMismatch):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrInvalidMode),
		errors.Is(err, service.ErrInvalidGitEmail),
		errors.Is(err, service.ErrConnectionURIRequired),
		errors.Is(err, service.ErrFilenameRequired),
		errors.Is(err, service.ErrEmptyFile),
		errors.Is(err, service.ErrNoChunks),
		errors.Is(err, service.ErrInvalidCollectionName),
		errors.Is(err, service.ErrEmptyDialogTitle),
		errors.Is(err, service.ErrInvalidDialogMode),
		errors.Is(err, service.ErrInvalidDialogParent),
		errors.Is(err, service.ErrInvalidMCPMetadata),
		errors.Is(err, service.ErrTooManyDialogTags),
		errors.Is(err, service.ErrInvalidVariableName),
		errors.Is(err, service.ErrInvalidVariableScope),
		errors.Is(err, service.ErrInvalidVariableValue),
		errors.Is(err, service.ErrSecretsEncryptionNotConfigured),
		errors.Is(err, service.ErrInvalidActionPlanScope),
		errors.Is(err, service.ErrActionPlanIndexOutOfRange),
		errors.Is(err, service.ErrActionNotFound),
		errors.Is(err, service.ErrActionNotCode),
		errors.Is(err, service.ErrActionRepositoryRequired),
		errors.Is(err, service.ErrExecutorTokenSecretRequired),
		errors.Is(err, service.ErrExecutorGitTokenSecretRequired),
		errors.Is(err, service.ErrExecutorSecretMissing):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrExecutionBusy),
		errors.Is(err, service.ErrActionAlreadyRunning),
		errors.Is(err, service.ErrNoExecutionRunning),
		errors.Is(err, service.ErrFanoutInProgress):
		// The request is well formed; something else holds the resource. The
		// message names what, because waiting or stopping it is the only choice
		// the operator has.
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrMCPConnectionUnavailable):
		writeError(w, http.StatusBadGateway, err.Error())
	case errors.Is(err, service.ErrTooManyToolFailures):
		writeError(w, http.StatusBadGateway, "The agent hit repeated tool failures and stopped. Review the tool configuration or retry the message.")
	case errors.Is(err, service.ErrVariableProtected):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusGatewayTimeout, "The AI provider did not respond in time. Try again.")
	default:
		var llmErr *llm.APIError
		if errors.As(err, &llmErr) {
			status, msg := llmErrorResponse(llmErr)
			slog.Error("llm provider error",
				"status", llmErr.StatusCode,
				"provider_message", llmErr.Message,
				"error", err,
			)
			writeError(w, status, msg)
			return
		}
		slog.Error("internal error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func llmErrorResponse(err *llm.APIError) (int, string) {
	switch err.StatusCode {
	case http.StatusPaymentRequired:
		return http.StatusPaymentRequired,
			"The AI provider rejected the request: not enough credits on the API key. Top up the provider balance or lower the model's max tokens limit, then retry."
	case http.StatusTooManyRequests:
		return http.StatusTooManyRequests,
			"The AI provider rate limit was reached. Wait a moment and retry."
	case http.StatusUnauthorized, http.StatusForbidden:
		return http.StatusBadGateway,
			"The AI provider rejected the API key. Check the provider credentials in the server configuration."
	case http.StatusNotFound:
		return http.StatusBadGateway,
			"The configured AI model is not available at the provider. Check the model name in the server configuration."
	case http.StatusBadRequest, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity:
		return http.StatusBadGateway,
			fmt.Sprintf("The AI provider rejected the request (HTTP %d). Check the model and request settings.", err.StatusCode)
	default:
		return http.StatusBadGateway,
			fmt.Sprintf("The AI provider is temporarily unavailable (HTTP %d). Try again in a moment.", err.StatusCode)
	}
}

// decodeJSON reads a JSON request body into dst. Returns false and writes
// an error response if the body cannot be decoded.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}
