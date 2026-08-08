package task

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"automation-hub-backend/internal/executionauth"
	"automation-hub-backend/internal/frameworkregistry"
)

// executionGovernanceEvidence converts server-generated planning records into
// an immutable authorization receipt binding. It deliberately carries no
// authority: executionauth independently evaluates the Constitution, mandate,
// agent assignment, approval, and emergency stop.
func executionGovernanceEvidence(plan *CompletionPlan) (executionauth.GovernanceEvidence, error) {
	if plan == nil || strings.TrimSpace(plan.ID) == "" {
		return executionauth.GovernanceEvidence{}, fmt.Errorf("task plan governance evidence requires a plan id")
	}
	if plan.FrameworkDecision == nil {
		return executionauth.GovernanceEvidence{}, fmt.Errorf("task plan governance evidence requires a framework selection")
	}
	framework := plan.FrameworkDecision
	if err := validateTaskFrameworkRiskContract(framework); err != nil {
		return executionauth.GovernanceEvidence{}, err
	}
	preflightDigest := ""
	if plan.FrameworkEvidencePreflight != nil {
		if !plan.FrameworkEvidencePreflight.Passed {
			return executionauth.GovernanceEvidence{}, fmt.Errorf("task plan governance evidence requires a passing framework evidence preflight")
		}
		preflightDigest = bareSHA256Digest(plan.FrameworkEvidencePreflight.Digest)
		if preflightDigest == "" {
			return executionauth.GovernanceEvidence{}, fmt.Errorf("task plan governance evidence requires a framework evidence preflight digest")
		}
	} else if frameworkEvidencePreflightRequired(plan.ValidationPlan.FrameworkEvidenceContracts) {
		return executionauth.GovernanceEvidence{}, fmt.Errorf("task plan governance evidence requires framework evidence preconditions")
	}
	var frameworkMaximumAutonomy *int
	var frameworkRequiresApproval *bool
	if strings.TrimSpace(framework.SelectorAlgorithmVersion) == "selector-v5" {
		maximumAutonomy := framework.MaximumAutonomyLevel
		requiresApproval := framework.RequiresApproval
		frameworkMaximumAutonomy = &maximumAutonomy
		frameworkRequiresApproval = &requiresApproval
	}
	governance := executionauth.GovernanceEvidence{
		TaskPlanID:                        strings.TrimSpace(plan.ID),
		FrameworkEvidencePreflightDigest:  preflightDigest,
		FrameworkSelectionID:              strings.TrimSpace(framework.ID),
		FrameworkCatalogVersion:           strings.TrimSpace(framework.CatalogVersion),
		FrameworkSelectorAlgorithmVersion: strings.TrimSpace(framework.SelectorAlgorithmVersion),
		FrameworkTaskRiskLevel:            executionauth.RiskLevel(strings.ToLower(strings.TrimSpace(framework.TaskRiskLevel))),
		FrameworkEffectiveRiskCeiling:     executionauth.RiskLevel(strings.ToLower(strings.TrimSpace(framework.EffectiveRiskCeiling))),
		FrameworkMaximumAutonomyLevel:     frameworkMaximumAutonomy,
		FrameworkRequiresApproval:         frameworkRequiresApproval,
		FrameworkCatalogDigest:            bareSHA256Digest(framework.CatalogDigest),
		FrameworkPreferenceDigest:         bareSHA256Digest(framework.EffectivePreferenceDigest),
		FrameworkConstitutionDigest:       bareSHA256Digest(framework.ConstitutionDigest),
		FrameworkOperatingContractDigest:  bareSHA256Digest(framework.OperatingContractDigest),
		EvidenceReferences:                taskEvidenceReferences(plan),
	}
	if plan.DomainPackDecision != nil {
		domain := plan.DomainPackDecision
		if !domain.AdvisoryOnly || domain.ExecutionAuthorityGranted {
			return executionauth.GovernanceEvidence{}, fmt.Errorf("domain pack governance crossed its advisory-only boundary")
		}
		governance.DomainPackDecisionID = strings.TrimSpace(domain.ID)
		governance.DomainPackCatalogVersion = strings.TrimSpace(domain.CatalogVersion)
		governance.DomainPackCatalogDigest = bareSHA256Digest(domain.CatalogDigest)
		governance.DomainPackDecisionDigest = bareSHA256Digest(domain.Digest)
	}
	if plan.ResourceDecision == nil {
		return executionauth.GovernanceEvidence{}, fmt.Errorf("task plan governance evidence requires a resource decision")
	}
	if plan.ResourceDecision.Authority != "advisory_only" || plan.ResourceDecision.CanExecute || plan.ResourceDecision.GrantsAuthority {
		return executionauth.GovernanceEvidence{}, fmt.Errorf("resource governance crossed its advisory-only boundary")
	}
	governance.ResourceDecisionDigest = bareSHA256Digest(plan.ResourceDecision.DecisionDigest)
	governance.ResourceFeasibility = string(plan.ResourceDecision.Feasibility)

	payload := struct {
		PlanID                            string   `json:"planId"`
		FrameworkEvidencePreflightDigest  string   `json:"frameworkEvidencePreflightDigest,omitempty"`
		OwnerIdentity                     string   `json:"ownerIdentity"`
		Request                           string   `json:"request"`
		ProjectKey                        string   `json:"projectKey"`
		RealGoal                          string   `json:"realGoal"`
		RiskLevel                         string   `json:"riskLevel"`
		ApprovalRequired                  bool     `json:"approvalRequired"`
		ApprovalGranted                   bool     `json:"approvalGranted"`
		AllowedNow                        bool     `json:"allowedNow"`
		ControlledExecutionMode           string   `json:"controlledExecutionMode"`
		FrameworkSelectionID              string   `json:"frameworkSelectionId"`
		FrameworkCatalogVersion           string   `json:"frameworkCatalogVersion"`
		FrameworkSelectorAlgorithmVersion string   `json:"frameworkSelectorAlgorithmVersion"`
		FrameworkTaskRiskLevel            string   `json:"frameworkTaskRiskLevel"`
		FrameworkEffectiveRiskCeiling     string   `json:"frameworkEffectiveRiskCeiling"`
		FrameworkMaximumAutonomyLevel     *int     `json:"frameworkMaximumAutonomyLevel,omitempty"`
		FrameworkRequiresApproval         *bool    `json:"frameworkRequiresApproval,omitempty"`
		FrameworkCatalogDigest            string   `json:"frameworkCatalogDigest"`
		FrameworkPreferenceDigest         string   `json:"frameworkPreferenceDigest"`
		FrameworkConstitutionDigest       string   `json:"frameworkConstitutionDigest"`
		FrameworkOperatingContractDigest  string   `json:"frameworkOperatingContractDigest"`
		DomainPackDecisionID              string   `json:"domainPackDecisionId"`
		DomainPackCatalogVersion          string   `json:"domainPackCatalogVersion"`
		DomainPackCatalogDigest           string   `json:"domainPackCatalogDigest"`
		DomainPackDecisionDigest          string   `json:"domainPackDecisionDigest"`
		ResourceDecisionDigest            string   `json:"resourceDecisionDigest"`
		ResourceFeasibility               string   `json:"resourceFeasibility"`
		EvidenceReferences                []string `json:"evidenceReferences"`
		CoordinationPlanID                string   `json:"coordinationPlanId,omitempty"`
		CoordinationPlanRevision          uint64   `json:"coordinationPlanRevision,omitempty"`
		CoordinationPlanDigest            string   `json:"coordinationPlanDigest,omitempty"`
		CoordinationPlanNodeID            string   `json:"coordinationPlanNodeId,omitempty"`
	}{
		PlanID:                            governance.TaskPlanID,
		FrameworkEvidencePreflightDigest:  governance.FrameworkEvidencePreflightDigest,
		OwnerIdentity:                     strings.TrimSpace(plan.OwnerIdentity),
		Request:                           strings.TrimSpace(plan.Request),
		ProjectKey:                        strings.TrimSpace(plan.ProjectKey),
		RealGoal:                          strings.TrimSpace(plan.RealGoal),
		RiskLevel:                         strings.TrimSpace(plan.RiskAssessment.Level),
		ApprovalRequired:                  plan.RiskAssessment.ApprovalRequired,
		ApprovalGranted:                   plan.RiskAssessment.ApprovalGranted,
		AllowedNow:                        plan.RiskAssessment.AllowedNow,
		ControlledExecutionMode:           strings.TrimSpace(plan.ExecutionPlan.ControlledExecutionMode),
		FrameworkSelectionID:              governance.FrameworkSelectionID,
		FrameworkCatalogVersion:           governance.FrameworkCatalogVersion,
		FrameworkSelectorAlgorithmVersion: governance.FrameworkSelectorAlgorithmVersion,
		FrameworkTaskRiskLevel:            string(governance.FrameworkTaskRiskLevel),
		FrameworkEffectiveRiskCeiling:     string(governance.FrameworkEffectiveRiskCeiling),
		FrameworkMaximumAutonomyLevel:     frameworkMaximumAutonomy,
		FrameworkRequiresApproval:         frameworkRequiresApproval,
		FrameworkCatalogDigest:            governance.FrameworkCatalogDigest,
		FrameworkPreferenceDigest:         governance.FrameworkPreferenceDigest,
		FrameworkConstitutionDigest:       governance.FrameworkConstitutionDigest,
		FrameworkOperatingContractDigest:  governance.FrameworkOperatingContractDigest,
		DomainPackDecisionID:              governance.DomainPackDecisionID,
		DomainPackCatalogVersion:          governance.DomainPackCatalogVersion,
		DomainPackCatalogDigest:           governance.DomainPackCatalogDigest,
		DomainPackDecisionDigest:          governance.DomainPackDecisionDigest,
		ResourceDecisionDigest:            governance.ResourceDecisionDigest,
		ResourceFeasibility:               governance.ResourceFeasibility,
		EvidenceReferences:                governance.EvidenceReferences,
	}
	if plan.CoordinationPlan != nil {
		if plan.CoordinationPlan.CanExecute {
			return executionauth.GovernanceEvidence{}, fmt.Errorf("coordination plan governance crossed its advisory-only boundary")
		}
		payload.CoordinationPlanID = plan.CoordinationPlan.PlanID.String()
		payload.CoordinationPlanRevision = plan.CoordinationPlan.Revision
		payload.CoordinationPlanDigest = bareSHA256Digest(plan.CoordinationPlan.Digest)
		payload.CoordinationPlanNodeID = strings.TrimSpace(plan.CoordinationPlan.NodeID)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return executionauth.GovernanceEvidence{}, fmt.Errorf("encode task governance evidence: %w", err)
	}
	digest := sha256.Sum256(encoded)
	governance.TaskPlanDigest = hex.EncodeToString(digest[:])
	return governance, nil
}

