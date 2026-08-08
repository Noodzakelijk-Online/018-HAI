package phase2

import (
	"strings"
	"testing"

	"automation-hub-backend/migrations"
)

func TestEvidencePackMigrationIsOwnerScopedImmutableAndReversible(t *testing.T) {
	t.Parallel()

	upBytes, err := migrations.Files.ReadFile("pre/0016_evidence_packs.up.sql")
	if err != nil {
		t.Fatalf("read evidence pack up migration: %v", err)
	}
	downBytes, err := migrations.Files.ReadFile("pre/0016_evidence_packs.down.sql")
	if err != nil {
		t.Fatalf("read evidence pack down migration: %v", err)
	}
	up := string(upBytes)
	down := string(downBytes)

	for _, fragment := range []string{
		"CREATE TABLE public.evidence_packs",
		"owner_identity text NOT NULL",
		"workspace_id text NOT NULL",
		"operation_id uuid NOT NULL",
		"UNIQUE (owner_identity, workspace_id, id)",
		"FOREIGN KEY (owner_identity, workspace_id, operation_id)",
		"REFERENCES public.operations (owner_user_id, workspace_id, id)",
		"source_revision_hash text NOT NULL",
		"content_digest character varying(71) NOT NULL",
		"content_digest ~ '^sha256:[0-9a-f]{64}$'",
		"idx_evidence_packs_owner_workspace_generated",
		"idx_evidence_packs_owner_workspace_operation",
		"CREATE TRIGGER trg_evidence_packs_immutable",
		"BEFORE UPDATE OR DELETE ON public.evidence_packs",
		"CREATE TRIGGER trg_evidence_packs_no_truncate",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("evidence pack up migration is missing %q", fragment)
		}
	}

	for _, fragment := range []string{
		"DROP TRIGGER IF EXISTS trg_evidence_packs_no_truncate",
		"DROP TRIGGER IF EXISTS trg_evidence_packs_immutable",
		"DROP FUNCTION IF EXISTS public.hai_reject_evidence_pack_mutation()",
		"DROP TABLE IF EXISTS public.evidence_packs",
		"DROP CONSTRAINT IF EXISTS uq_operations_owner_workspace_id",
	} {
		if !strings.Contains(down, fragment) {
			t.Errorf("evidence pack down migration is missing %q", fragment)
		}
	}
}
