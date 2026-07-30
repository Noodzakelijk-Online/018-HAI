package evaluation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"automation-hub-backend/internal/infra"
	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// GormRepository persists owner-scoped evaluation evidence in PostgreSQL.
// Every read reconstructs and validates the domain record before returning it.
type GormRepository struct {
	db *gorm.DB
}

func NewGormRepository(db *gorm.DB) Repository {
	return &GormRepository{db: db}
}

func DefaultRepository() (Repository, error) {
	db, err := infra.GetDefaultDB()
	if err != nil {
		return nil, err
	}
	return NewGormRepository(db), nil
}

func (repository *GormRepository) CreateDataset(
	ctx context.Context,
	owner string,
	dataset Dataset,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	owner, err := normalizeOwner(owner)
	if err != nil {
		return err
	}
	if err := ValidateDataset(dataset); err != nil {
		return err
	}
	return translateCreateError(repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := models.EvaluationDataset{
			OwnerIdentity:  owner,
			DatasetID:      dataset.ID,
			DatasetVersion: dataset.Version,
			SchemaVersion:  dataset.SchemaVersion,
			Name:           dataset.Name,
			Description:    dataset.Description,
			CreatedAtValue: formatExactTime(dataset.CreatedAt),
			Digest:         dataset.Digest,
			RecordedAt:     time.Now().UTC(),
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		for index, evalCase := range dataset.Cases {
			criteria, err := json.Marshal(evalCase.Criteria)
			if err != nil {
				return fmt.Errorf("evaluation: encode dataset case criteria: %w", err)
			}
			caseRow := models.EvaluationDatasetCase{
				OwnerIdentity:   owner,
				DatasetRecordID: row.ID,
				Ordinal:         index,
				CaseID:          evalCase.ID,
				CaseVersion:     evalCase.Version,
				InputJSON:       string(evalCase.Input),
				ExpectedJSON:    string(evalCase.Expected),
				CriteriaJSON:    string(criteria),
			}
			if err := tx.Create(&caseRow).Error; err != nil {
				return err
			}
		}
		return nil
	}), "dataset", caseKey(dataset.ID, dataset.Version))
}

func (repository *GormRepository) GetDataset(
	ctx context.Context,
	owner string,
	id string,
	version uint32,
) (Dataset, error) {
	owner, err := normalizeOwner(owner)
	if err != nil {
		return Dataset{}, err
	}
	return repository.getDataset(repository.db.WithContext(ctx), owner, id, version)
}

