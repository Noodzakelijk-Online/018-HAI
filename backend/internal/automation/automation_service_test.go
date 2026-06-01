package automation

import (
	"automation-hub-backend/internal/events"
	"automation-hub-backend/internal/models"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestLaunchExecutesAPITargetAndAuditsResult(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		_, _ = w.Write([]byte("started"))
	}))
	defer server.Close()

	id := uuid.New()
	repo := newFakeAutomationRepo(&models.Automation{
		ID:                 id,
		Name:               "API Automation",
		URLPath:            "api-automation",
		Host:               "localhost",
		Port:               8080,
		LaunchType:         "api",
		LaunchTarget:       server.URL,
		ExpectedHTTPStatus: http.StatusOK,
	})
	service := NewService(repo, events.Publisher{})

	result, err := service.Launch(id)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if !called {
		t.Fatalf("expected API target to be called")
	}
	if result.Status != "completed" {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	if result.Output != "started" {
		t.Fatalf("output = %q, want started", result.Output)
	}
	if len(repo.launchEvents) != 1 {
		t.Fatalf("expected launch event to be persisted")
	}
}

func TestLaunchDoesNotFollowAPIRedirect(t *testing.T) {
	redirectCalled := false
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectCalled = true
	}))
	defer redirectTarget.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectTarget.URL, http.StatusFound)
	}))
	defer server.Close()

	id := uuid.New()
	repo := newFakeAutomationRepo(&models.Automation{
		ID:                 id,
		Name:               "Redirect API Automation",
		URLPath:            "redirect-api-automation",
		LaunchType:         "api",
		LaunchTarget:       server.URL,
		ExpectedHTTPStatus: http.StatusOK,
	})
	service := NewService(repo, events.Publisher{})

	result, err := service.Launch(id)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if redirectCalled {
		t.Fatalf("redirect target was called; API launch must not follow redirects")
	}
	if result.Status != "failed" || result.ExitCode != http.StatusFound {
		t.Fatalf("status/exit = %q/%d, want failed/%d", result.Status, result.ExitCode, http.StatusFound)
	}
}

func TestLaunchRunsAllowlistedScriptWithoutShell(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "launch.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho script-ok\n"), 0755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("AUTOMATION_SCRIPT_EXECUTION_ENABLED", "true")
	t.Setenv("AUTOMATION_SCRIPT_DIR", dir)

	id := uuid.New()
	repo := newFakeAutomationRepo(&models.Automation{
		ID:           id,
		Name:         "Script Automation",
		URLPath:      "script-automation",
		Host:         "localhost",
		Port:         8080,
		LaunchType:   "script",
		LaunchTarget: "launch.sh",
	})
	service := NewService(repo, events.Publisher{})

	result, err := service.Launch(id)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("status = %q, want completed: %s", result.Status, result.Message)
	}
	if result.Output != "script-ok" {
		t.Fatalf("output = %q, want script-ok", result.Output)
	}
}

func TestLaunchRunsScriptWithMinimalEnvironment(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "env.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nif [ -n \"$SECRET_TOKEN\" ]; then echo leaked; else echo clean; fi\n"), 0755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("AUTOMATION_SCRIPT_EXECUTION_ENABLED", "true")
	t.Setenv("AUTOMATION_SCRIPT_DIR", dir)
	t.Setenv("SECRET_TOKEN", "must-not-leak")

	id := uuid.New()
	repo := newFakeAutomationRepo(&models.Automation{
		ID:           id,
		Name:         "Script Automation",
		URLPath:      "script-automation",
		Host:         "localhost",
		Port:         8080,
		LaunchType:   "script",
		LaunchTarget: "env.sh",
	})
	service := NewService(repo, events.Publisher{})

	result, err := service.Launch(id)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("status = %q, want completed: %s", result.Status, result.Message)
	}
	if result.Output != "clean" {
		t.Fatalf("output = %q, want clean", result.Output)
	}
}

