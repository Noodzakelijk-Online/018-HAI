package task

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"automation-hub-backend/internal/domainpack"
)

const domainPackAuthorityBoundary = "domain packs provide advisory planning guidance only; execution still requires the task risk gate, a valid mandate, any required human approval, and the controlled execution boundary"

// DomainPackPlanner is the narrow task-to-domain boundary. Implementations may
// classify and advise, but cannot return an execution receipt or mutate task
// authority.
type DomainPackPlanner interface {
	PlanDomainPacks(request DomainPackPlanningRequest) (*DomainPackDecision, error)
}

type DomainPackPlanningRequest struct {
	OwnerIdentity       string
	Text                string
	TaskType            string
	RiskLevel           string
	SuccessCriteria     []string
	ExecuteRequested    bool
	NeedsTools          bool
	NeedsDocuments      bool
	NeedsWebAccess      bool
	NeedsLocalExecution bool
}

type DomainPackPreferenceBinding struct {
	PackID         domainpack.PackID           `json:"packId"`
	CatalogVersion string                      `json:"catalogVersion"`
	Revision       int64                       `json:"revision"`
	Status         domainpack.PreferenceStatus `json:"status"`
	Digest         string                      `json:"digest"`
}

type DomainPackClassificationDecision struct {
	PackID                   domainpack.PackID `json:"packId"`
	PackVersion              string            `json:"packVersion"`
	Score                    int               `json:"score"`
	Sensitive                bool              `json:"sensitive"`
	Reasons                  []string          `json:"reasons"`
	LocalOnly                bool              `json:"localOnly"`
	GuidanceDigest           string            `json:"guidanceDigest"`
	PlaybookVersion          string            `json:"playbookVersion"`
	PlaybookDigest           string            `json:"playbookDigest"`
	PlaybookProvenanceDigest string            `json:"playbookProvenanceDigest"`
}

type DomainPackMethodDecision struct {
	PackID                domainpack.PackID           `json:"packId"`
	ID                    string                      `json:"id"`
	Version               string                      `json:"version"`
	Name                  string                      `json:"name"`
	Group                 string                      `json:"group"`
	Domain                string                      `json:"domain"`
	Purpose               string                      `json:"purpose"`
	Score                 int                         `json:"score"`
	Reasons               []string                    `json:"reasons"`
	AuthorityRequirements []string                    `json:"authorityRequirements"`
	SafetyInvariants      []string                    `json:"safetyInvariants"`
	RiskCeiling           domainpack.RiskLevel        `json:"riskCeiling"`
	EvidenceRequirements  []string                    `json:"evidenceRequirements"`
	Evaluation            domainpack.MethodEvaluation `json:"evaluation"`
	Provenance            domainpack.MethodProvenance `json:"provenance"`
	ProvenanceDigest      string                      `json:"provenanceDigest"`
}

type DomainPackRiskGuidance struct {
	PackID     domainpack.PackID      `json:"packId"`
	Trigger    domainpack.RiskTrigger `json:"trigger"`
	Applicable bool                   `json:"applicable"`
}

type DomainPackApprovalGuidance struct {
	PackID     domainpack.PackID       `json:"packId"`
	Rule       domainpack.ApprovalRule `json:"rule"`
	Applicable bool                    `json:"applicable"`
}

type DomainPackProhibitedGuidance struct {
	PackID     domainpack.PackID           `json:"packId"`
	Action     domainpack.ProhibitedAction `json:"action"`
	Applicable bool                        `json:"applicable"`
}

type DomainPackEvidenceGuidance struct {
	PackID      domainpack.PackID              `json:"packId"`
	Requirement domainpack.EvidenceRequirement `json:"requirement"`
	Applicable  bool                           `json:"applicable"`
}

type DomainPackSuccessGuidance struct {
	PackID     domainpack.PackID                  `json:"packId"`
	Template   domainpack.SuccessCriteriaTemplate `json:"template"`
	Applicable bool                               `json:"applicable"`
}

type DomainPackStopGuidance struct {
	PackID    domainpack.PackID        `json:"packId"`
	Condition domainpack.StopCondition `json:"condition"`
}

