package mcpconfig

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/javdet/nib/internal/repository"
)

// secretScope is the prompt secret scope searched for ${NAME} references.
// A blank scope resolves to the default ("global") inside the secret service,
// and a global secret carries an empty scope_name.
const (
	secretScope     = ""
	secretScopeName = ""
)

// SecretLookup resolves a secret value by its full identity. It is satisfied by
// *service.SecretService; declaring it here keeps mcpconfig free of a
// dependency on the service package.
type SecretLookup interface {
	GetValueByName(ctx context.Context, scope, scopeName, name string) (string, error)
}

// Resolved is a server entry whose ${NAME} references have been expanded,
// together with the substituted values so callers can strip them from logs.
type Resolved struct {
	Server

	values []string
}

// Redact replaces every value substituted into this server with "***".
func (r Resolved) Redact(s string) string {
	return RedactValues(s, r.values)
}

// Values returns the distinct non-empty values substituted into this server, so
// a caller that keeps the resolved URL and headers can redact them later.
func (r Resolved) Values() []string {
	return r.values
}

// RedactError wraps err so its message has substituted values replaced by
// "***". Transport errors embed the request URL, which may carry a resolved
// secret. The original error stays unwrappable.
func (r Resolved) RedactError(err error) error {
	if err == nil {
		return nil
	}
	msg := r.Redact(err.Error())
	if msg == err.Error() {
		return err
	}
	return &redactedError{msg: msg, err: err}
}

// redactedError hides substituted values in the message while preserving the
// error chain for errors.Is/As.
type redactedError struct {
	msg string
	err error
}

func (e *redactedError) Error() string { return e.msg }
func (e *redactedError) Unwrap() error { return e.err }

// SetSecretLookup registers the secret source used to expand ${NAME}
// references in mcp.json. With a nil lookup nothing resolves.
func (s *Service) SetSecretLookup(lookup SecretLookup) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secrets = lookup
}

func (s *Service) secretLookup() SecretLookup {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.secrets
}

// GetServerResolved returns a server with its ${NAME} references expanded.
func (s *Service) GetServerResolved(ctx context.Context, name string) (Resolved, error) {
	server, err := s.GetServer(name)
	if err != nil {
		return Resolved{}, err
	}
	return s.ResolveServer(ctx, server)
}

// ResolveServer expands the ${NAME} references in a server entry. The source
// server is left untouched; maps and slices are copied.
func (s *Service) ResolveServer(ctx context.Context, server Server) (Resolved, error) {
	r := &resolver{secrets: s.secretLookup(), cache: map[string]string{}}

	entry, err := r.resolveEntry(ctx, server.ServerEntry)
	if err != nil {
		return Resolved{}, fmt.Errorf("mcp server %q: %w", server.Name, err)
	}
	return Resolved{
		Server: Server{Name: server.Name, ServerEntry: entry},
		values: r.values,
	}, nil
}

// resolver expands variable references for a single server, memoising lookups
// so a value repeated across url and headers is fetched (and decrypted) once.
type resolver struct {
	secrets SecretLookup
	cache   map[string]string
	values  []string
}

func (r *resolver) resolveEntry(ctx context.Context, entry ServerEntry) (ServerEntry, error) {
	out := entry
	var err error

	if out.URL, err = r.expand(ctx, entry.URL); err != nil {
		return ServerEntry{}, err
	}
	if out.Command, err = r.expand(ctx, entry.Command); err != nil {
		return ServerEntry{}, err
	}
	if out.Headers, err = r.expandMap(ctx, entry.Headers); err != nil {
		return ServerEntry{}, err
	}
	if err := validateHeaders(entry.Headers, out.Headers); err != nil {
		return ServerEntry{}, err
	}
	if out.Env, err = r.expandMap(ctx, entry.Env); err != nil {
		return ServerEntry{}, err
	}
	if entry.Args != nil {
		args := make([]string, len(entry.Args))
		for i, arg := range entry.Args {
			if args[i], err = r.expand(ctx, arg); err != nil {
				return ServerEntry{}, err
			}
		}
		out.Args = args
	}
	return out, nil
}

func (r *resolver) expandMap(ctx context.Context, in map[string]string) (map[string]string, error) {
	if in == nil {
		return nil, nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		expanded, err := r.expand(ctx, v)
		if err != nil {
			return nil, fmt.Errorf("%q: %w", k, err)
		}
		out[k] = expanded
	}
	return out, nil
}

func (r *resolver) expand(ctx context.Context, s string) (string, error) {
	return expandString(ctx, s, r.lookup)
}

// lookup resolves a name from the encrypted secret store, and nowhere else.
//
// The process environment is deliberately not a source. The backend's own
// environment holds credentials that have nothing to do with MCP — LLM and
// database keys, executor tokens — and expanding them here would make an
// mcp.json edit a way to read them back out through any URL or header the
// agent is then told to call. Secrets added through the UI are the one supply
// of values, so what a server can be handed is exactly what someone chose to
// put in the secret store.
func (r *resolver) lookup(ctx context.Context, name string) (string, bool, error) {
	if v, ok := r.cache[name]; ok {
		return v, true, nil
	}
	if r.secrets == nil {
		return "", false, nil
	}

	v, err := r.secrets.GetValueByName(ctx, secretScope, secretScopeName, name)
	switch {
	case err == nil:
		// Surrounding whitespace is stripped because every field a reference
		// can land in — a URL, a header, an env value, an argument — is a
		// single line. A token pasted with its trailing newline would
		// otherwise be sent verbatim, and net/http rejects the whole request
		// with "invalid header field value". The stored secret is untouched:
		// only what is substituted here is trimmed.
		v = strings.TrimSpace(v)
		r.remember(name, v)
		return v, true, nil
	case errors.Is(err, repository.ErrNotFound):
		return "", false, nil
	default:
		return "", false, fmt.Errorf("resolve ${%s}: %w", name, err)
	}
}

func (r *resolver) remember(name, value string) {
	r.cache[name] = value
	if value != "" {
		r.values = append(r.values, value)
	}
}