func TestLaunchBlocksScriptWhenPolicyDisabled(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "launch.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho script-ok\n"), 0755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("AUTOMATION_SCRIPT_EXECUTION_ENABLED", "false")
	t.Setenv("AUTOMATION_SCRIPT_DIR", dir)

	id := uuid.New()
	repo := newFakeAutomationRepo(&models.Automation{
		ID:           id,
		Name:         "Script Automation",
		URLPath:      "script-automation",
		Host:         "localhost",
		Port:         8080,
		LaunchType:   "script",
		LaunchTarget: "launch.sh",
	})
	service := NewService(repo, events.Publisher{})

	result, err := service.Launch(id)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if result.Status != "blocked" {
		t.Fatalf("status = %q, want blocked", result.Status)
	}
	if !result.RequiresApproval {
		t.Fatalf("expected disabled script execution to require approval")
	}
}

func TestLaunchBlocksScriptOutsideAllowlist(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AUTOMATION_SCRIPT_EXECUTION_ENABLED", "true")
	t.Setenv("AUTOMATION_SCRIPT_DIR", dir)

	id := uuid.New()
	repo := newFakeAutomationRepo(&models.Automation{
		ID:           id,
		Name:         "Blocked Script",
		URLPath:      "blocked-script",
		Host:         "localhost",
		Port:         8080,
		LaunchType:   "script",
		LaunchTarget: "../outside.sh",
	})
	service := NewService(repo, events.Publisher{})

	result, err := service.Launch(id)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if result.Status != "blocked" {
		t.Fatalf("status = %q, want blocked", result.Status)
	}
	if !result.RequiresApproval {
		t.Fatalf("expected blocked script to require approval")
	}
}

func TestLaunchBlocksScriptSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.sh")
	if err := os.WriteFile(outside, []byte("#!/bin/sh\necho outside\n"), 0755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	link := filepath.Join(dir, "link.sh")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink not available on this platform: %v", err)
	}
	t.Setenv("AUTOMATION_SCRIPT_EXECUTION_ENABLED", "true")
	t.Setenv("AUTOMATION_SCRIPT_DIR", dir)

	id := uuid.New()
	repo := newFakeAutomationRepo(&models.Automation{
		ID:           id,
		Name:         "Blocked Script",
		URLPath:      "blocked-script",
		Host:         "localhost",
		Port:         8080,
		LaunchType:   "script",
		LaunchTarget: "link.sh",
	})
	service := NewService(repo, events.Publisher{})

	result, err := service.Launch(id)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if result.Status != "blocked" {
		t.Fatalf("status = %q, want blocked", result.Status)
	}
	if result.Output != "" {
		t.Fatalf("output = %q, want no script output", result.Output)
	}
}

func TestLaunchBlocksDockerWhenPolicyDisabled(t *testing.T) {
	t.Setenv("AUTOMATION_DOCKER_CONTROL_ENABLED", "false")

	id := uuid.New()
	repo := newFakeAutomationRepo(&models.Automation{
		ID:           id,
		Name:         "Docker Automation",
		URLPath:      "docker-automation",
		Host:         "localhost",
		Port:         8080,
		LaunchType:   "docker_service",
		LaunchTarget: "container-name",
	})
	service := NewService(repo, events.Publisher{})

	result, err := service.Launch(id)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if result.Status != "blocked" {
		t.Fatalf("status = %q, want blocked", result.Status)
	}
	if len(repo.launchEvents) != 1 {
		t.Fatalf("expected blocked launch to be audited")
	}
}

func TestLaunchBlocksDockerWhenContainerNotAllowlisted(t *testing.T) {
	t.Setenv("AUTOMATION_DOCKER_CONTROL_ENABLED", "true")
	t.Setenv("AUTOMATION_DOCKER_ALLOWED_CONTAINERS", "safe-container")

	id := uuid.New()
	repo := newFakeAutomationRepo(&models.Automation{
		ID:           id,
		Name:         "Docker Automation",
		URLPath:      "docker-automation",
		Host:         "localhost",
		Port:         8080,
		LaunchType:   "docker_service",
		LaunchTarget: "dangerous-container",
	})
	service := NewService(repo, events.Publisher{})

	result, err := service.Launch(id)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if result.Status != "blocked" {
		t.Fatalf("status = %q, want blocked", result.Status)
	}
	if !result.RequiresApproval {
		t.Fatalf("expected blocked docker launch to require approval")
	}
}

