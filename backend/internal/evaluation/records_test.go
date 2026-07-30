package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDatasetVersionsAreCanonicalAndContentAddressed(t *testing.T) {
	first := testDataset(t, 1, json.RawMessage(`{"b":2,"a":1}`))
	equivalent := testDataset(t, 1, json.RawMessage("{\n\"a\":1,\"b\":2}"))
	if first.Digest != equivalent.Digest {
		t.Fatalf("canonical inputs produced different digests: %s != %s", first.Digest, equivalent.Digest)
	}
	second := testDataset(t, 2, json.RawMessage(`{"a":1,"b":2}`))
	if first.Digest == second.Digest {
		t.Fatal("dataset version change must change the content digest")
	}
	tampered := first
	tampered.Cases[0].Expected = json.RawMessage(`{"answer":"wrong"}`)
	if err := ValidateDataset(tampered); !errors.Is(err, ErrInvalidDataset) {
		t.Fatalf("tampered dataset accepted: %v", err)
	}
}

func TestRunRecordDerivesMetricsAndDetectsTampering(t *testing.T) {
	dataset := testDataset(t, 1, json.RawMessage(`{"prompt":"hello"}`))
	record := testRun(t, dataset, RunModeCanary, "baseline-1", 10, CriterionPassed, 0.95)
	if record.OverallScore != 0.95 || record.CasePassRate != 1 ||
		record.RequiredFailureCount != 0 || record.CriterionErrorCount != 0 {
		t.Fatalf("unexpected derived metrics: %#v", record)
	}
	if err := ValidateRunRecord(record); err != nil {
		t.Fatalf("valid run rejected: %v", err)
	}
	tampered := cloneRun(record)
	tampered.CaseResults[0].Criteria[0].Score = 0.1
	if err := ValidateRunRecord(tampered); !errors.Is(err, ErrInvalidRun) {
		t.Fatalf("tampered run accepted: %v", err)
	}
	forged := cloneRun(record)
	forged.CaseResults[0].Criteria[0].Score = 0.1
	forged.CaseResults[0].Score = 0.1
	forged.OverallScore = 0.1
	forged.RecordDigest = runDigest(forged)
	if err := ValidateRunRecord(forged); !errors.Is(err, ErrInvalidRun) {
		t.Fatalf("internally inconsistent re-digested run accepted: %v", err)
	}

	differentSeed := testRun(t, dataset, RunModeCanary, "baseline-1", 11, CriterionPassed, 0.95)
	if record.ReproducibilityDigest == differentSeed.ReproducibilityDigest {
		t.Fatal("seed change must alter the reproducibility digest")
	}
}

func TestRunRecordRejectsIncompleteAndInconsistentObservations(t *testing.T) {
	dataset := testDataset(t, 1, json.RawMessage(`{"prompt":"hello"}`))
	spec := testRunSpec(dataset, RunModeShadow, "", 1, CriterionPassed, 0.95)
	spec.Observations = nil
	if _, err := NewRunRecord(spec); !errors.Is(err, ErrInvalidRun) {
		t.Fatalf("incomplete run accepted: %v", err)
	}
	spec = testRunSpec(dataset, RunModeShadow, "", 1, CriterionPassed, 0.5)
	if _, err := NewRunRecord(spec); !errors.Is(err, ErrInvalidRun) {
		t.Fatalf("criterion marked passed below threshold was accepted: %v", err)
	}
	spec = testRunSpec(dataset, RunModeShadow, "", 1, CriterionPassed, 0.95)
	spec.Config = json.RawMessage(`{"temperature":0} {"extra":true}`)
	if _, err := NewRunRecord(spec); !errors.Is(err, ErrInvalidRun) {
		t.Fatalf("trailing evaluator config JSON was accepted: %v", err)
	}
}

func TestMemoryRepositoryIsAppendOnlyAndReturnsDefensiveCopies(t *testing.T) {
	ctx := context.Background()
	repository := NewMemoryRepository()
	const owner = "alice"
	dataset := testDataset(t, 1, json.RawMessage(`{"prompt":"hello"}`))
	if err := repository.CreateDataset(ctx, owner, dataset); err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	if err := repository.CreateDataset(ctx, owner, dataset); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("duplicate dataset did not fail: %v", err)
	}
	storedDataset, err := repository.GetDataset(ctx, owner, dataset.ID, dataset.Version)
	if err != nil {
		t.Fatalf("get dataset: %v", err)
	}
	storedDataset.Cases[0].Input[0] = '['
	again, _ := repository.GetDataset(ctx, owner, dataset.ID, dataset.Version)
	if string(again.Cases[0].Input) != `{"prompt":"hello"}` {
		t.Fatal("caller mutated the repository dataset")
	}

	baseline := testRun(t, dataset, RunModeShadow, "", 1, CriterionPassed, 0.9)
	baseline.ID = "baseline-1"
	baseline.RecordDigest = runDigest(baseline)
	if err := repository.CreateRun(ctx, owner, baseline); err != nil {
		t.Fatalf("create baseline: %v", err)
	}
	candidate := testRun(t, dataset, RunModeCanary, baseline.ID, 2, CriterionPassed, 0.95)
	if err := repository.CreateRun(ctx, owner, candidate); err != nil {
		t.Fatalf("create candidate: %v", err)
	}
	stored, err := repository.GetRun(ctx, owner, candidate.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	stored.CaseResults[0].Criteria[0].Detail = "mutated"
	againRun, _ := repository.GetRun(ctx, owner, candidate.ID)
	if againRun.CaseResults[0].Criteria[0].Detail == "mutated" {
		t.Fatal("caller mutated the repository run")
	}
	if err := repository.CreateRun(ctx, owner, candidate); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("duplicate run did not fail: %v", err)
	}
}

