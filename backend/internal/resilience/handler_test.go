package resilience

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
)

func TestRegisterRoutesFailsClosedWithoutCompleteGuards(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	handler, err := NewAdvisoryAPI(NewMemoryRepository())
	if err != nil {
		t.Fatal(err)
	}
	if err := RegisterRoutes(engine.Group("/api/v1"), handler, RouteGuards{}); err == nil {
		t.Fatal("routes registered without required guards")
	}
}

func TestHandlerStrictScopeRedactionAndAdvisoryResponses(t *testing.T) {
	engine := resilienceTestEngine(t)
	hash := strings.Repeat("a", 64)
	key := strings.Repeat("b", 64)
	base := "/api/v1/resilience/workspaces/workspace-a"

	missing := performResilienceRequest(engine, http.MethodGet, base+"/status", "", "")
	if missing.Code != http.StatusUnauthorized {
		t.Fatalf("missing identity status = %d", missing.Code)
	}

	spoofed := performResilienceRequest(engine, http.MethodPost, base+"/leases/work-1/acquire", `{"ownerId":"owner-b","workerId":"worker-1","idempotencyKey":"`+key+`","payloadHash":"`+hash+`","ttlSeconds":300}`, "owner-a")
	if spoofed.Code != http.StatusBadRequest {
		t.Fatalf("owner spoof status = %d: %s", spoofed.Code, spoofed.Body.String())
	}

	wrongType := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, base+"/leases/work-1/acquire", strings.NewReader(`{}`))
	request.Header.Set("X-Owner", "owner-a")
	request.Header.Set("Content-Type", "text/plain")
	engine.ServeHTTP(wrongType, request)
	if wrongType.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("content type status = %d", wrongType.Code)
	}

	created := performResilienceRequest(engine, http.MethodPost, base+"/leases/work-1/acquire", `{"workerId":"worker-1","idempotencyKey":"`+key+`","payloadHash":"`+hash+`","ttlSeconds":300}`, "owner-a")
	if created.Code != http.StatusOK {
		t.Fatalf("create status = %d: %s", created.Code, created.Body.String())
	}
	assertNoExecutionAuthorityJSON(t, created.Body.String())

	ownerB := performResilienceRequest(engine, http.MethodGet, base+"/leases/work-1", "", "owner-b")
	if ownerB.Code != http.StatusNotFound {
		t.Fatalf("cross-owner get status = %d: %s", ownerB.Code, ownerB.Body.String())
	}

	retry := performResilienceRequest(engine, http.MethodPost, base+"/retries/work-1/advise", `{
		"attemptsCompleted":1,
		"failure":{"code":"network_timeout","class":"transient","message":"request token=super-secret-value"},
		"policy":{"maxAttempts":3,"baseDelaySeconds":1,"multiplier":2,"maxDelaySeconds":60}
	}`, "owner-a")
	if retry.Code != http.StatusOK {
		t.Fatalf("retry status = %d: %s", retry.Code, retry.Body.String())
	}
	if strings.Contains(retry.Body.String(), "super-secret-value") || !strings.Contains(retry.Body.String(), "REDACTED") {
		t.Fatalf("secret not redacted: %s", retry.Body.String())
	}
	assertNoExecutionAuthorityJSON(t, retry.Body.String())

	events := performResilienceRequest(engine, http.MethodGet, base+"/events?limit=100", "", "owner-a")
	if events.Code != http.StatusOK {
		t.Fatalf("events status = %d: %s", events.Code, events.Body.String())
	}
	var payload struct {
		Events []EventRecord `json:"events"`
	}
	if err := json.Unmarshal(events.Body.Bytes(), &payload); err != nil || len(payload.Events) != 2 {
		t.Fatalf("events payload = %s, %v", events.Body.String(), err)
	}
	if payload.Events[1].Event.PreviousHash != payload.Events[0].Hash {
		t.Fatal("HTTP-visible event chain is broken")
	}

	oversized := `{"workerId":"` + strings.Repeat("x", maxResilienceRequestBytes) + `"}`
	tooLarge := performResilienceRequest(engine, http.MethodPost, base+"/leases/work-2/acquire", oversized, "owner-a")
	if tooLarge.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized status = %d: %s", tooLarge.Code, tooLarge.Body.String())
	}
}

func TestHandlerRejectsMalformedAndReplayedFencingTransitions(t *testing.T) {
	engine := resilienceTestEngine(t)
	base := "/api/v1/resilience/workspaces/workspace-a"
	hash := strings.Repeat("c", 64)
	key := strings.Repeat("d", 64)
	create := performResilienceRequest(engine, http.MethodPost, base+"/leases/work-1/acquire", `{"workerId":"worker-1","idempotencyKey":"`+key+`","payloadHash":"`+hash+`","ttlSeconds":300}`, "owner-a")
	if create.Code != http.StatusOK {
		t.Fatalf("seed = %d: %s", create.Code, create.Body.String())
	}

	trailing := performResilienceRequest(engine, http.MethodPost, base+"/leases/work-1/release", `{"workerId":"worker-1","generation":1}{}`, "owner-a")
	if trailing.Code != http.StatusBadRequest {
		t.Fatalf("trailing JSON status = %d", trailing.Code)
	}

	stale := performResilienceRequest(engine, http.MethodPost, base+"/leases/work-1/release", `{"workerId":"worker-1","generation":0}`, "owner-a")
	if stale.Code != http.StatusBadRequest && stale.Code != http.StatusConflict {
		t.Fatalf("stale fence status = %d: %s", stale.Code, stale.Body.String())
	}

	badLimit := performResilienceRequest(engine, http.MethodGet, base+"/leases?limit=0", "", "owner-a")
	if badLimit.Code != http.StatusBadRequest {
		t.Fatalf("bad limit status = %d", badLimit.Code)
	}
}

func resilienceTestEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	handler, err := NewAdvisoryAPI(NewMemoryRepository(100))
	if err != nil {
		t.Fatal(err)
	}
	ownerGuard := func(c *gin.Context) {
		if owner := strings.TrimSpace(c.GetHeader("X-Owner")); owner != "" {
			c.Set(identity.ContextSubjectKey, owner)
		}
	}
	pass := func(c *gin.Context) {}
	guards := RouteGuards{AuthenticatedOwner: ownerGuard, RecognizedRole: pass, Read: pass, Write: pass, Govern: pass}
	if err := RegisterRoutes(engine.Group("/api/v1"), handler, guards); err != nil {
		t.Fatal(err)
	}
	return engine
}

func performResilienceRequest(engine http.Handler, method, path, body, owner string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if owner != "" {
		request.Header.Set("X-Owner", owner)
	}
	engine.ServeHTTP(recorder, request)
	return recorder
}

func assertNoExecutionAuthorityJSON(t *testing.T, payload string) {
	t.Helper()
	for _, forbidden := range []string{`"canExecute":true`, `"grantsAuthority":true`, `"consumesApproval":true`, `"dispatchesWork":true`} {
		if strings.Contains(payload, forbidden) {
			t.Fatalf("authority leaked: %s", payload)
		}
	}
}
