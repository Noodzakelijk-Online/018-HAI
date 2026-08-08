package migrations

import (
	"strings"
	"testing"
)

func TestEvidencePacksMigrationContract(t *testing.T) {
	upBytes, err := Files.ReadFile("pre/0016_evidence_packs.up.sql")
	if err != nil {
		t.Fatalf("read evidence packs up migration: %v", err)
	}
	downBytes, err := Files.ReadFile("pre/0016_evidence_packs.down.sql")
	if err != nil {
		t.Fatalf("read evidence packs down migration: %v", err)
	}

	up := string(upBytes)
	for _, fragment := range []string{
		"ADD CONSTRAINT uq_operations_owner_workspace_id",
		"CREATE TABLE public.evidence_packs",
		"UNIQUE (owner_identity, workspace_id, id)",
		"FOREIGN KEY (owner_identity, workspace_id, operation_id)",
		"REFERENCES public.operations (owner_user_id, workspace_id, id)",
		"ON UPDATE RESTRICT ON DELETE RESTRICT",
		"content_digest ~ '^sha256:[0-9a-f]{64}$'",
		"octet_length(markdown) BETWEEN 1 AND 2097152",
		"idx_evidence_packs_owner_workspace_generated",
		"idx_evidence_packs_owner_workspace_operation",
		"idx_evidence_packs_source_revision",
		"trg_evidence_packs_immutable",
		"trg_evidence_packs_no_truncate",
		"evidence packs are immutable",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("up migration is missing contract fragment %q", fragment)
		}
	}
	requireMigrationOrder(t, up,
		"ADD CONSTRAINT uq_operations_owner_workspace_id",
		"CREATE TABLE public.evidence_packs",
		"CREATE INDEX idx_evidence_packs_owner_workspace_generated",
		"CREATE OR REPLACE FUNCTION public.hai_reject_evidence_pack_mutation()",
		"CREATE TRIGGER trg_evidence_packs_immutable",
		"CREATE TRIGGER trg_evidence_packs_no_truncate",
	)

	down := string(downBytes)
	for _, fragment := range []string{
		"DROP TRIGGER IF EXISTS trg_evidence_packs_no_truncate",
		"DROP TRIGGER IF EXISTS trg_evidence_packs_immutable",
		"DROP FUNCTION IF EXISTS public.hai_reject_evidence_pack_mutation()",
		"DROP TABLE IF EXISTS public.evidence_packs",
		"DROP CONSTRAINT IF EXISTS uq_operations_owner_workspace_id",
	} {
		if !strings.Contains(down, fragment) {
			t.Errorf("down migration is missing contract fragment %q", fragment)
		}
	}
	requireMigrationOrder(t, down,
		"DROP TRIGGER IF EXISTS trg_evidence_packs_no_truncate",
		"DROP TRIGGER IF EXISTS trg_evidence_packs_immutable",
		"DROP FUNCTION IF EXISTS public.hai_reject_evidence_pack_mutation()",
		"DROP TABLE IF EXISTS public.evidence_packs",
		"DROP CONSTRAINT IF EXISTS uq_operations_owner_workspace_id",
	)
}