func (repository *GormRepository) ListDatasetVersions(
	ctx context.Context,
	owner string,
	id string,
) ([]Dataset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	owner, err := normalizeOwner(owner)
	if err != nil {
		return nil, err
	}
	var rows []models.EvaluationDataset
	if err := repository.db.WithContext(ctx).
		Where("owner_identity = ? AND dataset_id = ?", owner, strings.TrimSpace(id)).
		Order("dataset_version ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]Dataset, 0, len(rows))
	for _, row := range rows {
		dataset, err := repository.datasetFromRow(repository.db.WithContext(ctx), row)
		if err != nil {
			return nil, err
		}
		result = append(result, dataset)
	}
	return result, nil
}

func (repository *GormRepository) CreateRun(
	ctx context.Context,
	owner string,
	record RunRecord,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	owner, err := normalizeOwner(owner)
	if err != nil {
		return err
	}
	if err := ValidateRunRecord(record); err != nil {
		return err
	}
	return translateCreateError(repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		dataset, datasetRow, err := repository.getDatasetWithRow(
			tx,
			owner,
			record.Dataset.ID,
			record.Dataset.Version,
		)
		if err != nil {
			return err
		}
		if dataset.Digest != record.Dataset.Digest {
			return fmt.Errorf("%w: referenced dataset digest", ErrNotFound)
		}
		if err := validateRunAgainstDataset(record, dataset); err != nil {
			return err
		}

		var baselineRecordID *uuid.UUID
		if record.BaselineRunID != "" {
			baseline, baselineRow, err := repository.getRunWithRow(tx, owner, record.BaselineRunID)
			if err != nil {
				return err
			}
			if baseline.Dataset != record.Dataset {
				return fmt.Errorf("%w: baseline run uses another dataset version", ErrInvalidRun)
			}
			baselineRecordID = &baselineRow.ID
		}

		reproducibility, err := json.Marshal(record.Reproducibility)
		if err != nil {
			return fmt.Errorf("evaluation: encode reproducibility manifest: %w", err)
		}
		row := models.EvaluationRun{
			OwnerIdentity:         owner,
			RunID:                 record.ID,
			SchemaVersion:         record.SchemaVersion,
			DatasetRecordID:       datasetRow.ID,
			DatasetID:             record.Dataset.ID,
			DatasetVersion:        record.Dataset.Version,
			DatasetDigest:         record.Dataset.Digest,
			EvaluatorID:           record.Evaluator.ID,
			EvaluatorVersion:      record.Evaluator.Version,
			SubjectID:             record.Subject.ID,
			SubjectVersion:        record.Subject.Version,
			SubjectArtifactDigest: record.Subject.ArtifactDigest,
			Mode:                  string(record.Mode),
			CanaryPercent:         record.CanaryPercent,
			BaselineRunRecordID:   baselineRecordID,
			BaselineRunID:         record.BaselineRunID,
			StartedAtValue:        formatExactTime(record.StartedAt),
			CompletedAtValue:      formatExactTime(record.CompletedAt),
			CompletedAtIndex:      record.CompletedAt.UTC(),
			Status:                string(record.Status),
			FailureCode:           record.FailureCode,
			OverallScore:          record.OverallScore,
			CasePassRate:          record.CasePassRate,
			RequiredFailureCount:  record.RequiredFailureCount,
			CriterionErrorCount:   record.CriterionErrorCount,
			ReproducibilityJSON:   string(reproducibility),
			ReproducibilityDigest: record.ReproducibilityDigest,
			RecordDigest:          record.RecordDigest,
			RecordedAt:            time.Now().UTC(),
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		for index, caseResult := range record.CaseResults {
			criteria, err := json.Marshal(caseResult.Criteria)
			if err != nil {
				return fmt.Errorf("evaluation: encode run case criteria: %w", err)
			}
			resultRow := models.EvaluationRunCaseResult{
				OwnerIdentity: owner,
				RunRecordID:   row.ID,
				Ordinal:       index,
				CaseID:        caseResult.CaseID,
				CaseVersion:   caseResult.CaseVersion,
				Passed:        caseResult.Passed,
				Score:         caseResult.Score,
				CriteriaJSON:  string(criteria),
			}
			if err := tx.Create(&resultRow).Error; err != nil {
				return err
			}
		}
		return nil
	}), "run", record.ID)
}

func (repository *GormRepository) GetRun(
	ctx context.Context,
	owner string,
	id string,
) (RunRecord, error) {
	owner, err := normalizeOwner(owner)
	if err != nil {
		return RunRecord{}, err
	}
	record, _, err := repository.getRunWithRow(repository.db.WithContext(ctx), owner, id)
	return record, err
}

func (repository *GormRepository) ListRuns(
	ctx context.Context,
	owner string,
	query RunQuery,
) ([]RunRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	owner, err := normalizeOwner(owner)
	if err != nil {
		return nil, err
	}
	if query.Limit <= 0 || query.Limit > 500 {
		query.Limit = 100
	}
	db := repository.db.WithContext(ctx).
		Where("owner_identity = ?", owner)
	if query.DatasetID != "" {
		db = db.Where("dataset_id = ?", strings.TrimSpace(query.DatasetID))
	}
	if query.SubjectID != "" {
		db = db.Where("subject_id = ?", strings.TrimSpace(query.SubjectID))
	}
	if query.Mode != "" {
		db = db.Where("mode = ?", string(query.Mode))
	}
	var rows []models.EvaluationRun
	if err := db.Order("completed_at_index DESC, run_id ASC").Limit(query.Limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]RunRecord, 0, len(rows))
	for _, row := range rows {
		record, err := repository.runFromRow(repository.db.WithContext(ctx), row)
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, nil
}

