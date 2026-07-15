package phase2

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"automation-hub-backend/internal/operations"

	"github.com/gin-gonic/gin"
)

// awaitingApprovalID runs a background pass over the two-item feed and returns
// the id of the high-risk operation that landed in awaiting_approval.
func awaitingApprovalID(t *testing.T, r *gin.Engine) string {
	t.Helper()
	do(t, r, http.MethodPost, "/background/run")
	w := do(t, r, http.MethodGet, "/operations?status=awaiting_approval")
	var listed struct {
		Operations []struct {
			ID string `json:"id"`
		} `json:"operations"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &listed)
	if len(listed.Operations) != 1 {
		t.Fatalf("expected one awaiting-approval op, got %d", len(listed.Operations))
	}
	return listed.Operations[0].ID
}

func TestRejectDismissesOperation(t *testing.T) {
	r, _ := newTestServer(t)
	id := awaitingApprovalID(t, r)
	w := do(t, r, http.MethodPost, "/operations/"+id+"/reject")
	if w.Code != http.StatusOK {
		t.Fatalf("reject: status %d body %s", w.Code, w.Body.String())
	}
	got := do(t, r, http.MethodGet, "/operations/"+id)
	var op struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(got.Body.Bytes(), &op)
	if op.Status != string(operations.StatusDismissed) {
		t.Fatalf("rejected op must be dismissed, got %q", op.Status)
	}
}

func TestLaterPostponesOperation(t *testing.T) {
	r, _ := newTestServer(t)
	id := awaitingApprovalID(t, r)
	w := do(t, r, http.MethodPost, "/operations/"+id+"/later")
	if w.Code != http.StatusOK {
		t.Fatalf("later: status %d body %s", w.Code, w.Body.String())
	}
	var op struct {
		Status       string  `json:"status"`
		NextReviewAt *string `json:"nextReviewAt"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &op)
	if op.Status != string(operations.StatusAwaitingApproval) {
		t.Fatalf("postponed op must remain awaiting_approval, got %q", op.Status)
	}
	if op.NextReviewAt == nil {
		t.Fatalf("postponed op must have a next review time")
	}
}

func TestBlockSimilarBlocksAndAutoBlocksFuture(t *testing.T) {
	r, m := newTestServer(t)
	id := awaitingApprovalID(t, r)
	w := do(t, r, http.MethodPost, "/operations/"+id+"/block-similar")
	if w.Code != http.StatusOK {
		t.Fatalf("block-similar: status %d body %s", w.Code, w.Body.String())
	}
	got := do(t, r, http.MethodGet, "/operations/"+id)
	var op struct {
		Status        string `json:"status"`
		OperationType string `json:"operationType"`
	}
	_ = json.Unmarshal(got.Body.Bytes(), &op)
	if op.Status != string(operations.StatusBlocked) {
		t.Fatalf("block-similar op must be blocked, got %q", op.Status)
	}
	// A future operation of the same type must be auto-blocked by the rule.
	blocked, reason := m.blockRules.ShouldBlock(op.OperationType, "another payment task")
	if !blocked || reason == "" {
		t.Fatalf("block rule must auto-block future ops of the same type")
	}
	// Drive it through the worker: ingest a matching op and confirm it blocks.
	in := operations.NewOperationInput{
		OwnerUserID: "local-operator", WorkspaceID: "local", Title: "Pay another landlord invoice",
		Description: "pay the rent invoice", OperationType: op.OperationType, SourceType: "test", DedupeKey: "blk-1",
	}
	if _, err := m.Service().Ingest(in); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Worker().WithBlockRules(m.blockRules).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	blockedList, _ := m.Service().List(operations.Filter{OwnerUserID: "local-operator", WorkspaceID: "local", Status: operations.StatusBlocked})
	if len(blockedList) < 2 {
		t.Fatalf("the matching future operation must be auto-blocked, got %d blocked", len(blockedList))
	}
}

func TestApprovalsListsProvenance(t *testing.T) {
	r, _ := newTestServer(t)
	id := awaitingApprovalID(t, r)
	do(t, r, http.MethodPost, "/operations/"+id+"/approve")
	w := do(t, r, http.MethodGet, "/operations/"+id+"/approvals")
	if w.Code != http.StatusOK {
		t.Fatalf("approvals: status %d", w.Code)
	}
	var res struct {
		Approvals []map[string]any `json:"approvals"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if len(res.Approvals) < 2 { // awaiting_approval + approved
		t.Fatalf("approvals must include the awaiting + approved provenance, got %d", len(res.Approvals))
	}
}
