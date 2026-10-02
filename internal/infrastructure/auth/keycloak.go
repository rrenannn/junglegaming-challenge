package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"

	"github.com/rrenannn/junglegaming-challenge/internal/application/port"
	"github.com/rrenannn/junglegaming-challenge/internal/infrastructure/config"
)

type KeycloakVerifier struct {
	issuer   string
	audience string
	jwks     keyfunc.Keyfunc
}

type keycloakClaims struct {
	jwt.RegisteredClaims
	Scope      string `json:"scope"`
	ProviderID string `json:"provider_id"`
}

func NewKeycloakVerifier(cfg config.Config) (*KeycloakVerifier, error) {
	jwksURL := strings.TrimRight(cfg.OIDC.Issuer, "/") + "/protocol/openid-connect/certs"
	return newKeycloakVerifier(cfg.OIDC.Issuer, cfg.OIDC.Audience, jwksURL)
}

// newKeycloakVerifier lets callers fetch JWKS from a different URL than the
// expected issuer claim — needed by tests, which reach Keycloak through its
// host-mapped port while the issuer embedded in tokens is pinned to the
// Docker network hostname (see compose.yaml's KC_HOSTNAME).
func newKeycloakVerifier(issuer, audience, jwksURL string) (*KeycloakVerifier, error) {
	jwks, err := keyfunc.NewDefaultCtx(context.Background(), []string{jwksURL})
	if err != nil {
		return nil, fmt.Errorf("create JWKS client: %w", err)
	}

	return &KeycloakVerifier{issuer: issuer, audience: audience, jwks: jwks}, nil
}

func (v *KeycloakVerifier) Verify(ctx context.Context, bearerToken string) (port.Identity, error) {
	var claims keycloakClaims
	_, err := jwt.ParseWithClaims(bearerToken, &claims, v.jwks.Keyfunc,
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audience),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return port.Identity{}, fmt.Errorf("verify token: %w", err)
	}

	var scopes []string
	if claims.Scope != "" {
		scopes = strings.Fields(claims.Scope)
	}

	return port.Identity{
		Subject:    claims.Subject,
		ProviderID: claims.ProviderID,
		Scopes:     scopes,
	}, nil
}
