package authentication

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func newTestGoogleOAuth() *googleOAuth {
	g := newGoogleOAuth("test-jwt-secret")
	g.clientID = "client-1.apps.googleusercontent.com"
	g.clientSecret = "secret"
	g.redirectURL = "https://example.ngrok-free.dev/api/v1/auth/google/callback"
	return g
}

func TestGoogleConfiguredNeedsAllThree(t *testing.T) {
	g := newTestGoogleOAuth()
	if !g.Configured() {
		t.Fatal("fully populated should be configured")
	}
	g.clientSecret = ""
	if g.Configured() {
		t.Fatal("missing secret must not be configured")
	}
}

func TestGoogleAuthCodeURL(t *testing.T) {
	g := newTestGoogleOAuth()
	raw, err := g.AuthCodeURL()
	if err != nil {
		t.Fatalf("AuthCodeURL: %v", err)
	}
	u, _ := url.Parse(raw)
	q := u.Query()
	if q.Get("client_id") != g.clientID {
		t.Errorf("client_id = %q", q.Get("client_id"))
	}
	if q.Get("scope") != googleLoginScope {
		t.Errorf("scope = %q, want %q", q.Get("scope"), googleLoginScope)
	}
	if q.Get("state") == "" {
		t.Error("state should be present")
	}
	if !strings.HasPrefix(raw, googleDefaultAuthEndpoint) {
		t.Errorf("should target Google's auth endpoint")
	}
}

func TestGoogleStateRoundTrip(t *testing.T) {
	g := newTestGoogleOAuth()
	state, err := g.signState()
	if err != nil {
		t.Fatalf("signState: %v", err)
	}
	if err := g.verifyState(state); err != nil {
		t.Fatalf("verifyState should accept its own state: %v", err)
	}
	if err := g.verifyState(state + "x"); err == nil {
		t.Fatal("a tampered state must be rejected")
	}
	other := newGoogleOAuth("different-secret")
	if err := other.verifyState(state); err == nil {
		t.Fatal("state must not verify under a different signing secret")
	}
}

func TestGoogleExchangeReturnsEmail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			_ = r.ParseForm()
			if r.Form.Get("grant_type") != "authorization_code" {
				t.Errorf("grant_type = %q", r.Form.Get("grant_type"))
			}
			_, _ = w.Write([]byte(`{"access_token":"at-1","token_type":"Bearer"}`))
		case "/userinfo":
			if r.Header.Get("Authorization") != "Bearer at-1" {
				t.Errorf("missing bearer token on userinfo")
			}
			_, _ = w.Write([]byte(`{"email":"Person@Example.com","verified_email":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	g := newTestGoogleOAuth()
	g.tokenEndpoint = srv.URL + "/token"
	g.userInfoURL = srv.URL + "/userinfo"
	state, _ := g.signState()

	email, err := g.Exchange(context.Background(), "the-code", state)
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if email != "person@example.com" {
		t.Fatalf("email = %q, want normalized person@example.com", email)
	}
}

func TestGoogleExchangeRejectsBadState(t *testing.T) {
	g := newTestGoogleOAuth()
	if _, err := g.Exchange(context.Background(), "code", "forged.state"); err == nil {
		t.Fatal("Exchange must reject an unsigned/forged state before calling Google")
	}
}

func TestGoogleExchangeSurfacesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"bad"}`))
	}))
	defer srv.Close()

	g := newTestGoogleOAuth()
	g.tokenEndpoint = srv.URL
	state, _ := g.signState()
	if _, err := g.Exchange(context.Background(), "code", state); err == nil || !strings.Contains(err.Error(), "invalid_grant") {
		t.Fatalf("expected invalid_grant error, got %v", err)
	}
}

func TestGoogleExchangeRejectsUnverifiedEmail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			_, _ = w.Write([]byte(`{"access_token":"at-1","token_type":"Bearer"}`))
		case "/userinfo":
			_, _ = w.Write([]byte(`{"email":"person@example.com","verified_email":false}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	g := newTestGoogleOAuth()
	g.tokenEndpoint = srv.URL + "/token"
	g.userInfoURL = srv.URL + "/userinfo"
	state, _ := g.signState()

	if _, err := g.Exchange(context.Background(), "code", state); err == nil || !strings.Contains(err.Error(), "not verified") {
		t.Fatalf("expected unverified email to be rejected, got %v", err)
	}
}

func TestGoogleExchangeDoesNotFollowTokenEndpointRedirects(t *testing.T) {
	redirectFollowed := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			http.Redirect(w, r, "/unexpected", http.StatusFound)
		case "/unexpected":
			redirectFollowed = true
			_, _ = w.Write([]byte(`{"access_token":"redirected-token"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	g := newTestGoogleOAuth()
	g.tokenEndpoint = srv.URL + "/token"
	state, err := g.signState()
	if err != nil {
		t.Fatalf("signState: %v", err)
	}
	if _, err := g.Exchange(context.Background(), "code", state); err == nil {
		t.Fatal("redirect response must not be accepted as a token exchange")
	}
	if redirectFollowed {
		t.Fatal("OAuth client followed a token endpoint redirect")
	}
}
