package handler

import (
	"net/http"

	"github.com/javdet/nib/internal/executor"
)

// ExecutorHandler exposes HTTP endpoints for executor configuration.
type ExecutorHandler struct {
	svc *executor.Service
}

// NewExecutorHandler creates an ExecutorHandler.
func NewExecutorHandler(svc *executor.Service) *ExecutorHandler {
	return &ExecutorHandler{svc: svc}
}

type executorConfigPayload struct {
	Type                      executor.Type                `json:"type"`
	Platform                  executor.Platform            `json:"platform"`
	KubernetesAuthMode        executor.KubernetesAuthMode  `json:"kubernetesAuthMode"`
	KubernetesContext         string                       `json:"kubernetesContext"`
	KubernetesHost            string                       `json:"kubernetesHost"`
	KubernetesTokenSecretName string                       `json:"kubernetesTokenSecretName"`
	KubernetesCACert          string                       `json:"kubernetesCACert"`
	KubernetesInsecureSkipTLSVerify bool                   `json:"kubernetesInsecureSkipTLSVerify"`
	Namespace                 string                       `json:"namespace"`
	ServiceAccount            string                       `json:"serviceAccount"`
	AgentSecretName           string                       `json:"agentSecretName"`
	AgentLimitCPU             string                       `json:"agentLimitCPU"`
	AgentLimitMemory          string                       `json:"agentLimitMemory"`
	AgentRequestCPU           string                       `json:"agentRequestCPU"`
	AgentRequestMemory        string                       `json:"agentRequestMemory"`
	AgentMCPConfig            string                       `json:"agentMCPConfig"`
	JobTTLSeconds             int                          `json:"jobTTLSeconds"`
	Agent                     executor.Agent               `json:"agent"`
	AuthType                  executor.AuthType            `json:"authType"`
	TokenSecretName           string                       `json:"tokenSecretName"`
	GitTokenSecretName        string                       `json:"gitTokenSecretName"`
	BaseURL                     string `json:"baseURL"`
	WebhookBaseURL              string `json:"webhookBaseURL"`
	Image                       string `json:"image"`
	LLMModel                    string `json:"llmModel"`
}

type executorConfigResponse struct {
	Type                      executor.Type                       `json:"type"`
	Platform                  executor.Platform                   `json:"platform"`
	KubernetesAuthMode        executor.KubernetesAuthMode         `json:"kubernetesAuthMode"`
	KubernetesContext         string                              `json:"kubernetesContext"`
	KubernetesHost            string                              `json:"kubernetesHost"`
	KubernetesTokenSecretName string                              `json:"kubernetesTokenSecretName"`
	KubernetesCACert          string                              `json:"kubernetesCACert"`
	KubernetesInsecureSkipTLSVerify bool                          `json:"kubernetesInsecureSkipTLSVerify"`
	Namespace                 string                              `json:"namespace"`
	ServiceAccount            string                              `json:"serviceAccount"`
	AgentSecretName           string                              `json:"agentSecretName"`
	AgentLimitCPU             string                              `json:"agentLimitCPU"`
	AgentLimitMemory          string                              `json:"agentLimitMemory"`
	AgentRequestCPU           string                              `json:"agentRequestCPU"`
	AgentRequestMemory        string                              `json:"agentRequestMemory"`
	AgentMCPConfig            string                              `json:"agentMCPConfig"`
	JobTTLSeconds             int                                 `json:"jobTTLSeconds"`
	Agent                     executor.Agent                      `json:"agent"`
	AuthType                  executor.AuthType                   `json:"authType"`
	TokenSecretName           string                              `json:"tokenSecretName"`
	GitTokenSecretName        string                              `json:"gitTokenSecretName"`
	BaseURL                   string                              `json:"baseURL"`
	WebhookBaseURL            string                              `json:"webhookBaseURL"`
	Image                     string                              `json:"image"`
	LLMModel                  string                              `json:"llmModel"`
	TypeOptions               []executor.TypeOption               `json:"typeOptions"`
	PlatformOptions           []executor.PlatformOption           `json:"platformOptions"`
	KubernetesAuthModeOptions []executor.KubernetesAuthModeOption `json:"kubernetesAuthModeOptions"`
	AgentOptions              []executor.AgentOption              `json:"agentOptions"`
	AuthTypeOptions           []executor.AuthTypeOption           `json:"authTypeOptions"`
}

