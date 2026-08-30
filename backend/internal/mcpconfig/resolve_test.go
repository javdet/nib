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

// The process environment is not a source: the backend runs with credentials
// that have nothing to do with MCP, and mcp.json must not be able to name them.
func TestResolveServerIgnoresProcessEnv(t *testing.T) {
	t.Setenv("MCP_TEST_TOKEN", "from-env")

	svc := newTestService(t)
	svc.SetSecretLookup(&fakeSecrets{values: map[string]string{"MCP_TEST_TOKEN": "from-secret"}})

	got, err := svc.ResolveServer(context.Background(), Server{
		Name: "github",
		ServerEntry: ServerEntry{
			URL:     "https://example.com/mcp?t=${MCP_TEST_TOKEN}",
			Headers: map[string]string{"Authorization": "Bearer ${MCP_TEST_TOKEN}"},
		},
	})
	if err != nil {
		t.Fatalf("ResolveServer() error = %v", err)
	}
	if want := "Bearer from-secret"; got.Headers["Authorization"] != want {
		t.Fatalf("Authorization = %q, want %q", got.Headers["Authorization"], want)
	}
	if strings.Contains(got.URL, "from-env") {
		t.Fatalf("URL = %q, want the environment ignored", got.URL)
	}
}

func TestResolveServerEnvOnlyValueDoesNotResolve(t *testing.T) {
	t.Setenv("MCP_TEST_TOKEN", "from-env")

	svc := newTestService(t)
	svc.SetSecretLookup(&fakeSecrets{})

	_, err := svc.ResolveServer(context.Background(), Server{
		Name:        "github",
		ServerEntry: ServerEntry{URL: "https://example.com/mcp?t=${MCP_TEST_TOKEN}"},
	})
	if !errors.Is(err, ErrUnresolvedVariable) {
		t.Fatalf("error = %v, want ErrUnresolvedVariable", err)
	}
	if strings.Contains(err.Error(), "from-env") {
		t.Fatalf("error %q leaks the environment value", err.Error())
	}
}

func TestResolveServerNilLookupResolvesNothing(t *testing.T) {
	t.Setenv("MCP_TEST_TOKEN", "from-env")

	_, err := newTestService(t).ResolveServer(context.Background(), Server{
		Name:        "github",
		ServerEntry: ServerEntry{Command: "run", Args: []string{"--token", "${MCP_TEST_TOKEN}"}},
	})
	if !errors.Is(err, ErrUnresolvedVariable) {
		t.Fatalf("error = %v, want ErrUnresolvedVariable", err)
	}
}

// A secret store that is down or has no encryption key must say so rather than
// resolve to something else: there is no second source to fall back to.
func TestResolveServerSecretErrorSurfaces(t *testing.T) {
	t.Setenv("MCP_TEST_TOKEN", "from-env")

	sentinel := errors.New("secrets encryption not configured")
	svc := newTestService(t)
	svc.SetSecretLookup(&fakeSecrets{err: sentinel})

	_, err := svc.ResolveServer(context.Background(), Server{
		Name:        "github",
		ServerEntry: ServerEntry{Env: map[string]string{"TOKEN": "${MCP_TEST_TOKEN}"}},
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want %v", err, sentinel)
	}
}

func TestResolveServerSecretErrorNamesTheReference(t *testing.T) {
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

// A token pasted with the newline that came with it is the common way a
// perfectly valid secret produces "invalid header field value" from net/http.
func TestResolveServerTrimsWhitespaceAroundSecretValues(t *testing.T) {
	svc := newTestService(t)
	svc.SetSecretLookup(&fakeSecrets{values: map[string]string{"MCP_TEST_TOKEN": "ghp_secret\n"}})

	got, err := svc.ResolveServer(context.Background(), Server{
		Name: "github",
		ServerEntry: ServerEntry{
			URL:     "https://example.com/mcp?t=${MCP_TEST_TOKEN}",
			Headers: map[string]string{"Authorization": "Bearer ${MCP_TEST_TOKEN}"},
			Env:     map[string]string{"TOKEN": "${MCP_TEST_TOKEN}"},
		},
	})
	if err != nil {
		t.Fatalf("ResolveServer() error = %v", err)
	}
	if want := "Bearer ghp_secret"; got.Headers["Authorization"] != want {
		t.Fatalf("Authorization = %q, want %q", got.Headers["Authorization"], want)
	}
	if want := "https://example.com/mcp?t=ghp_secret"; got.URL != want {
		t.Fatalf("URL = %q, want %q", got.URL, want)
	}
	if want := "ghp_secret"; got.Env["TOKEN"] != want {
		t.Fatalf("env TOKEN = %q, want %q", got.Env["TOKEN"], want)
	}
	// Redaction has to follow the trimmed value, since that is what is sent.
	if got.Redact("token ghp_secret leaked") != "token *** leaked" {
		t.Fatalf("Redact did not cover the trimmed value")
	}
}

// A control character trimming cannot remove is reported against the header
// that carries it, so the fix is not a guess. The value stays out of the message.
func TestResolveServerRejectsControlCharacterInHeader(t *testing.T) {
	svc := newTestService(t)
	svc.SetSecretLookup(&fakeSecrets{values: map[string]string{"MCP_TEST_TOKEN": "ghp\nsecret"}})

	_, err := svc.ResolveServer(context.Background(), Server{
		Name: "github",
		ServerEntry: ServerEntry{
			URL:     "https://example.com/mcp",
			Headers: map[string]string{"Authorization": "Bearer ${MCP_TEST_TOKEN}"},
		},
	})
	if !errors.Is(err, ErrInvalidHeader) {
		t.Fatalf("error = %v, want ErrInvalidHeader", err)
	}
	for _, want := range []string{"github", "Authorization", "${MCP_TEST_TOKEN}"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %q", err.Error(), want)
		}
	}
	if strings.Contains(err.Error(), "ghp") {
		t.Fatalf("error %q leaks the secret value", err.Error())
	}
}

func TestValidHeaderValue(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"Bearer ghp_token", true},
		{"a\tb", true},
		{"наклейка", true}, // bytes >= 0x80 are legal in a field value
		{"Bearer ghp_token\n", false},
		{"Bearer ghp_token\r", false},
		{"Bearer \x00token", false},
		{"Bearer \x7ftoken", false},
	}
	for _, tt := range tests {
		if got := validHeaderValue(tt.value); got != tt.want {
			t.Errorf("validHeaderValue(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

func TestValidHeaderName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"Authorization", true},
		{"X-Api-Key", true},
		{"", false},
		{"Bad Header", false},
		{"Bad:Header", false},
	}
	for _, tt := range tests {
		if got := validHeaderName(tt.name); got != tt.want {
			t.Errorf("validHeaderName(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}