func (repository *GormRepository) CreateComparisonReceipt(
	ctx context.Context,
	owner string,
	receipt BaselineComparisonReceipt,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	owner, err := normalizeOwner(owner)
	if err != nil {
		return err
	}
	if err := ValidateBaselineComparisonReceipt(receipt); err != nil {
		return err
	}
	return translateCreateError(repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		candidate, _, err := repository.getRunWithRow(tx, owner, receipt.CandidateRunID)
		if err != nil {
			return err
		}
		baseline, _, err := repository.getRunWithRow(tx, owner, receipt.BaselineRunID)
		if err != nil {
			return err
		}
		if err := validateComparisonReceiptAgainstRuns(receipt, candidate, baseline); err != nil {
			return err
		}
		thresholds, comparison, err := encodeComparisonReceiptPayload(receipt)
		if err != nil {
			return err
		}
		row := models.EvaluationComparisonReceipt{
			OwnerIdentity:         owner,
			ReceiptID:             receipt.ID,
			SchemaVersion:         receipt.SchemaVersion,
			CandidateRunID:        receipt.CandidateRunID,
			CandidateRecordDigest: receipt.CandidateRecordDigest,
			BaselineRunID:         receipt.BaselineRunID,
			BaselineRecordDigest:  receipt.BaselineRecordDigest,
			ThresholdsJSON:        thresholds,
			ComparisonJSON:        comparison,
			CreatedAtValue:        formatExactTime(receipt.CreatedAt),
			ReceiptDigest:         receipt.ReceiptDigest,
			RecordedAt:            time.Now().UTC(),
		}
		return tx.Create(&row).Error
	}), "comparison receipt", receipt.ID)
}

func (repository *GormRepository) GetComparisonReceipt(
	ctx context.Context,
	owner string,
	id string,
) (BaselineComparisonReceipt, error) {
	owner, err := normalizeOwner(owner)
	if err != nil {
		return BaselineComparisonReceipt{}, err
	}
	var row models.EvaluationComparisonReceipt
	if err := repository.db.WithContext(ctx).
		Where("owner_identity = ? AND receipt_id = ?", owner, strings.TrimSpace(id)).
		First(&row).Error; err != nil {
		return BaselineComparisonReceipt{}, translateNotFound(err, "comparison receipt", id)
	}
	return repository.comparisonReceiptFromRow(repository.db.WithContext(ctx), row)
}

func (repository *GormRepository) ListComparisonReceipts(
	ctx context.Context,
	owner string,
	query ReceiptQuery,
) ([]BaselineComparisonReceipt, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	owner, err := normalizeOwner(owner)
	if err != nil {
		return nil, err
	}
	db := repository.db.WithContext(ctx).Where("owner_identity = ?", owner)
	if query.CandidateRunID != "" {
		db = db.Where("candidate_run_id = ?", strings.TrimSpace(query.CandidateRunID))
	}
	var rows []models.EvaluationComparisonReceipt
	if err := db.Order("recorded_at DESC, receipt_id ASC").
		Limit(boundedReceiptLimit(query.Limit)).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]BaselineComparisonReceipt, 0, len(rows))
	for _, row := range rows {
		receipt, err := repository.comparisonReceiptFromRow(repository.db.WithContext(ctx), row)
		if err != nil {
			return nil, err
		}
		result = append(result, receipt)
	}
	return result, nil
}

func (repository *GormRepository) CreatePromotionDecisionReceipt(
	ctx context.Context,
	owner string,
	receipt PromotionDecisionReceipt,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	owner, err := normalizeOwner(owner)
	if err != nil {
		return err
	}
	if err := ValidatePromotionDecisionReceipt(receipt); err != nil {
		return err
	}
	return translateCreateError(repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		candidate, _, err := repository.getRunWithRow(tx, owner, receipt.CandidateRunID)
		if err != nil {
			return err
		}
		var baseline *RunRecord
		if receipt.BaselineRunID != "" {
			stored, _, err := repository.getRunWithRow(tx, owner, receipt.BaselineRunID)
			if err != nil {
				return err
			}
			baseline = &stored
		}
		if err := validatePromotionReceiptAgainstRuns(receipt, candidate, baseline); err != nil {
			return err
		}

		var comparisonRecordID *uuid.UUID
		if receipt.ComparisonReceiptID != "" {
			comparison, comparisonRow, err := repository.getComparisonReceiptWithRow(
				tx,
				owner,
				receipt.ComparisonReceiptID,
			)
			if err != nil {
				return err
			}
			if comparison.CandidateRunID != receipt.CandidateRunID ||
				comparison.BaselineRunID != receipt.BaselineRunID ||
				!reflectThresholdsEqual(comparison.Thresholds, receipt.Thresholds) ||
				receipt.Decision.Comparison == nil ||
				!reflectComparisonEqual(comparison.Comparison, *receipt.Decision.Comparison) {
				return fmt.Errorf("evaluation: promotion receipt comparison binding mismatch")
			}
			comparisonRecordID = &comparisonRow.ID
		}
		thresholds, decision, err := encodePromotionReceiptPayload(receipt)
		if err != nil {
			return err
		}
		var baselineRunID *string
		var baselineRecordDigest *string
		if receipt.BaselineRunID != "" {
			baselineRunID = stringPointer(receipt.BaselineRunID)
			baselineRecordDigest = stringPointer(receipt.BaselineRecordDigest)
		}
		row := models.EvaluationPromotionDecisionReceipt{
			OwnerIdentity:         owner,
			ReceiptID:             receipt.ID,
			SchemaVersion:         receipt.SchemaVersion,
			CandidateRunID:        receipt.CandidateRunID,
			CandidateRecordDigest: receipt.CandidateRecordDigest,
			BaselineRunID:         baselineRunID,
			BaselineRecordDigest:  baselineRecordDigest,
			ComparisonReceiptID:   comparisonRecordID,
			ComparisonReceiptKey:  receipt.ComparisonReceiptID,
			ThresholdsJSON:        thresholds,
			DecisionJSON:          decision,
			CreatedAtValue:        formatExactTime(receipt.CreatedAt),
			ReceiptDigest:         receipt.ReceiptDigest,
			RecordedAt:            time.Now().UTC(),
		}
		return tx.Create(&row).Error
	}), "promotion receipt", receipt.ID)
}

