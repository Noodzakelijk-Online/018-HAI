package authentication

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestAuthCookiesAreSecureByDefault(t *testing.T) {
	t.Setenv("IDP_COOKIE_SECURE", "")
	recorder := httptest.NewRecorder()

	setAccessTokenCookie(recorder, "access-token", time.Now().Add(time.Hour))

	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure {
		t.Fatalf("default cookie = %#v, want one Secure cookie", cookies)
	}
}

func TestAuthCookiesAllowExplicitInsecureLocalHTTP(t *testing.T) {
	t.Setenv("IDP_COOKIE_SECURE", "false")
	recorder := httptest.NewRecorder()

	setRefreshTokenCookie(recorder, "refresh-token", time.Now().Add(time.Hour))

	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Secure {
		t.Fatalf("local cookie = %#v, want one explicitly insecure cookie", cookies)
	}
}

func TestClearAuthCookiesExpiresAccessAndRefreshTokens(t *testing.T) {
	t.Setenv("IDP_COOKIE_SECURE", "true")
	recorder := httptest.NewRecorder()

	clearAuthCookies(recorder)

	cookies := recorder.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("cookies = %d, want 2", len(cookies))
	}
	for _, cookie := range cookies {
		if cookie.MaxAge >= 0 || !cookie.HttpOnly || !cookie.Secure || cookie.Path != "/" {
			t.Fatalf("deletion cookie %q is not hardened: %#v", cookie.Name, cookie)
		}
	}
}
