package executionauth

import (
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"automation-hub-backend/internal/safety"
)

var boundedIdentifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@+-]*$`)

const (
	frameworkSelectorV4 = "selector-v4"
	frameworkSelectorV5 = "selector-v5"
)

func normalizeRequest(request Request) (Request, error) {
	request.OwnerIdentity = compact(request.OwnerIdentity)
	request.IdempotencyKey = compact(request.IdempotencyKey)
	request.ActorIdentity = compact(request.ActorIdentity)
	request.TaskID = compact(request.TaskID)
	request.Action = compact(request.Action)
	request.ResourceType = compact(request.ResourceType)
	request.ResourceID = compact(request.ResourceID)
	request.ProjectKey = compact(request.ProjectKey)
	request.Domain = compact(request.Domain)
	request.ToolID = compact(request.ToolID)
	request.RuntimeID = compact(request.RuntimeID)
	request.MandateID = compact(request.MandateID)
	request.AgentID = compact(request.AgentID)
	request.AssignmentID = compact(request.AssignmentID)
	request.ApprovalSourceID = compact(request.ApprovalSourceID)
	request.ApprovalBindingDigest = strings.ToLower(compact(request.ApprovalBindingDigest))
	effectDigest := strings.TrimSpace(request.EffectDigest)
	if effectDigest != request.EffectDigest ||
		effectDigest != strings.ToLower(effectDigest) {
		return Request{}, fmt.Errorf(
			"effect digest must be an exact lowercase SHA-256 digest",
		)
	}
	request.EffectDigest = effectDigest
	request.DataScopes = cleanValues(request.DataScopes, 32, 256)
	request.FolderPaths = cleanFolders(request.FolderPaths)
	request.SourceReferences = cleanReferences(request.SourceReferences, 32, 512)
	request.Facts = cleanFacts(request.Facts)
	if request.Governance != nil {
		governance, err := normalizeGovernanceEvidence(*request.Governance)
		if err != nil {
			return Request{}, err
		}
		if governanceEvidencePresent(governance) {
			request.Governance = &governance
		} else {
			request.Governance = nil
		}
	}

	for label, value := range map[string]string{
		"owner identity":  request.OwnerIdentity,
		"idempotency key": request.IdempotencyKey,
		"actor identity":  request.ActorIdentity,
		"task id":         request.TaskID,
		"action":          request.Action,
		"resource type":   request.ResourceType,
	} {
		if err := validateIdentifier(label, value); err != nil {
			return Request{}, err
		}
	}
	if len(request.Domain) > 64 {
		return Request{}, fmt.Errorf("domain exceeds 64 characters")
	}
	switch request.ActorKind {
	case ActorSystem, ActorAgent, ActorHuman:
	default:
		return Request{}, fmt.Errorf("actor kind %q is invalid", request.ActorKind)
	}
	switch request.Stage {
	case StageDataAccess, StageToolUse, StageExpenditure, StageCommunication,
		StageCommitment, StageExecution, StagePublication, StageDeletion,
		StagePrivilegeEscalation, StageSelfModification:
	default:
		return Request{}, fmt.Errorf("execution stage %q is invalid", request.Stage)
	}
	switch request.Risk {
	case RiskLow, RiskMedium, RiskHigh, RiskCritical:
	default:
		return Request{}, fmt.Errorf("risk level %q is invalid", request.Risk)
	}
	if request.Governance != nil {
		if err := validateFrameworkRiskForExecution(request.Risk, *request.Governance); err != nil {
			return Request{}, err
		}
	}
	if request.RequiredAuthority < 0 || request.RequiredAuthority > 10 ||
		request.RequestedAutonomy < 0 || request.RequestedAutonomy > 10 {
		return Request{}, fmt.Errorf("authority and autonomy must be between 0 and 10")
	}
	if request.Governance != nil {
		if err := validateFrameworkAutonomyForExecution(
			request.RequestedAutonomy,
			*request.Governance,
		); err != nil {
			return Request{}, err
		}
	}
	if math.IsNaN(request.EstimatedCostEUR) ||
		math.IsInf(request.EstimatedCostEUR, 0) ||
		request.EstimatedCostEUR < 0 ||
		request.EstimatedCostEUR > 1_000_000 {
		return Request{}, fmt.Errorf("estimated cost EUR is invalid")
	}
	if request.ActorKind == ActorAgent &&
		(request.AgentID == "" || request.AssignmentID == "") {
		return Request{}, fmt.Errorf("agent execution requires agent and assignment ids")
	}
	if request.ActorKind != ActorAgent &&
		(request.AgentID != "" || request.AssignmentID != "") {
		return Request{}, fmt.Errorf("agent evidence is only valid for an agent actor")
	}
	if request.ApprovalSourceID != "" && request.ApprovalBindingDigest == "" {
		return Request{}, fmt.Errorf("approval source requires an exact binding digest")
	}
	if request.ApprovalBindingDigest != "" && !validDigest(request.ApprovalBindingDigest) {
		return Request{}, fmt.Errorf("approval binding digest must be a SHA-256 digest")
	}
	if !validDigest(request.EffectDigest) {
		return Request{}, fmt.Errorf("effect digest must be an exact lowercase SHA-256 digest")
	}
	if !request.RequestedAt.IsZero() {
		request.RequestedAt = request.RequestedAt.UTC()
	}
	return request, nil
}

func normalizeGovernanceEvidence(value GovernanceEvidence) (GovernanceEvidence, error) {
	value.TaskPlanID = compact(value.TaskPlanID)
	value.FrameworkSelectionID = compact(value.FrameworkSelectionID)
	value.FrameworkCatalogVersion = compact(value.FrameworkCatalogVersion)
	value.FrameworkSelectorAlgorithmVersion = compact(value.FrameworkSelectorAlgorithmVersion)
	value.FrameworkTaskRiskLevel = RiskLevel(strings.ToLower(compact(string(value.FrameworkTaskRiskLevel))))
	value.FrameworkEffectiveRiskCeiling = RiskLevel(strings.ToLower(compact(string(value.FrameworkEffectiveRiskCeiling))))
	value.DomainPackDecisionID = compact(value.DomainPackDecisionID)
	value.DomainPackCatalogVersion = compact(value.DomainPackCatalogVersion)
	value.ResourceFeasibility = compact(value.ResourceFeasibility)
	value.EvidenceReferences = cleanReferences(value.EvidenceReferences, 32, 512)

	digests := map[string]*string{
		"task plan digest":                    &value.TaskPlanDigest,
		"framework evidence preflight digest": &value.FrameworkEvidencePreflightDigest,
		"framework catalog digest":            &value.FrameworkCatalogDigest,
		"framework preference digest":         &value.FrameworkPreferenceDigest,
		"framework Constitution digest":       &value.FrameworkConstitutionDigest,
		"framework operating contract digest": &value.FrameworkOperatingContractDigest,
		"domain pack catalog digest":          &value.DomainPackCatalogDigest,
		"domain pack decision digest":         &value.DomainPackDecisionDigest,
		"resource decision digest":            &value.ResourceDecisionDigest,
	}
	for label, target := range digests {
		raw := *target
		normalized := strings.ToLower(strings.TrimSpace(raw))
		if raw != normalized {
			return GovernanceEvidence{}, fmt.Errorf("%s must be an exact lowercase SHA-256 digest", label)
		}
		*target = normalized
		if normalized != "" && !validDigest(normalized) {
			return GovernanceEvidence{}, fmt.Errorf("%s must be an exact lowercase SHA-256 digest", label)
		}
	}

	frameworkPresent := value.FrameworkSelectionID != "" ||
		value.FrameworkCatalogVersion != "" || value.FrameworkCatalogDigest != "" ||
		value.FrameworkSelectorAlgorithmVersion != "" ||
		value.FrameworkTaskRiskLevel != "" || value.FrameworkEffectiveRiskCeiling != "" ||
		value.FrameworkMaximumAutonomyLevel != nil || value.FrameworkRequiresApproval != nil ||
		value.FrameworkPreferenceDigest != "" || value.FrameworkConstitutionDigest != "" ||
		value.FrameworkOperatingContractDigest != ""
	domainPackPresent := value.DomainPackDecisionID != "" ||
		value.DomainPackCatalogVersion != "" || value.DomainPackCatalogDigest != "" ||
		value.DomainPackDecisionDigest != ""
	resourcePresent := value.ResourceDecisionDigest != "" || value.ResourceFeasibility != ""
	governancePresent := governanceEvidencePresent(value)
	if !governancePresent {
		return GovernanceEvidence{}, nil
	}
	if err := validateIdentifier("governance task plan id", value.TaskPlanID); err != nil {
		return GovernanceEvidence{}, err
	}
	if value.TaskPlanDigest == "" {
		return GovernanceEvidence{}, fmt.Errorf("governance task plan digest is required")
	}
	if frameworkPresent {
		for label, candidate := range map[string]string{
			"framework selection id":    value.FrameworkSelectionID,
			"framework catalog version": value.FrameworkCatalogVersion,
		} {
			if err := validateIdentifier(label, candidate); err != nil {
				return GovernanceEvidence{}, err
			}
		}
		if value.FrameworkCatalogDigest == "" || value.FrameworkPreferenceDigest == "" ||
			value.FrameworkConstitutionDigest == "" || value.FrameworkOperatingContractDigest == "" {
			return GovernanceEvidence{}, fmt.Errorf("framework governance requires catalog, preference, Constitution, and operating contract digests")
		}
		if value.FrameworkSelectorAlgorithmVersion != "" {
			if err := validateIdentifier(
				"framework selector algorithm version",
				value.FrameworkSelectorAlgorithmVersion,
			); err != nil {
				return GovernanceEvidence{}, err
			}
		}
		if err := validateFrameworkRiskContract(value); err != nil {
			return GovernanceEvidence{}, err
		}
	}
	if domainPackPresent {
		for label, candidate := range map[string]string{
			"domain pack decision id":     value.DomainPackDecisionID,
			"domain pack catalog version": value.DomainPackCatalogVersion,
		} {
			if err := validateIdentifier(label, candidate); err != nil {
				return GovernanceEvidence{}, err
			}
		}
		if value.DomainPackCatalogDigest == "" || value.DomainPackDecisionDigest == "" {
			return GovernanceEvidence{}, fmt.Errorf("domain pack governance requires catalog and decision digests")
		}
	}
	if resourcePresent {
		if value.ResourceDecisionDigest == "" || value.ResourceFeasibility == "" {
			return GovernanceEvidence{}, fmt.Errorf("resource governance requires decision digest and feasibility")
		}
		switch value.ResourceFeasibility {
		case "feasible", "feasible_with_approvals", "infeasible":
		default:
			return GovernanceEvidence{}, fmt.Errorf("resource governance feasibility is invalid")
		}
	}
	return value, nil
}

func governanceEvidencePresent(value GovernanceEvidence) bool {
	return value.TaskPlanID != "" || value.TaskPlanDigest != "" ||
		value.FrameworkEvidencePreflightDigest != "" ||
		value.FrameworkSelectionID != "" || value.FrameworkCatalogVersion != "" ||
		value.FrameworkSelectorAlgorithmVersion != "" ||
		value.FrameworkTaskRiskLevel != "" || value.FrameworkEffectiveRiskCeiling != "" ||
		value.FrameworkMaximumAutonomyLevel != nil || value.FrameworkRequiresApproval != nil ||
		value.FrameworkCatalogDigest != "" || value.FrameworkPreferenceDigest != "" ||
		value.FrameworkConstitutionDigest != "" || value.FrameworkOperatingContractDigest != "" ||
		value.DomainPackDecisionID != "" || value.DomainPackCatalogVersion != "" ||
		value.DomainPackCatalogDigest != "" || value.DomainPackDecisionDigest != "" ||
		value.ResourceDecisionDigest != "" || value.ResourceFeasibility != "" ||
		len(value.EvidenceReferences) > 0
}

func validateFrameworkRiskContract(value GovernanceEvidence) error {
	version := value.FrameworkSelectorAlgorithmVersion
	taskRisk := value.FrameworkTaskRiskLevel
	ceiling := value.FrameworkEffectiveRiskCeiling

	switch version {
	case "", frameworkSelectorV4:
		if taskRisk != "" || ceiling != "" ||
			value.FrameworkMaximumAutonomyLevel != nil || value.FrameworkRequiresApproval != nil {
			return fmt.Errorf("legacy framework governance cannot assert a v5 execution contract")
		}
		return nil
	case frameworkSelectorV5:
		if taskRisk == "" || ceiling == "" ||
			value.FrameworkMaximumAutonomyLevel == nil || value.FrameworkRequiresApproval == nil {
			return fmt.Errorf("selector-v5 framework governance requires risk, autonomy, and approval contracts")
		}
	default:
		return fmt.Errorf("framework selector algorithm version %q is unsupported", version)
	}

	taskRank, taskOK := frameworkRiskRank(taskRisk)
	ceilingRank, ceilingOK := frameworkRiskRank(ceiling)
	if !taskOK {
		return fmt.Errorf("framework task risk level %q is invalid", taskRisk)
	}
	if !ceilingOK {
		return fmt.Errorf("framework effective risk ceiling %q is invalid", ceiling)
	}
	if taskRank > ceilingRank {
		return fmt.Errorf(
			"framework task risk %q exceeds effective risk ceiling %q",
			taskRisk,
			ceiling,
		)
	}
	if *value.FrameworkMaximumAutonomyLevel < 0 || *value.FrameworkMaximumAutonomyLevel > 10 {
		return fmt.Errorf("framework maximum autonomy level must be between 0 and 10")
	}
	return nil
}

func validateFrameworkAutonomyForExecution(requestedAutonomy int, value GovernanceEvidence) error {
	if value.FrameworkSelectorAlgorithmVersion != frameworkSelectorV5 {
		return nil
	}
	if value.FrameworkMaximumAutonomyLevel == nil {
		return fmt.Errorf("selector-v5 framework autonomy contract is missing")
	}
	if requestedAutonomy > *value.FrameworkMaximumAutonomyLevel {
		return fmt.Errorf(
			"requested autonomy level %d exceeds framework maximum autonomy level %d",
			requestedAutonomy,
			*value.FrameworkMaximumAutonomyLevel,
		)
	}
	return nil
}

func validateFrameworkRiskForExecution(requestRisk RiskLevel, value GovernanceEvidence) error {
	if value.FrameworkSelectorAlgorithmVersion != frameworkSelectorV5 {
		return nil
	}
	requestRank, requestOK := frameworkRiskRank(requestRisk)
	taskRank, taskOK := frameworkRiskRank(value.FrameworkTaskRiskLevel)
	ceilingRank, ceilingOK := frameworkRiskRank(value.FrameworkEffectiveRiskCeiling)
	if !requestOK || !taskOK || !ceilingOK {
		return fmt.Errorf("selector-v5 execution risk contract is invalid")
	}
	if requestRank < taskRank {
		return fmt.Errorf(
			"execution risk %q is below framework task risk %q",
			requestRisk,
			value.FrameworkTaskRiskLevel,
		)
	}
	if requestRank > ceilingRank {
		return fmt.Errorf(
			"execution risk %q exceeds framework effective risk ceiling %q",
			requestRisk,
			value.FrameworkEffectiveRiskCeiling,
		)
	}
	return nil
}

func frameworkRiskRank(value RiskLevel) (int, bool) {
	switch value {
	case RiskLow:
		return 1, true
	case RiskMedium:
		return 2, true
	case RiskHigh:
		return 3, true
	default:
		return 0, false
	}
}

func validateIdentifier(label, value string) error {
	if value == "" || len(value) > 256 || !boundedIdentifier.MatchString(value) {
		return fmt.Errorf("%s must be a bounded identifier", label)
	}
	return nil
}

func compact(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(safety.RedactSecrets(value))), " ")
}

func cleanValues(values []string, limit, maxRunes int) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = compact(value)
		if value == "" {
			continue
		}
		runes := []rune(value)
		if len(runes) > maxRunes {
			value = string(runes[:maxRunes])
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
		if len(result) == limit {
			break
		}
	}
	sort.Strings(result)
	return result
}

func cleanReferences(values []string, limit, maxRunes int) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		// RedactURL already applies generic secret redaction to non-URLs. Do not
		// run compact afterwards: its key/value regex would consume subsequent
		// URL query parameters after a redacted sensitive value.
		value = strings.Join(strings.Fields(strings.TrimSpace(safety.RedactURL(value))), " ")
		if value == "" {
			continue
		}
		runes := []rune(value)
		if len(runes) > maxRunes {
			value = string(runes[:maxRunes])
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
		if len(result) == limit {
			break
		}
	}
	sort.Strings(result)
	return result
}

func cleanFolders(values []string) []string {
	result := make([]string, 0, min(len(values), 32))
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		value := compact(raw)
		if value == "" {
			continue
		}
		runes := []rune(value)
		if len(runes) > 512 {
			value = string(runes[:512])
		}
		value = filepath.Clean(value)
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	if len(result) > 32 {
		result = result[:32]
	}
	return result
}

func cleanFacts(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := map[string]string{}
	for _, key := range keys {
		if len(result) == 32 {
			break
		}
		cleanKey := compact(key)
		cleanValue := compact(values[key])
		if cleanKey == "" || cleanValue == "" {
			continue
		}
		keyRunes := []rune(cleanKey)
		if len(keyRunes) > 128 {
			cleanKey = string(keyRunes[:128])
		}
		valueRunes := []rune(cleanValue)
		if len(valueRunes) > 512 {
			cleanValue = string(valueRunes[:512])
		}
		result[cleanKey] = cleanValue
	}
	return result
}

func validDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}

func monotonicNow(now func() time.Time) time.Time {
	if now == nil {
		return time.Now().UTC()
	}
	return now().UTC()
}
