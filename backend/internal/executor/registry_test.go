package executor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/errdefs"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// stubSecretLookup answers every lookup with one canned value and records the
// name it was asked for.
type stubSecretLookup struct {
	value string
	err   error
	asked []string
}

func (s *stubSecretLookup) GetValueByName(_ context.Context, _, _, name string) (string, error) {
	s.asked = append(s.asked, name)
	return s.value, s.err
}

func newRegistryTestService(lookup SecretLookup) *Service {
	svc := NewService(nil, Secrets{})
	if lookup != nil {
		svc.SetSecretLookup(lookup)
	}
	return svc
}

func TestRegistryServerAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		image string
		want  string
	}{
		{image: "javdet/nib-agent:latest", want: dockerHubServerAddress},
		{image: "nginx", want: dockerHubServerAddress},
		{image: "docker.io/javdet/nib-agent:v0.8.2", want: dockerHubServerAddress},
		{image: "ghcr.io/acme/nib-agent:v1", want: "ghcr.io"},
		{image: "registry.example.com:5000/team/agent@sha256:" + sha256Zero, want: "registry.example.com:5000"},
	}
	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			t.Parallel()
			got, err := registryServerAddress(tt.image)
			if err != nil {
				t.Fatalf("registryServerAddress(%q) error = %v", tt.image, err)
			}
			if got != tt.want {
				t.Fatalf("registryServerAddress(%q) = %q, want %q", tt.image, got, tt.want)
			}
		})
	}

	if _, err := registryServerAddress("Not/A Valid:ref"); !errors.Is(err, ErrRegistryAuth) {
		t.Fatalf("registryServerAddress(invalid) error = %v, want ErrRegistryAuth", err)
	}
}

const sha256Zero = "0000000000000000000000000000000000000000000000000000000000000000"

