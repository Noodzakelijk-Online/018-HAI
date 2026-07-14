package authentication

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// googleOAuth implements "Sign in with Google" for the IDP: the OAuth
// authorization-code flow scoped to the user's identity (openid email profile),
// used to establish a HAI session — distinct from the backend's Gmail connector,
// which reads mail. Endpoints are injectable so the flow is unit-testable.
type googleOAuth struct {
	clientID      string
	clientSecret  string
	redirectURL   string
	stateSecret   []byte
	authEndpoint  string
	tokenEndpoint string
	userInfoURL   string
	httpClient    *http.Client
}

const (
	googleDefaultAuthEndpoint  = "https://accounts.google.com/o/oauth2/v2/auth"
	googleDefaultTokenEndpoint = "https://oauth2.googleapis.com/token"
	googleDefaultUserInfoURL   = "https://www.googleapis.com/oauth2/v2/userinfo"
	googleLoginScope           = "openid email profile"
)

// newGoogleOAuth builds the flow from the environment. jwtSecret signs the CSRF
// state so it needs no server-side storage.
func newGoogleOAuth(jwtSecret string) *googleOAuth {
	sum := sha256.Sum256([]byte("google-login-state|" + jwtSecret))
	return &googleOAuth{
		clientID:      strings.TrimSpace(os.Getenv("GOOGLE_OAUTH_CLIENT_ID")),
		clientSecret:  strings.TrimSpace(os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET")),
		redirectURL:   strings.TrimSpace(os.Getenv("GOOGLE_LOGIN_REDIRECT_URL")),
		stateSecret:   sum[:],
		authEndpoint:  googleDefaultAuthEndpoint,
		tokenEndpoint: googleDefaultTokenEndpoint,
		userInfoURL:   googleDefaultUserInfoURL,
		httpClient:    &http.Client{Timeout: 20 * time.Second},
	}
}

// Configured reports whether Google login can run.
func (g *googleOAuth) Configured() bool {
	return g.clientID != "" && g.clientSecret != "" && g.redirectURL != ""
}

func (g *googleOAuth) signState() (string, error) {
	buf := make([]byte, 24)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return "", err
	}
	nonce := base64.RawURLEncoding.EncodeToString(buf)
	payload := fmt.Sprintf("%d|%s", time.Now().Add(10*time.Minute).Unix(), nonce)
	enc := base64.RawURLEncoding.EncodeToString([]byte(payload))
	mac := hmac.New(sha256.New, g.stateSecret)
	mac.Write([]byte(enc))
	return enc + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (g *googleOAuth) verifyState(state string) error {
	parts := strings.SplitN(strings.TrimSpace(state), ".", 2)
	if len(parts) != 2 {
		return fmt.Errorf("malformed state")
	}
	mac := hmac.New(sha256.New, g.stateSecret)
	mac.Write([]byte(parts[0]))
	if !hmac.Equal([]byte(base64.RawURLEncoding.EncodeToString(mac.Sum(nil))), []byte(parts[1])) {
		return fmt.Errorf("state signature mismatch")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return fmt.Errorf("state decode: %w", err)
	}
	fields := strings.SplitN(string(raw), "|", 2)
	exp, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return fmt.Errorf("state expired")
	}
	return nil
}

// AuthCodeURL returns the Google consent URL, with a fresh signed state.
func (g *googleOAuth) AuthCodeURL() (string, error) {
	state, err := g.signState()
	if err != nil {
		return "", err
	}
	q := url.Values{}
	q.Set("client_id", g.clientID)
	q.Set("redirect_uri", g.redirectURL)
	q.Set("response_type", "code")
	q.Set("scope", googleLoginScope)
	q.Set("state", state)
	return g.authEndpoint + "?" + q.Encode(), nil
}

// Exchange verifies the state, trades the code for an access token, and returns
// the authenticated user's verified email address.
func (g *googleOAuth) Exchange(ctx context.Context, code, state string) (string, error) {
	if err := g.verifyState(state); err != nil {
		return "", err
	}
	if strings.TrimSpace(code) == "" {
		return "", fmt.Errorf("missing authorization code")
	}

	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", g.clientID)
	form.Set("client_secret", g.clientSecret)
	form.Set("redirect_uri", g.redirectURL)
	form.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token exchange failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var tok struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", fmt.Errorf("token response unparseable (HTTP %d)", resp.StatusCode)
	}
	if tok.Error != "" {
		return "", fmt.Errorf("google oauth error %q: %s", tok.Error, tok.ErrorDesc)
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("no access token returned")
	}

	return g.fetchEmail(ctx, tok.AccessToken)
}

func (g *googleOAuth) fetchEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.userInfoURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("userinfo request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("userinfo returned HTTP %d", resp.StatusCode)
	}

	var info struct {
		Email         string `json:"email"`
		VerifiedEmail bool   `json:"verified_email"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return "", fmt.Errorf("userinfo unparseable: %w", err)
	}
	email := strings.ToLower(strings.TrimSpace(info.Email))
	if email == "" {
		return "", fmt.Errorf("google did not return an email address")
	}
	return email, nil
}
