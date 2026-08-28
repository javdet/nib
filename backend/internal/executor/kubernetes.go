package executor

import (
	"context"
	"fmt"
	"os"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func (s *Service) runActionKubernetes(ctx context.Context, cfg Config, req ActionRunRequest) (ActionRunResult, error) {
	if strings.TrimSpace(cfg.Image) == "" {
		return ActionRunResult{}, ErrImageRequired
	}
	if strings.TrimSpace(cfg.AgentSecretName) == "" {
		return ActionRunResult{}, ErrAgentSecretRequired
	}

	jobName, err := buildJobName()
	if err != nil {
		return ActionRunResult{}, err
	}

	values := buildActionRunValues(cfg, req, jobName)
	job, err := renderJobManifest(jobDataDir(s.config), values)
	if err != nil {
		return ActionRunResult{}, err
	}

	restCfg, err := s.loadKubernetesConfig(ctx, cfg)
	if err != nil {
		return ActionRunResult{}, err
	}

	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return ActionRunResult{}, fmt.Errorf("%w: create clientset: %v", ErrKubernetesConfigLoad, err)
	}

	namespace := cfg.Namespace
	created, err := clientset.BatchV1().Jobs(namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return ActionRunResult{}, fmt.Errorf("%w: %v", ErrKubernetesJobCreate, err)
	}

	return ActionRunResult{
		JobName:       created.Name,
		Namespace:     namespace,
		ContainerName: created.Name,
		ContainerID:   string(created.UID),
		TargetBranch:  strings.TrimSpace(req.TargetBranch),
		RepoURL:       strings.TrimSpace(req.RepoURL),
		Status:        "started",
	}, nil
}

func (s *Service) loadKubernetesConfig(ctx context.Context, cfg Config) (*rest.Config, error) {
	switch cfg.KubernetesAuthMode {
	case KubernetesAuthModeLocalConfig:
		return s.loadLocalKubeConfig(cfg)
	case KubernetesAuthModeToken:
		return s.loadTokenKubeConfig(ctx, cfg)
	default:
		return nil, fmt.Errorf("%w: unsupported auth mode %q", ErrKubernetesConfigLoad, cfg.KubernetesAuthMode)
	}
}

func (s *Service) loadLocalKubeConfig(cfg Config) (*rest.Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	if contextName := strings.TrimSpace(cfg.KubernetesContext); contextName != "" {
		configOverrides.CurrentContext = contextName
	}

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	restCfg, err := clientConfig.ClientConfig()
	if err == nil {
		return restCfg, nil
	}

	if _, statErr := os.Stat(loadingRules.GetDefaultFilename()); statErr == nil || os.IsNotExist(statErr) {
		inCluster, inErr := rest.InClusterConfig()
		if inErr == nil {
			return inCluster, nil
		}
	}

	return nil, fmt.Errorf("%w: %v", ErrKubernetesConfigLoad, err)
}

func (s *Service) loadTokenKubeConfig(ctx context.Context, cfg Config) (*rest.Config, error) {
	lookup := s.secretLookup()
	if lookup == nil {
		return nil, fmt.Errorf("%w: secret lookup is not configured", ErrKubernetesConfigLoad)
	}

	token, err := lookup.GetValueByName(ctx, secretScope, cfg.KubernetesTokenSecretName)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve kubernetes token secret %q: %v", ErrKubernetesConfigLoad, cfg.KubernetesTokenSecretName, err)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("%w: kubernetes token secret %q is empty", ErrKubernetesConfigLoad, cfg.KubernetesTokenSecretName)
	}

	tlsCfg := rest.TLSClientConfig{
		Insecure: cfg.KubernetesInsecureSkipTLSVerify,
	}
	if ca := strings.TrimSpace(cfg.KubernetesCACert); ca != "" {
		tlsCfg.CAData = []byte(ca)
	}

	return &rest.Config{
		Host:        cfg.KubernetesHost,
		BearerToken: token,
		TLSClientConfig: tlsCfg,
	}, nil
}