package task

import "strings"

const (
	MinimalityNotApplicable      = "not_applicable"
	MinimalityRejectUnneeded     = "reject_unneeded"
	MinimalityStandardLibrary    = "standard_library"
	MinimalityPlatformNative     = "platform_native"
	MinimalityExistingDependency = "existing_dependency"
	MinimalitySmallPatch         = "small_patch"
	MinimalityMinimalCustom      = "minimal_custom"
)

type MinimalityGate struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Status   string `json:"status"`
	Evidence string `json:"evidence"`
}

type MinimalityDecision struct {
	Applicable                bool             `json:"applicable"`
	Necessary                 bool             `json:"necessary"`
	SelectedLevel             string           `json:"selectedLevel"`
	SelectedStrategy          string           `json:"selectedStrategy"`
	Reason                    string           `json:"reason"`
	Ladder                    []MinimalityGate `json:"ladder"`
	NewDependenciesAllowed    bool             `json:"newDependenciesAllowed"`
	CustomArchitectureAllowed bool             `json:"customArchitectureAllowed"`
	RequiresRepositoryCheck   bool             `json:"requiresRepositoryCheck"`
	BenchmarkClaimsStatus     string           `json:"benchmarkClaimsStatus"`
}

func decideMinimality(request IntakeRequest, intake IntakeAnalysis) MinimalityDecision {
	if intake.TaskType != "coding" && intake.TaskType != "architecture" {
		return MinimalityDecision{
			Applicable:            false,
			Necessary:             true,
			SelectedLevel:         MinimalityNotApplicable,
			SelectedStrategy:      "use the normal completion-first task policy",
			Reason:                "the YAGNI coding ladder applies only to coding and architecture work",
			BenchmarkClaimsStatus: "Ponytail savings claims are recorded as unverified",
		}
	}

	text := strings.ToLower(strings.TrimSpace(request.Request))
	necessary := !containsAny(text, "hypothetical only", "do not implement", "brainstorm only", "maybe someday", "speculative idea")
	selected := MinimalitySmallPatch
	strategy := "inspect the existing code and deliver the smallest verified patch"
	reason := "coding work defaults to a small change in the current architecture"

	switch {
	case !necessary:
		selected = MinimalityRejectUnneeded
		strategy = "do not create implementation code; retain the result as analysis only"
		reason = "the request explicitly describes non-implementation or speculative work"
	case intake.TaskType == "architecture" && containsAny(text, "new service", "new module", "new runtime", "new adapter", "new connector"):
		selected = MinimalityMinimalCustom
		strategy = "allow a narrowly scoped custom module only after documenting why earlier ladder rungs are insufficient"
		reason = "the request explicitly requires a new ownership boundary or adapter"
	case containsWordOrPhrase(text, "json", "parse", "parser", "http", "url", "path", "file", "hash", "time", "date", "validation", "logging", "regex"):
		selected = MinimalityStandardLibrary
		strategy = "attempt the language standard library before adding helpers or dependencies"
		reason = "the requested capability is commonly available in standard libraries"
	case containsAny(text, "browser", "windows", "docker", "compose", "postgres", "github actions", "calendar", "filesystem", "operating system"):
		selected = MinimalityPlatformNative
		strategy = "use the existing platform or operating-system capability before custom infrastructure"
		reason = "the task maps to a native platform capability already present in HAI's environment"
	case containsAny(text, "angular", "gin", "gorm", "ng-zorro", "redis", "kafka", "ollama", "existing dependency", "existing library"):
		selected = MinimalityExistingDependency
		strategy = "reuse the project's installed framework or dependency before introducing another package"
		reason = "the request names or strongly implies an existing project dependency"
	}

	ladderOrder := []struct {
		key, label, level string
	}{
		{"necessity", "Does this need to exist?", MinimalityRejectUnneeded},
		{"standard_library", "Can the standard library do it?", MinimalityStandardLibrary},
		{"platform_native", "Can the platform, browser, or OS do it?", MinimalityPlatformNative},
		{"existing_dependency", "Can an existing dependency do it?", MinimalityExistingDependency},
		{"small_patch", "Can it be a one-line or small patch?", MinimalitySmallPatch},
		{"minimal_custom", "Is minimal custom code justified?", MinimalityMinimalCustom},
	}
	ladder := make([]MinimalityGate, 0, len(ladderOrder))
	selectedIndex := minimalityIndex(selected)
	for index, gate := range ladderOrder {
		status := "skipped"
		evidence := "not selected by the deterministic intake gate"
		if gate.key == "necessity" {
			if necessary {
				status = "passed"
				evidence = "the user requested an implementation and did not mark it speculative"
			} else {
				status = "rejected"
				evidence = reason
			}
		} else if necessary && index == selectedIndex {
			status = "selected"
			evidence = reason
		} else if necessary && index < selectedIndex {
			status = "inspect_first"
			evidence = "the executor must verify this cheaper rung is insufficient before moving lower"
		} else if necessary && index > selectedIndex {
			status = "blocked_by_default"
			evidence = "a lower-complexity rung was selected"
		}
		ladder = append(ladder, MinimalityGate{
			Key: gate.key, Label: gate.label, Status: status, Evidence: evidence,
		})
	}

	return MinimalityDecision{
		Applicable:                true,
		Necessary:                 necessary,
		SelectedLevel:             selected,
		SelectedStrategy:          strategy,
		Reason:                    reason,
		Ladder:                    ladder,
		NewDependenciesAllowed:    false,
		CustomArchitectureAllowed: selected == MinimalityMinimalCustom,
		RequiresRepositoryCheck:   necessary,
		BenchmarkClaimsStatus:     "Ponytail code and cost reductions are vendor/public claims requiring local replication",
	}
}

func minimalityIndex(level string) int {
	switch level {
	case MinimalityStandardLibrary:
		return 1
	case MinimalityPlatformNative:
		return 2
	case MinimalityExistingDependency:
		return 3
	case MinimalitySmallPatch:
		return 4
	case MinimalityMinimalCustom:
		return 5
	default:
		return 0
	}
}

func minimalitySystemContract(decision MinimalityDecision) string {
	if !decision.Applicable {
		return ""
	}
	return " Apply the YAGNI decision ladder. Selected strategy: " + decision.SelectedStrategy +
		". Do not add a dependency unless repository inspection proves the standard library, native platform, existing dependencies, and a small patch are insufficient." +
		" Every new file, dependency, or abstraction must map to an explicit success criterion."
}
