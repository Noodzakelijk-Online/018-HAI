package resilience

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
)

func TestRegisterRoutesRequiresEveryGuard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	if err := RegisterRoutes(router.Group("/api/v1"), NewHandler(NewService(NewMemoryRepository())), RouteGuards{}); err == nil {
		t.Fatal("unguarded resilience routes were accepted")
	}
}

func TestResilienceRoutesApplyReadWriteAndGovernGuards(t *testing.T) {
	gin.SetMode(gin.TestMode)
	counts := map[string]int{}
	guard := func(name string) gin.HandlerFunc {
		return func(c *gin.Context) {
			counts[name]++
			if name == "owner" {
				c.Set(identity.ContextSubjectKey, "owner-1")
			}
			c.Next()
		}
	}
	router := gin.New()
	err := RegisterRoutes(router.Group("/api/v1"), NewHandler(newServiceWithClock(NewMemoryRepository(), steppingClock(testNow))), RouteGuards{
		AuthenticatedOwner: guard("owner"), RecognizedRole: guard("role"), Read: guard("read"),
		Write: guard("write"), Govern: guard("govern"),
	})
	if err != nil {
		t.Fatal(err)
	}

	performJSON(router, http.MethodGet, "/api/v1/resilience/workspaces/workspace-1/events", "", nil)
	performJSON(router, http.MethodPost, "/api/v1/resilience/workspaces/workspace-1/leases/work-1/acquire", `{}`, nil)
	recovery := `{"heartbeatMaxAgeSeconds":60,"attemptsCompleted":0,"retryPolicy":{"maxAttempts":3,"baseDelaySeconds":1,"multiplier":2,"maxDelaySeconds":10}}`
	performJSON(router, http.MethodPost, "/api/v1/resilience/workspaces/workspace-1/recoveries/work-2/advise", recovery, nil)
	if counts["read"] != 1 || counts["write"] != 1 || counts["govern"] != 1 || counts["owner"] != 3 || counts["role"] != 3 {
		t.Fatalf("guard counts=%v", counts)
	}
}

func TestHandlerStrictJSONIsolationAndNoAuthorityResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := newServiceWithClock(NewMemoryRepository(), steppingClock(testNow))
	alice := resilienceTestRouter(t, service, "alice")
	base := "/api/v1/resilience/workspaces/workspace-1"
	leaseBody := `{"idempotencyKey":"` + strings.Repeat("b", 64) + `","payloadHash":"` + testPayload + `","workerId":"worker-1","ttlSeconds":60}`
	lease := performJSON(alice, http.MethodPost, base+"/leases/work-1/acquire", leaseBody, map[string]string{"Content-Type": "application/json"})
	if lease.Code != http.StatusOK {
		t.Fatalf("lease status=%d body=%s", lease.Code, lease.Body.String())
	}
	assertHTTPAdvisory(t, lease.Body.Bytes())

	bob := resilienceTestRouter(t, service, "bob")
	crossOwner := performJSON(bob, http.MethodGet, base+"/leases/work-1", "", nil)
	if crossOwner.Code != http.StatusNotFound {
		t.Fatalf("cross-owner status=%d body=%s", crossOwner.Code, crossOwner.Body.String())
	}
	assertHTTPAdvisory(t, crossOwner.Body.Bytes())
	otherWorkspace := performJSON(alice, http.MethodGet, "/api/v1/resilience/workspaces/workspace-2/leases/work-1", "", nil)
	if otherWorkspace.Code != http.StatusNotFound {
		t.Fatalf("cross-workspace status=%d body=%s", otherWorkspace.Code, otherWorkspace.Body.String())
	}

	tests := []struct {
		name        string
		method      string
		path        string
		body        string
		contentType string
		wantStatus  int
	}{
		{name: "spoofed owner field", method: http.MethodPost, path: base + "/leases/work-2/acquire", body: strings.TrimSuffix(leaseBody, "}") + `,"ownerId":"mallory"}`, contentType: "application/json", wantStatus: http.StatusBadRequest},
		{name: "unknown field", method: http.MethodPost, path: base + "/leases/work-2/acquire", body: strings.TrimSuffix(leaseBody, "}") + `,"execute":true}`, contentType: "application/json", wantStatus: http.StatusBadRequest},
		{name: "trailing json", method: http.MethodPost, path: base + "/leases/work-2/acquire", body: leaseBody + `{}`, contentType: "application/json", wantStatus: http.StatusBadRequest},
		{name: "wrong media type", method: http.MethodPost, path: base + "/leases/work-2/acquire", body: leaseBody, contentType: "text/plain", wantStatus: http.StatusUnsupportedMediaType},
		{name: "unknown query", method: http.MethodPost, path: base + "/leases/work-2/acquire?owner=mallory", body: leaseBody, contentType: "application/json", wantStatus: http.StatusBadRequest},
		{name: "release-only schema", method: http.MethodPost, path: base + "/leases/work-1/release", body: `{"workerId":"worker-1","generation":1,"ttlSeconds":60}`, contentType: "application/json", wantStatus: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := performJSON(alice, test.method, test.path, test.body, map[string]string{"Content-Type": test.contentType})
			if response.Code != test.wantStatus {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			assertHTTPAdvisory(t, response.Body.Bytes())
		})
	}
}

