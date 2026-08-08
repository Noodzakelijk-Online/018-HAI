package frameworkevidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"automation-hub-backend/internal/sourceevidence"
)

type persistedAssertion struct {
	RequirementID       string                 `json:"requirementId"`
	FrameworkID         string                 `json:"frameworkId"`
	Requirement         string                 `json:"requirement"`
	Phase               string                 `json:"phase"`
	Validator           string                 `json:"validator"`
	Status              string                 `json:"status"`
	Evidence            []string               `json:"evidence"`
	ApplicabilityReason string                 `json:"applicabilityReason,omitempty"`
	Failure             string                 `json:"failure,omitempty"`
	SourceClaims        []sourceevidence.Claim `json:"sourceClaims,omitempty"`
}

type digestAssertion struct {
	RequirementID       string                 `json:"requirementId"`
	FrameworkID         string                 `json:"frameworkId"`
	Phase               string                 `json:"phase"`
	Validator           string                 `json:"validator"`
	Status              string                 `json:"status"`
	Evidence            []string               `json:"evidence"`
	ApplicabilityReason string                 `json:"applicabilityReason,omitempty"`
	Failure             string                 `json:"failure,omitempty"`
	SourceClaims        []sourceevidence.Claim `json:"sourceClaims,omitempty"`
}

// PreflightDigest recomputes the canonical selector-v5 preflight digest from
// the durable assertion payload. It intentionally derives counters and
// failures rather than trusting duplicated caller-supplied summary fields.
func PreflightDigest(
	ownerIdentity string,
	taskPlanID string,
	frameworkSelectionID string,
	evaluatedAt time.Time,
	assertionsJSON json.RawMessage,
) (string, error) {
	var persisted []persistedAssertion
	if len(assertionsJSON) == 0 || json.Unmarshal(assertionsJSON, &persisted) != nil {
		return "", fmt.Errorf("%w: assertions JSON must be an array", ErrInvalidRecord)
	}
	assertions := make([]digestAssertion, 0, len(persisted))
	checked := 0
	verified := 0
	missing := 0
	failures := []string{}
	for index := range persisted {
		value := &persisted[index]
		value.Evidence = sortedStrings(value.Evidence)
		value.SourceClaims = canonicalSourceClaims(value.SourceClaims)
		switch strings.TrimSpace(value.Status) {
		case "verified":
			checked++
			verified++
		case "missing":
			checked++
			missing++
		case "not_applicable":
		default:
			return "", fmt.Errorf("%w: assertion status is invalid", ErrInvalidRecord)
		}
		if failure := strings.TrimSpace(value.Failure); failure != "" && value.Status == "missing" {
			failures = append(
				failures,
				strings.TrimSpace(value.FrameworkID)+": "+strings.TrimSpace(value.Requirement)+" ("+failure+")",
			)
		}
		assertions = append(assertions, digestAssertion{
			RequirementID: value.RequirementID, FrameworkID: value.FrameworkID,
			Phase: value.Phase, Validator: value.Validator, Status: value.Status,
			Evidence: value.Evidence, ApplicabilityReason: value.ApplicabilityReason,
			Failure: value.Failure, SourceClaims: value.SourceClaims,
		})
	}
	sort.Slice(assertions, func(i, j int) bool {
		return assertions[i].RequirementID < assertions[j].RequirementID
	})
	failures = uniqueSortedTrimmedStrings(failures)
	passed := missing == 0
	status := "blocked"
	if passed {
		status = StatusPassed
	}
	payload := struct {
		Version              string            `json:"version"`
		OwnerIdentity        string            `json:"ownerIdentity"`
		TaskPlanID           string            `json:"taskPlanId"`
		FrameworkSelectionID string            `json:"frameworkSelectionId"`
		Passed               bool              `json:"passed"`
		Status               string            `json:"status"`
		Checked              int               `json:"checked"`
		Verified             int               `json:"verified"`
		Missing              int               `json:"missing"`
		Assertions           []digestAssertion `json:"assertions"`
		Failures             []string          `json:"failures"`
		EvaluatedAt          time.Time         `json:"evaluatedAt"`
	}{
		Version:              "framework-evidence-preflight-v1",
		OwnerIdentity:        strings.TrimSpace(ownerIdentity),
		TaskPlanID:           strings.TrimSpace(taskPlanID),
		FrameworkSelectionID: strings.TrimSpace(frameworkSelectionID),
		Passed:               passed,
		Status:               status,
		Checked:              checked,
		Verified:             verified,
		Missing:              missing,
		Assertions:           assertions,
		Failures:             failures,
		EvaluatedAt:          evaluatedAt.UTC(),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("%w: encode canonical preflight", ErrInvalidRecord)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func sortedStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

func uniqueSortedTrimmedStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func canonicalSourceClaims(values []sourceevidence.Claim) []sourceevidence.Claim {
	result := append([]sourceevidence.Claim(nil), values...)
	sort.Slice(result, func(i, j int) bool {
		if result[i].RequirementID != result[j].RequirementID {
			return result[i].RequirementID < result[j].RequirementID
		}
		if result[i].ExtractionID != result[j].ExtractionID {
			return result[i].ExtractionID < result[j].ExtractionID
		}
		return false
	})
	return result
}
