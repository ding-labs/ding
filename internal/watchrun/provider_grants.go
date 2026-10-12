package watchrun

import (
	"context"
	"fmt"
	"regexp"
)

// CreateProviderIntegrationGrant materializes consent already verified by a
// hosted OAuth provider. It is never exposed through a browser/daemon grant API.
// The consent key binds issuer, subject, client and the provider's consent ID.
func (a *App) CreateProviderIntegrationGrant(ctx context.Context, r GrantRequest, consent string) (GrantPairing, error) {
	if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(consent) {
		return GrantPairing{}, fmt.Errorf("invalid provider consent")
	}
	return a.createIntegrationGrant(ctx, r, consent)
}
