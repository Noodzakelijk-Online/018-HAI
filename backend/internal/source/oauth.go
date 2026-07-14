package source

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"automation-hub-backend/internal/config"
	"automation-hub-backend/internal/googleoauth"
	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
)

const (
	googleProvider    = "google"
	gmailConnectorKey = "gmail"
	gmailFetchLimit   = 25
)

// googleOAuthConfig builds the OAuth client from the environment.
func googleOAuthConfig() googleoauth.Config {
	return googleoauth.Config{
		ClientID:     config.AppConfig.GoogleOAuthClientID,
		ClientSecret: config.AppConfig.GoogleOAuthClientSecret,
		RedirectURL:  config.AppConfig.GoogleOAuthRedirectURL,
		Scopes:       []string{googleoauth.GmailReadonlyScope},
	}
}

// tokenCodec encrypts tokens at rest with the configured encryption secret,
// falling back to the backend key (the same precedence doctor documents).
func tokenCodec() (*googleoauth.Codec, error) {
	secret := strings.TrimSpace(config.AppConfig.MemoryEngineKey)
	if secret == "" {
		secret = strings.TrimSpace(config.AppConfig.BackendAPIKey)
	}
	return googleoauth.NewCodec(secret)
}

// --- signed, stateless CSRF state -------------------------------------------
// The state carries the source id and an expiry, HMAC-signed with a server
// secret. It needs no server-side storage, so it survives a restart mid-flow
// and cannot be forged without the secret. Format: base64(payload)."."base64(mac).

func stateSecret() []byte {
	s := strings.TrimSpace(config.AppConfig.JWTSecret)
	if s == "" {
		s = strings.TrimSpace(config.AppConfig.BackendAPIKey)
	}
	sum := sha256.Sum256([]byte("oauth-state|" + s))
	return sum[:]
}

func signState(sourceID uuid.UUID) (string, error) {
	nonce, err := googleoauth.NewState()
	if err != nil {
		return "", err
	}
	payload := fmt.Sprintf("%s|%d|%s", sourceID.String(), time.Now().Add(10*time.Minute).Unix(), nonce)
	enc := base64.RawURLEncoding.EncodeToString([]byte(payload))
	mac := hmac.New(sha256.New, stateSecret())
	mac.Write([]byte(enc))
	return enc + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func verifyState(state string) (uuid.UUID, error) {
	parts := strings.SplitN(strings.TrimSpace(state), ".", 2)
	if len(parts) != 2 {
		return uuid.Nil, fmt.Errorf("malformed oauth state")
	}
	mac := hmac.New(sha256.New, stateSecret())
	mac.Write([]byte(parts[0]))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return uuid.Nil, fmt.Errorf("oauth state signature mismatch")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return uuid.Nil, fmt.Errorf("oauth state decode: %w", err)
	}
	fields := strings.Split(string(raw), "|")
	if len(fields) != 3 {
		return uuid.Nil, fmt.Errorf("oauth state payload shape")
	}
	exp, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return uuid.Nil, fmt.Errorf("oauth state expired; restart the connection")
	}
	id, err := uuid.Parse(fields[0])
	if err != nil {
		return uuid.Nil, fmt.Errorf("oauth state source id invalid")
	}
	return id, nil
}

// StartGoogleOAuth returns the Google consent URL for connecting a gmail source.
func (s *service) StartGoogleOAuth(sourceID uuid.UUID) (string, error) {
	cfg := googleOAuthConfig()
	if !cfg.Configured() {
		return "", fmt.Errorf("google oauth is not configured; set GOOGLE_OAUTH_CLIENT_ID, _SECRET and _REDIRECT_URL")
	}
	source, err := s.repo.FindSource(sourceID)
	if err != nil {
		return "", err
	}
	if source.ConnectorKey != gmailConnectorKey {
		return "", fmt.Errorf("source is a %q connector, not gmail", source.ConnectorKey)
	}
	state, err := signState(sourceID)
	if err != nil {
		return "", err
	}
	return cfg.AuthorizeURL(state), nil
}