func validateTaskFrameworkRiskContract(framework *frameworkregistry.SelectionDecision) error {
	version := strings.TrimSpace(framework.SelectorAlgorithmVersion)
	taskRisk := strings.ToLower(strings.TrimSpace(framework.TaskRiskLevel))
	ceiling := strings.ToLower(strings.TrimSpace(framework.EffectiveRiskCeiling))
	if version == "" || version == "selector-v4" {
		if taskRisk != "" || ceiling != "" {
			return fmt.Errorf("legacy framework selection cannot assert a v5 risk contract")
		}
		return nil
	}
	if version != "selector-v5" {
		return fmt.Errorf("framework selector algorithm version %q is unsupported for execution", version)
	}
	if taskRisk == "" || ceiling == "" {
		return fmt.Errorf("selector-v5 framework selection requires task risk and effective risk ceiling")
	}
	if framework.MaximumAutonomyLevel < 0 || framework.MaximumAutonomyLevel > 10 {
		return fmt.Errorf("selector-v5 framework selection contains an invalid autonomy ceiling")
	}
	taskRank, taskOK := taskFrameworkRiskRank(taskRisk)
	ceilingRank, ceilingOK := taskFrameworkRiskRank(ceiling)
	if !taskOK || !ceilingOK {
		return fmt.Errorf("selector-v5 framework selection contains an invalid risk contract")
	}
	if taskRank > ceilingRank {
		return fmt.Errorf(
			"framework task risk %q exceeds effective risk ceiling %q",
			taskRisk,
			ceiling,
		)
	}
	return nil
}

