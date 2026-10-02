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

	jwks, err := keyfunc.NewDefaultCtx(context.Background(), []string{jwksURL})
	if err != nil {
		return nil, fmt.Errorf("create JWKS client: %w", err)
	}

	return &KeycloakVerifier{issuer: cfg.OIDC.Issuer, audience: cfg.OIDC.Audience, jwks: jwks}, nil
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