func TestMemoryRepositoryHonorsCancellationAndFilters(t *testing.T) {
	repository := NewMemoryRepository()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := repository.ListRuns(cancelled, "alice", RunQuery{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled query returned %v", err)
	}

	ctx := context.Background()
	dataset := testDataset(t, 1, json.RawMessage(`{"prompt":"hello"}`))
	_ = repository.CreateDataset(ctx, "alice", dataset)
	run := testRun(t, dataset, RunModeShadow, "", 1, CriterionPassed, 0.9)
	_ = repository.CreateRun(ctx, "alice", run)
	matches, err := repository.ListRuns(ctx, "alice", RunQuery{DatasetID: dataset.ID, SubjectID: "candidate", Mode: RunModeShadow})
	if err != nil || len(matches) != 1 {
		t.Fatalf("filtered list = %#v, %v", matches, err)
	}
	misses, err := repository.ListRuns(ctx, "alice", RunQuery{SubjectID: "other"})
	if err != nil || len(misses) != 0 {
		t.Fatalf("unexpected filtered results = %#v, %v", misses, err)
	}
}

func TestMemoryRepositoryOwnerIsolationAndReceiptRoundTrip(t *testing.T) {
	ctx := context.Background()
	repository := NewMemoryRepository()
	dataset := testDataset(t, 1, json.RawMessage(`{"prompt":"hello"}`))
	for _, owner := range []string{"alice", "bob"} {
		if err := repository.CreateDataset(ctx, owner, dataset); err != nil {
			t.Fatalf("create %s dataset: %v", owner, err)
		}
	}

	baseline := testRun(t, dataset, RunModeShadow, "", 1, CriterionPassed, 0.9)
	baseline.ID = "baseline"
	baseline.RecordDigest = runDigest(baseline)
	candidate := testRun(t, dataset, RunModeCanary, baseline.ID, 2, CriterionPassed, 0.95)
	if err := repository.CreateRun(ctx, "alice", baseline); err != nil {
		t.Fatalf("create baseline: %v", err)
	}
	if err := repository.CreateRun(ctx, "alice", candidate); err != nil {
		t.Fatalf("create candidate: %v", err)
	}
	if _, err := repository.GetRun(ctx, "bob", candidate.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob read alice run: %v", err)
	}
	if err := repository.CreateRun(ctx, "bob", candidate); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob referenced alice baseline: %v", err)
	}

	thresholds := RegressionThresholds{
		MinOverallScore: 0.8, MinCasePassRate: 1,
		MaxOverallScoreDrop: 0.1, MaxCasePassRateDrop: 0,
		MaxRequiredFailures: 0, MaxCriterionErrors: 0,
	}
	comparison, err := NewBaselineComparisonReceipt(BaselineComparisonReceiptSpec{
		ID: "comparison-1", Candidate: candidate, Baseline: baseline,
		Thresholds: thresholds, CreatedAt: time.Date(2026, 7, 30, 13, 0, 0, 1, time.UTC),
	})
	if err != nil {
		t.Fatalf("new comparison receipt: %v", err)
	}
	if err := repository.CreateComparisonReceipt(ctx, "alice", comparison); err != nil {
		t.Fatalf("create comparison receipt: %v", err)
	}
	promotion, err := NewPromotionDecisionReceipt(PromotionDecisionReceiptSpec{
		ID: "promotion-1", Candidate: candidate, Baseline: &baseline,
		ComparisonReceiptID: comparison.ID,
		Thresholds:          thresholds,
		CreatedAt:           time.Date(2026, 7, 30, 13, 1, 0, 2, time.UTC),
	})
	if err != nil {
		t.Fatalf("new promotion receipt: %v", err)
	}
	if err := repository.CreatePromotionDecisionReceipt(ctx, "alice", promotion); err != nil {
		t.Fatalf("create promotion receipt: %v", err)
	}
	if err := repository.CreatePromotionDecisionReceipt(ctx, "alice", promotion); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("duplicate promotion receipt did not fail: %v", err)
	}

	storedComparison, err := repository.GetComparisonReceipt(ctx, "alice", comparison.ID)
	if err != nil || storedComparison.ReceiptDigest != comparison.ReceiptDigest {
		t.Fatalf("comparison round trip = %#v, %v", storedComparison, err)
	}
	storedPromotion, err := repository.GetPromotionDecisionReceipt(ctx, "alice", promotion.ID)
	if err != nil || storedPromotion.ReceiptDigest != promotion.ReceiptDigest ||
		!storedPromotion.Decision.Allowed {
		t.Fatalf("promotion round trip = %#v, %v", storedPromotion, err)
	}
	storedPromotion.Decision.Reasons[0] = "mutated"
	again, _ := repository.GetPromotionDecisionReceipt(ctx, "alice", promotion.ID)
	if again.Decision.Reasons[0] == "mutated" {
		t.Fatal("caller mutated persisted promotion receipt")
	}
	if _, err := repository.GetComparisonReceipt(ctx, "bob", comparison.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob read alice comparison receipt: %v", err)
	}

	comparisons, err := repository.ListComparisonReceipts(ctx, "alice", ReceiptQuery{CandidateRunID: candidate.ID})
	if err != nil || len(comparisons) != 1 {
		t.Fatalf("comparison list = %#v, %v", comparisons, err)
	}
	promotions, err := repository.ListPromotionDecisionReceipts(ctx, "alice", ReceiptQuery{CandidateRunID: candidate.ID})
	if err != nil || len(promotions) != 1 {
		t.Fatalf("promotion list = %#v, %v", promotions, err)
	}
}

