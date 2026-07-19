package browserverify

import "testing"

func TestStatusKeepsBrowserVerificationDisabledUntilFullyConfigured(t *testing.T) {
	svc := NewService(nil, false, "http://browser-verifier:8080", "not-used", nil)
	if status := svc.Status(); status.Enabled || status.Configured {
		t.Fatalf("disabled browser verification must not report configured: %#v", status)
	}
	profiles := []Profile{{ID: "local-login", Name: "Local login", URL: "http://frontend/login", ExpectedPath: "/login"}}
	svc = NewService(nil, true, "http://browser-verifier:8080", "0123456789abcdef", profiles)
	if status := svc.Status(); !status.Configured || status.Scope == "" {
		t.Fatalf("complete local browser profile should be configured: %#v", status)
	}
}

func TestBrowserVerificationRejectsRemoteAndQueryBearingProfiles(t *testing.T) {
	profiles := []Profile{{ID: "remote", Name: "Remote", URL: "https://example.com/"}}
	svc := NewService(nil, true, "http://browser-verifier:8080", "0123456789abcdef", profiles)
	if svc.Status().ConfigError == "" {
		t.Fatalf("remote profile must be rejected")
	}
	profiles = []Profile{{ID: "query", Name: "Query", URL: "http://frontend/login?token=secret"}}
	svc = NewService(nil, true, "http://browser-verifier:8080", "0123456789abcdef", profiles)
	if svc.Status().ConfigError == "" {
		t.Fatalf("query-bearing profile must be rejected")
	}
}
