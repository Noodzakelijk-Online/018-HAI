package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"automation-hub-backend/internal/doctor"

	"github.com/gin-gonic/gin"
)

func serveReadiness(t *testing.T, report doctor.Report) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/readyz", nil)
	readinessHandler(func() doctor.Report { return report })(c)

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode readiness body: %v (%s)", err, rec.Body.String())
	}
	return rec.Code, body
}

func TestReadinessReadyWhenNoFailures(t *testing.T) {
	report := doctor.Report{Checks: []doctor.Check{
		{Name: "database.host", Severity: doctor.SeverityOK, Detail: "postgres"},
		{Name: "security.backendApiKey", Severity: doctor.SeverityWarn, Detail: "empty"},
	}}
	code, body := serveReadiness(t, report)
	if code != http.StatusOK {
		t.Fatalf("code = %d, want 200", code)
	}
	if body["status"] != "ready" {
		t.Fatalf("status = %v, want ready", body["status"])
	}
}

func TestReadinessNotReadyWhenAnyFailure(t *testing.T) {
	report := doctor.Report{Checks: []doctor.Check{
		{Name: "database.host", Severity: doctor.SeverityFail, Detail: "DB_HOST empty"},
	}}
	code, body := serveReadiness(t, report)
	if code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want 503", code)
	}
	if body["status"] != "not_ready" {
		t.Fatalf("status = %v, want not_ready", body["status"])
	}
}