func TestResolveRegistryCredentials(t *testing.T) {
	t.Parallel()

	cfg := Config{
		Image:                      "registry.example.com/team/agent:v1",
		RegistryUsername:           "robot",
		RegistryPasswordSecretName: "REGISTRY_PASSWORD",
	}

	t.Run("none configured", func(t *testing.T) {
		t.Parallel()
		lookup := &stubSecretLookup{value: "unused"}
		creds, err := newRegistryTestService(lookup).resolveRegistryCredentials(context.Background(), Config{Image: cfg.Image})
		if err != nil || creds != nil {
			t.Fatalf("resolveRegistryCredentials() = %v, %v, want nil, nil", creds, err)
		}
		if len(lookup.asked) != 0 {
			t.Fatalf("asked = %v, want no secret lookup for a public image", lookup.asked)
		}
	})

	t.Run("resolved", func(t *testing.T) {
		t.Parallel()
		lookup := &stubSecretLookup{value: "s3cret"}
		creds, err := newRegistryTestService(lookup).resolveRegistryCredentials(context.Background(), cfg)
		if err != nil {
			t.Fatalf("resolveRegistryCredentials() error = %v", err)
		}
		want := registryCredentials{Username: "robot", Password: "s3cret", ServerAddress: "registry.example.com"}
		if *creds != want {
			t.Fatalf("resolveRegistryCredentials() = %+v, want %+v", *creds, want)
		}
		if len(lookup.asked) != 1 || lookup.asked[0] != "REGISTRY_PASSWORD" {
			t.Fatalf("asked = %v, want [REGISTRY_PASSWORD]", lookup.asked)
		}
	})

	failures := []struct {
		name   string
		lookup SecretLookup
		cfg    Config
		want   error
	}{
		{name: "missing secret", lookup: &stubSecretLookup{err: errors.New("not found")}, cfg: cfg, want: ErrRegistryAuth},
		{name: "empty secret", lookup: &stubSecretLookup{}, cfg: cfg, want: ErrRegistryAuth},
		{name: "no lookup", lookup: nil, cfg: cfg, want: ErrRegistryAuth},
		{name: "username only", lookup: &stubSecretLookup{value: "x"}, cfg: Config{Image: cfg.Image, RegistryUsername: "robot"}, want: ErrRegistryAuthIncomplete},
	}
	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := newRegistryTestService(tt.lookup).resolveRegistryCredentials(context.Background(), tt.cfg)
			if !errors.Is(err, tt.want) {
				t.Fatalf("resolveRegistryCredentials() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestValidateConfigRegistryPairing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		username string
		secret   string
		want     error
	}{
		{name: "neither", want: nil},
		{name: "both", username: "robot", secret: "REGISTRY_PASSWORD", want: nil},
		{name: "username only", username: "robot", want: ErrRegistryAuthIncomplete},
		{name: "secret only", secret: "REGISTRY_PASSWORD", want: ErrRegistryAuthIncomplete},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := NormalizeConfig(Config{Type: TypeLocal, RegistryUsername: tt.username, RegistryPasswordSecretName: tt.secret})
			if err := ValidateConfig(cfg); !errors.Is(err, tt.want) {
				t.Fatalf("ValidateConfig() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestParseConfigRegistryFields(t *testing.T) {
	t.Parallel()

	cfg, err := parseConfig(`{"type":"local","registryUsername":"robot","registryPasswordSecretName":"REGISTRY_PASSWORD"}`)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.RegistryUsername != "robot" || cfg.RegistryPasswordSecretName != "REGISTRY_PASSWORD" {
		t.Fatalf("parseConfig() registry = %q/%q, want robot/REGISTRY_PASSWORD", cfg.RegistryUsername, cfg.RegistryPasswordSecretName)
	}
}

func TestPullImageSendsRegistryAuth(t *testing.T) {
	t.Parallel()

	cli := &fakeImageClient{inspectErr: errdefs.NotFound(errors.New("No such image"))}
	svc := newRegistryTestService(&stubSecretLookup{value: "s3cret"})
	cfg := Config{
		Image:                      "registry.example.com/team/agent:v1",
		RegistryUsername:           "robot",
		RegistryPasswordSecretName: "REGISTRY_PASSWORD",
	}
	if err := svc.pullImage(context.Background(), cli, cfg); err != nil {
		t.Fatalf("pullImage() error = %v", err)
	}
	if len(cli.pullOpts) != 1 {
		t.Fatalf("pulls = %d, want 1", len(cli.pullOpts))
	}
	got, err := registry.DecodeAuthConfig(cli.pullOpts[0].RegistryAuth)
	if err != nil {
		t.Fatalf("DecodeAuthConfig() error = %v", err)
	}
	if got.Username != "robot" || got.Password != "s3cret" || got.ServerAddress != "registry.example.com" {
		t.Fatalf("registry auth = %+v, want robot/s3cret@registry.example.com", got)
	}
}

func TestPullImageCachedSkipsSecretLookup(t *testing.T) {
	t.Parallel()

	// A cached image is used as-is, so a deleted password secret must not
	// stop a run that would never have pulled.
	lookup := &stubSecretLookup{err: errors.New("not found")}
	cli := &fakeImageClient{}
	cfg := Config{Image: "registry.example.com/team/agent:v1", RegistryUsername: "robot", RegistryPasswordSecretName: "GONE"}
	if err := newRegistryTestService(lookup).pullImage(context.Background(), cli, cfg); err != nil {
		t.Fatalf("pullImage() error = %v, want nil for a cached image", err)
	}
	if len(lookup.asked) != 0 {
		t.Fatalf("asked = %v, want no lookup", lookup.asked)
	}
}

func TestPullImageAnonymousWithoutCredentials(t *testing.T) {
	t.Parallel()

	cli := &fakeImageClient{inspectErr: errdefs.NotFound(errors.New("No such image"))}
	if err := newRegistryTestService(nil).pullImage(context.Background(), cli, Config{Image: "javdet/nib-agent:latest"}); err != nil {
		t.Fatalf("pullImage() error = %v", err)
	}
	if len(cli.pullOpts) != 1 || cli.pullOpts[0].RegistryAuth != "" {
		t.Fatalf("pullOpts = %+v, want one anonymous pull", cli.pullOpts)
	}
}

func TestDockerConfigJSON(t *testing.T) {
	t.Parallel()

	creds := &registryCredentials{Username: "robot", Password: "s3cret", ServerAddress: dockerHubServerAddress}
	body, err := creds.dockerConfigJSON()
	if err != nil {
		t.Fatalf("dockerConfigJSON() error = %v", err)
	}
	var doc struct {
		Auths map[string]struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Auth     string `json:"auth"`
		} `json:"auths"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	entry, ok := doc.Auths[dockerHubServerAddress]
	if !ok {
		t.Fatalf("auths = %v, want an entry for %q", doc.Auths, dockerHubServerAddress)
	}
	if entry.Username != "robot" || entry.Password != "s3cret" {
		t.Fatalf("entry = %+v, want robot/s3cret", entry)
	}
	if want := base64.StdEncoding.EncodeToString([]byte("robot:s3cret")); entry.Auth != want {
		t.Fatalf("auth = %q, want %q", entry.Auth, want)
	}
}

func TestPullSecretLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	secrets := fake.NewClientset().CoreV1().Secrets("agents")
	creds := &registryCredentials{Username: "robot", Password: "s3cret", ServerAddress: "registry.example.com"}

	name, err := createPullSecret(ctx, secrets, "nib-12345678", creds)
	if err != nil {
		t.Fatalf("createPullSecret() error = %v", err)
	}
	if name != "nib-12345678-registry" {
		t.Fatalf("name = %q, want nib-12345678-registry", name)
	}
	stored, err := secrets.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get secret: %v", err)
	}
	if stored.Type != corev1.SecretTypeDockerConfigJson {
		t.Fatalf("type = %q, want %q", stored.Type, corev1.SecretTypeDockerConfigJson)
	}
	if len(stored.Data[corev1.DockerConfigJsonKey]) == 0 {
		t.Fatalf("data = %v, want a %s key", stored.Data, corev1.DockerConfigJsonKey)
	}

	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "nib-12345678", UID: "job-uid"}}
	adoptPullSecret(ctx, secrets, name, job)
	stored, err = secrets.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get secret: %v", err)
	}
	if len(stored.OwnerReferences) != 1 {
		t.Fatalf("ownerReferences = %v, want the job", stored.OwnerReferences)
	}
	if ref := stored.OwnerReferences[0]; ref.Kind != "Job" || ref.Name != job.Name || ref.UID != job.UID {
		t.Fatalf("ownerReference = %+v, want Job %s/%s", ref, job.Name, job.UID)
	}

	deletePullSecret(ctx, secrets, name)
	if _, err := secrets.Get(ctx, name, metav1.GetOptions{}); err == nil {
		t.Fatal("secret still exists after deletePullSecret")
	}
}

func TestSetImagePullSecretOnRenderedJob(t *testing.T) {
	t.Parallel()

	job, err := renderJobManifest(t.TempDir(), buildActionRunValues(
		NormalizeConfig(Config{Type: TypeRemote, Platform: PlatformKubernetes, AgentSecretName: "agent"}),
		ActionRunRequest{Prompt: "p", RepoURL: "https://example.com/r.git", TargetBranch: "main"},
		"nib-12345678",
	))
	if err != nil {
		t.Fatalf("renderJobManifest() error = %v", err)
	}
	setImagePullSecret(job, "nib-12345678-registry")
	setImagePullSecret(job, "nib-12345678-registry")

	refs := job.Spec.Template.Spec.ImagePullSecrets
	if len(refs) != 1 || refs[0].Name != "nib-12345678-registry" {
		t.Fatalf("imagePullSecrets = %v, want exactly [nib-12345678-registry]", refs)
	}
}