type DomainPackValidatorGuidance struct {
	PackID    domainpack.PackID                 `json:"packId"`
	Validator domainpack.DeterministicValidator `json:"validator"`
}

// DomainPackDecision is persisted with the CompletionPlan. It contains
// reproducible planning evidence and intentionally has no executable action,
// mandate, approval token, or authority-bearing field.
type DomainPackDecision struct {
	ID                        string                             `json:"id"`
	Digest                    string                             `json:"digest"`
	RequestDigest             string                             `json:"requestDigest"`
	CatalogVersion            string                             `json:"catalogVersion"`
	CatalogDigest             string                             `json:"catalogDigest"`
	AdvisoryOnly              bool                               `json:"advisoryOnly"`
	ExecutionAuthorityGranted bool                               `json:"executionAuthorityGranted"`
	AuthorityBoundary         string                             `json:"authorityBoundary"`
	Classified                []DomainPackClassificationDecision `json:"classified"`
	Suppressed                []domainpack.SuppressedMatch       `json:"suppressed"`
	Preferences               []DomainPackPreferenceBinding      `json:"preferences"`
	Methods                   []DomainPackMethodDecision         `json:"methods"`
	RiskGuidance              []DomainPackRiskGuidance           `json:"riskGuidance"`
	ApprovalGuidance          []DomainPackApprovalGuidance       `json:"approvalGuidance"`
	ProhibitedGuidance        []DomainPackProhibitedGuidance     `json:"prohibitedGuidance"`
	EvidenceGuidance          []DomainPackEvidenceGuidance       `json:"evidenceGuidance"`
	SuccessGuidance           []DomainPackSuccessGuidance        `json:"successGuidance"`
	StopGuidance              []DomainPackStopGuidance           `json:"stopGuidance"`
	ValidatorGuidance         []DomainPackValidatorGuidance      `json:"validatorGuidance"`
	LocalOnly                 bool                               `json:"localOnly"`
	AgentCapabilities         []string                           `json:"agentCapabilities"`
	AdvisoryRiskLevel         domainpack.RiskLevel               `json:"advisoryRiskLevel,omitempty"`
	RequiresApproval          bool                               `json:"requiresApproval"`
	ApprovalReasons           []string                           `json:"approvalReasons"`
	BlockedAutonomousActions  []string                           `json:"blockedAutonomousActions"`
}

type domainPackPlanningBridge struct {
	registry    *domainpack.Registry
	preferences domainpack.PreferenceRepository
}

func NewDomainPackPlanningBridge(
	registry *domainpack.Registry,
	preferences domainpack.PreferenceRepository,
) (DomainPackPlanner, error) {
	if registry == nil {
		return nil, fmt.Errorf("domain pack registry is required")
	}
	return &domainPackPlanningBridge{registry: registry, preferences: preferences}, nil
}

// WithDomainPackPlanning replaces the default built-in advisory bridge on a
// concrete task service. This is how a composition root supplies the same
// owner-scoped preference repository used by domain-pack management routes.
func WithDomainPackPlanning(
	base Service,
	registry *domainpack.Registry,
	preferences domainpack.PreferenceRepository,
) (Service, error) {
	planner, err := NewDomainPackPlanningBridge(registry, preferences)
	if err != nil {
		return nil, err
	}
	implementation, ok := base.(*service)
	if !ok {
		return nil, fmt.Errorf("domain pack planning requires the built-in task service")
	}
	implementation.domainPackPlanner = planner
	return implementation, nil
}

func defaultDomainPackPlanner() DomainPackPlanner {
	registry, err := domainpack.NewBuiltinRegistry()
	if err != nil {
		panic(fmt.Sprintf("initialize domain pack planner: %v", err))
	}
	planner, err := NewDomainPackPlanningBridge(registry, nil)
	if err != nil {
		panic(fmt.Sprintf("initialize domain pack planning bridge: %v", err))
	}
	return planner
}

