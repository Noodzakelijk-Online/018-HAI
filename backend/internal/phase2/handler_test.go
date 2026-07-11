package phase2

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"automation-hub-backend/internal/autonomypolicy"
	"automation-hub-backend/internal/operations"

	"github.com/gin-gonic/gin"
)

const feedJSON = `[
  {"externalId":"a1","title":"Organize workspace notes","body":"Consolidate personal notes into a local file"},
  {"externalId":"a2","title":"Pay invoice to landlord","body":"Send payment for the rent invoice"}
]`

func newTestServer(t *testing.T) (*gin.Engine, *Module) {
	t.Helper()
	feedsDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(feedsDir, "inbox.json"), []byte(feedJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		OwnerUserID:  "local-operator",
		WorkspaceID:  "local",
		WorkspaceDir: t.TempDir(),
		FeedsDir:     feedsDir,
		FeedFiles:    []string{"inbox.json"},
		Mode:         autonomypolicy.ModeAutonomousSafe,
	}
	m := NewModule(operations.NewService(operations.NewMemoryRepository()), cfg)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := m.Handler()
	ops := r.Group("/operations")
	ops.GET("", h.ListOperations)
	ops.GET("/dashboard", h.Dashboard)
	ops.GET("/:id", h.GetOperation)
	ops.GET("/:id/events", h.OperationEvents)
	ops.POST("/:id/approve", h.Approve)
	ops.POST("/:id/run", h.RunOperation)
	r.POST("/background/run", h.RunBackground)
	r.GET("/account-feeds", h.ListFeeds)
	return r, m
}

func do(t *testing.T, r *gin.Engine, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestBackgroundRunAndDashboardOverHTTP(t *testing.T) {
	r, _ := newTestServer(t)

	// Trigger a background pass.
	w := do(t, r, http.MethodPost, "/background/run")
	if w.Code != http.StatusOK {
		t.Fatalf("background run: status %d body %s", w.Code, w.Body.String())
	}
	var rep struct {
		OperationsCreated int `json:"operationsCreated"`
		Verified          int `json:"verified"`
		AwaitingApproval  int `json:"awaitingApproval"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.OperationsCreated != 2 || rep.Verified != 1 || rep.AwaitingApproval != 1 {
		t.Fatalf("unexpected report: %+v", rep)
	}

	// Dashboard reflects the results.
	w = do(t, r, http.MethodGet, "/operations/dashboard")
	if w.Code != http.StatusOK {
		t.Fatalf("dashboard: status %d", w.Code)
	}
	var dash operations.Dashboard
	if err := json.Unmarshal(w.Body.Bytes(), &dash); err != nil {
		t.Fatal(err)
	}
	if dash.DoneWhileAway != 1 || dash.NeedsRobert != 1 {
		t.Fatalf("dashboard counts wrong: %+v", dash)
	}

	// The completed operation is listed and carries an audit trail.
	w = do(t, r, http.MethodGet, "/operations?status=completed")
	var listed struct {
		Operations []struct {
			ID                 string `json:"id"`
			VerificationStatus string `json:"verificationStatus"`
		} `json:"operations"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Operations) != 1 {
		t.Fatalf("want 1 completed operation, got %d", len(listed.Operations))
	}
	if listed.Operations[0].VerificationStatus != string(operations.VerificationPassed) {
		t.Fatalf("completed op must be verification-passed")
	}

	w = do(t, r, http.MethodGet, "/operations/"+listed.Operations[0].ID+"/events")
	if w.Code != http.StatusOK {
		t.Fatalf("events: status %d", w.Code)
	}
}

func TestAccountFeedsListed(t *testing.T) {
	r, _ := newTestServer(t)
	w := do(t, r, http.MethodGet, "/account-feeds")
	if w.Code != http.StatusOK {
		t.Fatalf("feeds: status %d", w.Code)
	}
	var got struct {
		Feeds []struct {
			Name string `json:"name"`
		} `json:"feeds"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Feeds) != 1 || got.Feeds[0].Name != "inbox" {
		t.Fatalf("expected the inbox feed to be listed, got %+v", got.Feeds)
	}
}

func TestRunRefusesNonSafeOperation(t *testing.T) {
	r, _ := newTestServer(t)
	// Ingest + classify so the high-risk op reaches awaiting_approval.
	do(t, r, http.MethodPost, "/background/run")

	w := do(t, r, http.MethodGet, "/operations?status=awaiting_approval")
	var listed struct {
		Operations []struct {
			ID string `json:"id"`
		} `json:"operations"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &listed)
	if len(listed.Operations) != 1 {
		t.Fatalf("want the high-risk op awaiting approval, got %d", len(listed.Operations))
	}
	// Attempting to run it must be refused (no real runtime in 2A).
	w = do(t, r, http.MethodPost, "/operations/"+listed.Operations[0].ID+"/run")
	if w.Code != http.StatusConflict {
		t.Fatalf("running a non-safe operation must be refused with 409, got %d", w.Code)
	}
}