func taskFrameworkRiskRank(value string) (int, bool) {
	switch value {
	case "low":
		return 1, true
	case "medium":
		return 2, true
	case "high":
		return 3, true
	default:
		return 0, false
	}
}

func taskEvidenceReferences(plan *CompletionPlan) []string {
	values := []string{
		"task-plan://" + strings.TrimSpace(plan.ID),
		"framework-selection://" + strings.TrimSpace(plan.FrameworkDecision.ID),
	}
	if plan.CoordinationPlan != nil {
		values = append(values, fmt.Sprintf(
			"plan-graph://%s/revisions/%d/nodes/%s#sha256:%s",
			plan.CoordinationPlan.PlanID,
			plan.CoordinationPlan.Revision,
			strings.TrimSpace(plan.CoordinationPlan.NodeID),
			bareSHA256Digest(plan.CoordinationPlan.Digest),
		))
	}
	if plan.FrameworkEvidencePreflight != nil && strings.TrimSpace(plan.FrameworkEvidencePreflight.Digest) != "" {
		values = append(values, "framework-evidence-preflight://sha256:"+bareSHA256Digest(plan.FrameworkEvidencePreflight.Digest))
	}
	for _, ranked := range plan.ContextPlan.SourceContext {
		extraction := ranked.Extraction
		if extraction.ID.String() == "00000000-0000-0000-0000-000000000000" {
			continue
		}
		reference := "source-extraction://" + extraction.ID.String()
		if digest := bareSHA256Digest(extraction.ContentHash); digest != "" {
			reference += "#sha256:" + digest
		}
		values = append(values, reference)
	}
	for _, ranked := range plan.ContextPlan.UsedContext {
		memory := ranked.Memory
		if memory.ID.String() == "00000000-0000-0000-0000-000000000000" {
			continue
		}
		reference := "memory://" + memory.ID.String()
		if digest := bareSHA256Digest(memory.ContentHash); digest != "" {
			reference += "#sha256:" + digest
		}
		values = append(values, reference)
	}
	if plan.DomainPackDecision != nil {
		values = append(values, "domain-pack-decision://"+strings.TrimSpace(plan.DomainPackDecision.ID))
	}
	if plan.ResourceDecision != nil {
		values = append(values, "resource-decision://"+strings.TrimSpace(plan.ID)+"#sha256:"+bareSHA256Digest(plan.ResourceDecision.DecisionDigest))
	}
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
	if len(result) > 32 {
		result = result[:32]
	}
	return result
}

func bareSHA256Digest(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.TrimPrefix(value, "sha256:")
}
