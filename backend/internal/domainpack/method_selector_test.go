package domainpack

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
)

func TestMethodSelectionIsDeterministicExplainableAndAdvisoryOnly(t *testing.T) {
	t.Parallel()
	registry := mustBuiltinRegistry(t)
	request := MethodSelectionRequest{
		Text:              "Prepare a legal chronology and contradiction matrix from the case evidence.",
		ClassifiedPackIDs: []PackID{PackLegalGovernment},
		Limit:             5,
	}
	first, err := registry.SelectMethods(request, nil)
	if err != nil {
		t.Fatalf("first SelectMethods: %v", err)
	}
	second, err := registry.SelectMethods(request, nil)
	if err != nil {
		t.Fatalf("second SelectMethods: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("selection is not deterministic:\n%#v\n%#v", first, second)
	}
	if !first.AdvisoryOnly || first.ExecutionAuthorityGranted {
		t.Fatalf("selection authority flags = %#v", first)
	}
	if first.CatalogVersion != CatalogVersion || first.CatalogDigest != registry.Metadata().Digest {
		t.Fatalf("selection catalog metadata = %#v", first)
	}
	for _, id := range []string{
		"legal_government_case.chronology",
		"legal_government_case.contradiction_matrix",
	} {
		selection, ok := findMethodSelection(first.Selections, id)
		if !ok || selection.Score == 0 || len(selection.Reasons) == 0 {
			t.Errorf("missing explainable selection %s in %#v", id, first.Selections)
		}
	}
}

func TestMethodSelectionExplicitMethodStaysWithinClassifiedPack(t *testing.T) {
	t.Parallel()
	registry := mustBuiltinRegistry(t)
	result, err := registry.SelectMethods(MethodSelectionRequest{
		Text:              "Help structure the report.",
		ClassifiedPackIDs: []PackID{PackHealthWellbeing},
		ExplicitMethodIDs: []string{"health_personal_care.soap"},
	}, nil)
	if err != nil {
		t.Fatalf("SelectMethods: %v", err)
	}
	selection, ok := findMethodSelection(result.Selections, "health_personal_care.soap")
	if !ok || !selection.Explicit || selection.Score < 1000 {
		t.Fatalf("explicit selection = %#v", selection)
	}
	if _, err := registry.SelectMethods(MethodSelectionRequest{
		ClassifiedPackIDs: []PackID{PackHealthWellbeing},
		ExplicitMethodIDs: []string{"financial_management.four_eyes_payments"},
	}, nil); err == nil || !strings.Contains(err.Error(), "classified enabled packs") {
		t.Fatalf("cross-pack explicit method error = %v", err)
	}
}

func TestMethodSelectionOwnerIsolationAndPreferenceSuppression(t *testing.T) {
	t.Parallel()
	registry := mustBuiltinRegistry(t)
	preferences := NewMemoryPreferenceRepository(nil)
	disabled := false
	if _, err := preferences.Upsert(PackPreference{
		OwnerIdentity: "alice",
		PackID:        PackWorkVenture,
		Enabled:       &disabled,
	}); err != nil {
		t.Fatalf("disable alice pack: %v", err)
	}
	request := MethodSelectionRequest{
		Text:              "Use the Business Model Canvas.",
		ClassifiedPackIDs: []PackID{PackWorkVenture},
		OwnerIdentity:     "alice",
	}
	alice, err := registry.SelectMethods(request, preferences)
	if err != nil {
		t.Fatalf("alice SelectMethods: %v", err)
	}
	if len(alice.Selections) != 0 || len(alice.Suppressed) != 1 ||
		!strings.Contains(alice.Suppressed[0].Reason, "owner-scoped") {
		t.Fatalf("alice result = %#v", alice)
	}
	request.OwnerIdentity = "bob"
	bob, err := registry.SelectMethods(request, preferences)
	if err != nil {
		t.Fatalf("bob SelectMethods: %v", err)
	}
	if _, ok := findMethodSelection(bob.Selections,
		"entrepreneurship_venture.business_model_canvas"); !ok {
		t.Fatalf("alice preference leaked into bob result: %#v", bob)
	}
	if _, err := registry.SelectMethods(MethodSelectionRequest{
		Text:              request.Text,
		ClassifiedPackIDs: request.ClassifiedPackIDs,
	}, preferences); err == nil || !strings.Contains(err.Error(), "owner identity") {
		t.Fatalf("ownerless preference resolution error = %v", err)
	}
}

func TestMethodSelectionResultCannotMutateRegistry(t *testing.T) {
	t.Parallel()
	registry := mustBuiltinRegistry(t)
	result, err := registry.SelectMethods(MethodSelectionRequest{
		Text:              "Run a debt avalanche analysis.",
		ClassifiedPackIDs: []PackID{PackFinancial},
	}, nil)
	if err != nil || len(result.Selections) == 0 {
		t.Fatalf("SelectMethods result=%#v err=%v", result, err)
	}
	result.Selections[0].Method.SafetyInvariants[0] = "mutated"
	result.Selections[0].Method.Evaluation.Criteria[0] = "mutated"
	pack, _ := registry.Lookup(PackFinancial)
	for _, method := range pack.Playbook.Methods {
		if method.SafetyInvariants[0] == "mutated" || method.Evaluation.Criteria[0] == "mutated" {
			t.Fatal("selection result exposed registry state")
		}
	}
}

func TestMethodSelectionValidationAndLimit(t *testing.T) {
	t.Parallel()
	registry := mustBuiltinRegistry(t)
	for name, request := range map[string]MethodSelectionRequest{
		"missing classified pack": {Text: "SOAP"},
		"unknown pack":            {Text: "SOAP", ClassifiedPackIDs: []PackID{"unknown"}},
		"invalid limit":           {Text: "SOAP", ClassifiedPackIDs: []PackID{PackHealthWellbeing}, Limit: 51},
	} {
		if _, err := registry.SelectMethods(request, nil); err == nil {
			t.Errorf("%s unexpectedly succeeded", name)
		}
	}
	result, err := registry.SelectMethods(MethodSelectionRequest{
		Text:              "Use SWOT TOWS PESTEL VRIO AARRR OKRs and scenario planning.",
		ClassifiedPackIDs: []PackID{PackWorkVenture},
		Limit:             3,
	}, nil)
	if err != nil || len(result.Selections) != 3 {
		t.Fatalf("limited selection = %#v err=%v", result, err)
	}
}

func TestPlaybookHandlerMethodsRequireOwnerAndDoNotGrantAuthority(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := mustDomainPackHandler(t)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if owner := c.GetHeader("X-Test-Owner"); owner != "" {
			c.Set(identity.ContextSubjectKey, owner)
		}
		c.Next()
	})
	router.GET("/playbooks/:id", handler.Playbook)
	router.POST("/methods/select", handler.SelectMethods)

	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, httptest.NewRequest(http.MethodGet,
		"/playbooks/financial", nil))
	if missing.Code != http.StatusUnauthorized {
		t.Fatalf("missing owner status = %d", missing.Code)
	}

	detail := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/playbooks/work_venture", nil)
	request.Header.Set("X-Test-Owner", "alice")
	router.ServeHTTP(detail, request)
	if detail.Code != http.StatusOK ||
		!strings.Contains(detail.Body.String(), `"work_service_delivery"`) ||
		!strings.Contains(detail.Body.String(), `"entrepreneurship_venture"`) ||
		!strings.Contains(detail.Body.String(), `"executionAuthorityGranted":false`) {
		t.Fatalf("playbook detail = %d %s", detail.Code, detail.Body.String())
	}

	selected := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/methods/select", strings.NewReader(
		`{"text":"Use a debt avalanche.","classifiedPackIds":["financial"]}`,
	))
	request.Header.Set("X-Test-Owner", "alice")
	router.ServeHTTP(selected, request)
	if selected.Code != http.StatusOK ||
		!strings.Contains(selected.Body.String(), `"financial_management.debt_avalanche"`) ||
		!strings.Contains(selected.Body.String(), `"advisoryOnly":true`) ||
		!strings.Contains(selected.Body.String(), `"executionAuthorityGranted":false`) {
		t.Fatalf("method selection = %d %s", selected.Code, selected.Body.String())
	}

	spoofed := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/methods/select", strings.NewReader(
		`{"text":"SOAP","classifiedPackIds":["health_wellbeing"],"ownerIdentity":"bob"}`,
	))
	request.Header.Set("X-Test-Owner", "alice")
	router.ServeHTTP(spoofed, request)
	if spoofed.Code != http.StatusBadRequest {
		t.Fatalf("spoofed identity status = %d body=%s", spoofed.Code, spoofed.Body.String())
	}
}

func findMethodSelection(selections []MethodSelection, id string) (MethodSelection, bool) {
	for _, selection := range selections {
		if selection.Method.ID == id {
			return selection, true
		}
	}
	return MethodSelection{}, false
}
