package browserverify

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"automation-hub-backend/internal/models"
	"github.com/google/uuid"
)

type runRepositoryStub struct{ runs map[uuid.UUID]models.BrowserVerificationRun }

func (r *runRepositoryStub) Create(run *models.BrowserVerificationRun) (*models.BrowserVerificationRun, error) {
	if run.ID == uuid.Nil {
		run.ID = uuid.New()
	}
	if r.runs == nil {
		r.runs = map[uuid.UUID]models.BrowserVerificationRun{}
	}
	r.runs[run.ID] = *run
	return run, nil
}
func (r *runRepositoryStub) Update(run *models.BrowserVerificationRun) (*models.BrowserVerificationRun, error) {
	r.runs[run.ID] = *run
	return run, nil
}
func (r *runRepositoryStub) List(owner string, _ int) ([]models.BrowserVerificationRun, error) {
	result := []models.BrowserVerificationRun{}
	for _, run := range r.runs {
		if run.OwnerIdentity == owner {
			result = append(result, run)
		}
	}
	return result, nil
}

type workflowLinkerStub struct {
	owner, workflowID, runID, profileID, status, finalPath, pageTitle, summary string
	err                                                                        error
}

func (s *workflowLinkerStub) AttachBrowserVerification(owner, workflowID, runID, profileID, status, finalPath, pageTitle, summary string) error {
	s.owner, s.workflowID, s.runID, s.profileID = owner, workflowID, runID, profileID
	s.status, s.finalPath, s.pageTitle, s.summary = status, finalPath, pageTitle, summary
	return s.err
}

func TestStatusKeepsBrowserVerificationDisabledUntilFullyConfigured(t *testing.T) {
	svc := NewService(nil, false, "http://browser-verifier:8080", "not-used", nil)
	if status := svc.Status(); status.Enabled || status.Configured {
		t.Fatalf("disabled browser verification must not report configured: %#v", status)
	}
	profiles := []Profile{{ID: "local-login", Name: "Local login", URL: "http://frontend/login", ExpectedPath: "/login"}}
	svc = NewService(nil, true, "http://browser-verifier:8080", "0123456789abcdef", profiles)
	if status := svc.Status(); !status.Configured || status.Scope == "" {
		t.Fatalf("complete local browser profile should be configured: %#v", status)
	}
}

func TestBrowserVerificationRejectsRemoteAndQueryBearingProfiles(t *testing.T) {
	profiles := []Profile{{ID: "remote", Name: "Remote", URL: "https://example.com/"}}
	svc := NewService(nil, true, "http://browser-verifier:8080", "0123456789abcdef", profiles)
	if svc.Status().ConfigError == "" {
		t.Fatalf("remote profile must be rejected")
	}
	profiles = []Profile{{ID: "query", Name: "Query", URL: "http://frontend/login?token=secret"}}
	svc = NewService(nil, true, "http://browser-verifier:8080", "0123456789abcdef", profiles)
	if svc.Status().ConfigError == "" {
		t.Fatalf("query-bearing profile must be rejected")
	}
}

func TestCompletedBrowserVerificationCanLinkAWorkflowQualitySignal(t *testing.T) {
	runner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/verify" || r.Header.Get("X-HAI-Browser-Token") != "0123456789abcdef" {
			t.Fatalf("unexpected browser verifier request")
		}
		_, _ = w.Write([]byte(`{"status":"passed","finalPath":"/login","pageTitle":"HAI","summary":"named local route reached"}`))
	}))
	defer runner.Close()
	repo := &runRepositoryStub{}
	linker := &workflowLinkerStub{}
	svc := NewService(repo, true, runner.URL, "0123456789abcdef", []Profile{{ID: "local-login", Name: "Local login", URL: "http://frontend/login", ExpectedPath: "/login"}}, linker)
	svc.now = func() time.Time { return time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC) }
	workflowID := uuid.New().String()

	run, err := svc.Run(context.Background(), "owner@example.test", "local-login", workflowID)
	if err != nil || run.Status != "passed" || run.WorkflowLinkStatus != "linked_quality_signal" {
		t.Fatalf("run = %#v err=%v", run, err)
	}
	if linker.owner != "owner@example.test" || linker.workflowID != workflowID || linker.runID != run.ID || linker.status != "passed" || linker.finalPath != "/login" || linker.pageTitle != "HAI" {
		t.Fatalf("unexpected workflow link: %#v", linker)
	}
	if stored := repo.runs[uuid.MustParse(run.ID)]; stored.Status != "passed" || stored.CompletedAt == nil {
		t.Fatalf("browser result was not persisted before linkage: %#v", stored)
	}
}

func TestBrowserVerificationReportsWorkflowLinkFailureWithoutHidingItsResult(t *testing.T) {
	runner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"failed","summary":"unexpected local route"}`))
	}))
	defer runner.Close()
	svc := NewService(&runRepositoryStub{}, true, runner.URL, "0123456789abcdef", []Profile{{ID: "local-route", Name: "Local route", URL: "http://frontend/", ExpectedPath: "/"}}, &workflowLinkerStub{err: errors.New("workflow not found")})
	run, err := svc.Run(context.Background(), "owner@example.test", "local-route", uuid.New().String())
	if err != nil || run.Status != "failed" || run.WorkflowLinkStatus != "link_failed" || run.WorkflowLinkError == "" {
		t.Fatalf("run = %#v err=%v", run, err)
	}
}
