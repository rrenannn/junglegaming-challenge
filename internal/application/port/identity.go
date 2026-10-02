package port

import "context"

type Identity struct {
	Subject    string
	ProviderID string
	Scopes     []string
}

func (i Identity) IsInternal() bool { return i.ProviderID == "" }

func (i Identity) HasScope(scope string) bool {
	for _, s := range i.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

type IdentityVerifier interface {
	Verify(ctx context.Context, bearerToken string) (Identity, error)
}
