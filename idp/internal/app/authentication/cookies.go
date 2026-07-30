package authentication

import (
	"net/http"
	"os"
	"strings"
	"time"
)

const googleOAuthStateCookie = "hai_google_oauth_state"

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

func clearAuthCookies(w http.ResponseWriter) {
	expired := time.Unix(1, 0).UTC()
	for _, name := range []string{"access_token", "refresh_token"} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Expires:  expired,
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   authCookieSecure(),
			SameSite: http.SameSiteStrictMode,
			Path:     "/",
		})
	}
}

func setGoogleOAuthStateCookie(w http.ResponseWriter, state string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     googleOAuthStateCookie,
		Value:    state,
		Expires:  expires,
		HttpOnly: true,
		Secure:   authCookieSecure(),
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})
}

func clearGoogleOAuthStateCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     googleOAuthStateCookie,
		Value:    "",
		Expires:  time.Unix(1, 0).UTC(),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   authCookieSecure(),
		SameSite: http.SameSiteLaxMode,
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
		return true
	}
}