func (bridge *domainPackPlanningBridge) PlanDomainPacks(
	request DomainPackPlanningRequest,
) (*DomainPackDecision, error) {
	if bridge == nil || bridge.registry == nil {
		return nil, fmt.Errorf("domain pack planning bridge is not configured")
	}
	signal := domainPackTaskSignal(request)
	effectivePreferences := bridge.preferences
	if strings.TrimSpace(request.OwnerIdentity) == "" {
		effectivePreferences = nil
	}
	classification, err := bridge.registry.Classify(domainpack.ClassificationRequest{
		Text:          signal,
		OwnerIdentity: strings.TrimSpace(request.OwnerIdentity),
	}, effectivePreferences)
	if err != nil {
		return nil, fmt.Errorf("classify task domain packs: %w", err)
	}
	metadata := bridge.registry.Metadata()
	decision := &DomainPackDecision{
		RequestDigest:             digestValue(signal),
		CatalogVersion:            metadata.Version,
		CatalogDigest:             metadata.Digest,
		AdvisoryOnly:              true,
		ExecutionAuthorityGranted: false,
		AuthorityBoundary:         domainPackAuthorityBoundary,
		Suppressed:                append([]domainpack.SuppressedMatch(nil), classification.Suppressed...),
	}
	if effectivePreferences != nil {
		preferences, listErr := effectivePreferences.List(strings.TrimSpace(request.OwnerIdentity))
		if listErr != nil {
			return nil, fmt.Errorf("bind owner domain pack preferences: %w", listErr)
		}
		for _, preference := range preferences {
			bindingPayload := struct {
				PackID              domainpack.PackID
				CatalogVersion      string
				Revision            int64
				Status              domainpack.PreferenceStatus
				Enabled             *bool
				ClassificationBoost int
				ForceLocalOnly      bool
				Adaptation          domainpack.PackAdaptation
			}{
				preference.PackID,
				preference.CatalogVersion,
				preference.Revision,
				preference.Status,
				preference.Enabled,
				preference.ClassificationBoost,
				preference.ForceLocalOnly,
				preference.Adaptation,
			}
			decision.Preferences = append(decision.Preferences, DomainPackPreferenceBinding{
				PackID: preference.PackID, CatalogVersion: preference.CatalogVersion,
				Revision: preference.Revision, Status: preference.Status,
				Digest: digestValue(bindingPayload),
			})
		}
	}

	classifiedIDs := make([]domainpack.PackID, 0, len(classification.Matches))
	actions := inferDomainPackActions(request)
	for _, match := range classification.Matches {
		view, resolveErr := bridge.registry.Resolve(request.OwnerIdentity, match.PackID, effectivePreferences)
		if resolveErr != nil {
			return nil, fmt.Errorf("resolve effective domain pack %q: %w", match.PackID, resolveErr)
		}
		if !view.Enabled {
			return nil, fmt.Errorf("classified domain pack %q became disabled during planning", match.PackID)
		}
		classifiedIDs = append(classifiedIDs, match.PackID)
		provenancePayload := make([]struct {
			MethodID   string
			Provenance domainpack.MethodProvenance
		}, 0, len(view.Pack.Playbook.Methods))
		for _, method := range view.Pack.Playbook.Methods {
			provenancePayload = append(provenancePayload, struct {
				MethodID   string
				Provenance domainpack.MethodProvenance
			}{method.ID, method.Provenance})
		}
		decision.Classified = append(decision.Classified, DomainPackClassificationDecision{
			PackID: match.PackID, PackVersion: view.Pack.Version, Score: match.Score,
			Sensitive: match.Sensitive, Reasons: append([]string(nil), match.Reasons...),
			LocalOnly: view.LocalOnly, GuidanceDigest: digestValue(view.Pack),
			PlaybookVersion: view.Pack.Playbook.Version, PlaybookDigest: view.Pack.Playbook.Digest,
			PlaybookProvenanceDigest: digestValue(provenancePayload),
		})
		decision.LocalOnly = decision.LocalOnly || view.LocalOnly
		decision.AgentCapabilities = append(decision.AgentCapabilities, view.Pack.SuitableAgentCapabilities...)
		appendDomainPackGuidance(decision, match.PackID, view.Pack, actions, request)
	}

	if len(classifiedIDs) > 0 {
		selection, selectErr := bridge.registry.SelectMethods(domainpack.MethodSelectionRequest{
			Text: signal, ClassifiedPackIDs: classifiedIDs, Limit: 8,
			OwnerIdentity: strings.TrimSpace(request.OwnerIdentity),
		}, effectivePreferences)
		if selectErr != nil {
			return nil, fmt.Errorf("select advisory domain playbook methods: %w", selectErr)
		}
		if !selection.AdvisoryOnly || selection.ExecutionAuthorityGranted {
			return nil, fmt.Errorf("domain method selector violated the advisory-only authority boundary")
		}
		if selection.CatalogVersion != metadata.Version || selection.CatalogDigest != metadata.Digest {
			return nil, fmt.Errorf("domain method selection catalog binding changed during planning")
		}
		for _, selected := range selection.Selections {
			method := selected.Method
			decision.Methods = append(decision.Methods, DomainPackMethodDecision{
				PackID: selected.PackID, ID: method.ID, Version: method.Version,
				Name: method.Name, Group: method.Group, Domain: method.Domain, Purpose: method.Purpose,
				Score: selected.Score, Reasons: append([]string(nil), selected.Reasons...),
				AuthorityRequirements: append([]string(nil), method.AuthorityRequirements...),
				SafetyInvariants:      append([]string(nil), method.SafetyInvariants...),
				RiskCeiling:           method.RiskCeiling,
				EvidenceRequirements:  append([]string(nil), method.EvidenceRequirements...),
				Evaluation:            method.Evaluation, Provenance: method.Provenance,
				ProvenanceDigest: digestValue(method.Provenance),
			})
		}
	}

	normalizeDomainPackDecision(decision)
	decision.Digest = digestValue(*decision)
	decision.ID = "domain-pack-decision-" + strings.TrimPrefix(decision.Digest, "sha256:")[:24]
	return decision, nil
}

