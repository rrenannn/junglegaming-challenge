package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/rrenannn/junglegaming-challenge/internal/application/port"
)

type fakeVerifier struct {
	identity port.Identity
	err      error
}

func (f fakeVerifier) Verify(context.Context, string) (port.Identity, error) {
	if f.err != nil {
		return port.Identity{}, f.err
	}
	return f.identity, nil
}

func newRequestWithAuth(t *testing.T, header string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	return req
}

func TestAuthenticate_MissingHeader(t *testing.T) {
	handlerCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handlerCalled = true })

	rec := httptest.NewRecorder()
	Authenticate(fakeVerifier{})(next).ServeHTTP(rec, newRequestWithAuth(t, ""))

	if handlerCalled {
		t.Fatalf("next handler should not be called without a token")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAuthenticate_VerifierError(t *testing.T) {
	handlerCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handlerCalled = true })

	rec := httptest.NewRecorder()
	Authenticate(fakeVerifier{err: errors.New("bad token")})(next).ServeHTTP(rec, newRequestWithAuth(t, "Bearer abc"))

	if handlerCalled {
		t.Fatalf("next handler should not be called when verification fails")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAuthenticate_Success(t *testing.T) {
	want := port.Identity{Subject: "sub-1", ProviderID: "provider-a", Scopes: []string{"wagering.write"}}

	var gotIdentity port.Identity
	var gotOK bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIdentity, gotOK = IdentityFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	Authenticate(fakeVerifier{identity: want})(next).ServeHTTP(rec, newRequestWithAuth(t, "Bearer abc"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !gotOK {
		t.Fatalf("identity not found in context")
	}
	if !reflect.DeepEqual(gotIdentity, want) {
		t.Fatalf("identity = %+v, want %+v", gotIdentity, want)
	}
}

func TestRequireScope_Forbidden(t *testing.T) {
	identity := port.Identity{ProviderID: "provider-a", Scopes: []string{"wagering.read"}}
	handlerCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handlerCalled = true })

	chain := Authenticate(fakeVerifier{identity: identity})(RequireScope("wagering.write")(next))

	rec := httptest.NewRecorder()
	chain.ServeHTTP(rec, newRequestWithAuth(t, "Bearer abc"))

	if handlerCalled {
		t.Fatalf("next handler should not be called without the required scope")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRequireScope_Allowed(t *testing.T) {
	identity := port.Identity{ProviderID: "provider-a", Scopes: []string{"wagering.write"}}
	handlerCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	chain := Authenticate(fakeVerifier{identity: identity})(RequireScope("wagering.write")(next))

	rec := httptest.NewRecorder()
	chain.ServeHTTP(rec, newRequestWithAuth(t, "Bearer abc"))

	if !handlerCalled {
		t.Fatalf("next handler should be called when the scope is present")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
