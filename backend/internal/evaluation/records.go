package evaluation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"
)

var (
	ErrInvalidDataset = errors.New("evaluation: invalid dataset")
	ErrInvalidRun     = errors.New("evaluation: invalid run")
)

func NewDataset(spec DatasetSpec) (Dataset, error) {
	dataset := Dataset{
		SchemaVersion: DatasetSchemaVersion,
		ID:            strings.TrimSpace(spec.ID),
		Version:       spec.Version,
		Name:          strings.TrimSpace(spec.Name),
		Description:   strings.TrimSpace(spec.Description),
		Cases:         cloneCases(spec.Cases),
		CreatedAt:     spec.CreatedAt.UTC(),
	}
	if err := normalizeDataset(&dataset); err != nil {
		return Dataset{}, err
	}
	dataset.Digest = datasetDigest(dataset)
	return dataset, nil
}

func ValidateDataset(dataset Dataset) error {
	copy := cloneDataset(dataset)
	expected := strings.ToLower(strings.TrimSpace(copy.Digest))
	copy.Digest = ""
	if err := normalizeDataset(&copy); err != nil {
		return err
	}
	if !validDigest(expected) || datasetDigest(copy) != expected {
		return fmt.Errorf("%w: digest mismatch", ErrInvalidDataset)
	}
	return nil
}

func NewRunRecord(spec RunSpec) (RunRecord, error) {
	if err := ValidateDataset(spec.Dataset); err != nil {
		return RunRecord{}, fmt.Errorf("%w: %v", ErrInvalidRun, err)
	}
	record := RunRecord{
		SchemaVersion: RunSchemaVersion,
		ID:            strings.TrimSpace(spec.ID),
		Dataset: DatasetRef{
			ID:      spec.Dataset.ID,
			Version: spec.Dataset.Version,
			Digest:  spec.Dataset.Digest,
		},
		Evaluator:     trimEvaluator(spec.Evaluator),
		Subject:       trimSubject(spec.Subject),
		Mode:          spec.Mode,
		CanaryPercent: spec.CanaryPercent,
		BaselineRunID: strings.TrimSpace(spec.BaselineRunID),
		StartedAt:     spec.StartedAt.UTC(),
		CompletedAt:   spec.CompletedAt.UTC(),
		Status:        spec.Status,
		FailureCode:   strings.TrimSpace(spec.FailureCode),
	}
	config, err := canonicalJSON(spec.Config)
	if err != nil {
		return RunRecord{}, fmt.Errorf("%w: invalid evaluator config: %v", ErrInvalidRun, err)
	}
	record.Reproducibility = ReproducibilityManifest{
		Dataset:           record.Dataset,
		Evaluator:         record.Evaluator,
		Subject:           record.Subject,
		Seed:              spec.Seed,
		ConfigDigest:      digestBytes(config),
		EnvironmentDigest: strings.ToLower(strings.TrimSpace(spec.EnvironmentDigest)),
	}
	if spec.Status == RunStatusCompleted {
		results, metrics, err := buildResults(spec.Dataset, spec.Observations)
		if err != nil {
			return RunRecord{}, err
		}
		record.CaseResults = results
		record.OverallScore = metrics.overallScore
		record.CasePassRate = metrics.casePassRate
		record.RequiredFailureCount = metrics.requiredFailures
		record.CriterionErrorCount = metrics.criterionErrors
	}
	record.ReproducibilityDigest = digestJSON(record.Reproducibility)
	if err := validateRunShape(record); err != nil {
		return RunRecord{}, err
	}
	record.RecordDigest = runDigest(record)
	return record, nil
}

func ValidateRunRecord(record RunRecord) error {
	copy := cloneRun(record)
	expected := strings.ToLower(strings.TrimSpace(copy.RecordDigest))
	copy.RecordDigest = ""
	if err := validateRunShape(copy); err != nil {
		return err
	}
	if copy.ReproducibilityDigest != digestJSON(copy.Reproducibility) {
		return fmt.Errorf("%w: reproducibility digest mismatch", ErrInvalidRun)
	}
	if !validDigest(expected) || runDigest(copy) != expected {
		return fmt.Errorf("%w: record digest mismatch", ErrInvalidRun)
	}
	if copy.Status == RunStatusCompleted {
		if err := validateResultSnapshots(copy.CaseResults); err != nil {
			return err
		}
		metrics := metricsFromResults(copy.CaseResults)
		if !sameFloat(metrics.overallScore, copy.OverallScore) ||
			!sameFloat(metrics.casePassRate, copy.CasePassRate) ||
			metrics.requiredFailures != copy.RequiredFailureCount ||
			metrics.criterionErrors != copy.CriterionErrorCount {
			return fmt.Errorf("%w: aggregate metrics do not match criterion results", ErrInvalidRun)
		}
	}
	return nil
}