func (repository *GormRepository) GetPromotionDecisionReceipt(
	ctx context.Context,
	owner string,
	id string,
) (PromotionDecisionReceipt, error) {
	owner, err := normalizeOwner(owner)
	if err != nil {
		return PromotionDecisionReceipt{}, err
	}
	var row models.EvaluationPromotionDecisionReceipt
	if err := repository.db.WithContext(ctx).
		Where("owner_identity = ? AND receipt_id = ?", owner, strings.TrimSpace(id)).
		First(&row).Error; err != nil {
		return PromotionDecisionReceipt{}, translateNotFound(err, "promotion receipt", id)
	}
	return repository.promotionReceiptFromRow(repository.db.WithContext(ctx), row)
}

func (repository *GormRepository) ListPromotionDecisionReceipts(
	ctx context.Context,
	owner string,
	query ReceiptQuery,
) ([]PromotionDecisionReceipt, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	owner, err := normalizeOwner(owner)
	if err != nil {
		return nil, err
	}
	db := repository.db.WithContext(ctx).Where("owner_identity = ?", owner)
	if query.CandidateRunID != "" {
		db = db.Where("candidate_run_id = ?", strings.TrimSpace(query.CandidateRunID))
	}
	var rows []models.EvaluationPromotionDecisionReceipt
	if err := db.Order("recorded_at DESC, receipt_id ASC").
		Limit(boundedReceiptLimit(query.Limit)).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]PromotionDecisionReceipt, 0, len(rows))
	for _, row := range rows {
		receipt, err := repository.promotionReceiptFromRow(repository.db.WithContext(ctx), row)
		if err != nil {
			return nil, err
		}
		result = append(result, receipt)
	}
	return result, nil
}

func (repository *GormRepository) getDataset(
	db *gorm.DB,
	owner string,
	id string,
	version uint32,
) (Dataset, error) {
	dataset, _, err := repository.getDatasetWithRow(db, owner, id, version)
	return dataset, err
}

func (repository *GormRepository) getDatasetWithRow(
	db *gorm.DB,
	owner string,
	id string,
	version uint32,
) (Dataset, models.EvaluationDataset, error) {
	var row models.EvaluationDataset
	if err := db.
		Where(
			"owner_identity = ? AND dataset_id = ? AND dataset_version = ?",
			owner,
			strings.TrimSpace(id),
			version,
		).
		First(&row).Error; err != nil {
		return Dataset{}, models.EvaluationDataset{}, translateNotFound(
			err,
			"dataset",
			caseKey(id, version),
		)
	}
	dataset, err := repository.datasetFromRow(db, row)
	return dataset, row, err
}