func newExecutorConfigResponse(cfg executor.Config) executorConfigResponse {
	return executorConfigResponse{
		Type:                      cfg.Type,
		Platform:                  cfg.Platform,
		KubernetesAuthMode:        cfg.KubernetesAuthMode,
		KubernetesContext:         cfg.KubernetesContext,
		KubernetesHost:            cfg.KubernetesHost,
		KubernetesTokenSecretName: cfg.KubernetesTokenSecretName,
		KubernetesCACert:          cfg.KubernetesCACert,
		KubernetesInsecureSkipTLSVerify: cfg.KubernetesInsecureSkipTLSVerify,
		Namespace:                 cfg.Namespace,
		ServiceAccount:            cfg.ServiceAccount,
		AgentSecretName:           cfg.AgentSecretName,
		AgentLimitCPU:             cfg.AgentLimitCPU,
		AgentLimitMemory:          cfg.AgentLimitMemory,
		AgentRequestCPU:           cfg.AgentRequestCPU,
		AgentRequestMemory:        cfg.AgentRequestMemory,
		AgentMCPConfig:            cfg.AgentMCPConfig,
		JobTTLSeconds:             cfg.JobTTLSeconds,
		Agent:                     cfg.Agent,
		AuthType:                  cfg.AuthType,
		TokenSecretName:           cfg.TokenSecretName,
		GitTokenSecretName:        cfg.GitTokenSecretName,
		BaseURL:                   cfg.BaseURL,
		WebhookBaseURL:            cfg.WebhookBaseURL,
		Image:                     cfg.Image,
		LLMModel:                  cfg.LLMModel,
		TypeOptions:               executor.TypeOptions,
		PlatformOptions:           executor.PlatformOptions,
		KubernetesAuthModeOptions: executor.KubernetesAuthModeOptions,
		AgentOptions:              executor.AgentOptions,
		AuthTypeOptions:           executor.AuthTypeOptions,
	}
}

func (h *ExecutorHandler) GetConfig() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, err := h.svc.ConfigStore().Get()
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, newExecutorConfigResponse(cfg))
	}
}

func (h *ExecutorHandler) UpdateConfig() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req executorConfigPayload
		if !decodeJSON(w, r, &req) {
			return
		}
		if err := h.svc.ConfigStore().Set(executor.Config{
			Type:                      req.Type,
			Platform:                  req.Platform,
			KubernetesAuthMode:        req.KubernetesAuthMode,
			KubernetesContext:         req.KubernetesContext,
			KubernetesHost:            req.KubernetesHost,
			KubernetesTokenSecretName: req.KubernetesTokenSecretName,
			KubernetesCACert:          req.KubernetesCACert,
			KubernetesInsecureSkipTLSVerify: req.KubernetesInsecureSkipTLSVerify,
			Namespace:                 req.Namespace,
			ServiceAccount:            req.ServiceAccount,
			AgentSecretName:           req.AgentSecretName,
			AgentLimitCPU:             req.AgentLimitCPU,
			AgentLimitMemory:          req.AgentLimitMemory,
			AgentRequestCPU:           req.AgentRequestCPU,
			AgentRequestMemory:        req.AgentRequestMemory,
			AgentMCPConfig:            req.AgentMCPConfig,
			JobTTLSeconds:             req.JobTTLSeconds,
			Agent:                     req.Agent,
			AuthType:                  req.AuthType,
			TokenSecretName:           req.TokenSecretName,
			GitTokenSecretName:        req.GitTokenSecretName,
			BaseURL:                   req.BaseURL,
			WebhookBaseURL:            req.WebhookBaseURL,
			Image:                     req.Image,
			LLMModel:                  req.LLMModel,
		}); err != nil {
			handleServiceError(w, err)
			return
		}
		cfg, err := h.svc.ConfigStore().Get()
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, newExecutorConfigResponse(cfg))
	}
}
