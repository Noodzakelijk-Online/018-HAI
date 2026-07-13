package pursuit

import (
	"net/http/httptest"
	"testing"

	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
)

func TestVerifiedActorUsesAuthenticatedPrincipal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Set(identity.ContextSubjectKey, "bundled-idp-user")

	if got := verifiedActor(context, "operator"); got != "bundled-idp-user" {
		t.Fatalf("verifiedActor() = %q, want authenticated principal", got)
	}
}

func TestVerifiedActorDoesNotUseClientSuppliedActor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())

	if got := verifiedActor(context, "operator"); got != "operator" {
		t.Fatalf("verifiedActor() = %q, want honest local fallback", got)
	}
}