func (repository *GormRepository) datasetFromRow(
	db *gorm.DB,
	row models.EvaluationDataset,
) (Dataset, error) {
	createdAt, err := parseExactTime(row.CreatedAtValue)
	if err != nil {
		return Dataset{}, fmt.Errorf("evaluation: decode dataset created time: %w", err)
	}
	var caseRows []models.EvaluationDatasetCase
	if err := db.
		Where("owner_identity = ? AND dataset_record_id = ?", row.OwnerIdentity, row.ID).
		Order("ordinal ASC").
		Find(&caseRows).Error; err != nil {
		return Dataset{}, err
	}
	cases := make([]EvaluationCase, 0, len(caseRows))
	for index, caseRow := range caseRows {
		if caseRow.Ordinal != index {
			return Dataset{}, fmt.Errorf("evaluation: dataset case ordinals are not contiguous")
		}
		var criteria []Criterion
		if err := decodeExactJSON(caseRow.CriteriaJSON, &criteria); err != nil {
			return Dataset{}, fmt.Errorf("evaluation: decode dataset case criteria: %w", err)
		}
		cases = append(cases, EvaluationCase{
			ID:       caseRow.CaseID,
			Version:  caseRow.CaseVersion,
			Input:    json.RawMessage(caseRow.InputJSON),
			Expected: json.RawMessage(caseRow.ExpectedJSON),
			Criteria: criteria,
		})
	}
	dataset := Dataset{
		SchemaVersion: row.SchemaVersion,
		ID:            row.DatasetID,
		Version:       row.DatasetVersion,
		Name:          row.Name,
		Description:   row.Description,
		Cases:         cases,
		CreatedAt:     createdAt,
		Digest:        row.Digest,
	}
	if err := ValidateDataset(dataset); err != nil {
		return Dataset{}, fmt.Errorf("evaluation: stored dataset failed validation: %w", err)
	}
	return dataset, nil
}

func (repository *GormRepository) getRunWithRow(
	db *gorm.DB,
	owner string,
	id string,
) (RunRecord, models.EvaluationRun, error) {
	var row models.EvaluationRun
	if err := db.
		Where("owner_identity = ? AND run_id = ?", owner, strings.TrimSpace(id)).
		First(&row).Error; err != nil {
		return RunRecord{}, models.EvaluationRun{}, translateNotFound(err, "run", id)
	}
	record, err := repository.runFromRow(db, row)
	return record, row, err
}

func (repository *GormRepository) runFromRow(
	db *gorm.DB,
	row models.EvaluationRun,
) (RunRecord, error) {
	startedAt, err := parseExactTime(row.StartedAtValue)
	if err != nil {
		return RunRecord{}, fmt.Errorf("evaluation: decode run start time: %w", err)
	}
	completedAt, err := parseExactTime(row.CompletedAtValue)
	if err != nil {
		return RunRecord{}, fmt.Errorf("evaluation: decode run completion time: %w", err)
	}
	var reproducibility ReproducibilityManifest
	if err := decodeExactJSON(row.ReproducibilityJSON, &reproducibility); err != nil {
		return RunRecord{}, fmt.Errorf("evaluation: decode reproducibility manifest: %w", err)
	}
	var resultRows []models.EvaluationRunCaseResult
	if err := db.
		Where("owner_identity = ? AND run_record_id = ?", row.OwnerIdentity, row.ID).
		Order("ordinal ASC").
		Find(&resultRows).Error; err != nil {
		return RunRecord{}, err
	}
	results := make([]CaseResult, 0, len(resultRows))
	for index, resultRow := range resultRows {
		if resultRow.Ordinal != index {
			return RunRecord{}, fmt.Errorf("evaluation: run case-result ordinals are not contiguous")
		}
		var criteria []CriterionResult
		if err := decodeExactJSON(resultRow.CriteriaJSON, &criteria); err != nil {
			return RunRecord{}, fmt.Errorf("evaluation: decode run result criteria: %w", err)
		}
		results = append(results, CaseResult{
			CaseID:      resultRow.CaseID,
			CaseVersion: resultRow.CaseVersion,
			Passed:      resultRow.Passed,
			Score:       resultRow.Score,
			Criteria:    criteria,
		})
	}
	record := RunRecord{
		SchemaVersion: row.SchemaVersion,
		ID:            row.RunID,
		Dataset: DatasetRef{
			ID: row.DatasetID, Version: row.DatasetVersion, Digest: row.DatasetDigest,
		},
		Evaluator: EvaluatorDescriptor{ID: row.EvaluatorID, Version: row.EvaluatorVersion},
		Subject: SubjectDescriptor{
			ID:             row.SubjectID,
			Version:        row.SubjectVersion,
			ArtifactDigest: row.SubjectArtifactDigest,
		},
		Mode:                  RunMode(row.Mode),
		CanaryPercent:         row.CanaryPercent,
		BaselineRunID:         row.BaselineRunID,
		StartedAt:             startedAt,
		CompletedAt:           completedAt,
		Status:                RunStatus(row.Status),
		FailureCode:           row.FailureCode,
		CaseResults:           results,
		OverallScore:          row.OverallScore,
		CasePassRate:          row.CasePassRate,
		RequiredFailureCount:  row.RequiredFailureCount,
		CriterionErrorCount:   row.CriterionErrorCount,
		Reproducibility:       reproducibility,
		ReproducibilityDigest: row.ReproducibilityDigest,
		RecordDigest:          row.RecordDigest,
	}
	if err := ValidateRunRecord(record); err != nil {
		return RunRecord{}, fmt.Errorf("evaluation: stored run failed validation: %w", err)
	}
	dataset, err := repository.getDataset(db, row.OwnerIdentity, row.DatasetID, row.DatasetVersion)
	if err != nil {
		return RunRecord{}, err
	}
	if err := validateRunAgainstDataset(record, dataset); err != nil {
		return RunRecord{}, fmt.Errorf("evaluation: stored run no longer matches its dataset: %w", err)
	}
	return record, nil
}

