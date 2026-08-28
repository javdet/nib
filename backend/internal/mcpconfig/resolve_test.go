package mcpconfig

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/javdet/nib/internal/repository"
)

// fakeSecrets is a SecretLookup backed by a map. Names absent from values
// return the error in err, defaulting to repository.ErrNotFound.
type fakeSecrets struct {
	values map[string]string
	err    error
	calls  int
}

func (f *fakeSecrets) GetValueByName(_ context.Context, _, name string) (string, error) {
	f.calls++
	if v, ok := f.values[name]; ok {
		return v, nil
	}
	if f.err != nil {
		return "", f.err
	}
	return "", repository.ErrNotFound
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	return NewServiceAtPath(filepath.Join(t.TempDir(), "mcp.json"))
}

func TestResolveServerSecretWinsOverEnv(t *testing.T) {
	t.Setenv("MCP_TEST_TOKEN", "from-env")

	svc := newTestService(t)
	svc.SetSecretLookup(&fakeSecrets{values: map[string]string{"MCP_TEST_TOKEN": "from-secret"}})

	got, err := svc.ResolveServer(context.Background(), Server{
		Name:        "github",
		ServerEntry: ServerEntry{Headers: map[string]string{"Authorization": "Bearer ${MCP_TEST_TOKEN}"}},
	})
	if err != nil {
		t.Fatalf("ResolveServer() error = %v", err)
	}
	if want := "Bearer from-secret"; got.Headers["Authorization"] != want {
		t.Fatalf("Authorization = %q, want %q", got.Headers["Authorization"], want)
	}
}

func TestResolveServerFallsBackToEnv(t *testing.T) {
	t.Setenv("MCP_TEST_TOKEN", "from-env")

	svc := newTestService(t)
	svc.SetSecretLookup(&fakeSecrets{})

	got, err := svc.ResolveServer(context.Background(), Server{
		Name:        "github",
		ServerEntry: ServerEntry{URL: "https://example.com/mcp?t=${MCP_TEST_TOKEN}"},
	})
	if err != nil {
		t.Fatalf("ResolveServer() error = %v", err)
	}
	if want := "https://example.com/mcp?t=from-env"; got.URL != want {
		t.Fatalf("URL = %q, want %q", got.URL, want)
	}
}

func TestResolveServerNilLookupUsesEnvOnly(t *testing.T) {
	t.Setenv("MCP_TEST_TOKEN", "from-env")

	got, err := newTestService(t).ResolveServer(context.Background(), Server{
		Name:        "github",
		ServerEntry: ServerEntry{Command: "run", Args: []string{"--token", "${MCP_TEST_TOKEN}"}},
	})
	if err != nil {
		t.Fatalf("ResolveServer() error = %v", err)
	}
	if want := "from-env"; got.Args[1] != want {
		t.Fatalf("Args[1] = %q, want %q", got.Args[1], want)
	}
}

// A secret store failure must not mask a working environment variable: an
// unset SECRETS_ENCRYPTION_KEY makes every secret read fail, and env-only
// deployments have to keep working.
func TestResolveServerSecretErrorFallsBackToEnv(t *testing.T) {
	t.Setenv("MCP_TEST_TOKEN", "from-env")

	svc := newTestService(t)
	svc.SetSecretLookup(&fakeSecrets{err: errors.New("secrets encryption not configured")})

	got, err := svc.ResolveServer(context.Background(), Server{
		Name:        "github",
		ServerEntry: ServerEntry{Env: map[string]string{"TOKEN": "${MCP_TEST_TOKEN}"}},
	})
	if err != nil {
		t.Fatalf("ResolveServer() error = %v", err)
	}
	if want := "from-env"; got.Env["TOKEN"] != want {
		t.Fatalf("Env[TOKEN] = %q, want %q", got.Env["TOKEN"], want)
	}
}

