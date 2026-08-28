package mcpconfig

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/javdet/nib/internal/repository"
)

// secretScope is the prompt secret scope searched for ${NAME} references.
// A blank scope resolves to the default ("global") inside the secret service.
const secretScope = ""

// SecretLookup resolves a secret value by scope and name. It is satisfied by
// *service.SecretService; declaring it here keeps mcpconfig free of a
// dependency on the service package.
type SecretLookup interface {
	GetValueByName(ctx context.Context, scope, name string) (string, error)
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
// references in mcp.json. A nil lookup means environment variables only.
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

// lookup resolves a name from the encrypted secret store first, then the
// process environment. A secret that simply does not exist falls through to
// the environment; any other secret store failure is reported only when the
// environment has no value either, so a working env var still wins.
func (r *resolver) lookup(ctx context.Context, name string) (string, bool, error) {
	if v, ok := r.cache[name]; ok {
		return v, true, nil
	}

	var secretErr error
	if r.secrets != nil {
		v, err := r.secrets.GetValueByName(ctx, secretScope, name)
		switch {
		case err == nil:
			r.remember(name, v)
			return v, true, nil
		case errors.Is(err, repository.ErrNotFound):
			// Not configured as a secret; try the environment.
		default:
			secretErr = err
		}
	}

	if v, ok := os.LookupEnv(name); ok {
		r.remember(name, v)
		return v, true, nil
	}

	if secretErr != nil {
		return "", false, fmt.Errorf("resolve ${%s}: %w", name, secretErr)
	}
	return "", false, nil
}

func (r *resolver) remember(name, value string) {
	r.cache[name] = value
	if value != "" {
		r.values = append(r.values, value)
	}
}
