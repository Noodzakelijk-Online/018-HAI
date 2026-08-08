package lifeontology

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPostgresRepositoryRequiresDatabase(t *testing.T) {
	t.Parallel()

	repository := NewPostgresRepository(nil)
	if _, err := repository.GetEntity(t.Context(), "owner", "entity"); err == nil ||
		!strings.Contains(err.Error(), "database is required") {
		t.Fatalf("nil database error = %v", err)
	}
	if _, err := repository.AppendRelation(t.Context(), Relation{}); err == nil ||
		!strings.Contains(err.Error(), "database is required") {
		t.Fatalf("nil database append error = %v", err)
	}
}

func TestPostgresEntityDecoderStrictlyRevalidatesPayloadAndColumns(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryRepository(), func() time.Time { return fixedNow() })
	entity := mustEntity(t, service, entityRequest(EntityAsset, "Durable laptop"))
	payload, err := json.Marshal(entity)
	if err != nil {
		t.Fatal(err)
	}
	row := postgresEntityRow{
		OwnerIdentity: entity.OwnerIdentity, EntityID: entity.ID, EntityType: entity.Type,
		LifeDomain: entity.Domain, LifecycleStatus: entity.Status,
		VerificationStatus: entity.VerificationStatus, Sensitivity: entity.Sensitivity,
		LocalOnly: entity.LocalOnly, Priority: entity.Priority, EntityDigest: entity.EntityDigest,
		ProvenanceDigest: entity.ProvenanceDigest, ValidFrom: entity.ValidFrom,
		ValidUntil: entity.ValidUntil, ObservedAt: entity.ObservedAt,
		CreatedAt: entity.CreatedAt, Payload: string(payload),
	}
	if _, err := decodePostgresEntityRow(row, entity.OwnerIdentity); err != nil {
		t.Fatalf("valid entity row rejected: %v", err)
	}

	malformed := row
	malformed.Payload = `{"id":`
	if _, err := decodePostgresEntityRow(malformed, entity.OwnerIdentity); !errors.Is(err, ErrCorruptStorage) {
		t.Fatalf("malformed payload error = %v", err)
	}

	unknown := row
	unknown.Payload = strings.TrimSuffix(string(payload), "}") + `,"unexpected":true}`
	if _, err := decodePostgresEntityRow(unknown, entity.OwnerIdentity); !errors.Is(err, ErrCorruptStorage) {
		t.Fatalf("unknown field error = %v", err)
	}

	columnMismatch := row
	columnMismatch.Priority++
	if _, err := decodePostgresEntityRow(columnMismatch, entity.OwnerIdentity); !errors.Is(err, ErrCorruptStorage) {
		t.Fatalf("column mismatch error = %v", err)
	}

	var envelope map[string]any
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatal(err)
	}
	envelope["name"] = "tampered"
	tamperedPayload, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	tampered := row
	tampered.Payload = string(tamperedPayload)
	if _, err := decodePostgresEntityRow(tampered, entity.OwnerIdentity); !errors.Is(err, ErrCorruptStorage) {
		t.Fatalf("tampered digest error = %v", err)
	}

	if _, err := decodePostgresEntityRow(row, "other-owner"); !errors.Is(err, ErrCorruptStorage) {
		t.Fatalf("owner mismatch error = %v", err)
	}
}

func TestPostgresRelationAndMergeDecodersFailClosed(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryRepository(), func() time.Time { return fixedNow() })
	person := mustEntity(t, service, entityRequest(EntityPerson, "Robert"))
	goalRequest := entityRequest(EntityGoal, "Stable goal")
	goal := mustEntity(t, service, goalRequest)
	relationResult, err := service.RecordRelation(t.Context(), relationRequest(RelationPursuesGoal, person.ID, goal.ID))
	if err != nil {
		t.Fatal(err)
	}
	relation := relationResult.Relation
	relationPayload, err := json.Marshal(relation)
	if err != nil {
		t.Fatal(err)
	}
	relationRow := postgresRelationRow{
		OwnerIdentity: relation.OwnerIdentity, RelationID: relation.ID,
		RelationType: relation.Type, FromEntityID: relation.FromEntityID,
		ToEntityID: relation.ToEntityID, VerificationStatus: relation.VerificationStatus,
		Sensitivity: relation.Sensitivity, LocalOnly: relation.LocalOnly,
		RelationDigest: relation.RelationDigest, ProvenanceDigest: relation.ProvenanceDigest,
		ValidFrom: relation.ValidFrom, ValidUntil: relation.ValidUntil,
		ObservedAt: relation.ObservedAt, CreatedAt: relation.CreatedAt,
		Payload: string(relationPayload),
	}
	if _, err := decodePostgresRelationRow(relationRow, relation.OwnerIdentity); err != nil {
		t.Fatalf("valid relation row rejected: %v", err)
	}
	relationRow.ToEntityID = person.ID
	if _, err := decodePostgresRelationRow(relationRow, relation.OwnerIdentity); !errors.Is(err, ErrCorruptStorage) {
		t.Fatalf("relation metadata mismatch error = %v", err)
	}

	proposal, err := buildMergeProposal(person.OwnerIdentity, person.ID, goal.ID, MergeSemanticIdentity, []string{"review candidate"}, 0.8, fixedNow())
	if err != nil {
		t.Fatal(err)
	}
	proposalPayload, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	proposalRow := postgresMergeProposalRow{
		OwnerIdentity: proposal.OwnerIdentity, ProposalID: proposal.ID,
		CandidateLeftID: proposal.CandidateEntityIDs[0], CandidateRightID: proposal.CandidateEntityIDs[1],
		MatchType: proposal.Match, ProposalStatus: proposal.Status,
		Confidence: proposal.Confidence, ProposalDigest: proposal.ProposalDigest,
		CreatedAt: proposal.CreatedAt, Payload: string(proposalPayload),
	}
	if _, err := decodePostgresMergeProposalRow(proposalRow, proposal.OwnerIdentity); err != nil {
		t.Fatalf("valid proposal row rejected: %v", err)
	}
	proposalRow.Payload += `{}`
	if _, err := decodePostgresMergeProposalRow(proposalRow, proposal.OwnerIdentity); !errors.Is(err, ErrCorruptStorage) {
		t.Fatalf("multiple JSON values error = %v", err)
	}
}