func TestHandlerErrorsNeverReflectSecretsAndBodiesAreBounded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := resilienceTestRouter(t, newServiceWithClock(NewMemoryRepository(), steppingClock(testNow)), "alice")
	base := "/api/v1/resilience/workspaces/workspace-1"
	secret := "Bearer abcdefghijklmnopqrstuvwxyz"
	body := `{"heartbeatMaxAgeSeconds":60,"attemptsCompleted":1,"failure":{"code":"upstream","class":"transient","message":"` + secret + `"},"retryPolicy":{"maxAttempts":3,"baseDelaySeconds":1,"multiplier":2,"maxDelaySeconds":10}}`
	response := performJSON(router, http.MethodPost, base+"/recoveries/work-1/advise", body, map[string]string{"Content-Type": "application/json"})
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), secret) || strings.Contains(response.Body.String(), "abcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("secret-safe response status=%d body=%s", response.Code, response.Body.String())
	}
	assertHTTPAdvisory(t, response.Body.Bytes())

	oversized := `{"idempotencyKey":"` + strings.Repeat("b", 64) + `","payloadHash":"` + testPayload + `","workerId":"` + strings.Repeat("x", maxResilienceRequestBytes) + `","ttlSeconds":60}`
	response = performJSON(router, http.MethodPost, base+"/leases/work-1/acquire", oversized, map[string]string{"Content-Type": "application/json"})
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized status=%d body=%s", response.Code, response.Body.String())
	}
	assertHTTPAdvisory(t, response.Body.Bytes())
}

func TestReadRoutesAlwaysIncludeAdvisoryAuthority(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := resilienceTestRouter(t, newServiceWithClock(NewMemoryRepository(), steppingClock(testNow)), "alice")
	base := "/api/v1/resilience/workspaces/workspace-1"
	for _, path := range []string{
		"/status", "/leases", "/workers", "/retries", "/circuits", "/recoveries", "/events",
	} {
		response := performJSON(router, http.MethodGet, base+path, "", nil)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status=%d body=%s", path, response.Code, response.Body.String())
		}
		assertHTTPAdvisory(t, response.Body.Bytes())
	}
}

func resilienceTestRouter(t *testing.T, service *Service, owner string) *gin.Engine {
	t.Helper()
	router := gin.New()
	ownerGuard := func(c *gin.Context) { c.Set(identity.ContextSubjectKey, owner); c.Next() }
	pass := func(c *gin.Context) { c.Next() }
	if err := RegisterRoutes(router.Group("/api/v1"), NewHandler(service), RouteGuards{
		AuthenticatedOwner: ownerGuard, RecognizedRole: pass, Read: pass, Write: pass, Govern: pass,
	}); err != nil {
		t.Fatal(err)
	}
	return router
}

func performJSON(router http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func assertHTTPAdvisory(t *testing.T, body []byte) {
	t.Helper()
	var envelope struct {
		Authority AuthorityBoundary `json:"authority"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("response JSON: %v (%s)", err, body)
	}
	assertAdvisoryOnly(t, envelope.Authority)
}