// CompleteGoogleOAuth handles the callback: verify state, exchange the code, and
// store the encrypted tokens against the source.
func (s *service) CompleteGoogleOAuth(ctx context.Context, code, state string) (uuid.UUID, error) {
	cfg := googleOAuthConfig()
	if !cfg.Configured() {
		return uuid.Nil, fmt.Errorf("google oauth is not configured")
	}
	if strings.TrimSpace(code) == "" {
		return uuid.Nil, fmt.Errorf("missing authorization code")
	}
	sourceID, err := verifyState(state)
	if err != nil {
		return uuid.Nil, err
	}
	token, err := cfg.ExchangeCode(ctx, code)
	if err != nil {
		return uuid.Nil, err
	}
	if err := s.storeToken(sourceID, token); err != nil {
		return uuid.Nil, err
	}
	s.audit(sourceID, "source.oauth_connected", "google account connected for gmail sync")
	return sourceID, nil
}

func (s *service) storeToken(sourceID uuid.UUID, token *googleoauth.Token) error {
	codec, err := tokenCodec()
	if err != nil {
		return err
	}
	accessCt, err := codec.Encrypt(token.AccessToken)
	if err != nil {
		return err
	}
	refreshCt, err := codec.Encrypt(token.RefreshToken)
	if err != nil {
		return err
	}
	return s.repo.SaveOAuthToken(&models.SourceOAuthToken{
		SourceID:     sourceID,
		Provider:     googleProvider,
		AccessToken:  accessCt,
		RefreshToken: refreshCt,
		Scope:        token.Scope,
		Expiry:       token.Expiry,
	})
}

// gmailAccessToken returns a currently-valid access token, transparently
// refreshing and persisting a new one when the stored token has expired.
func (s *service) gmailAccessToken(ctx context.Context, sourceID uuid.UUID) (string, error) {
	stored, err := s.repo.FindOAuthToken(sourceID)
	if err != nil {
		return "", fmt.Errorf("no connected google account for this source; connect it first")
	}
	codec, err := tokenCodec()
	if err != nil {
		return "", err
	}
	access, err := codec.Decrypt(stored.AccessToken)
	if err != nil {
		return "", err
	}
	refresh, err := codec.Decrypt(stored.RefreshToken)
	if err != nil {
		return "", err
	}
	tok := googleoauth.Token{AccessToken: access, RefreshToken: refresh, Expiry: stored.Expiry, Scope: stored.Scope}
	if tok.Valid(time.Now()) {
		return tok.AccessToken, nil
	}
	if strings.TrimSpace(refresh) == "" {
		return "", fmt.Errorf("access token expired and no refresh token stored; reconnect the google account")
	}
	refreshed, err := googleOAuthConfig().Refresh(ctx, refresh)
	if err != nil {
		return "", fmt.Errorf("token refresh failed: %w", err)
	}
	if err := s.storeToken(sourceID, refreshed); err != nil {
		return "", err
	}
	return refreshed.AccessToken, nil
}

// fetchGmailSource pulls recent message metadata as import items.
func (s *service) fetchGmailSource(ctx context.Context, source *models.ConnectedSource) ([]ImportItem, string, error) {
	access, err := s.gmailAccessToken(ctx, source.ID)
	if err != nil {
		return nil, "", err
	}
	messages, err := (googleoauth.GmailClient{AccessToken: access}).FetchRecent(ctx, gmailFetchLimit)
	if err != nil {
		return nil, "", err
	}
	projectKey := firstNonEmpty(source.DefaultProjectKey, "Robert-life-os")
	items := make([]ImportItem, 0, len(messages))
	for _, m := range messages {
		items = append(items, ImportItem{
			ExternalID: "gmail:" + m.ID,
			Title:      firstNonEmpty(m.Subject, "(no subject)"),
			Content:    fmt.Sprintf("From: %s\nSubject: %s\nDate: %s\n\n%s", m.From, m.Subject, m.Date.Format(time.RFC3339), m.Snippet),
			SourceURI:  "https://mail.google.com/mail/u/0/#all/" + m.ID,
			ItemType:   "email_message",
			ProjectKey: projectKey,
			Metadata:   fmt.Sprintf("source=gmail;from=%s;date=%s", m.From, m.Date.Format(time.RFC3339)),
		})
	}
	return items, "", nil
}
