package autogencompat

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPreviewHandlerIsBoundedAndSideEffectFree(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler(DefaultService())
	router.GET("/status", handler.Status)
	router.POST("/preview", handler.Preview)
	router.POST("/migration-plan", handler.MigrationPlan)

	status := httptest.NewRecorder()
	router.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/status", nil))
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), "no AutoGen package") {
		t.Fatalf("status = %d %s", status.Code, status.Body.String())
	}

	request := httptest.NewRequest(http.MethodPost, "/preview", strings.NewReader(`{"workloadId":"legacy-team","events":[{"id":"m1","type":"approval_request","summary":"Request approval"}]}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "approval queue") || !strings.Contains(response.Body.String(), `"executionAllowed":false`) {
		t.Fatalf("preview = %d %s", response.Code, response.Body.String())
	}

	bad := httptest.NewRecorder()
	router.ServeHTTP(bad, httptest.NewRequest(http.MethodPost, "/preview", strings.NewReader(`{"workloadId":"legacy","events":[]}`)))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("bad preview = %d %s", bad.Code, bad.Body.String())
	}

	migration := httptest.NewRecorder()
	migrationRequest := httptest.NewRequest(http.MethodPost, "/migration-plan", strings.NewReader(`{"target":"microsoft-agent-framework","workloadId":"legacy-team","events":[{"id":"m1","type":"tool_call","summary":"Inspect local source"}]}`))
	migrationRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(migration, migrationRequest)
	if migration.Code != http.StatusOK || !strings.Contains(migration.Body.String(), `"executionAllowed":false`) || !strings.Contains(migration.Body.String(), "did not install") {
		t.Fatalf("migration = %d %s", migration.Code, migration.Body.String())
	}
}
