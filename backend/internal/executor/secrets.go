package executor

import (
	"context"
)

const secretScope = "global"

// SecretLookup resolves a secret value by scope and name. It is satisfied by
// *service.SecretService; declaring it here keeps executor free of a
// dependency on the service package.
type SecretLookup interface {
	GetValueByName(ctx context.Context, scope, name string) (string, error)
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
