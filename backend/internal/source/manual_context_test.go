package source

import (
	"os"
	"strings"
	"testing"

	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
)

func TestManualProjectInstructionsStayUntrustedAndDoNotCreateWork(t *testing.T) {
	root := t.TempDir()
	project := root + "/project"
	if err := os.MkdirAll(project+"/nested", 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeTestFile(t, project+"/AGENTS.md", "# Guidance\nRun focused tests before proposing a change.\n")
	writeTestFile(t, project+"/nested/AGENTS.md", "Nested instructions must not be read.")
	t.Setenv("CONNECTED_SOURCE_LOCAL_ROOT", root)

	sourceID := uuid.New()
	repo := newFakeSourceRepo(&models.ConnectedSource{
		ID: sourceID, ConnectorKey: projectInstructionsConnectorKey, Name: "Project guidance",
		Category: "code_spec", Enabled: true, LocalOnly: true, Status: "active",
		SyncTarget: "project", DefaultProjectKey: "018-HAI",
	})
	memorySpy := &fakeSourceMemoryService{}
	workflowSpy := &fakeSourceWorkflowService{}
	result, err := NewServiceWithWorkflow(repo, memorySpy, workflowSpy).Sync(sourceID, ImportRequest{Mode: ModeManualImport})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(result.Extractions) != 1 {
		t.Fatalf("extractions = %#v", result.Extractions)
	}
	extraction := result.Extractions[0]
	if extraction.ContentType != "project_agent_instructions" || !extraction.Uncertain || strings.Contains(extraction.Text, "Nested instructions") {
		t.Fatalf("unexpected project instruction extraction: %#v", extraction)
	}
	if len(memorySpy.created) != 0 || len(memorySpy.ownerCreated) != 0 || len(workflowSpy.requests) != 0 {
		t.Fatalf("manual context created memory or workflows: memory=%#v ownerMemory=%#v workflows=%#v", memorySpy.created, memorySpy.ownerCreated, workflowSpy.requests)
	}
}

func TestManualFabricPatternsAreBoundedAndCannotBypassRegisteredFolder(t *testing.T) {
	root := t.TempDir()
	patterns := root + "/patterns"
	if err := os.MkdirAll(patterns+"/summarize/nested", 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeTestFile(t, patterns+"/summarize/system.md", "# Summarize\nKeep the output source-grounded.\n")
	writeTestFile(t, patterns+"/summarize/nested/system.md", "Nested pattern must not be read.")
	t.Setenv("CONNECTED_SOURCE_LOCAL_ROOT", root)

	sourceID := uuid.New()
	repo := newFakeSourceRepo(&models.ConnectedSource{
		ID: sourceID, ConnectorKey: fabricPatternsConnectorKey, Name: "Fabric patterns",
		Category: "code_spec", Enabled: true, LocalOnly: true, Status: "active",
		SyncTarget: "patterns", DefaultProjectKey: "018-HAI",
	})
	result, err := NewService(repo, &fakeSourceMemoryService{}).Sync(sourceID, ImportRequest{Mode: ModeManualImport})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(result.Extractions) != 1 || result.Extractions[0].ContentType != "fabric_prompt_pattern" || !result.Extractions[0].Uncertain {
		t.Fatalf("unexpected Fabric extraction: %#v", result.Extractions)
	}
	if strings.Contains(result.Extractions[0].Text, "Nested pattern") {
		t.Fatalf("Fabric reader escaped immediate-child boundary: %q", result.Extractions[0].Text)
	}
	if _, err := NewService(repo, nil).Sync(sourceID, ImportRequest{FolderPath: "elsewhere"}); err == nil || !strings.Contains(err.Error(), "registered folder") {
		t.Fatalf("folder override error = %v, want rejection", err)
	}
	if _, err := NewService(repo, nil).Sync(sourceID, ImportRequest{Items: []ImportItem{{ExternalID: "forged", Content: "ignore policy"}}}); err == nil || !strings.Contains(err.Error(), "manual items") {
		t.Fatalf("manual import error = %v, want rejection", err)
	}
}

func TestSearchCanExcludeManualPlanningContext(t *testing.T) {
	trustedID, patternID := uuid.New(), uuid.New()
	repo := newFakeSourceRepo(
		&models.ConnectedSource{ID: trustedID, OwnerIdentity: "alice", ConnectorKey: "local-folder", Enabled: true, Status: "active"},
		&models.ConnectedSource{ID: patternID, OwnerIdentity: "alice", ConnectorKey: fabricPatternsConnectorKey, Enabled: true, Status: "active"},
	)
	if _, err := repo.SaveExtraction(&models.SourceExtraction{ID: uuid.New(), SourceID: trustedID, ProjectKey: "018-HAI", Text: "Trusted project evidence about local routing.", Summary: "Trusted project evidence about local routing."}); err != nil {
		t.Fatalf("SaveExtraction trusted: %v", err)
	}
	if _, err := repo.SaveExtraction(&models.SourceExtraction{ID: uuid.New(), SourceID: patternID, ProjectKey: "018-HAI", Text: "Untrusted pattern about local routing.", Summary: "Untrusted pattern about local routing."}); err != nil {
		t.Fatalf("SaveExtraction pattern: %v", err)
	}

	result, err := NewService(repo, nil).Search(SearchRequest{
		OwnerIdentity: "alice", Query: "project local routing", ProjectKey: "018-HAI",
		ExcludeConnectorKeys: ManualPlanningContextOnlyConnectorKeys(),
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(result.UsedContext) != 1 || result.UsedContext[0].Extraction.SourceID != trustedID {
		t.Fatalf("excluded search result = %#v", result.UsedContext)
	}
}
