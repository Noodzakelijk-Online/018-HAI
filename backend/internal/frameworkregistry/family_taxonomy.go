package frameworkregistry

import (
	"fmt"
	"strings"
)

// FrameworkFamilyTaxonomyVersion versions the numbered family mapping from the
// operator-supplied HAI architecture specification independently of owner
// preferences and selector implementation changes.
const FrameworkFamilyTaxonomyVersion = "1.1.0"

// FrameworkFamilyRecord adds the specification's stable section number to the
// existing Framework contract. Framework remains the sole metadata source.
type FrameworkFamilyRecord struct {
	Section int `json:"section"`
	Framework
}

// FrameworkFamilyTaxonomy is an immutable-by-copy, content-addressed snapshot
// of all numbered framework families.
type FrameworkFamilyTaxonomy struct {
	Version  string                  `json:"version"`
	Digest   string                  `json:"digest"`
	Families []FrameworkFamilyRecord `json:"families"`
}

type frameworkFamilyTaxonomyFingerprint struct {
	Version  string                  `json:"version"`
	Families []FrameworkFamilyRecord `json:"families"`
}

// BuiltinFamilyTaxonomy returns a fresh snapshot derived from BuiltinCatalog.
// It deliberately does not maintain a second set of family definitions.
func BuiltinFamilyTaxonomy() (*FrameworkFamilyTaxonomy, error) {
	return buildFrameworkFamilyTaxonomy(BuiltinCatalog())
}

// FamilyTaxonomy exposes the exact taxonomy used by this service. The returned
// value does not share mutable slices with the service's internal catalog.
func (s *Service) FamilyTaxonomy() (*FrameworkFamilyTaxonomy, error) {
	if s == nil {
		return nil, fmt.Errorf("framework registry service is required")
	}
	return buildFrameworkFamilyTaxonomy(s.activeCatalogSnapshot())
}

func buildFrameworkFamilyTaxonomy(frameworks []Framework) (*FrameworkFamilyTaxonomy, error) {
	if err := ValidateCatalog(frameworks); err != nil {
		return nil, err
	}

	families := make([]FrameworkFamilyRecord, len(frameworks))
	for index, framework := range frameworks {
		families[index] = FrameworkFamilyRecord{
			Section:   index + 1,
			Framework: cloneFramework(framework),
		}
	}

	digest, err := familyTaxonomyDigest(FrameworkFamilyTaxonomyVersion, families)
	if err != nil {
		return nil, err
	}
	result := &FrameworkFamilyTaxonomy{
		Version:  FrameworkFamilyTaxonomyVersion,
		Digest:   digest,
		Families: families,
	}
	if err := ValidateFamilyTaxonomy(*result); err != nil {
		return nil, err
	}
	return result, nil
}

// ValidateFamilyTaxonomy verifies the sequential section mapping, underlying
// catalog governance contract, semantic version, and content digest.
func ValidateFamilyTaxonomy(taxonomy FrameworkFamilyTaxonomy) error {
	version := strings.TrimSpace(taxonomy.Version)
	if !catalogSemanticVersionPattern.MatchString(version) {
		return fmt.Errorf("framework family taxonomy has invalid semantic version %q", taxonomy.Version)
	}
	if len(taxonomy.Families) != 55 {
		return fmt.Errorf("framework family taxonomy must contain 55 entries, got %d", len(taxonomy.Families))
	}

	frameworks := make([]Framework, len(taxonomy.Families))
	seenSections := make(map[int]struct{}, len(taxonomy.Families))
	for index, family := range taxonomy.Families {
		expectedSection := index + 1
		if family.Section != expectedSection {
			return fmt.Errorf(
				"framework family at index %d has section %d, want sequential section %d",
				index,
				family.Section,
				expectedSection,
			)
		}
		if _, exists := seenSections[family.Section]; exists {
			return fmt.Errorf("framework family taxonomy contains duplicate section %d", family.Section)
		}
		seenSections[family.Section] = struct{}{}
		frameworks[index] = cloneFramework(family.Framework)
	}
	if err := ValidateCatalog(frameworks); err != nil {
		return fmt.Errorf("framework family taxonomy catalog: %w", err)
	}

	expectedDigest, err := familyTaxonomyDigest(version, taxonomy.Families)
	if err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(taxonomy.Digest), expectedDigest) {
		return fmt.Errorf("framework family taxonomy digest does not match its content")
	}
	return nil
}

func familyTaxonomyDigest(version string, families []FrameworkFamilyRecord) (string, error) {
	copied := make([]FrameworkFamilyRecord, len(families))
	for index, family := range families {
		copied[index] = FrameworkFamilyRecord{
			Section:   family.Section,
			Framework: cloneFramework(family.Framework),
		}
	}
	digest, err := canonicalSHA256(frameworkFamilyTaxonomyFingerprint{
		Version:  strings.TrimSpace(version),
		Families: copied,
	})
	if err != nil {
		return "", fmt.Errorf("digest framework family taxonomy: %w", err)
	}
	return digest, nil
}

func cloneFramework(framework Framework) Framework {
	cloned := framework
	cloned.SuitableProblemTypes = append([]string(nil), framework.SuitableProblemTypes...)
	cloned.TriggerConditions = append([]string(nil), framework.TriggerConditions...)
	cloned.RequiredInputs = append([]string(nil), framework.RequiredInputs...)
	cloned.ProducedOutputs = append([]string(nil), framework.ProducedOutputs...)
	cloned.RequiredAgents = append([]string(nil), framework.RequiredAgents...)
	cloned.WorkflowTemplate = append([]string(nil), framework.WorkflowTemplate...)
	cloned.DecisionRules = append([]string(nil), framework.DecisionRules...)
	cloned.SafetyInvariants = append([]string(nil), framework.SafetyInvariants...)
	cloned.EvidenceRequirements = append([]string(nil), framework.EvidenceRequirements...)
	cloned.EvaluationMethod = append([]string(nil), framework.EvaluationMethod...)
	cloned.ConflictsWith = append([]string(nil), framework.ConflictsWith...)
	cloned.UserSpecificAdaptations = append([]string(nil), framework.UserSpecificAdaptations...)
	cloned.CandidateImplementations = append([]string(nil), framework.CandidateImplementations...)
	return cloned
}
