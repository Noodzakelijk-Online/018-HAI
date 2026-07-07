package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"automation-hub-backend/internal/doctor"

	"github.com/gin-gonic/gin"
)

func healthyReport() doctor.Report {
	return doctor.Report{Checks: []doctor.Check{
		{Name: "database.host", Severity: doctor.SeverityOK},
		{Name: "runtime.mode", Severity: doctor.SeverityOK},
	}}
}

func TestSystemInfoExposesBuildModeLanguagesReadiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/system/info", nil)
	systemInfoHandler(healthyReport)(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Build     map[string]any `json:"build"`
		RunMode   string         `json:"runMode"`
		Languages []string       `json:"languages"`
		Readiness map[string]any `json:"readiness"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Build["goVersion"] == "" {
		t.Fatalf("build info missing goVersion")
	}
	if body.RunMode == "" {
		t.Fatalf("runMode missing")
	}
	if len(body.Languages) != 2 {
		t.Fatalf("languages = %v, want en+nl", body.Languages)
	}
	if body.Readiness["ready"] != true {
		t.Fatalf("readiness not ready: %+v", body.Readiness)
	}
}

func TestSupportBundleEndpointIncludesCounts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/system/support-bundle", nil)
	counts := func() map[string]int { return map[string]int{"languages": 2} }
	supportBundleHandler(healthyReport, counts)(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Counts    map[string]int `json:"counts"`
		Readiness map[string]any `json:"readiness"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Counts["languages"] != 2 {
		t.Fatalf("counts not carried: %+v", body.Counts)
	}
}
