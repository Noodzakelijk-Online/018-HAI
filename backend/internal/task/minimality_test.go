package task

import "testing"

func TestDecideMinimalityPrefersStandardLibrary(t *testing.T) {
	decision := decideMinimality(
		IntakeRequest{Request: "Parse JSON and validate URLs in Go."},
		IntakeAnalysis{TaskType: "coding"},
	)
	if decision.SelectedLevel != MinimalityStandardLibrary {
		t.Fatalf("selected level = %q, want %q", decision.SelectedLevel, MinimalityStandardLibrary)
	}
	if decision.NewDependenciesAllowed {
		t.Fatal("new dependencies should be blocked by default")
	}
}

func TestDecideMinimalityUsesExistingDependency(t *testing.T) {
	decision := decideMinimality(
		IntakeRequest{Request: "Add an Angular ng-zorro status table."},
		IntakeAnalysis{TaskType: "coding"},
	)
	if decision.SelectedLevel != MinimalityExistingDependency {
		t.Fatalf("selected level = %q, want %q", decision.SelectedLevel, MinimalityExistingDependency)
	}
}

func TestDecideMinimalityRejectsSpeculativeImplementation(t *testing.T) {
	decision := decideMinimality(
		IntakeRequest{Request: "Brainstorm only, do not implement this speculative idea."},
		IntakeAnalysis{TaskType: "architecture"},
	)
	if decision.Necessary || decision.SelectedLevel != MinimalityRejectUnneeded {
		t.Fatalf("unexpected decision: %+v", decision)
	}
}

func TestDecideMinimalityAllowsNarrowCustomBoundary(t *testing.T) {
	decision := decideMinimality(
		IntakeRequest{Request: "Create a new adapter for a new runtime."},
		IntakeAnalysis{TaskType: "architecture"},
	)
	if decision.SelectedLevel != MinimalityMinimalCustom || !decision.CustomArchitectureAllowed {
		t.Fatalf("unexpected decision: %+v", decision)
	}
}

func TestAnalyzeIntakeRecognizesSoftwareVocabulary(t *testing.T) {
	coding := analyzeIntake(IntakeRequest{Request: "Parse JSON and validate URLs in Go."})
	if coding.TaskType != "coding" {
		t.Fatalf("task type = %q, want coding", coding.TaskType)
	}
	architecture := analyzeIntake(IntakeRequest{Request: "Create a new adapter for a new runtime."})
	if architecture.TaskType != "architecture" {
		t.Fatalf("task type = %q, want architecture", architecture.TaskType)
	}
}
