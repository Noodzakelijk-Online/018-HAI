package migrations

import (
	"strings"
	"testing"
)

func TestAgentTeamMessageAcknowledgmentMigrationIsExactAndAppendOnly(t *testing.T) {
	t.Parallel()

	upBytes, err := Files.ReadFile("pre/0059_agent_team_message_acknowledgments.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := string(upBytes)
	for _, fragment := range []string{
		"CREATE TABLE public.agent_team_message_acknowledgments",
		"FOREIGN KEY (owner_identity, team_id, team_version, message_id)",
		"REFERENCES public.agent_team_coordination_messages",
		"UNIQUE (owner_identity, team_id, team_version, idempotency_key)",
		"status IN ('accepted', 'rejected', 'deferred')",
		"message_record.payload #>> '{recipient,id}' <> NEW.recipient_id",
		"message_record.payload #>> '{requiresAck}' <> 'true'",
		"previous_record.status IN ('accepted', 'rejected')",
		"agent team message acknowledgments are append-only",
		"BEFORE UPDATE ON public.agent_team_message_acknowledgments",
		"BEFORE DELETE ON public.agent_team_message_acknowledgments",
		"BEFORE TRUNCATE ON public.agent_team_message_acknowledgments",
	} {
		if !strings.Contains(up, fragment) {
			t.Fatalf("0059 up migration missing %q", fragment)
		}
	}

	downBytes, err := Files.ReadFile("pre/0059_agent_team_message_acknowledgments.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	down := string(downBytes)
	if !strings.Contains(down, "refusing to remove non-empty agent team message acknowledgment ledger") ||
		!strings.Contains(down, "DROP TABLE IF EXISTS public.agent_team_message_acknowledgments") {
		t.Fatal("0059 down migration must refuse data loss before removing the ledger")
	}
}
