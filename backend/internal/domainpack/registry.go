package domainpack

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var semanticVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

var mandatoryApprovalActions = []string{
	"paid_model_usage",
	"financial_transaction",
	"legal_or_government_action",
	"medical_action",
	"public_post",
}

type Registry struct {
	version string
	digest  string
	packs   map[PackID]DomainPack
	ids     []PackID
}

func NewBuiltinRegistry() (*Registry, error) {
	return NewRegistry(CatalogVersion, BuiltinPacks())
}

func NewRegistry(version string, packs []DomainPack) (*Registry, error) {
	if !semanticVersionPattern.MatchString(strings.TrimSpace(version)) {
		return nil, fmt.Errorf("catalog version must be semantic")
	}
	if len(packs) == 0 {
		return nil, fmt.Errorf("at least one domain pack is required")
	}

	byID := make(map[PackID]DomainPack, len(packs))
	ids := make([]PackID, 0, len(packs))
	for index := range packs {
		pack := clonePack(packs[index])
		if err := ValidatePack(pack); err != nil {
			return nil, fmt.Errorf("validate domain pack %q: %w", pack.ID, err)
		}
		if pack.Version != version {
			return nil, fmt.Errorf("domain pack %q version %q does not match catalog %q", pack.ID, pack.Version, version)
		}
		if _, exists := byID[pack.ID]; exists {
			return nil, fmt.Errorf("duplicate domain pack id %q", pack.ID)
		}
		byID[pack.ID] = pack
		ids = append(ids, pack.ID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	canonical := make([]DomainPack, 0, len(ids))
	for _, id := range ids {
		canonical = append(canonical, clonePack(byID[id]))
	}
	digest, err := catalogDigest(version, canonical)
	if err != nil {
		return nil, err
	}
	return &Registry{version: version, digest: digest, packs: byID, ids: ids}, nil
}

func (registry *Registry) Metadata() CatalogMetadata {
	if registry == nil {
		return CatalogMetadata{}
	}
	return CatalogMetadata{Version: registry.version, Digest: registry.digest, PackCount: len(registry.ids)}
}

func (registry *Registry) Lookup(id PackID) (DomainPack, bool) {
	if registry == nil {
		return DomainPack{}, false
	}
	pack, ok := registry.packs[id]
	if !ok {
		return DomainPack{}, false
	}
	return clonePack(pack), true
}

func (registry *Registry) List() []DomainPack {
	if registry == nil {
		return nil
	}
	result := make([]DomainPack, 0, len(registry.ids))
	for _, id := range registry.ids {
		result = append(result, clonePack(registry.packs[id]))
	}
	return result
}

func ValidatePack(pack DomainPack) error {
	if strings.TrimSpace(string(pack.ID)) == "" {
		return fmt.Errorf("id is required")
	}
	if !semanticVersionPattern.MatchString(pack.Version) {
		return fmt.Errorf("version must be semantic")
	}
	if strings.TrimSpace(pack.Name) == "" || strings.TrimSpace(pack.Description) == "" {
		return fmt.Errorf("name and description are required")
	}
	requiredCollections := map[string]int{
		"classification signals":         len(pack.ClassificationSignals),
		"intake questions":               len(pack.IntakeQuestions),
		"common entities":                len(pack.CommonEntities),
		"risk triggers":                  len(pack.RiskTriggers),
		"approval rules":                 len(pack.ApprovalRules),
		"prohibited autonomous actions":  len(pack.ProhibitedAutonomousActions),
		"source authority rules":         len(pack.SourceAuthorityRules),
		"evidence requirements":          len(pack.EvidenceRequirements),
		"deterministic validators":       len(pack.DeterministicValidators),
		"success criteria templates":     len(pack.SuccessCriteriaTemplates),
		"stop and escalation conditions": len(pack.StopEscalationConditions),
		"agent capabilities":             len(pack.SuitableAgentCapabilities),
		"audit events":                   len(pack.AuditEvents),
	}
	for name, count := range requiredCollections {
		if count == 0 {
			return fmt.Errorf("%s are required", name)
		}
	}
	if pack.Retention.DefaultDays <= 0 {
		return fmt.Errorf("positive retention period is required")
	}
	if pack.Sensitive && !pack.Retention.LocalOnly {
		return fmt.Errorf("sensitive packs must default to local-only retention")
	}
	if err := validateUniqueStrings("common entity", pack.CommonEntities); err != nil {
		return err
	}
	if err := validateUniqueStrings("agent capability", pack.SuitableAgentCapabilities); err != nil {
		return err
	}
	if err := validateUniqueStrings("audit event", pack.AuditEvents); err != nil {
		return err
	}
	if err := validateSignals(pack.ClassificationSignals); err != nil {
		return err
	}
	if err := validateApprovalPolicy(pack); err != nil {
		return err
	}
	if err := validateStructuredFields(pack); err != nil {
		return err
	}
	return nil
}

func validateSignals(signals []ClassificationSignal) error {
	seen := map[string]struct{}{}
	for _, signal := range signals {
		phrase := normalizeText(signal.Phrase)
		if phrase == "" || strings.TrimSpace(signal.Reason) == "" {
			return fmt.Errorf("classification signal phrase and reason are required")
		}
		if signal.Strength < SignalWeak || signal.Strength > SignalStrong {
			return fmt.Errorf("classification signal %q has invalid strength", signal.Phrase)
		}
		if _, exists := seen[phrase]; exists {
			return fmt.Errorf("duplicate classification signal %q", signal.Phrase)
		}
		seen[phrase] = struct{}{}
	}
	return nil
}

func validateApprovalPolicy(pack DomainPack) error {
	rules := make(map[string]ApprovalRule, len(pack.ApprovalRules))
	for _, rule := range pack.ApprovalRules {
		action := normalizeIdentifier(rule.Action)
		if action == "" || strings.TrimSpace(rule.Reason) == "" {
			return fmt.Errorf("approval action and reason are required")
		}
		if rule.MinimumRisk != RiskLow && rule.MinimumRisk != RiskMedium && rule.MinimumRisk != RiskHigh && rule.MinimumRisk != RiskCritical {
			return fmt.Errorf("approval rule %q has invalid risk", action)
		}
		if prior, exists := rules[action]; exists && prior.Required != rule.Required {
			return fmt.Errorf("conflicting approval rules for %q", action)
		}
		rules[action] = rule
	}
	for _, action := range mandatoryApprovalActions {
		rule, exists := rules[action]
		if !exists || !rule.Required {
			return fmt.Errorf("%s must require approval", action)
		}
	}

	prohibited := map[string]struct{}{}
	for _, rule := range pack.ProhibitedAutonomousActions {
		action := normalizeIdentifier(rule.Action)
		if action == "" || strings.TrimSpace(rule.Reason) == "" {
			return fmt.Errorf("prohibited action and reason are required")
		}
		if _, exists := prohibited[action]; exists {
			return fmt.Errorf("duplicate prohibited action %q", action)
		}
		prohibited[action] = struct{}{}
		if approval, exists := rules[action]; exists && !approval.Required {
			return fmt.Errorf("policy conflict: prohibited autonomous action %q is marked approval-free", action)
		}
	}
	return nil
}

func validateStructuredFields(pack DomainPack) error {
	questionIDs := map[string]struct{}{}
	for _, question := range pack.IntakeQuestions {
		if err := addStructuredID(questionIDs, question.ID, question.Question, "intake question"); err != nil {
			return err
		}
	}
	riskIDs := map[string]struct{}{}
	for _, trigger := range pack.RiskTriggers {
		if err := addStructuredID(riskIDs, trigger.ID, trigger.Signal, "risk trigger"); err != nil {
			return err
		}
		if trigger.Level != RiskLow && trigger.Level != RiskMedium && trigger.Level != RiskHigh && trigger.Level != RiskCritical {
			return fmt.Errorf("risk trigger %q has invalid level", trigger.ID)
		}
	}
	for _, rule := range pack.SourceAuthorityRules {
		if strings.TrimSpace(rule.ClaimType) == "" || len(rule.AcceptedSources) == 0 || rule.MinimumSources <= 0 || strings.TrimSpace(rule.Reason) == "" {
			return fmt.Errorf("source authority rules require claim type, sources, minimum, and reason")
		}
	}
	for _, requirement := range pack.EvidenceRequirements {
		if strings.TrimSpace(requirement.ID) == "" || strings.TrimSpace(requirement.Description) == "" || len(requirement.RequiredForActions) == 0 || strings.TrimSpace(requirement.MinimumVerification) == "" {
			return fmt.Errorf("evidence requirements must be complete")
		}
	}
	for _, validator := range pack.DeterministicValidators {
		if strings.TrimSpace(validator.ID) == "" || strings.TrimSpace(validator.Kind) == "" || strings.TrimSpace(validator.Description) == "" {
			return fmt.Errorf("deterministic validators must be complete")
		}
	}
	for _, template := range pack.SuccessCriteriaTemplates {
		if strings.TrimSpace(template.ID) == "" || len(template.Criteria) == 0 {
			return fmt.Errorf("success criteria templates must be complete")
		}
	}
	for _, condition := range pack.StopEscalationConditions {
		if strings.TrimSpace(condition.ID) == "" || strings.TrimSpace(condition.Condition) == "" || strings.TrimSpace(condition.EscalateTo) == "" {
			return fmt.Errorf("stop and escalation conditions must be complete")
		}
	}
	return nil
}

func addStructuredID(seen map[string]struct{}, id, value, kind string) error {
	id = normalizeIdentifier(id)
	if id == "" || strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s id and content are required", kind)
	}
	if _, exists := seen[id]; exists {
		return fmt.Errorf("duplicate %s id %q", kind, id)
	}
	seen[id] = struct{}{}
	return nil
}

func validateUniqueStrings(kind string, values []string) error {
	seen := map[string]struct{}{}
	for _, value := range values {
		normalized := normalizeIdentifier(value)
		if normalized == "" {
			return fmt.Errorf("%s cannot be empty", kind)
		}
		if _, exists := seen[normalized]; exists {
			return fmt.Errorf("duplicate %s %q", kind, value)
		}
		seen[normalized] = struct{}{}
	}
	return nil
}

func catalogDigest(version string, packs []DomainPack) (string, error) {
	payload := struct {
		Version string       `json:"version"`
		Packs   []DomainPack `json:"packs"`
	}{Version: version, Packs: packs}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode domain pack catalog: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func clonePacks(packs []DomainPack) []DomainPack {
	result := make([]DomainPack, len(packs))
	for index := range packs {
		result[index] = clonePack(packs[index])
	}
	return result
}

func clonePack(pack DomainPack) DomainPack {
	copy := pack
	copy.ClassificationSignals = cloneSignals(pack.ClassificationSignals)
	copy.IntakeQuestions = append([]IntakeQuestion(nil), pack.IntakeQuestions...)
	copy.CommonEntities = cloneStrings(pack.CommonEntities)
	copy.RiskTriggers = append([]RiskTrigger(nil), pack.RiskTriggers...)
	copy.ApprovalRules = append([]ApprovalRule(nil), pack.ApprovalRules...)
	copy.ProhibitedAutonomousActions = append([]ProhibitedAction(nil), pack.ProhibitedAutonomousActions...)
	copy.SourceAuthorityRules = make([]SourceAuthorityRule, len(pack.SourceAuthorityRules))
	for index, rule := range pack.SourceAuthorityRules {
		copy.SourceAuthorityRules[index] = rule
		copy.SourceAuthorityRules[index].AcceptedSources = cloneStrings(rule.AcceptedSources)
	}
	copy.EvidenceRequirements = make([]EvidenceRequirement, len(pack.EvidenceRequirements))
	for index, requirement := range pack.EvidenceRequirements {
		copy.EvidenceRequirements[index] = requirement
		copy.EvidenceRequirements[index].RequiredForActions = cloneStrings(requirement.RequiredForActions)
	}
	copy.DeterministicValidators = append([]DeterministicValidator(nil), pack.DeterministicValidators...)
	copy.SuccessCriteriaTemplates = make([]SuccessCriteriaTemplate, len(pack.SuccessCriteriaTemplates))
	for index, template := range pack.SuccessCriteriaTemplates {
		copy.SuccessCriteriaTemplates[index] = template
		copy.SuccessCriteriaTemplates[index].Criteria = cloneStrings(template.Criteria)
	}
	copy.StopEscalationConditions = append([]StopCondition(nil), pack.StopEscalationConditions...)
	copy.SuitableAgentCapabilities = cloneStrings(pack.SuitableAgentCapabilities)
	copy.AuditEvents = cloneStrings(pack.AuditEvents)
	return copy
}

func cloneSignals(signals []ClassificationSignal) []ClassificationSignal {
	return append([]ClassificationSignal(nil), signals...)
}

func cloneStrings(values []string) []string {
	return append([]string(nil), values...)
}

func normalizeIdentifier(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.Join(strings.Fields(value), " ")
}