type evaluatorStub struct{}

func (evaluatorStub) Descriptor() EvaluatorDescriptor {
	return EvaluatorDescriptor{ID: "stub", Version: "1.0.0"}
}

func (evaluatorStub) Evaluate(_ context.Context, request EvaluationRequest) ([]CaseObservation, error) {
	return []CaseObservation{{
		CaseID: request.Dataset.Cases[0].ID, CaseVersion: request.Dataset.Cases[0].Version,
		Criteria: []CriterionObservation{{CriterionID: "correct", Status: CriterionPassed, Score: 1}},
	}}, nil
}

func TestEvaluatorContractProducesObservationsNotTrustedRecords(t *testing.T) {
	var evaluator Evaluator = evaluatorStub{}
	dataset := testDataset(t, 1, json.RawMessage(`{"prompt":"hello"}`))
	observations, err := evaluator.Evaluate(context.Background(), EvaluationRequest{Dataset: dataset})
	if err != nil || len(observations) != 1 || evaluator.Descriptor().ID != "stub" {
		t.Fatalf("unexpected evaluator result: %#v, %v", observations, err)
	}
}

func testDataset(t *testing.T, version uint32, input json.RawMessage) Dataset {
	t.Helper()
	dataset, err := NewDataset(DatasetSpec{
		ID: "routing", Version: version, Name: "Routing regression",
		CreatedAt: time.Date(2026, 7, 30, 12, 0, 0, 0, time.FixedZone("test", 3600)),
		Cases: []EvaluationCase{{
			ID: "simple-task", Version: 1, Input: input,
			Expected: json.RawMessage(`{"answer":"ok"}`),
			Criteria: []Criterion{{ID: "correct", Required: true, Weight: 1, MinScore: 0.8}},
		}},
	})
	if err != nil {
		t.Fatalf("new dataset: %v", err)
	}
	return dataset
}

func testRun(t *testing.T, dataset Dataset, mode RunMode, baseline string, seed int64, status CriterionStatus, score float64) RunRecord {
	t.Helper()
	record, err := NewRunRecord(testRunSpec(dataset, mode, baseline, seed, status, score))
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	return record
}

func testRunSpec(dataset Dataset, mode RunMode, baseline string, seed int64, status CriterionStatus, score float64) RunSpec {
	canaryPercent := float64(0)
	if mode == RunModeCanary {
		canaryPercent = 10
	}
	return RunSpec{
		ID:        "candidate-" + strings.ReplaceAll(strings.ToLower(string(mode)), "_", "-"),
		Dataset:   dataset,
		Evaluator: EvaluatorDescriptor{ID: "judge", Version: "1.2.3"},
		Subject: SubjectDescriptor{
			ID: "candidate", Version: "2026.07",
			ArtifactDigest: strings.Repeat("a", 64),
		},
		Mode: mode, CanaryPercent: canaryPercent, BaselineRunID: baseline, Seed: seed,
		Config:            json.RawMessage(`{"temperature":0}`),
		EnvironmentDigest: strings.Repeat("b", 64),
		StartedAt:         time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC),
		CompletedAt:       time.Date(2026, 7, 30, 12, 1, 0, 0, time.UTC),
		Status:            RunStatusCompleted,
		Observations: []CaseObservation{{
			CaseID: "simple-task", CaseVersion: 1,
			Criteria: []CriterionObservation{{CriterionID: "correct", Status: status, Score: score}},
		}},
	}
}
