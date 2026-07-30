package privacyfilter

import (
	"automation-hub-backend/internal/identity"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPrivacyHistoryIsScopedToVerifiedOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := NewService()
	handler := NewHandler(service)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if owner := c.GetHeader("X-Test-Owner"); owner != "" {
			c.Set(identity.ContextSubjectKey, owner)
		}
	})
	router.POST("/privacy/scan", handler.ScanContent)
	router.GET("/privacy/scans", handler.Scans)
	router.GET("/privacy/scans/:id", handler.ScanByID)

	aliceID := createScan(t, router, "alice", "alice@example.com")
	bobID := createScan(t, router, "bob", "bob@example.com")
	systemRecord := service.Scan("system@example.com", "system", "", 120)

	aliceScans := listScans(t, router, "alice")
	if len(aliceScans) != 1 || aliceScans[0].ID != aliceID {
		t.Fatalf("Alice scans = %#v, want only %s", aliceScans, aliceID)
	}
	bobScans := listScans(t, router, "bob")
	if len(bobScans) != 1 || bobScans[0].ID != bobID {
		t.Fatalf("Bob scans = %#v, want only %s", bobScans, bobID)
	}

	assertScanStatus(t, router, "alice", aliceID, http.StatusOK)
	assertScanStatus(t, router, "alice", bobID, http.StatusNotFound)
	assertScanStatus(t, router, "alice", systemRecord.ID, http.StatusNotFound)
}

func TestPrivacyHandlersRequireVerifiedOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHandler(NewService())
	router := gin.New()
	router.Use(func(c *gin.Context) {
		switch c.GetHeader("X-Test-Identity") {
		case "blank":
			c.Set(identity.ContextSubjectKey, "  ")
		case "wrong-type":
			c.Set(identity.ContextSubjectKey, []string{"alice"})
		}
	})
	router.POST("/privacy/scan", handler.ScanContent)
	router.GET("/privacy/scans", handler.Scans)
	router.GET("/privacy/scans/:id", handler.ScanByID)

	for _, request := range []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/privacy/scan", `{"content":"private"}`},
		{http.MethodGet, "/privacy/scans", ""},
		{http.MethodGet, "/privacy/scans/scan-1", ""},
	} {
		for _, identityState := range []string{"missing", "blank", "wrong-type"} {
			req := httptest.NewRequest(request.method, request.path, strings.NewReader(request.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Test-Identity", identityState)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("%s %s with %s identity status=%d body=%s", request.method, request.path, identityState, response.Code, response.Body.String())
			}
		}
	}
}

func createScan(t *testing.T, router http.Handler, ownerIdentity, content string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/privacy/scan", strings.NewReader(`{"content":"`+content+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Owner", ownerIdentity)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("create scan for %s status=%d body=%s", ownerIdentity, response.Code, response.Body.String())
	}
	var record ScanRecord
	if err := json.Unmarshal(response.Body.Bytes(), &record); err != nil {
		t.Fatalf("decode scan for %s: %v", ownerIdentity, err)
	}
	return record.ID
}

func listScans(t *testing.T, router http.Handler, ownerIdentity string) []ScanRecord {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/privacy/scans", nil)
	req.Header.Set("X-Test-Owner", ownerIdentity)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("list scans for %s status=%d body=%s", ownerIdentity, response.Code, response.Body.String())
	}
	var payload struct {
		Scans []ScanRecord `json:"scans"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode scans for %s: %v", ownerIdentity, err)
	}
	return payload.Scans
}

func assertScanStatus(t *testing.T, router http.Handler, ownerIdentity, id string, want int) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/privacy/scans/"+id, nil)
	req.Header.Set("X-Test-Owner", ownerIdentity)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != want {
		t.Fatalf("get scan %s as %s status=%d want=%d body=%s", id, ownerIdentity, response.Code, want, response.Body.String())
	}
}
