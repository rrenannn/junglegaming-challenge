package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/rrenannn/junglegaming-challenge/internal/adapter/http/response"
	"github.com/rrenannn/junglegaming-challenge/internal/application/port"
)

type identityContextKey struct{}

func IdentityFromContext(ctx context.Context) (port.Identity, bool) {
	identity, ok := ctx.Value(identityContextKey{}).(port.Identity)
	return identity, ok
}

func Authenticate(verifier port.IdentityVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				response.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing bearer token")
				return
			}

			identity, err := verifier.Verify(r.Context(), token)
			if err != nil {
				response.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid bearer token")
				return
			}

			ctx := context.WithValue(r.Context(), identityContextKey{}, identity)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func bearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	token := strings.TrimSpace(header[len(prefix):])
	if token == "" {
		return "", false
	}
	return token, true
}
