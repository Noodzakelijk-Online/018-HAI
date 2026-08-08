package standingmandate

import (
	"context"
	"testing"
	"time"

	"automation-hub-backend/internal/lifeontology"
)

func TestStandingMandateLifecycleProjectsWithoutGrantingGraphAuthority(t *testing.T) {
	now := time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC)
	repository := NewMemoryRepository()
	service, err := NewService(repository, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	graph := lifeontology.NewService(lifeontology.NewMemoryRepository(), func() time.Time { return now.Add(time.Second) })
	if _, err := service.WithLifeOntologyProjection(graph); err != nil {
		t.Fatal(err)
	}
	request := testCreateRequest(now)
	request.Scopes[0].Domains = []string{string(lifeontology.DomainWorkVenture)}
	draft, err := service.Create(context.Background(), request)
	if err != nil || draft.LifeGraph == nil || draft.LifeGraphWarning != "" {
		t.Fatalf("draft projection = %#v err=%v", draft, err)
	}
	if !draft.LifeGraph.AdvisoryOnly || draft.LifeGraph.CanExecute || draft.LifeGraph.GrantsAuthority {
		t.Fatalf("draft graph crossed authority boundary: %#v", draft.LifeGraph)
	}
	active, err := service.Activate(context.Background(), "robert", draft.ID, draft.Revision)
	if err != nil || active.LifeGraph == nil || active.LifeGraph.Primary.Domain != lifeontology.DomainWorkVenture {
		t.Fatalf("active projection = %#v err=%v", active, err)
	}
	action := validAction(now)
	action.Domain = string(lifeontology.DomainWorkVenture)
	decision, err := service.Authorize(context.Background(), active.ID, action)
	if err != nil || decision.Outcome != DecisionAuthorized || len(decision.Evidence.MandateDigest) != 64 {
		t.Fatalf("authorization after projection = %#v err=%v", decision, err)
	}
	revoked, err := service.Revoke(context.Background(), "robert", active.ID, active.Revision, "robert", "bounded work completed")
	if err != nil || revoked.LifeGraph == nil || revoked.LifeGraph.Primary.Status != lifeontology.StatusArchived {
		t.Fatalf("revoked projection = %#v err=%v", revoked, err)
	}
	entities, err := graph.QueryEntities(context.Background(), "robert", lifeontology.EntityQuery{AllowLocalOnly: true, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	mandateRevisions := 0
	projects := 0
	for _, entity := range entities {
		if entity.Type == lifeontology.EntityOutcome {
			mandateRevisions++
		}
		if entity.Type == lifeontology.EntityProject {
			projects++
		}
	}
	if mandateRevisions != 3 || projects != 1 {
		t.Fatalf("projected mandate revisions=%d projects=%d entities=%#v", mandateRevisions, projects, entities)
	}
}