func appendDomainPackGuidance(
	decision *DomainPackDecision,
	packID domainpack.PackID,
	pack domainpack.DomainPack,
	actions map[string]bool,
	request DomainPackPlanningRequest,
) {
	for _, trigger := range pack.RiskTriggers {
		applicable := domainRiskTriggerApplies(trigger.ID, actions, request)
		decision.RiskGuidance = append(decision.RiskGuidance, DomainPackRiskGuidance{PackID: packID, Trigger: trigger, Applicable: applicable})
		if applicable {
			decision.AdvisoryRiskLevel = maxDomainRisk(decision.AdvisoryRiskLevel, trigger.Level)
		}
	}
	for _, rule := range pack.ApprovalRules {
		applicable := actions[rule.Action]
		decision.ApprovalGuidance = append(decision.ApprovalGuidance, DomainPackApprovalGuidance{PackID: packID, Rule: rule, Applicable: applicable})
		if applicable && rule.Required {
			decision.RequiresApproval = true
			decision.AdvisoryRiskLevel = maxDomainRisk(decision.AdvisoryRiskLevel, rule.MinimumRisk)
			decision.ApprovalReasons = append(decision.ApprovalReasons, rule.Reason)
		}
	}
	for _, prohibited := range pack.ProhibitedAutonomousActions {
		applicable := prohibitedDomainActionApplies(prohibited.Action, actions)
		decision.ProhibitedGuidance = append(decision.ProhibitedGuidance, DomainPackProhibitedGuidance{PackID: packID, Action: prohibited, Applicable: applicable})
		if applicable {
			decision.BlockedAutonomousActions = append(decision.BlockedAutonomousActions, prohibited.Action)
		}
	}
	for _, requirement := range pack.EvidenceRequirements {
		applicable := evidenceRequirementApplies(requirement, actions, request)
		decision.EvidenceGuidance = append(decision.EvidenceGuidance, DomainPackEvidenceGuidance{PackID: packID, Requirement: requirement, Applicable: applicable})
	}
	for _, template := range pack.SuccessCriteriaTemplates {
		applicable := template.ID == "prepared" || (request.ExecuteRequested && (template.ID == "executed" || template.ID == "completed"))
		decision.SuccessGuidance = append(decision.SuccessGuidance, DomainPackSuccessGuidance{PackID: packID, Template: template, Applicable: applicable})
	}
	for _, condition := range pack.StopEscalationConditions {
		decision.StopGuidance = append(decision.StopGuidance, DomainPackStopGuidance{PackID: packID, Condition: condition})
	}
	for _, validator := range pack.DeterministicValidators {
		decision.ValidatorGuidance = append(decision.ValidatorGuidance, DomainPackValidatorGuidance{PackID: packID, Validator: validator})
	}
}

