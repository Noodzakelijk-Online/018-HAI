package router

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/domainpack"
	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
)

func TestDomainPackPlaybookRoutesRequireOwnerAndRemainAdvisory(t *testing.T) {
	engine := newDomainPackRouteTestEngine(t)

	unauthenticated := performControlledLearningRouteRequest(
		engine,
		http.MethodGet,
		"/api/v1/domain-packs/work_venture/playbook",
		"",
		"",
		"",
	)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf(
			"unauthenticated playbook status = %d, want %d: %s",
			unauthenticated.Code,
			http.StatusUnauthorized,
			unauthenticated.Body.String(),
		)
	}

	viewer := performControlledLearningRouteRequest(
		engine,
		http.MethodGet,
		"/api/v1/domain-packs/work_venture/playbook",
		"",
		"alice",
		"viewer",
	)
	if viewer.Code != http.StatusOK {
		t.Fatalf("viewer playbook status = %d: %s", viewer.Code, viewer.Body.String())
	}
	for _, expected := range []string{
		`"advisoryOnly":true`,
		`"executionAuthorityGranted":false`,
		`"work_service_delivery"`,
		`"entrepreneurship_venture"`,
	} {
		if !strings.Contains(viewer.Body.String(), expected) {
			t.Fatalf("playbook response missing %s: %s", expected, viewer.Body.String())
		}
	}

	selected := performControlledLearningRouteRequest(
		engine,
		http.MethodPost,
		"/api/v1/domain-packs/methods/select",
		`{"text":"Use a debt avalanche.","classifiedPackIds":["financial"]}`,
		"alice",
		"viewer",
	)
	if selected.Code != http.StatusOK {
		t.Fatalf("method selection status = %d: %s", selected.Code, selected.Body.String())
	}
	for _, expected := range []string{
		`"financial_management.debt_avalanche"`,
		`"advisoryOnly":true`,
		`"executionAuthorityGranted":false`,
	} {
		if !strings.Contains(selected.Body.String(), expected) {
			t.Fatalf("method selection missing %s: %s", expected, selected.Body.String())
		}
	}

	unknownRole := performControlledLearningRouteRequest(
		engine,
		http.MethodGet,
		"/api/v1/domain-packs/work_venture/playbook",
		"",
		"alice",
		"unknown",
	)
	if unknownRole.Code != http.StatusForbidden {
		t.Fatalf(
			"unknown role status = %d, want %d: %s",
			unknownRole.Code,
			http.StatusForbidden,
			unknownRole.Body.String(),
		)
	}
}

func newDomainPackRouteTestEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	registry, err := domainpack.NewBuiltinRegistry()
	if err != nil {
		t.Fatalf("domainpack.NewBuiltinRegistry: %v", err)
	}
	handler, err := domainpack.NewHandler(
		registry,
		domainpack.NewMemoryPreferenceRepository(time.Now),
	)
	if err != nil {
		t.Fatalf("domainpack.NewHandler: %v", err)
	}

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		if subject := strings.TrimSpace(c.GetHeader("X-Test-Verified-Subject")); subject != "" {
			c.Set(identity.ContextSubjectKey, subject)
		}
		if role := strings.TrimSpace(c.GetHeader("X-Test-Verified-Role")); role != "" {
			c.Set(identity.ContextRoleKey, role)
		}
		c.Next()
	})
	initializeDomainPackRoutes(engine.Group("/api/v1"), handler)
	return engine
}
