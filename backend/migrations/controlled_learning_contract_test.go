package migrations

import (
	"strings"
	"testing"
)

func TestControlledLearningMigrationContract(t *testing.T) {
	upBytes, err := Files.ReadFile("pre/0015_controlled_learning.up.sql")
	if err != nil {
		t.Fatalf("read controlled learning up migration: %v", err)
	}
	downBytes, err := Files.ReadFile("pre/0015_controlled_learning.down.sql")
	if err != nil {
		t.Fatalf("read controlled learning down migration: %v", err)
	}

	up := string(upBytes)
	for _, fragment := range []string{
		"CREATE TABLE public.controlled_learning_outcomes",
		"CREATE TABLE public.controlled_learning_proposals",
		"CREATE TABLE public.controlled_learning_proposal_evidence",
		"CREATE TABLE public.controlled_learning_review_decisions",
		"UNIQUE (owner_identity, idempotency_key)",
		"UNIQUE (owner_identity, proposal_id, proposal_revision)",
		"FOREIGN KEY (owner_identity, proposal_id)",
		"FOREIGN KEY (owner_identity, outcome_id)",
		"evidence_digest ~ '^sha256:[0-9a-f]{64}$'",
		"proposal_digest ~ '^sha256:[0-9a-f]{64}$'",
		"decision_digest ~ '^sha256:[0-9a-f]{64}$'",
		"definition_payload #>> '{proposalDigest}' = proposal_digest",
		"payload #>> '{decisionDigest}' = decision_digest",
		"trg_controlled_learning_outcomes_immutable",
		"trg_controlled_learning_proposals_guard_update",
		"trg_controlled_learning_proposals_validate_insert",
		"trg_controlled_learning_proposals_no_delete",
		"trg_controlled_learning_proposal_evidence_immutable",
		"trg_controlled_learning_review_decisions_immutable",
		"trg_controlled_learning_proposals_require_decision",
		"trg_controlled_learning_decisions_require_state",
		"controlled learning ledger records are append-only",
		"controlled learning proposal definitions are immutable",
		"controlled learning proposal initial state is invalid",
		"controlled learning review decision and state transition must commit atomically",
		"DEFERRABLE INITIALLY DEFERRED",
		"NEW.revision <> OLD.revision + 1",
		"OLD.proposal_status = 'governance_required'",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("up migration is missing contract fragment %q", fragment)
		}
	}
	requireMigrationOrder(t, up,
		"CREATE TABLE public.controlled_learning_outcomes",
		"CREATE TABLE public.controlled_learning_proposals",
		"CREATE TABLE public.controlled_learning_proposal_evidence",
		"CREATE TABLE public.controlled_learning_review_decisions",
		"CREATE OR REPLACE FUNCTION public.hai_reject_controlled_learning_mutation()",
		"CREATE OR REPLACE FUNCTION public.hai_guard_controlled_learning_proposal_state()",
		"CREATE OR REPLACE FUNCTION public.hai_validate_controlled_learning_proposal_insert()",
		"CREATE OR REPLACE FUNCTION public.hai_require_controlled_learning_review_pair()",
		"CREATE TRIGGER trg_controlled_learning_outcomes_immutable",
		"CREATE CONSTRAINT TRIGGER trg_controlled_learning_proposals_require_decision",
		"CREATE CONSTRAINT TRIGGER trg_controlled_learning_decisions_require_state",
	)

	down := string(downBytes)
	for _, fragment := range []string{
		"DROP TRIGGER IF EXISTS trg_controlled_learning_review_decisions_no_truncate",
		"DROP TRIGGER IF EXISTS trg_controlled_learning_proposal_evidence_immutable",
		"DROP TRIGGER IF EXISTS trg_controlled_learning_proposals_guard_update",
		"DROP TRIGGER IF EXISTS trg_controlled_learning_outcomes_immutable",
		"DROP TRIGGER IF EXISTS trg_controlled_learning_proposals_require_decision",
		"DROP TRIGGER IF EXISTS trg_controlled_learning_decisions_require_state",
		"DROP FUNCTION IF EXISTS public.hai_guard_controlled_learning_proposal_state()",
		"DROP FUNCTION IF EXISTS public.hai_validate_controlled_learning_proposal_insert()",
		"DROP FUNCTION IF EXISTS public.hai_require_controlled_learning_review_pair()",
		"DROP FUNCTION IF EXISTS public.hai_reject_controlled_learning_mutation()",
		"DROP TABLE IF EXISTS public.controlled_learning_review_decisions",
		"DROP TABLE IF EXISTS public.controlled_learning_proposal_evidence",
		"DROP TABLE IF EXISTS public.controlled_learning_proposals",
		"DROP TABLE IF EXISTS public.controlled_learning_outcomes",
	} {
		if !strings.Contains(down, fragment) {
			t.Errorf("down migration is missing contract fragment %q", fragment)
		}
	}
	requireMigrationOrder(t, down,
		"DROP TRIGGER IF EXISTS trg_controlled_learning_review_decisions_no_truncate",
		"DROP TRIGGER IF EXISTS trg_controlled_learning_outcomes_immutable",
		"DROP FUNCTION IF EXISTS public.hai_require_controlled_learning_review_pair()",
		"DROP FUNCTION IF EXISTS public.hai_validate_controlled_learning_proposal_insert()",
		"DROP FUNCTION IF EXISTS public.hai_guard_controlled_learning_proposal_state()",
		"DROP FUNCTION IF EXISTS public.hai_reject_controlled_learning_mutation()",
		"DROP TABLE IF EXISTS public.controlled_learning_review_decisions",
		"DROP TABLE IF EXISTS public.controlled_learning_proposal_evidence",
		"DROP TABLE IF EXISTS public.controlled_learning_proposals",
		"DROP TABLE IF EXISTS public.controlled_learning_outcomes",
	)
}