func validateRunAgainstDataset(record RunRecord, dataset Dataset) error {
	if record.Dataset != (DatasetRef{ID: dataset.ID, Version: dataset.Version, Digest: dataset.Digest}) {
		return fmt.Errorf("%w: run does not reference the supplied dataset version", ErrInvalidRun)
	}
	if record.Status != RunStatusCompleted {
		return nil
	}
	if len(record.CaseResults) != len(dataset.Cases) {
		return fmt.Errorf("%w: result count does not match the dataset", ErrInvalidRun)
	}
	results := make(map[string]CaseResult, len(record.CaseResults))
	for _, result := range record.CaseResults {
		results[caseKey(result.CaseID, result.CaseVersion)] = result
	}
	for _, evalCase := range dataset.Cases {
		result, ok := results[caseKey(evalCase.ID, evalCase.Version)]
		if !ok || len(result.Criteria) != len(evalCase.Criteria) {
			return fmt.Errorf("%w: run results do not match dataset case %q", ErrInvalidRun, evalCase.ID)
		}
		criteria := make(map[string]CriterionResult, len(result.Criteria))
		for _, criterion := range result.Criteria {
			criteria[criterion.CriterionID] = criterion
		}
		for _, expected := range evalCase.Criteria {
			actual, ok := criteria[expected.ID]
			if !ok || actual.Required != expected.Required || !sameFloat(actual.Weight, expected.Weight) ||
				!sameFloat(actual.MinScore, expected.MinScore) {
				return fmt.Errorf("%w: run criterion snapshot does not match dataset criterion %q", ErrInvalidRun, expected.ID)
			}
		}
	}
	return nil
}

func validateResultSnapshots(results []CaseResult) error {
	seenCases := make(map[string]struct{}, len(results))
	for _, result := range results {
		key := caseKey(result.CaseID, result.CaseVersion)
		if !validID(result.CaseID) || result.CaseVersion == 0 || len(result.Criteria) == 0 {
			return fmt.Errorf("%w: invalid case result %q", ErrInvalidRun, key)
		}
		if _, exists := seenCases[key]; exists {
			return fmt.Errorf("%w: duplicate case result %q", ErrInvalidRun, key)
		}
		seenCases[key] = struct{}{}
		seenCriteria := make(map[string]struct{}, len(result.Criteria))
		var weightedScore, totalWeight float64
		passed := true
		for _, criterion := range result.Criteria {
			if !validID(criterion.CriterionID) || criterion.Weight <= 0 ||
				math.IsNaN(criterion.Weight) || math.IsInf(criterion.Weight, 0) ||
				criterion.MinScore < 0 || criterion.MinScore > 1 ||
				math.IsNaN(criterion.MinScore) || math.IsInf(criterion.MinScore, 0) {
				return fmt.Errorf("%w: invalid criterion result %q", ErrInvalidRun, criterion.CriterionID)
			}
			if _, exists := seenCriteria[criterion.CriterionID]; exists {
				return fmt.Errorf("%w: duplicate criterion result %q", ErrInvalidRun, criterion.CriterionID)
			}
			seenCriteria[criterion.CriterionID] = struct{}{}
			observation := CriterionObservation{
				CriterionID: criterion.CriterionID, Status: criterion.Status,
				Score: criterion.Score, EvidenceDigest: criterion.EvidenceDigest,
			}
			definition := Criterion{
				ID: criterion.CriterionID, Required: criterion.Required,
				Weight: criterion.Weight, MinScore: criterion.MinScore,
			}
			if err := validateObservation(definition, observation); err != nil {
				return err
			}
			totalWeight += criterion.Weight
			if criterion.Status != CriterionError {
				weightedScore += criterion.Score * criterion.Weight
			}
			if criterion.Status == CriterionError || (criterion.Required && criterion.Status != CriterionPassed) {
				passed = false
			}
		}
		if !sameFloat(result.Score, weightedScore/totalWeight) || result.Passed != passed {
			return fmt.Errorf("%w: case result aggregates do not match criterion results", ErrInvalidRun)
		}
	}
	return nil
}

type runMetrics struct {
	overallScore     float64
	casePassRate     float64
	requiredFailures int
	criterionErrors  int
}