func (repository *GormRepository) getComparisonReceiptWithRow(
	db *gorm.DB,
	owner string,
	id string,
) (BaselineComparisonReceipt, models.EvaluationComparisonReceipt, error) {
	var row models.EvaluationComparisonReceipt
	if err := db.
		Where("owner_identity = ? AND receipt_id = ?", owner, strings.TrimSpace(id)).
		First(&row).Error; err != nil {
		return BaselineComparisonReceipt{}, models.EvaluationComparisonReceipt{},
			translateNotFound(err, "comparison receipt", id)
	}
	receipt, err := repository.comparisonReceiptFromRow(db, row)
	return receipt, row, err
}

func (repository *GormRepository) comparisonReceiptFromRow(
	db *gorm.DB,
	row models.EvaluationComparisonReceipt,
) (BaselineComparisonReceipt, error) {
	createdAt, err := parseExactTime(row.CreatedAtValue)
	if err != nil {
		return BaselineComparisonReceipt{}, fmt.Errorf("evaluation: decode comparison time: %w", err)
	}
	var thresholds RegressionThresholds
	if err := decodeExactJSON(row.ThresholdsJSON, &thresholds); err != nil {
		return BaselineComparisonReceipt{}, fmt.Errorf("evaluation: decode comparison thresholds: %w", err)
	}
	var comparison BaselineComparison
	if err := decodeExactJSON(row.ComparisonJSON, &comparison); err != nil {
		return BaselineComparisonReceipt{}, fmt.Errorf("evaluation: decode comparison result: %w", err)
	}
	receipt := BaselineComparisonReceipt{
		SchemaVersion:         row.SchemaVersion,
		ID:                    row.ReceiptID,
		CandidateRunID:        row.CandidateRunID,
		CandidateRecordDigest: row.CandidateRecordDigest,
		BaselineRunID:         row.BaselineRunID,
		BaselineRecordDigest:  row.BaselineRecordDigest,
		Thresholds:            thresholds,
		Comparison:            comparison,
		CreatedAt:             createdAt,
		ReceiptDigest:         row.ReceiptDigest,
	}
	candidate, _, err := repository.getRunWithRow(db, row.OwnerIdentity, row.CandidateRunID)
	if err != nil {
		return BaselineComparisonReceipt{}, err
	}
	baseline, _, err := repository.getRunWithRow(db, row.OwnerIdentity, row.BaselineRunID)
	if err != nil {
		return BaselineComparisonReceipt{}, err
	}
	if err := validateComparisonReceiptAgainstRuns(receipt, candidate, baseline); err != nil {
		return BaselineComparisonReceipt{}, fmt.Errorf("evaluation: stored comparison failed validation: %w", err)
	}
	return receipt, nil
}