func domainPackTaskSignal(request DomainPackPlanningRequest) string {
	parts := []string{strings.TrimSpace(request.Text), strings.TrimSpace(request.TaskType)}
	parts = append(parts, request.SuccessCriteria...)
	return strings.Join(uniqueStrings(parts), "\n")
}

func inferDomainPackActions(request DomainPackPlanningRequest) map[string]bool {
	text := strings.ToLower(strings.TrimSpace(request.Text))
	actions := map[string]bool{
		"complete_task": request.ExecuteRequested,
		"state_change":  request.ExecuteRequested && (request.NeedsTools || request.NeedsLocalExecution),
	}
	actions["paid_model_usage"] = containsWordOrPhrase(text, "paid model", "paid provider", "buy credits")
	explicitNoSend := containsWordOrPhrase(text,
		"do not send", "don't send", "not send", "never send", "without sending", "leave unsent")
	actions["external_send"] = !explicitNoSend && containsWordOrPhrase(text, "send", "contact", "submit")
	actions["public_post"] = containsWordOrPhrase(text, "publish", "public post", "post publicly", "social media")
	actions["financial_transaction"] = containsWordOrPhrase(text, "pay", "payment", "transfer", "spend", "purchase", "buy")
	actions["legal_or_government_action"] = containsWordOrPhrase(text, "legal", "lawyer", "court", "government", "municipality", "filing", "objection")
	actions["medical_action"] = containsWordOrPhrase(text, "diagnose", "prescribe", "treatment", "medication", "medical action")
	actions["destructive_change"] = containsWordOrPhrase(text, "delete", "destroy", "overwrite", "purge", "revoke")
	actions["account_change"] = containsWordOrPhrase(text, "account change", "password", "credential", "permission", "access change")
	actions["store_fact"] = request.NeedsDocuments || request.NeedsWebAccess
	actions["high_risk_action"] = request.RiskLevel == "high" || request.RiskLevel == "critical"
	return actions
}

func domainRiskTriggerApplies(id string, actions map[string]bool, request DomainPackPlanningRequest) bool {
	switch id {
	case "external_effect":
		return actions["external_send"] || actions["public_post"] || actions["financial_transaction"] || actions["legal_or_government_action"] || actions["medical_action"] || actions["account_change"] || actions["state_change"]
	case "irreversible":
		return actions["destructive_change"]
	case "uncertain_evidence":
		return (request.NeedsDocuments || request.NeedsWebAccess) && len(request.SuccessCriteria) == 0
	default:
		return false
	}
}

func prohibitedDomainActionApplies(action string, actions map[string]bool) bool {
	switch action {
	case "spend_or_transfer_money":
		return actions["financial_transaction"]
	case "make_legal_filing_or_concession":
		return actions["legal_or_government_action"]
	case "diagnose_prescribe_or_change_treatment":
		return actions["medical_action"]
	case "publish_or_send_as_owner":
		return actions["public_post"] || actions["external_send"]
	case "permanently_delete_or_revoke_access":
		return actions["destructive_change"] || actions["account_change"]
	default:
		return false
	}
}

func evidenceRequirementApplies(requirement domainpack.EvidenceRequirement, actions map[string]bool, request DomainPackPlanningRequest) bool {
	if len(requirement.RequiredForActions) == 0 {
		return true
	}
	for _, action := range requirement.RequiredForActions {
		if actions[action] {
			return true
		}
	}
	return request.NeedsDocuments && requirement.ID == "source_provenance"
}

func maxDomainRisk(left, right domainpack.RiskLevel) domainpack.RiskLevel {
	order := map[domainpack.RiskLevel]int{domainpack.RiskLow: 1, domainpack.RiskMedium: 2, domainpack.RiskHigh: 3, domainpack.RiskCritical: 4}
	if order[right] > order[left] {
		return right
	}
	return left
}

