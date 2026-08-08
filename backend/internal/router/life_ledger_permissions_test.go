package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"automation-hub-backend/internal/lifeledger"

	"github.com/gin-gonic/gin"
)

func TestLifeLedgerRoutesEnforceOwnerRoleAndPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(testIdentityMiddleware())
	service, err := lifeledger.NewService(lifeledger.NewMemoryRepository(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := initializeLifeLedgerRoutes(engine.Group("/api/v1"), lifeledger.NewHandler(service)); err != nil {
		t.Fatalf("initialize routes: %v", err)
	}

	for _, test := range []struct {
		name   string
		method string
		path   string
		role   string
		body   string
		want   int
	}{
		{name: "viewer may inspect commitments", method: http.MethodGet, path: "/api/v1/life-ledger/commitments", role: "viewer", want: http.StatusOK},
		{name: "viewer may inspect costs", method: http.MethodGet, path: "/api/v1/life-ledger/costs", role: "viewer", want: http.StatusOK},
		{name: "unknown role fails closed", method: http.MethodGet, path: "/api/v1/life-ledger/commitments", role: "future-role", want: http.StatusForbidden},
		{name: "viewer cannot append", method: http.MethodPost, path: "/api/v1/life-ledger/costs", role: "viewer", body: `{}`, want: http.StatusForbidden},
		{name: "operator reaches strict validation", method: http.MethodPost, path: "/api/v1/life-ledger/costs", role: "operator", body: `{}`, want: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Test-Verified-Role", test.role)
			engine.ServeHTTP(recorder, request)
			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, test.want, recorder.Body.String())
			}
		})
	}
}

func TestLifeLedgerRouteRegistrationRejectsMissingHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if err := initializeLifeLedgerRoutes(gin.New().Group("/api/v1"), nil); err == nil {
		t.Fatal("expected nil handler registration failure")
	}
}
