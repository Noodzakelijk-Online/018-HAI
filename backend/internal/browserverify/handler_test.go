package browserverify

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"automation-hub-backend/internal/identity"
	"github.com/gin-gonic/gin"
)

func TestRunPassesWorkflowIDToOwnerScopedLinker(t *testing.T) {
	gin.SetMode(gin.TestMode)
	runner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/verify" {
			t.Fatalf("unexpected verifier request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"status":"passed","finalPath":"/control-center","pageTitle":"HAI","summary":"named local route reached"}`))
	}))
	defer runner.Close()

	linker := &workflowLinkerStub{}
	service := NewService(&runRepositoryStub{}, true, runner.URL, "0123456789abcdef", []Profile{{
		ID: "control-center", Name: "Control center", URL: "http://frontend/control-center", ExpectedPath: "/control-center",
	}}, linker)
	handler := NewHandler(service)

	engine := gin.New()
	engine.POST("/profiles/:id/run", func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "owner@example.test")
		handler.Run(c)
	})
	workflowID := "7f4b2da3-4678-47dc-9558-feb9540c3a3a"
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/profiles/control-center/run", bytes.NewBufferString(`{"workflowId":"`+workflowID+`"}`))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if linker.owner != "owner@example.test" || linker.workflowID != workflowID || linker.status != "passed" {
		t.Fatalf("unexpected owner-scoped workflow link: %#v", linker)
	}
}