func TestResolveServerSecretErrorSurfacesWithoutEnv(t *testing.T) {
	sentinel := errors.New("database is down")

	svc := newTestService(t)
	svc.SetSecretLookup(&fakeSecrets{err: sentinel})

	_, err := svc.ResolveServer(context.Background(), Server{
		Name:        "github",
		ServerEntry: ServerEntry{Headers: map[string]string{"Authorization": "Bearer ${MCP_MISSING_TOKEN}"}},
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want %v", err, sentinel)
	}
}

func TestResolveServerMissingVariable(t *testing.T) {
	svc := newTestService(t)
	svc.SetSecretLookup(&fakeSecrets{})

	_, err := svc.ResolveServer(context.Background(), Server{
		Name:        "github",
		ServerEntry: ServerEntry{Headers: map[string]string{"Authorization": "Bearer ${MCP_MISSING_TOKEN}"}},
	})
	if !errors.Is(err, ErrUnresolvedVariable) {
		t.Fatalf("error = %v, want ErrUnresolvedVariable", err)
	}
	for _, want := range []string{"github", "Authorization", "${MCP_MISSING_TOKEN}"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %q", err.Error(), want)
		}
	}
}

func TestResolveServerDoesNotMutateSource(t *testing.T) {
	svc := newTestService(t)
	svc.SetSecretLookup(&fakeSecrets{values: map[string]string{"MCP_TEST_TOKEN": "from-secret"}})

	src := Server{
		Name: "github",
		ServerEntry: ServerEntry{
			URL:     "https://example.com/mcp",
			Headers: map[string]string{"Authorization": "Bearer ${MCP_TEST_TOKEN}"},
			Args:    []string{"${MCP_TEST_TOKEN}"},
			Env:     map[string]string{"TOKEN": "${MCP_TEST_TOKEN}"},
		},
	}

	if _, err := svc.ResolveServer(context.Background(), src); err != nil {
		t.Fatalf("ResolveServer() error = %v", err)
	}
	if got := src.Headers["Authorization"]; got != "Bearer ${MCP_TEST_TOKEN}" {
		t.Fatalf("source header mutated: %q", got)
	}
	if src.Args[0] != "${MCP_TEST_TOKEN}" {
		t.Fatalf("source args mutated: %q", src.Args[0])
	}
	if src.Env["TOKEN"] != "${MCP_TEST_TOKEN}" {
		t.Fatalf("source env mutated: %q", src.Env["TOKEN"])
	}
}

func TestResolveServerMemoisesLookups(t *testing.T) {
	secrets := &fakeSecrets{values: map[string]string{"MCP_TEST_TOKEN": "from-secret"}}

	svc := newTestService(t)
	svc.SetSecretLookup(secrets)

	if _, err := svc.ResolveServer(context.Background(), Server{
		Name: "github",
		ServerEntry: ServerEntry{
			URL:     "https://example.com/mcp?t=${MCP_TEST_TOKEN}",
			Headers: map[string]string{"Authorization": "Bearer ${MCP_TEST_TOKEN}"},
		},
	}); err != nil {
		t.Fatalf("ResolveServer() error = %v", err)
	}
	if secrets.calls != 1 {
		t.Fatalf("secret lookups = %d, want 1", secrets.calls)
	}
}

func TestResolvedRedactError(t *testing.T) {
	svc := newTestService(t)
	svc.SetSecretLookup(&fakeSecrets{values: map[string]string{"MCP_TEST_TOKEN": "ghp_secret"}})

	resolved, err := svc.ResolveServer(context.Background(), Server{
		Name:        "github",
		ServerEntry: ServerEntry{URL: "https://example.com/mcp?t=${MCP_TEST_TOKEN}"},
	})
	if err != nil {
		t.Fatalf("ResolveServer() error = %v", err)
	}

	sentinel := errors.New(`Get "https://example.com/mcp?t=ghp_secret": 401`)
	redacted := resolved.RedactError(sentinel)
	if strings.Contains(redacted.Error(), "ghp_secret") {
		t.Fatalf("redacted error still leaks the value: %q", redacted.Error())
	}
	if !errors.Is(redacted, sentinel) {
		t.Fatal("redacted error lost the original error chain")
	}
}