func buildResults(dataset Dataset, observations []CaseObservation) ([]CaseResult, runMetrics, error) {
	if len(observations) != len(dataset.Cases) {
		return nil, runMetrics{}, fmt.Errorf("%w: completed run must contain every dataset case", ErrInvalidRun)
	}
	byCase := make(map[string]CaseObservation, len(observations))
	for _, observation := range observations {
		key := caseKey(observation.CaseID, observation.CaseVersion)
		if _, exists := byCase[key]; exists {
			return nil, runMetrics{}, fmt.Errorf("%w: duplicate case observation %q", ErrInvalidRun, key)
		}
		byCase[key] = observation
	}
	results := make([]CaseResult, 0, len(dataset.Cases))
	for _, evalCase := range dataset.Cases {
		observation, ok := byCase[caseKey(evalCase.ID, evalCase.Version)]
		if !ok {
			return nil, runMetrics{}, fmt.Errorf("%w: missing case observation %q", ErrInvalidRun, evalCase.ID)
		}
		result, err := buildCaseResult(evalCase, observation)
		if err != nil {
			return nil, runMetrics{}, err
		}
		results = append(results, result)
	}
	return results, metricsFromResults(results), nil
}

func buildCaseResult(evalCase EvaluationCase, observation CaseObservation) (CaseResult, error) {
	if len(observation.Criteria) != len(evalCase.Criteria) {
		return CaseResult{}, fmt.Errorf("%w: case %q must contain every criterion", ErrInvalidRun, evalCase.ID)
	}
	observed := make(map[string]CriterionObservation, len(observation.Criteria))
	for _, item := range observation.Criteria {
		item.CriterionID = strings.TrimSpace(item.CriterionID)
		if _, exists := observed[item.CriterionID]; exists {
			return CaseResult{}, fmt.Errorf("%w: duplicate criterion observation %q", ErrInvalidRun, item.CriterionID)
		}
		observed[item.CriterionID] = item
	}
	result := CaseResult{CaseID: evalCase.ID, CaseVersion: evalCase.Version, Passed: true}
	var weightedScore, totalWeight float64
	for _, criterion := range evalCase.Criteria {
		item, ok := observed[criterion.ID]
		if !ok {
			return CaseResult{}, fmt.Errorf("%w: missing criterion observation %q", ErrInvalidRun, criterion.ID)
		}
		if err := validateObservation(criterion, item); err != nil {
			return CaseResult{}, err
		}
		criterionResult := CriterionResult{
			CriterionID:    criterion.ID,
			Status:         item.Status,
			Score:          item.Score,
			Required:       criterion.Required,
			Weight:         criterion.Weight,
			MinScore:       criterion.MinScore,
			EvidenceDigest: strings.ToLower(strings.TrimSpace(item.EvidenceDigest)),
			Detail:         strings.TrimSpace(item.Detail),
		}
		result.Criteria = append(result.Criteria, criterionResult)
		totalWeight += criterion.Weight
		if item.Status != CriterionError {
			weightedScore += item.Score * criterion.Weight
		}
		if item.Status == CriterionError || (criterion.Required && item.Status != CriterionPassed) {
			result.Passed = false
		}
	}
	result.Score = weightedScore / totalWeight
	return result, nil
}

func validateObservation(criterion Criterion, observation CriterionObservation) error {
	if math.IsNaN(observation.Score) || math.IsInf(observation.Score, 0) || observation.Score < 0 || observation.Score > 1 {
		return fmt.Errorf("%w: criterion %q score must be between 0 and 1", ErrInvalidRun, criterion.ID)
	}
	switch observation.Status {
	case CriterionPassed:
		if observation.Score < criterion.MinScore {
			return fmt.Errorf("%w: criterion %q is marked passed below its threshold", ErrInvalidRun, criterion.ID)
		}
	case CriterionFailed:
		if observation.Score >= criterion.MinScore {
			return fmt.Errorf("%w: criterion %q is marked failed at or above its threshold", ErrInvalidRun, criterion.ID)
		}
	case CriterionError:
	default:
		return fmt.Errorf("%w: criterion %q has an unknown status", ErrInvalidRun, criterion.ID)
	}
	if digest := strings.TrimSpace(observation.EvidenceDigest); digest != "" && !validDigest(digest) {
		return fmt.Errorf("%w: criterion %q has an invalid evidence digest", ErrInvalidRun, criterion.ID)
	}
	return nil
}

