package migrations

import (
	"strings"
	"testing"
)

func TestLifeOntologyMigrationIsOwnerScopedImmutableAndReversible(t *testing.T) {
	t.Parallel()

	upBytes, err := Files.ReadFile("pre/0019_life_ontology.up.sql")
	if err != nil {
		t.Fatalf("read life ontology migration: %v", err)
	}
	downBytes, err := Files.ReadFile("pre/0019_life_ontology.down.sql")
	if err != nil {
		t.Fatalf("read life ontology rollback: %v", err)
	}
	up := strings.ToLower(string(upBytes))
	down := strings.ToLower(string(downBytes))

	for _, fragment := range []string{
		"primary key (owner_identity, entity_id)",
		"primary key (owner_identity, relation_id)",
		"primary key (owner_identity, proposal_id)",
		"foreign key (owner_identity, from_entity_id)",
		"foreign key (owner_identity, candidate_left_id)",
		"entity_id = 'life-entity-' || entity_digest",
		"relation_id = 'life-relation-' || relation_digest",
		"proposal_id = 'life-merge-' || proposal_digest",
		"payload #>> '{owneridentity}' = owner_identity",
		"payload #>> '{advisoryonly}' = 'true'",
		"payload #>> '{canexecute}' = 'false'",
		"life ontology records are append-only",
		"before update or delete on public.life_ontology_entities",
		"before truncate on public.life_ontology_merge_proposals",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("life ontology up migration missing %q", fragment)
		}
	}
	if strings.Contains(up, " cascade") || strings.Contains(down, " cascade") {
		t.Fatal("life ontology migration must not use CASCADE")
	}
	for _, fragment := range []string{
		"refusing to roll back non-empty life ontology tables",
		"drop table if exists public.life_ontology_merge_proposals",
		"drop table if exists public.life_ontology_relations",
		"drop table if exists public.life_ontology_entities",
		"drop function if exists public.hai_reject_life_ontology_mutation()",
	} {
		if !strings.Contains(down, fragment) {
			t.Errorf("life ontology down migration missing %q", fragment)
		}
	}
}
