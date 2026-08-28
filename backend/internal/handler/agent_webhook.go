package handler

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/javdet/nib/internal/service"
	"github.com/google/uuid"
)

// AgentWebhookHandler receives completion callbacks from agent-runner containers.
type AgentWebhookHandler struct {
	chatSvc      *service.ChatService
	webhookToken string
}

// NewAgentWebhookHandler creates an AgentWebhookHandler.
func NewAgentWebhookHandler(chatSvc *service.ChatService, webhookToken string) *AgentWebhookHandler {
	return &AgentWebhookHandler{
		chatSvc:      chatSvc,
		webhookToken: strings.TrimSpace(webhookToken),
	}
}

type agentWebhookPayload struct {
	Status       string  `json:"status"`
	ExitCode     int     `json:"exit_code"`
	Result       string  `json:"result"`
	LogTail      string  `json:"log_tail"`
	TaskID       string  `json:"task_id"`
	ThreadRootID string  `json:"thread_root_id"`
	ChatID       string  `json:"chat_id"`
	Job          jobInfo `json:"job"`
	Repo         repoInfo `json:"repo"`
	Usage        usageInfo `json:"usage"`
}

type jobInfo struct {
	Name         string `json:"name"`
	Pod          string `json:"pod"`
	Namespace    string `json:"namespace"`
	ImageVersion string `json:"image_version"`
}

type repoInfo struct {
	URL          string `json:"url"`
	BaseBranch   string `json:"base_branch"`
	TargetBranch string `json:"target_branch"`
	Pushed       bool   `json:"pushed"`
	PRURL        string `json:"pr_url"`
}

type usageInfo struct {
	DurationMS   int     `json:"duration_ms"`
	NumTurns     int     `json:"num_turns"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	SessionID    string  `json:"session_id"`
}

func (h *AgentWebhookHandler) Receive() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.authorize(r) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		var req agentWebhookPayload
		if !decodeJSON(w, r, &req) {
			return
		}

		chatID := strings.TrimSpace(req.ChatID)
		if chatID == "" {
			writeError(w, http.StatusBadRequest, "chat_id is required")
			return
		}
		dialogID, err := uuid.Parse(chatID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "chat_id must be a valid UUID")
			return
		}

		_, err = h.chatSvc.AppendAgentResult(r.Context(), dialogID, service.AgentRunResult{
			Status:       req.Status,
			ExitCode:     req.ExitCode,
			Result:       req.Result,
			LogTail:      req.LogTail,
			TaskID:       req.TaskID,
			ThreadRootID: req.ThreadRootID,
			ChatID:       req.ChatID,
			JobName:      req.Job.Name,
			JobPod:       req.Job.Pod,
			JobNamespace: req.Job.Namespace,
			ImageVersion: req.Job.ImageVersion,
			RepoURL:      req.Repo.URL,
			BaseBranch:   req.Repo.BaseBranch,
			TargetBranch: req.Repo.TargetBranch,
			RepoPushed:   req.Repo.Pushed,
			PRURL:        req.Repo.PRURL,
			DurationMS:   req.Usage.DurationMS,
			NumTurns:     req.Usage.NumTurns,
			TotalCostUSD: req.Usage.TotalCostUSD,
			SessionID:    req.Usage.SessionID,
		})
		if err != nil {
			handleServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusAccepted, map[string]string{"status": "ok"})
	}
}

func (h *AgentWebhookHandler) authorize(r *http.Request) bool {
	if h.webhookToken == "" {
		return true
	}

	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return false
	}
	token := strings.TrimSpace(strings.TrimPrefix(auth, prefix))
	return subtle.ConstantTimeCompare([]byte(token), []byte(h.webhookToken)) == 1
}