func TestLaunchBlocksAPITargetOutsideAllowlist(t *testing.T) {
	id := uuid.New()
	repo := newFakeAutomationRepo(&models.Automation{
		ID:           id,
		Name:         "External API Automation",
		URLPath:      "external-api-automation",
		Host:         "localhost",
		Port:         8080,
		LaunchType:   "api",
		LaunchTarget: "POST https://example.com/start",
	})
	service := NewService(repo, events.Publisher{})

	result, err := service.Launch(id)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if result.Status != "blocked" {
		t.Fatalf("status = %q, want blocked", result.Status)
	}
	if !result.RequiresApproval {
		t.Fatalf("expected blocked API launch to require approval")
	}
}

func TestLaunchBlocksAPILinkLocalTarget(t *testing.T) {
	t.Setenv("AUTOMATION_API_ALLOWED_HOSTS", "169.254.169.254")

	id := uuid.New()
	repo := newFakeAutomationRepo(&models.Automation{
		ID:           id,
		Name:         "Metadata API Automation",
		URLPath:      "metadata-api-automation",
		Host:         "localhost",
		Port:         8080,
		LaunchType:   "api",
		LaunchTarget: "POST http://169.254.169.254/latest/meta-data",
	})
	service := NewService(repo, events.Publisher{})

	result, err := service.Launch(id)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if result.Status != "blocked" {
		t.Fatalf("status = %q, want blocked", result.Status)
	}
}

type fakeAutomationRepo struct {
	automation   *models.Automation
	launchEvents []models.AutomationLaunchEvent
	healthEvents []models.AutomationHealthEvent
}

func newFakeAutomationRepo(automation *models.Automation) *fakeAutomationRepo {
	return &fakeAutomationRepo{automation: automation}
}

func (r *fakeAutomationRepo) FindByID(id uuid.UUID) (*models.Automation, error) {
	if r.automation.ID != id {
		return nil, gorm.ErrRecordNotFound
	}
	copied := *r.automation
	return &copied, nil
}

func (r *fakeAutomationRepo) Create(automation *models.Automation) (*models.Automation, error) {
	r.automation = automation
	return automation, nil
}

func (r *fakeAutomationRepo) Update(automation *models.Automation) (*models.Automation, error) {
	r.automation = automation
	return automation, nil
}

func (r *fakeAutomationRepo) Delete(id uuid.UUID) error {
	return nil
}

func (r *fakeAutomationRepo) FindAll() ([]*models.Automation, error) {
	return []*models.Automation{r.automation}, nil
}

func (r *fakeAutomationRepo) MaxPosition() (int, error) {
	return 0, nil
}

func (r *fakeAutomationRepo) GetByURLPath(urlPath string) (*models.Automation, error) {
	return nil, gorm.ErrRecordNotFound
}

func (r *fakeAutomationRepo) Transaction(txFunc func(tx *gorm.DB) error) (err error) {
	return nil
}

func (r *fakeAutomationRepo) SaveHealthEvent(event *models.AutomationHealthEvent) error {
	r.healthEvents = append(r.healthEvents, *event)
	return nil
}

func (r *fakeAutomationRepo) FindHealthEvents(automationID uuid.UUID, limit int) ([]models.AutomationHealthEvent, error) {
	return r.healthEvents, nil
}

func (r *fakeAutomationRepo) SaveLaunchEvent(event *models.AutomationLaunchEvent) error {
	event.ID = uuid.New()
	r.launchEvents = append(r.launchEvents, *event)
	return nil
}

func (r *fakeAutomationRepo) FindLaunchEvents(automationID uuid.UUID, limit int) ([]models.AutomationLaunchEvent, error) {
	return r.launchEvents, nil
}