func (repository *GormRepository) promotionReceiptFromRow(
	db *gorm.DB,
	row models.EvaluationPromotionDecisionReceipt,
) (PromotionDecisionReceipt, error) {
	createdAt, err := parseExactTime(row.CreatedAtValue)
	if err != nil {
		return PromotionDecisionReceipt{}, fmt.Errorf("evaluation: decode promotion time: %w", err)
	}
	var thresholds RegressionThresholds
	if err := decodeExactJSON(row.ThresholdsJSON, &thresholds); err != nil {
		return PromotionDecisionReceipt{}, fmt.Errorf("evaluation: decode promotion thresholds: %w", err)
	}
	var decision PromotionDecision
	if err := decodeExactJSON(row.DecisionJSON, &decision); err != nil {
		return PromotionDecisionReceipt{}, fmt.Errorf("evaluation: decode promotion decision: %w", err)
	}
	receipt := PromotionDecisionReceipt{
		SchemaVersion:         row.SchemaVersion,
		ID:                    row.ReceiptID,
		CandidateRunID:        row.CandidateRunID,
		CandidateRecordDigest: row.CandidateRecordDigest,
		BaselineRunID:         stringValue(row.BaselineRunID),
		BaselineRecordDigest:  stringValue(row.BaselineRecordDigest),
		ComparisonReceiptID:   row.ComparisonReceiptKey,
		Thresholds:            thresholds,
		Decision:              decision,
		CreatedAt:             createdAt,
		ReceiptDigest:         row.ReceiptDigest,
	}
	candidate, _, err := repository.getRunWithRow(db, row.OwnerIdentity, row.CandidateRunID)
	if err != nil {
		return PromotionDecisionReceipt{}, err
	}
	var baseline *RunRecord
	if row.BaselineRunID != nil {
		stored, _, err := repository.getRunWithRow(db, row.OwnerIdentity, *row.BaselineRunID)
		if err != nil {
			return PromotionDecisionReceipt{}, err
		}
		baseline = &stored
	}
	if err := validatePromotionReceiptAgainstRuns(receipt, candidate, baseline); err != nil {
		return PromotionDecisionReceipt{}, fmt.Errorf("evaluation: stored promotion failed validation: %w", err)
	}
	if receipt.ComparisonReceiptID != "" {
		comparison, _, err := repository.getComparisonReceiptWithRow(
			db,
			row.OwnerIdentity,
			receipt.ComparisonReceiptID,
		)
		if err != nil {
			return PromotionDecisionReceipt{}, err
		}
		if comparison.CandidateRunID != receipt.CandidateRunID ||
			comparison.BaselineRunID != receipt.BaselineRunID ||
			!reflectThresholdsEqual(comparison.Thresholds, receipt.Thresholds) ||
			receipt.Decision.Comparison == nil ||
			!reflectComparisonEqual(comparison.Comparison, *receipt.Decision.Comparison) {
			return PromotionDecisionReceipt{}, fmt.Errorf(
				"evaluation: stored promotion comparison binding mismatch",
			)
		}
	}
	return receipt, nil
}

func encodeComparisonReceiptPayload(
	receipt BaselineComparisonReceipt,
) (string, string, error) {
	thresholds, err := json.Marshal(receipt.Thresholds)
	if err != nil {
		return "", "", fmt.Errorf("evaluation: encode comparison thresholds: %w", err)
	}
	comparison, err := json.Marshal(receipt.Comparison)
	if err != nil {
		return "", "", fmt.Errorf("evaluation: encode comparison result: %w", err)
	}
	return string(thresholds), string(comparison), nil
}

func encodePromotionReceiptPayload(
	receipt PromotionDecisionReceipt,
) (string, string, error) {
	thresholds, err := json.Marshal(receipt.Thresholds)
	if err != nil {
		return "", "", fmt.Errorf("evaluation: encode promotion thresholds: %w", err)
	}
	decision, err := json.Marshal(receipt.Decision)
	if err != nil {
		return "", "", fmt.Errorf("evaluation: encode promotion decision: %w", err)
	}
	return string(thresholds), string(decision), nil
}

func formatExactTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func parseExactTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

func decodeExactJSON(raw string, target any) error {
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func translateCreateError(err error, kind string, id string) error {
	if err == nil {
		return nil
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23505" {
		return fmt.Errorf("%w: %s %s", ErrAlreadyExists, kind, id)
	}
	return err
}

func translateNotFound(err error, kind string, id string) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%w: %s %s", ErrNotFound, kind, id)
	}
	return err
}

func stringPointer(value string) *string {
	copy := value
	return &copy
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

var _ Repository = (*GormRepository)(nil)