func normalizeDomainPackDecision(decision *DomainPackDecision) {
	decision.AgentCapabilities = uniqueStrings(decision.AgentCapabilities)
	decision.ApprovalReasons = uniqueStrings(decision.ApprovalReasons)
	decision.BlockedAutonomousActions = uniqueStrings(decision.BlockedAutonomousActions)
	sort.Slice(decision.Preferences, func(i, j int) bool { return decision.Preferences[i].PackID < decision.Preferences[j].PackID })
	sort.Slice(decision.Methods, func(i, j int) bool {
		if decision.Methods[i].PackID != decision.Methods[j].PackID {
			return decision.Methods[i].PackID < decision.Methods[j].PackID
		}
		return decision.Methods[i].ID < decision.Methods[j].ID
	})
	sort.Slice(decision.RiskGuidance, func(i, j int) bool {
		return domainGuidanceKey(decision.RiskGuidance[i].PackID, decision.RiskGuidance[i].Trigger.ID) < domainGuidanceKey(decision.RiskGuidance[j].PackID, decision.RiskGuidance[j].Trigger.ID)
	})
	sort.Slice(decision.ApprovalGuidance, func(i, j int) bool {
		return domainGuidanceKey(decision.ApprovalGuidance[i].PackID, decision.ApprovalGuidance[i].Rule.Action) < domainGuidanceKey(decision.ApprovalGuidance[j].PackID, decision.ApprovalGuidance[j].Rule.Action)
	})
	sort.Slice(decision.ProhibitedGuidance, func(i, j int) bool {
		return domainGuidanceKey(decision.ProhibitedGuidance[i].PackID, decision.ProhibitedGuidance[i].Action.Action) < domainGuidanceKey(decision.ProhibitedGuidance[j].PackID, decision.ProhibitedGuidance[j].Action.Action)
	})
	sort.Slice(decision.EvidenceGuidance, func(i, j int) bool {
		return domainGuidanceKey(decision.EvidenceGuidance[i].PackID, decision.EvidenceGuidance[i].Requirement.ID) < domainGuidanceKey(decision.EvidenceGuidance[j].PackID, decision.EvidenceGuidance[j].Requirement.ID)
	})
	sort.Slice(decision.SuccessGuidance, func(i, j int) bool {
		return domainGuidanceKey(decision.SuccessGuidance[i].PackID, decision.SuccessGuidance[i].Template.ID) < domainGuidanceKey(decision.SuccessGuidance[j].PackID, decision.SuccessGuidance[j].Template.ID)
	})
	sort.Slice(decision.StopGuidance, func(i, j int) bool {
		return domainGuidanceKey(decision.StopGuidance[i].PackID, decision.StopGuidance[i].Condition.ID) < domainGuidanceKey(decision.StopGuidance[j].PackID, decision.StopGuidance[j].Condition.ID)
	})
	sort.Slice(decision.ValidatorGuidance, func(i, j int) bool {
		return domainGuidanceKey(decision.ValidatorGuidance[i].PackID, decision.ValidatorGuidance[i].Validator.ID) < domainGuidanceKey(decision.ValidatorGuidance[j].PackID, decision.ValidatorGuidance[j].Validator.ID)
	})
}

func domainGuidanceKey(packID domainpack.PackID, id string) string {
	return string(packID) + "\x00" + id
}

func digestValue(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("encode deterministic domain pack evidence: %v", err))
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func applyDomainPackRisk(risk RiskAssessment, decision *DomainPackDecision) RiskAssessment {
	if decision == nil {
		return risk
	}
	if !decision.AdvisoryOnly || decision.ExecutionAuthorityGranted {
		risk.AllowedNow = false
		risk.Reasons = append(risk.Reasons, "invalid domain pack decision attempted to cross the advisory-only authority boundary")
		return risk
	}
	if domainRiskRank(string(decision.AdvisoryRiskLevel)) > domainRiskRank(risk.Level) {
		risk.Level = string(decision.AdvisoryRiskLevel)
	}
	if decision.RequiresApproval {
		risk.ApprovalRequired = true
		risk.Reasons = append(risk.Reasons, decision.ApprovalReasons...)
		if !risk.ApprovalGranted {
			risk.AllowedNow = false
		}
	}
	if len(decision.BlockedAutonomousActions) > 0 && !risk.ApprovalGranted {
		risk.AllowedNow = false
		risk.Reasons = append(risk.Reasons, "domain pack prohibits autonomous action: "+strings.Join(decision.BlockedAutonomousActions, ", "))
	}
	risk.Reasons = uniqueStrings(append(risk.Reasons, "domain pack guidance was evaluated without granting execution authority"))
	return risk
}

