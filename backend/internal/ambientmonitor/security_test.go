package ambientmonitor

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestOwnerAndWorkspaceIsolationAtRepositoryBoundary(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 5, 13, 0, 0, 0, time.UTC)
	repository := NewMemoryRepository()
	service := newService(repository, nil, nil, func() time.Time { return now })
	scopeA := Scope{OwnerID: "owner-a", WorkspaceID: "workspace-a"}
	target, _, err := service.RegisterTarget(t.Context(), testRegisterRequest(scopeA, now))
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []Scope{{OwnerID: "owner-b", WorkspaceID: "workspace-a"}, {OwnerID: "owner-a", WorkspaceID: "workspace-b"}} {
		if _, err := service.Target(t.Context(), scope, target.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("cross-scope Target(%+v) error = %v, want not found", scope, err)
		}
		items, err := service.Targets(t.Context(), scope)
		if err != nil || len(items) != 0 {
			t.Errorf("cross-scope Targets(%+v) = (%+v, %v)", scope, items, err)
		}
		claims, err := service.ClaimDue(t.Context(), ClaimDueRequest{Scope: scope, WorkerID: "worker-a", Now: now, LeaseDuration: time.Minute, Limit: 1})
		if err != nil || len(claims) != 0 {
			t.Errorf("cross-scope claims = (%+v, %v)", claims, err)
		}
	}
}

func TestSecretsAreRejectedAndCollectorErrorsAreNeverPersisted(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"Authorization: Bearer private", "client_secret=value", "password=hunter2", "sk-secret", "ghp_private", "xoxb-private"} {
		if err := validateBoundedText("text", value, maxFailureLength, true); err == nil {
			t.Errorf("secret text %q accepted", value)
		}
		if err := validateIdentifier("id", strings.ReplaceAll(value, " ", "-")); err == nil {
			t.Errorf("secret identifier %q accepted", value)
		}
	}
}

func TestMemoryRepositoryRejectsInjectedAuthorityAndPreservesCopies(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 5, 14, 0, 0, 0, time.UTC)
	scope := Scope{OwnerID: "owner-a", WorkspaceID: "workspace-a"}
	target := MonitorTarget{ContractVersion: ContractVersion, ID: "target-a", Scope: scope, OutcomeID: "outcome-a", IndicatorID: "indicator-a", SourceKind: SourceOverdueCommitmentCount, Enabled: true, Cadence: time.Hour, NextRunAt: now, CreatedAt: now, UpdatedAt: now, Authority: advisoryAuthority()}
	target.Authority.CanExecute = true
	if _, _, err := NewMemoryRepository().CreateTarget(t.Context(), scope.OwnerID, scope.WorkspaceID, "create-target", target); err == nil {
		t.Fatal("repository accepted execution authority")
	}
	target.Authority = advisoryAuthority()
	repository := NewMemoryRepository()
	stored, created, err := repository.CreateTarget(t.Context(), scope.OwnerID, scope.WorkspaceID, "create-target", target)
	if err != nil || !created {
		t.Fatalf("CreateTarget() = (%+v, %v, %v)", stored, created, err)
	}
	stored.OutcomeID = "mutated"
	again, err := repository.GetTarget(t.Context(), scope.OwnerID, scope.WorkspaceID, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.OutcomeID != "outcome-a" {
		t.Fatalf("repository alias mutated stored target: %+v", again)
	}
}

func TestInterfacesExposeNoOperationalMutationCapability(t *testing.T) {
	t.Parallel()
	for _, value := range []any{(*Collector)(nil), (*Sink)(nil)} {
		typeValue := reflect.TypeOf(value).Elem()
		for index := 0; index < typeValue.NumMethod(); index++ {
			name := strings.ToLower(typeValue.Method(index).Name)
			for _, forbidden := range []string{"execute", "deliver", "notify", "calendar", "workflow", "mandate", "learn", "send", "publish"} {
				if strings.Contains(name, forbidden) {
					t.Fatalf("%s exposes forbidden method %q", typeValue.Name(), name)
				}
			}
		}
	}
	if reflect.TypeOf((*Collector)(nil)).Elem().NumMethod() != 1 || reflect.TypeOf((*Sink)(nil)).Elem().NumMethod() != 1 {
		t.Fatal("collector and sink contracts must remain narrow")
	}
}

func TestServiceRejectsTypedNilDependencies(t *testing.T) {
	t.Parallel()
	var repository *MemoryRepository
	service := NewService(repository, nil, nil)
	if _, err := service.Targets(context.Background(), Scope{OwnerID: "owner-a", WorkspaceID: "workspace-a"}); !errors.Is(err, ErrRepositoryUnavailable) {
		t.Fatalf("typed-nil repository error = %v", err)
	}
	var collector *deterministicCollector
	service = NewService(NewMemoryRepository(), collector, nil)
	if _, err := service.ProcessClaim(context.Background(), ProcessClaimRequest{}); !errors.Is(err, ErrCollectorUnavailable) {
		t.Fatalf("typed-nil collector error = %v", err)
	}
}
