package middleware

import (
	"net/http"

	"github.com/rrenannn/junglegaming-challenge/internal/adapter/http/response"
)

func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity, ok := IdentityFromContext(r.Context())
			if !ok || !identity.HasScope(scope) {
				response.Error(w, http.StatusForbidden, "FORBIDDEN", "missing required scope")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
