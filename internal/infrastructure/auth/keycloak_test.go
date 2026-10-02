package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/rrenannn/junglegaming-challenge/internal/infrastructure/config"
)

const (
	testIssuer      = "http://localhost:8081/realms/jungle-gaming"
	testTokenURL    = testIssuer + "/protocol/openid-connect/token"
	testAudience    = "jungle-api"
	providerAClient = "provider-a"
	providerASecret = "provider-a-local-secret"
	internalClient  = "wallet-service"
	internalSecret  = "wallet-service-local-secret"
)

func newTestVerifier(t *testing.T) *KeycloakVerifier {
	t.Helper()

	cfg := config.Config{OIDC: config.OIDCConfig{Issuer: testIssuer, Audience: testAudience}}
	verifier, err := NewKeycloakVerifier(cfg)
	if err != nil {
		t.Skipf("keycloak not available: %v", err)
	}
	return verifier
}

func fetchClientCredentialsToken(t *testing.T, clientID, clientSecret string) string {
	t.Helper()

	body := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
	}

	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.PostForm(testTokenURL, body)
	if err != nil {
		t.Skipf("keycloak not reachable: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("token request failed: status=%d", resp.StatusCode)
	}

	var payload struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	if payload.AccessToken == "" {
		t.Fatalf("empty access_token in response")
	}
	return payload.AccessToken
}

func TestKeycloakVerifier_ProviderToken(t *testing.T) {
	verifier := newTestVerifier(t)
	token := fetchClientCredentialsToken(t, providerAClient, providerASecret)

	identity, err := verifier.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if identity.ProviderID != "provider-a" {
		t.Fatalf("ProviderID = %q, want %q", identity.ProviderID, "provider-a")
	}
	if identity.IsInternal() {
		t.Fatalf("provider token should not be internal")
	}
	if !identity.HasScope("wagering.write") {
		t.Fatalf("scopes = %v, want to contain %q", identity.Scopes, "wagering.write")
	}
}

func TestKeycloakVerifier_InternalToken(t *testing.T) {
	verifier := newTestVerifier(t)
	token := fetchClientCredentialsToken(t, internalClient, internalSecret)

	identity, err := verifier.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !identity.IsInternal() {
		t.Fatalf("wallet-service token should be internal, got ProviderID=%q", identity.ProviderID)
	}
	if !identity.HasScope("wallets.manage") {
		t.Fatalf("scopes = %v, want to contain %q", identity.Scopes, "wallets.manage")
	}
}

func TestKeycloakVerifier_RejectsTokenSignedByUntrustedKey(t *testing.T) {
	verifier := newTestVerifier(t)

	untrustedKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}

	claims := keycloakClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    testIssuer,
			Audience:  jwt.ClaimStrings{testAudience},
			Subject:   "forged-subject",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Scope:      "wagering.write",
		ProviderID: "provider-a",
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(untrustedKey)
	if err != nil {
		t.Fatalf("sign forged token: %v", err)
	}

	if _, err := verifier.Verify(context.Background(), token); err == nil {
		t.Fatalf("Verify succeeded for a token signed by an untrusted key, want error")
	}
}