func metricsFromResults(results []CaseResult) runMetrics {
	var weightedScore, totalWeight float64
	metrics := runMetrics{}
	for _, result := range results {
		if result.Passed {
			metrics.casePassRate++
		}
		for _, criterion := range result.Criteria {
			totalWeight += criterion.Weight
			if criterion.Status != CriterionError {
				weightedScore += criterion.Score * criterion.Weight
			}
			if criterion.Required && criterion.Status != CriterionPassed {
				metrics.requiredFailures++
			}
			if criterion.Status == CriterionError {
				metrics.criterionErrors++
			}
		}
	}
	if totalWeight > 0 {
		metrics.overallScore = weightedScore / totalWeight
	}
	if len(results) > 0 {
		metrics.casePassRate /= float64(len(results))
	}
	return metrics
}

func normalizeDataset(dataset *Dataset) error {
	if dataset.SchemaVersion != DatasetSchemaVersion || !validID(dataset.ID) || dataset.Version == 0 ||
		dataset.Name == "" || dataset.CreatedAt.IsZero() || len(dataset.Cases) == 0 {
		return fmt.Errorf("%w: schema, id, version, name, created time, and cases are required", ErrInvalidDataset)
	}
	seenCases := make(map[string]struct{}, len(dataset.Cases))
	for caseIndex := range dataset.Cases {
		evalCase := &dataset.Cases[caseIndex]
		evalCase.ID = strings.TrimSpace(evalCase.ID)
		if !validID(evalCase.ID) || evalCase.Version == 0 || len(evalCase.Criteria) == 0 {
			return fmt.Errorf("%w: each case requires an id, version, and criteria", ErrInvalidDataset)
		}
		key := caseKey(evalCase.ID, evalCase.Version)
		if _, exists := seenCases[key]; exists {
			return fmt.Errorf("%w: duplicate case %q", ErrInvalidDataset, key)
		}
		seenCases[key] = struct{}{}
		var err error
		if evalCase.Input, err = canonicalJSON(evalCase.Input); err != nil {
			return fmt.Errorf("%w: case %q input: %v", ErrInvalidDataset, evalCase.ID, err)
		}
		if evalCase.Expected, err = canonicalJSON(evalCase.Expected); err != nil {
			return fmt.Errorf("%w: case %q expected: %v", ErrInvalidDataset, evalCase.ID, err)
		}
		seenCriteria := make(map[string]struct{}, len(evalCase.Criteria))
		for criterionIndex := range evalCase.Criteria {
			criterion := &evalCase.Criteria[criterionIndex]
			criterion.ID = strings.TrimSpace(criterion.ID)
			criterion.Description = strings.TrimSpace(criterion.Description)
			if !validID(criterion.ID) || criterion.Weight <= 0 || math.IsNaN(criterion.Weight) ||
				math.IsInf(criterion.Weight, 0) || criterion.MinScore < 0 || criterion.MinScore > 1 ||
				math.IsNaN(criterion.MinScore) || math.IsInf(criterion.MinScore, 0) {
				return fmt.Errorf("%w: invalid criterion %q", ErrInvalidDataset, criterion.ID)
			}
			if _, exists := seenCriteria[criterion.ID]; exists {
				return fmt.Errorf("%w: duplicate criterion %q", ErrInvalidDataset, criterion.ID)
			}
			seenCriteria[criterion.ID] = struct{}{}
		}
	}
	dataset.CreatedAt = dataset.CreatedAt.UTC()
	return nil
}

