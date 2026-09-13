package handler

import (
	"crypto/subtle"
	"net/http"
	"strconv"
	"strings"

	"github.com/javdet/nib/internal/metrics"
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
	DurationMS int `json:"duration_ms"`
	NumTurns   int `json:"num_turns"`
	// TotalCostUSD is a pointer, and 0 is still treated as "unreported" by the
	// service: agent-entrypoint.sh builds this with jq '... // 0', so an agent
	// that prices nothing (the Codex path does not) sends 0 rather than
	// omitting the field. Storing that 0 would let the cost chart claim a run
	// was free when it merely stayed silent.
	TotalCostUSD *float64 `json:"total_cost_usd"`
	SessionID    string   `json:"session_id"`
	// Already sent by the container and, until now, silently dropped here.
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

func (h *AgentWebhookHandler) Receive() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.authorize(r) {
			metrics.RecordAgentRunnerWebhook(metrics.WebhookUnauthorized)
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		var req agentWebhookPayload
		if !decodeJSON(w, r, &req) {
			metrics.RecordAgentRunnerWebhook(metrics.WebhookBadRequest)
			return
		}

		chatID := strings.TrimSpace(req.ChatID)
		if chatID == "" {
			metrics.RecordAgentRunnerWebhook(metrics.WebhookBadRequest)
			writeError(w, http.StatusBadRequest, "chat_id is required")
			return
		}
		dialogID, err := uuid.Parse(chatID)
		if err != nil {
			metrics.RecordAgentRunnerWebhook(metrics.WebhookBadRequest)
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
			InputTokens:  req.Usage.InputTokens,
			OutputTokens: req.Usage.OutputTokens,
		})
		if err != nil {
			metrics.RecordAgentRunnerWebhook(metrics.WebhookError)
			handleServiceError(w, err)
			return
		}

		metrics.RecordAgentRunnerWebhook(metrics.WebhookAccepted)
		// The status a container reports is free text off the network, so it is
		// mapped onto a known set before it becomes a label.
		metrics.RecordAgentRunnerResult(
			metrics.NormalizeAgentRunnerStatus(strings.TrimSpace(req.Status)),
			strconv.FormatBool(req.Repo.Pushed),
			req.Usage.DurationMS,
			req.Usage.NumTurns,
			costOrZero(req.Usage.TotalCostUSD),
		)

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

// costOrZero feeds the Prometheus counter, which cannot express "unknown" and
// already ignores a zero.
func costOrZero(cost *float64) float64 {
	if cost == nil {
		return 0
	}
	return *cost
}
