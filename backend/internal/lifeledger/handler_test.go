package lifeledger

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
)

func TestHandlerRejectsMalformedLimits(t *testing.T) {
	service, err := NewService(NewMemoryRepository(), func() time.Time { return ledgerTestNow })
	if err != nil {
		t.Fatal(err)
	}
	router := lifeLedgerHandlerRouter(t, service, "alice", true)

	for _, path := range []string{
		"/api/v1/life-ledger/commitments?limit=not-a-number",
		"/api/v1/life-ledger/commitments?limit=0",
		"/api/v1/life-ledger/commitments/example/history?limit=-1",
		"/api/v1/life-ledger/costs?limit=201",
		"/api/v1/life-ledger/costs?limit=",
	} {
		t.Run(path, func(t *testing.T) {
			response := performLifeLedgerRequest(router, http.MethodGet, path, nil)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if body := response.Body.String(); !strings.Contains(body, "between 1 and 200") {
				t.Fatalf("malformed limit response disclosed an unexpected error: %s", body)
			}
		})
	}
}

func TestHandlerRequiresAuthenticatedOwnerBeforeParsingInput(t *testing.T) {
	service, err := NewService(NewMemoryRepository(), func() time.Time { return ledgerTestNow })
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		subject any
		set     bool
	}{
		{name: "missing"},
		{name: "blank", subject: "  ", set: true},
		{name: "wrong type", subject: []string{"alice"}, set: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := lifeLedgerHandlerRouter(t, service, test.subject, test.set)
			response := performLifeLedgerRequest(
				router,
				http.MethodGet,
				"/api/v1/life-ledger/commitments?limit=not-a-number",
				nil,
			)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if body := response.Body.String(); strings.Contains(body, "between 1 and 200") || strings.Contains(body, "not-a-number") {
				t.Fatalf("unauthenticated response disclosed request-processing detail: %s", body)
			}
		})
	}
}

func TestHandlerUsesSessionOwnerAndRejectsOwnerSpoofing(t *testing.T) {
	repository := NewMemoryRepository()
	service, err := NewService(repository, func() time.Time { return ledgerTestNow })
	if err != nil {
		t.Fatal(err)
	}
	request := commitmentRequest("alice", "private/commitment", 0, CommitmentActive, "alice-private")
	if _, err := service.RecordCommitment(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	bobRequest := commitmentRequest("bob", "bob/secret", 0, CommitmentActive, "bob-secret")
	if _, err := service.RecordCommitment(t.Context(), bobRequest); err != nil {
		t.Fatal(err)
	}

	aliceRouter := lifeLedgerHandlerRouter(t, service, "alice", true)
	response := performLifeLedgerRequest(aliceRouter, http.MethodGet, "/api/v1/life-ledger/commitments?limit=10", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", response.Code, response.Body.String())
	}
	if body := response.Body.String(); !strings.Contains(body, "private/commitment") || strings.Contains(body, "bob/secret") {
		t.Fatalf("owner-scoped response = %s", body)
	}

	spoofed := []byte(`{"ownerIdentity":"bob","domain":"financial","title":"Spoofed cost","kind":"estimate","amountMinor":100,"currency":"EUR","verification":"source_supported","evidence":[],"idempotencyKey":"spoof","observedAt":"2026-08-03T11:59:00Z"}`)
	response = performLifeLedgerRequest(aliceRouter, http.MethodPost, "/api/v1/life-ledger/costs", spoofed)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("spoofed owner status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "bob") {
		t.Fatalf("spoofed owner was reflected in response: %s", response.Body.String())
	}
}

func TestHandlerRejectsWeakPaidVerificationWithoutPersisting(t *testing.T) {
	repository := NewMemoryRepository()
	service, err := NewService(repository, func() time.Time { return ledgerTestNow })
	if err != nil {
		t.Fatal(err)
	}
	router := lifeLedgerHandlerRouter(t, service, "alice", true)

	request := costRequest("alice", CostPaid, "paid-source-only")
	body, err := json.Marshal(costBody{
		Domain: request.Domain, Title: request.Title, Summary: request.Summary,
		Kind: request.Kind, AmountMinor: request.AmountMinor, Currency: request.Currency,
		Verification: request.Verification, Evidence: request.Evidence,
		IdempotencyKey: request.IdempotencyKey, ObservedAt: request.ObservedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	response := performLifeLedgerRequest(router, http.MethodPost, "/api/v1/life-ledger/costs", body)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("weak paid verification status=%d body=%s", response.Code, response.Body.String())
	}
	if records, listErr := service.ListCosts(t.Context(), "alice", 10); listErr != nil || len(records) != 0 {
		t.Fatalf("rejected paid event was persisted: records=%#v err=%v", records, listErr)
	}

	request.Verification = VerificationHumanConfirmed
	request.IdempotencyKey = "paid-human-confirmed"
	body, err = json.Marshal(costBody{
		Domain: request.Domain, Title: request.Title, Summary: request.Summary,
		Kind: request.Kind, AmountMinor: request.AmountMinor, Currency: request.Currency,
		Verification: request.Verification, Evidence: request.Evidence,
		IdempotencyKey: request.IdempotencyKey, ObservedAt: request.ObservedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	response = performLifeLedgerRequest(router, http.MethodPost, "/api/v1/life-ledger/costs", body)
	if response.Code != http.StatusCreated {
		t.Fatalf("human-confirmed paid status=%d body=%s", response.Code, response.Body.String())
	}
	if records, listErr := service.ListCosts(t.Context(), "alice", 10); listErr != nil || len(records) != 1 || records[0].Verification != VerificationHumanConfirmed {
		t.Fatalf("human-confirmed paid event missing: records=%#v err=%v", records, listErr)
	}
}

func lifeLedgerHandlerRouter(t *testing.T, service *Service, subject any, setSubject bool) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	ownerGuard := func(c *gin.Context) {
		if setSubject {
			c.Set(identity.ContextSubjectKey, subject)
		}
		c.Next()
	}
	pass := func(c *gin.Context) { c.Next() }
	if err := RegisterRoutes(router.Group("/api/v1"), NewHandler(service), RouteGuards{
		AuthenticatedOwner: ownerGuard,
		RecognizedRole:     pass,
		Read:               pass,
		Write:              pass,
	}); err != nil {
		t.Fatal(err)
	}
	return router
}

func performLifeLedgerRequest(router http.Handler, method, path string, body []byte) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
