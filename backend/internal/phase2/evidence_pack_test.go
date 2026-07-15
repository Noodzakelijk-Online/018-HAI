package phase2

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/operations"
	"automation-hub-backend/internal/privacyfilter"
)

func TestEvidencePackGenerationAndRetrieval(t *testing.T) {
	r, _ := newTestServer(t)
	// Run a pass so there is a completed operation with full provenance.
	do(t, r, http.MethodPost, "/background/run")
	w := do(t, r, http.MethodGet, "/operations?status=completed")
	var listed struct {
		Operations []struct {
			ID string `json:"id"`
		} `json:"operations"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &listed)
	if len(listed.Operations) == 0 {
		t.Fatalf("need a completed operation to build an evidence pack")
	}
	id := listed.Operations[0].ID

	gen := do(t, r, http.MethodPost, "/operations/"+id+"/evidence-pack")
	if gen.Code != http.StatusCreated {
		t.Fatalf("generate: status %d body %s", gen.Code, gen.Body.String())
	}
	var pack struct {
		ID       string `json:"id"`
		Markdown string `json:"markdown"`
	}
	_ = json.Unmarshal(gen.Body.Bytes(), &pack)
	if pack.ID == "" {
		t.Fatalf("evidence pack must get an id")
	}
	// Must contain the mandatory §10.18 sections.
	for _, section := range []string{
		"# Evidence Pack:", "## Operation", "## Source Evidence", "## Privacy Scan",
		"## Policy Decision", "## Approval", "## Runtime / Model Attempts",
		"## Verification", "## Audit Timeline", "## Known Limits",
	} {
		if !strings.Contains(pack.Markdown, section) {
			t.Fatalf("evidence pack missing section %q", section)
		}
	}
	// It must reference the source revision hash and be verification-passed.
	if !strings.Contains(pack.Markdown, "sourceRevisionHash:") {
		t.Fatalf("evidence pack must include the source revision hash")
	}

	// Retrieval by id works.
	got := do(t, r, http.MethodGet, "/evidence-packs/"+pack.ID)
	if got.Code != http.StatusOK {
		t.Fatalf("get evidence pack: status %d", got.Code)
	}
}

func TestEvidencePackRedactsSecrets(t *testing.T) {
	// A pack built directly over an operation whose content carries a secret must
	// never embed the raw secret — only the redacted preview.
	r, m := newTestServer(t)
	do(t, r, http.MethodPost, "/background/run")
	ops, _ := m.Service().List(operationsFilterAll())
	if len(ops) == 0 {
		t.Fatalf("need an operation")
	}
	op := ops[0]
	op.Description = "api_key = sk-live-SECRETVALUE123456 do not leak"
	saved, _ := m.Service().Save(op, "test", "hai", "inject secret")
	events, _ := m.Service().Events(saved.ID)
	scan := privacyfilter.Scan(saved.Title+"\n"+saved.Description, 280)
	pack := buildEvidencePack(*saved, events, scan, nil, time.Now().UTC())
	if strings.Contains(pack.Markdown, "sk-live-SECRETVALUE123456") {
		t.Fatalf("evidence pack must not embed the raw secret")
	}
	if !strings.Contains(pack.Markdown, "REDACTED") {
		t.Fatalf("evidence pack privacy scan must show redaction")
	}
}

func operationsFilterAll() operations.Filter {
	return operations.Filter{OwnerUserID: "local-operator", WorkspaceID: "local", Limit: 50}
}