func domainRiskRank(level string) int {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func applyDomainPackValidation(plan ValidationPlan, decision *DomainPackDecision) ValidationPlan {
	if decision == nil {
		return plan
	}
	for _, guidance := range decision.EvidenceGuidance {
		if guidance.Applicable {
			value := guidance.Requirement.ID + ": " + guidance.Requirement.Description + " [" + guidance.Requirement.MinimumVerification + "]"
			plan.DomainPackEvidenceRequirements = append(plan.DomainPackEvidenceRequirements, value)
			plan.Steps = append(plan.Steps, "domain evidence: "+value)
		}
	}
	for _, guidance := range decision.SuccessGuidance {
		if guidance.Applicable {
			plan.DomainPackSuccessCriteria = append(plan.DomainPackSuccessCriteria, guidance.Template.Criteria...)
		}
	}
	for _, guidance := range decision.ValidatorGuidance {
		value := guidance.Validator.ID + ": " + guidance.Validator.Description
		plan.DomainPackValidators = append(plan.DomainPackValidators, value)
		plan.Steps = append(plan.Steps, "domain validator: "+value)
	}
	for _, method := range decision.Methods {
		plan.DomainPackMethodEvaluation = append(plan.DomainPackMethodEvaluation, method.Evaluation.Criteria...)
	}
	plan.Steps = uniqueStrings(plan.Steps)
	plan.DomainPackEvidenceRequirements = uniqueStrings(plan.DomainPackEvidenceRequirements)
	plan.DomainPackSuccessCriteria = uniqueStrings(plan.DomainPackSuccessCriteria)
	plan.DomainPackValidators = uniqueStrings(plan.DomainPackValidators)
	plan.DomainPackMethodEvaluation = uniqueStrings(plan.DomainPackMethodEvaluation)
	if len(plan.DomainPackEvidenceRequirements)+len(plan.DomainPackSuccessCriteria)+len(plan.DomainPackValidators) > 0 {
		plan.CompletionGate = "task is complete only after task, framework, and applicable domain-pack evidence, success, and deterministic validation criteria pass"
	}
	return plan
}

func applyDomainPackExecution(plan ExecutionPlan, decision *DomainPackDecision) ExecutionPlan {
	if decision == nil {
		return plan
	}
	plan.DomainPackLocalOnly = decision.LocalOnly
	plan.DomainPackAuthorityBoundary = decision.AuthorityBoundary
	plan.AdvisoryAgentCapabilities = append([]string(nil), decision.AgentCapabilities...)
	plan.ApprovalRequiredFor = uniqueStrings(append(plan.ApprovalRequiredFor, decision.ApprovalReasons...))
	for _, guidance := range decision.StopGuidance {
		plan.StopConditions = append(plan.StopConditions, guidance.Condition.Condition+" -> "+guidance.Condition.EscalateTo)
	}
	plan.StopConditions = uniqueStrings(plan.StopConditions)
	plan.AuditEvents = uniqueStrings(append(plan.AuditEvents,
		"domain pack classified",
		"owner domain preferences resolved",
		"advisory domain methods selected",
		"domain pack authority boundary evaluated",
	))
	return plan
}

func domainPackSelectionSummary(decision *DomainPackDecision) string {
	if decision == nil {
		return "no domain-pack decision was available"
	}
	packIDs := make([]string, 0, len(decision.Classified))
	for _, classified := range decision.Classified {
		packIDs = append(packIDs, string(classified.PackID))
	}
	if len(packIDs) == 0 {
		return "no domain pack matched; catalog " + decision.CatalogVersion + " remained advisory and granted no execution authority"
	}
	return fmt.Sprintf(
		"classified %s; selected %d advisory methods; catalog %s; execution authority granted=false",
		strings.Join(packIDs, ", "),
		len(decision.Methods),
		decision.CatalogVersion,
	)
}
