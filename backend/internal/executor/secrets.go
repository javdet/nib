package executor

import (
	"context"
)

const (
	secretScope = "global"
	// A global secret has no owning entity, so its scope_name is empty. It is
	// spelled out because (scope, scope_name, name) is the whole identity.
	secretScopeName = ""
)

// SecretLookup resolves a secret value by its full identity. It is satisfied by
// *service.SecretService; declaring it here keeps executor free of a
// dependency on the service package.
type SecretLookup interface {
	GetValueByName(ctx context.Context, scope, scopeName, name string) (string, error)
}

func (s *Service) SetSecretLookup(lookup SecretLookup) {
	s.secretMu.Lock()
	defer s.secretMu.Unlock()
	s.secretsLookup = lookup
}

func (s *Service) secretLookup() SecretLookup {
	s.secretMu.RLock()
	defer s.secretMu.RUnlock()
	return s.secretsLookup
}
