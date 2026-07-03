package authentication

import (
	"net/http"
	"os"
	"strings"
	"time"
)

func setAccessTokenCookie(w http.ResponseWriter, value string, expires time.Time) {
	setAuthCookie(w, "access_token", value, expires)
}

func setRefreshTokenCookie(w http.ResponseWriter, value string, expires time.Time) {
	setAuthCookie(w, "refresh_token", value, expires)
}

func setAuthCookie(w http.ResponseWriter, name string, value string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Expires:  expires,
		HttpOnly: true,
		Secure:   authCookieSecure(),
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
	})
}

func authCookieSecure() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("IDP_COOKIE_SECURE")))
	switch value {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return false
	}
}
