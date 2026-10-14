package executor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/distribution/reference"
	"github.com/docker/docker/api/types/registry"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

// dockerHubServerAddress is the key Docker Hub credentials go under. Both the
// Docker CLI and the kubelet's keyring look Hub images up by this legacy
// address rather than by "docker.io".
const dockerHubServerAddress = "https://index.docker.io/v1/"

// registryCredentials is a registry login resolved for one launch.
type registryCredentials struct {
	Username      string
	Password      string
	ServerAddress string
}

// resolveRegistryCredentials returns nil when no registry login is configured,
// which is the public-image case and leaves every pull anonymous.
func (s *Service) resolveRegistryCredentials(ctx context.Context, cfg Config) (*registryCredentials, error) {
	if cfg.RegistryUsername == "" && cfg.RegistryPasswordSecretName == "" {
		return nil, nil
	}
	if cfg.RegistryUsername == "" || cfg.RegistryPasswordSecretName == "" {
		return nil, ErrRegistryAuthIncomplete
	}

	lookup := s.secretLookup()
	if lookup == nil {
		return nil, fmt.Errorf("%w: secret lookup is not configured", ErrRegistryAuth)
	}
	password, err := lookup.GetValueByName(ctx, secretScope, secretScopeName, cfg.RegistryPasswordSecretName)
	if err != nil {
		return nil, fmt.Errorf("%w: password secret %q: %v", ErrRegistryAuth, cfg.RegistryPasswordSecretName, err)
	}
	if password == "" {
		return nil, fmt.Errorf("%w: password secret %q is empty", ErrRegistryAuth, cfg.RegistryPasswordSecretName)
	}

	server, err := registryServerAddress(cfg.Image)
	if err != nil {
		return nil, err
	}
	return &registryCredentials{
		Username:      cfg.RegistryUsername,
		Password:      password,
		ServerAddress: server,
	}, nil
}

// registryServerAddress names the registry an image reference is pulled from.
func registryServerAddress(image string) (string, error) {
	named, err := reference.ParseNormalizedNamed(strings.TrimSpace(image))
	if err != nil {
		return "", fmt.Errorf("%w: parse image %q: %v", ErrRegistryAuth, image, err)
	}
	domain := reference.Domain(named)
	if domain == "docker.io" {
		return dockerHubServerAddress, nil
	}
	return domain, nil
}

// dockerRegistryAuth encodes the credentials for the X-Registry-Auth header of
// a Docker Engine pull. A nil login encodes as no header at all.
func (c *registryCredentials) dockerRegistryAuth() (string, error) {
	if c == nil {
		return "", nil
	}
	encoded, err := registry.EncodeAuthConfig(registry.AuthConfig{
		Username:      c.Username,
		Password:      c.Password,
		ServerAddress: c.ServerAddress,
	})
	if err != nil {
		return "", fmt.Errorf("%w: encode: %v", ErrRegistryAuth, err)
	}
	return encoded, nil
}

// dockerConfigJSON renders the credentials as the body of a
// kubernetes.io/dockerconfigjson Secret.
func (c *registryCredentials) dockerConfigJSON() ([]byte, error) {
	type entry struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Auth     string `json:"auth"`
	}
	data, err := json.Marshal(map[string]map[string]entry{
		"auths": {
			c.ServerAddress: {
				Username: c.Username,
				Password: c.Password,
				Auth:     base64.StdEncoding.EncodeToString([]byte(c.Username + ":" + c.Password)),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("%w: encode docker config: %v", ErrRegistryAuth, err)
	}
	return data, nil
}

func pullSecretName(jobName string) string {
	return jobName + "-registry"
}

// createPullSecret stores the registry login as a dockerconfigjson Secret for
// one Job. A Secret per Job, rather than one shared Secret nib keeps updated,
// means a password changed in Variables → Secrets reaches the next launch
// without nib having to reconcile anything already in the cluster.
func createPullSecret(ctx context.Context, secrets corev1client.SecretInterface, jobName string, creds *registryCredentials) (string, error) {
	body, err := creds.dockerConfigJSON()
	if err != nil {
		return "", err
	}
	name := pullSecretName(jobName)
	_, err = secrets.Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"app.kubernetes.io/name":      jobName,
				"app.kubernetes.io/component": "registry-auth",
			},
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{corev1.DockerConfigJsonKey: body},
	}, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("%w: create pull secret %q: %v", ErrKubernetesJobCreate, name, err)
	}
	return name, nil
}

// adoptPullSecret makes the Job the owner of its pull secret, so the garbage
// collector removes the Secret with the Job — on its TTL or a force stop alike.
// A failure only leaves a Secret behind, never breaks the run, so it is logged.
func adoptPullSecret(ctx context.Context, secrets corev1client.SecretInterface, name string, job *batchv1.Job) {
	secret, err := secrets.Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		secret.OwnerReferences = append(secret.OwnerReferences, metav1.OwnerReference{
			APIVersion: "batch/v1",
			Kind:       "Job",
			Name:       job.Name,
			UID:        job.UID,
		})
		_, err = secrets.Update(ctx, secret, metav1.UpdateOptions{})
	}
	if err != nil {
		slog.Warn("executor pull secret left without an owner; it will not be garbage collected",
			"secret", name, "job", job.Name, "error", err)
	}
}

// deletePullSecret removes a pull secret whose Job was never created.
func deletePullSecret(ctx context.Context, secrets corev1client.SecretInterface, name string) {
	if err := secrets.Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		slog.Warn("delete executor pull secret", "secret", name, "error", err)
	}
}

// setImagePullSecret adds name to the Job's pod spec after rendering, so an
// operator's job.yaml.tmpl override still pulls with the configured login.
func setImagePullSecret(job *batchv1.Job, name string) {
	spec := &job.Spec.Template.Spec
	for _, ref := range spec.ImagePullSecrets {
		if ref.Name == name {
			return
		}
	}
	spec.ImagePullSecrets = append(spec.ImagePullSecrets, corev1.LocalObjectReference{Name: name})
}
