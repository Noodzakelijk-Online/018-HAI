package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/safety"

	"github.com/google/uuid"
)

func newWorkflowCompletionAttestation(
	item models.WorkflowItem,
	result *TaskRunResult,
	completedAt time.Time,
) (*models.WorkflowCompletionAttestation, error) {
	if result == nil || item.ID == uuid.Nil ||
		strings.TrimSpace(result.PlanID) == "" || strings.TrimSpace(result.VerificationStatus) == "" {
		return nil, fmt.Errorf("complete workflow result evidence is required")
	}
	verificationStatus := strings.ToLower(strings.TrimSpace(result.VerificationStatus))
	if verificationStatus != "verified" && verificationStatus != "test_passed" {
		return nil, fmt.Errorf("workflow completion requires verified or test-passed evidence")
	}
	resultDigest := sha256.Sum256([]byte(strings.Join([]string{
		strings.TrimSpace(result.CompletionStatus),
		strings.TrimSpace(result.VerificationStatus),
		result.Output,
		result.FailureReason,
	}, "\n")))
	runtimeURI := strings.TrimSpace(result.RuntimeEvidenceURI)
	if runtimeURI == "" {
		runtimeURI = "hai://task-plans/" + result.PlanID + "/results/" + hex.EncodeToString(resultDigest[:])
	}
	if len(runtimeURI) > 2048 || safety.RedactSecrets(runtimeURI) != runtimeURI {
		return nil, fmt.Errorf("workflow runtime evidence URI is invalid")
	}
	evidenceDigest := sha256.Sum256([]byte(strings.Join([]string{
		runtimeURI,
		strings.TrimSpace(result.RuntimeEvidenceLabel),
	}, "\n")))
	runtimeID := ""
	if result.RuntimeRouteTrace != nil {
		runtimeID = strings.TrimSpace(result.RuntimeRouteTrace.RuntimeID)
	}
	if runtimeID == "" {
		runtimeID = "task-engine"
	}
	attestation := &models.WorkflowCompletionAttestation{
		ID: uuid.New(), WorkflowID: item.ID, OwnerIdentity: firstNonEmpty(strings.TrimSpace(item.OwnerIdentity), "system"),
		TaskPlanID: strings.TrimSpace(result.PlanID), CompletionStatus: "completed",
		VerificationStatus: verificationStatus,
		RuntimeID:          runtimeID, RuntimeEvidenceURI: runtimeURI, RuntimeEvidenceDigest: hex.EncodeToString(evidenceDigest[:]),
		ResultDigest: hex.EncodeToString(resultDigest[:]), CompletedAt: completedAt.UTC(),
		CreatedAt: completedAt.UTC(),
	}
	digest, err := workflowCompletionAttestationDigest(attestation)
	if err != nil {
		return nil, err
	}
	attestation.RecordDigest = digest
	return attestation, nil
}

func workflowCompletionAttestationDigest(value *models.WorkflowCompletionAttestation) (string, error) {
	if value == nil {
		return "", fmt.Errorf("workflow completion attestation is required")
	}
	payload := struct {
		WorkflowID, OwnerIdentity, TaskPlanID, CompletionStatus, VerificationStatus string
		RuntimeID, RuntimeEvidenceURI, RuntimeEvidenceDigest, ResultDigest          string
		CompletedAt                                                                 string
	}{
		value.WorkflowID.String(), strings.TrimSpace(value.OwnerIdentity),
		strings.TrimSpace(value.TaskPlanID), strings.TrimSpace(value.CompletionStatus),
		strings.TrimSpace(value.VerificationStatus), strings.TrimSpace(value.RuntimeID), strings.TrimSpace(value.RuntimeEvidenceURI),
		strings.TrimSpace(value.RuntimeEvidenceDigest), strings.TrimSpace(value.ResultDigest),
		value.CompletedAt.UTC().Format(time.RFC3339Nano),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode workflow completion attestation: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}