func validateRunShape(record RunRecord) error {
	if record.SchemaVersion != RunSchemaVersion || !validID(record.ID) || !validID(record.Dataset.ID) ||
		record.Dataset.Version == 0 || !validDigest(record.Dataset.Digest) ||
		!validID(record.Evaluator.ID) || strings.TrimSpace(record.Evaluator.Version) == "" ||
		!validID(record.Subject.ID) || strings.TrimSpace(record.Subject.Version) == "" ||
		!validDigest(record.Subject.ArtifactDigest) || record.StartedAt.IsZero() ||
		record.CompletedAt.IsZero() || record.CompletedAt.Before(record.StartedAt) ||
		!validDigest(record.Reproducibility.ConfigDigest) ||
		!validDigest(record.Reproducibility.EnvironmentDigest) {
		return fmt.Errorf("%w: required identity, time, and digest fields are invalid", ErrInvalidRun)
	}
	if record.Reproducibility.Dataset != record.Dataset ||
		record.Reproducibility.Evaluator != record.Evaluator ||
		record.Reproducibility.Subject != record.Subject {
		return fmt.Errorf("%w: reproducibility manifest does not match the run", ErrInvalidRun)
	}
	switch record.Mode {
	case RunModeShadow:
		if record.CanaryPercent != 0 {
			return fmt.Errorf("%w: shadow runs cannot have canary exposure", ErrInvalidRun)
		}
	case RunModeCanary:
		if record.CanaryPercent <= 0 || record.CanaryPercent > 100 || math.IsNaN(record.CanaryPercent) {
			return fmt.Errorf("%w: canary exposure must be greater than 0 and at most 100", ErrInvalidRun)
		}
	default:
		return fmt.Errorf("%w: unknown run mode", ErrInvalidRun)
	}
	switch record.Status {
	case RunStatusCompleted:
		if record.FailureCode != "" || len(record.CaseResults) == 0 {
			return fmt.Errorf("%w: completed runs require results and no failure code", ErrInvalidRun)
		}
	case RunStatusFailed:
		if record.FailureCode == "" || len(record.CaseResults) != 0 ||
			record.OverallScore != 0 || record.CasePassRate != 0 ||
			record.RequiredFailureCount != 0 || record.CriterionErrorCount != 0 {
			return fmt.Errorf("%w: failed runs require a failure code and no claimed results", ErrInvalidRun)
		}
	default:
		return fmt.Errorf("%w: unknown run status", ErrInvalidRun)
	}
	return nil
}

func datasetDigest(dataset Dataset) string {
	dataset.Digest = ""
	dataset.CreatedAt = time.Time{}
	return digestJSON(struct {
		SchemaVersion uint32           `json:"schemaVersion"`
		ID            string           `json:"id"`
		Version       uint32           `json:"version"`
		Name          string           `json:"name"`
		Description   string           `json:"description,omitempty"`
		Cases         []EvaluationCase `json:"cases"`
	}{
		SchemaVersion: dataset.SchemaVersion,
		ID:            dataset.ID, Version: dataset.Version, Name: dataset.Name,
		Description: dataset.Description, Cases: dataset.Cases,
	})
}

func runDigest(record RunRecord) string {
	record.RecordDigest = ""
	return digestJSON(record)
}

func canonicalJSON(raw json.RawMessage) ([]byte, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = json.RawMessage(`{}`)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("multiple JSON values")
		}
		return nil, err
	}
	return json.Marshal(value)
}

func digestJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("evaluation: internal digest encoding failed: %v", err))
	}
	return digestBytes(encoded)
}

func digestBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func validDigest(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func validID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			strings.ContainsRune("._:/-", character)) {
			return false
		}
	}
	return true
}

func caseKey(id string, version uint32) string {
	return fmt.Sprintf("%s@%d", strings.TrimSpace(id), version)
}

func trimEvaluator(value EvaluatorDescriptor) EvaluatorDescriptor {
	value.ID = strings.TrimSpace(value.ID)
	value.Version = strings.TrimSpace(value.Version)
	return value
}

func trimSubject(value SubjectDescriptor) SubjectDescriptor {
	value.ID = strings.TrimSpace(value.ID)
	value.Version = strings.TrimSpace(value.Version)
	value.ArtifactDigest = strings.ToLower(strings.TrimSpace(value.ArtifactDigest))
	return value
}

func sameFloat(left, right float64) bool {
	return math.Abs(left-right) <= 1e-12
}

func cloneDataset(dataset Dataset) Dataset {
	copy := dataset
	copy.Cases = cloneCases(dataset.Cases)
	return copy
}

func cloneCases(cases []EvaluationCase) []EvaluationCase {
	result := make([]EvaluationCase, len(cases))
	for index, evalCase := range cases {
		result[index] = evalCase
		result[index].Input = append(json.RawMessage(nil), evalCase.Input...)
		result[index].Expected = append(json.RawMessage(nil), evalCase.Expected...)
		result[index].Criteria = append([]Criterion(nil), evalCase.Criteria...)
	}
	return result
}

func cloneRun(record RunRecord) RunRecord {
	copy := record
	copy.CaseResults = make([]CaseResult, len(record.CaseResults))
	for index, result := range record.CaseResults {
		copy.CaseResults[index] = result
		copy.CaseResults[index].Criteria = append([]CriterionResult(nil), result.Criteria...)
	}
	return copy
}

func sortRuns(records []RunRecord) {
	sort.Slice(records, func(left, right int) bool {
		if records[left].CompletedAt.Equal(records[right].CompletedAt) {
			return records[left].ID < records[right].ID
		}
		return records[left].CompletedAt.After(records[right].CompletedAt)
	})
}
