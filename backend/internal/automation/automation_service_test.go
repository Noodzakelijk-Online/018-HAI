package automation

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"automation-hub-backend/internal/config"
	"automation-hub-backend/internal/events"
	"automation-hub-backend/internal/models"

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

func TestResolveImagePathRejectsTraversal(t *testing.T) {
	previousDir := config.AppConfig.ImageSaveDir
	previousExtensions := config.AppConfig.ImageExtensions
	t.Cleanup(func() {
		config.AppConfig.ImageSaveDir = previousDir
		config.AppConfig.ImageExtensions = previousExtensions
	})
	config.AppConfig.ImageSaveDir = t.TempDir()
	config.AppConfig.ImageExtensions = []string{".png", ".jpg", ".jpeg"}

	if _, err := resolveImagePath("../secret.png"); err == nil {
		t.Fatalf("expected traversal image path to be rejected")
	}
	if _, err := resolveImagePath(`folder\secret.png`); err == nil {
		t.Fatalf("expected backslash image path to be rejected")
	}
}

func TestResolveImagePathAllowsSingleGeneratedFileName(t *testing.T) {
	previousDir := config.AppConfig.ImageSaveDir
	previousExtensions := config.AppConfig.ImageExtensions
	t.Cleanup(func() {
		config.AppConfig.ImageSaveDir = previousDir
		config.AppConfig.ImageExtensions = previousExtensions
	})
	config.AppConfig.ImageSaveDir = t.TempDir()
	config.AppConfig.ImageExtensions = []string{".png", ".jpg", ".jpeg"}

	path, err := resolveImagePath("123e4567-e89b-12d3-a456-426614174000.png")
	if err != nil {
		t.Fatalf("resolveImagePath: %v", err)
	}
	rel, err := filepath.Rel(config.AppConfig.ImageSaveDir, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		t.Fatalf("path = %q, want inside %q", path, config.AppConfig.ImageSaveDir)
	}
}

func TestProcessImageFileRejectsCorruptImage(t *testing.T) {
	previousDir := config.AppConfig.ImageSaveDir
	previousExtensions := config.AppConfig.ImageExtensions
	previousMaxSize := config.AppConfig.ImageMaxSize
	t.Cleanup(func() {
		config.AppConfig.ImageSaveDir = previousDir
		config.AppConfig.ImageExtensions = previousExtensions
		config.AppConfig.ImageMaxSize = previousMaxSize
	})
	config.AppConfig.ImageSaveDir = t.TempDir()
	config.AppConfig.ImageExtensions = []string{".gif"}
	config.AppConfig.ImageMaxSize = 1024

	file := testMultipartFileHeader(t, "imageFile", "bad.gif", []byte("GIF89a"))
	if _, err := (&service{}).processImageFile(file); err == nil {
		t.Fatalf("expected corrupt image to be rejected")
	}
}

func TestCreateRejectsUnsafeHostForGeneratedNginxConfig(t *testing.T) {
	repo := newFakeAutomationRepo(&models.Automation{ID: uuid.New(), Name: "Existing", URLPath: "existing", Host: "backend", Port: 80})
	service := NewService(repo, events.Publisher{})

	created, err := service.Create(&models.Automation{
		Name: "Unsafe Host",
		Host: "backend;\nproxy_pass http://example.com;",
		Port: 80,
	})
	if err == nil {
		t.Fatalf("expected unsafe host validation error")
	}
	if created != nil {
		t.Fatalf("created = %#v, want nil", created)
	}
}

func testMultipartFileHeader(t *testing.T, fieldName, fileName string, content []byte) *multipart.FileHeader {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile(fieldName, fileName)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if err := request.ParseMultipartForm(1024 * 1024); err != nil {
		t.Fatalf("ParseMultipartForm: %v", err)
	}
	files := request.MultipartForm.File[fieldName]
	if len(files) != 1 {
		t.Fatalf("files[%s] length = %d, want 1", fieldName, len(files))
	}
	return files[0]
}

func TestHealthCheckBlocksHTTPOutsideAllowlist(t *testing.T) {
	id := uuid.New()
	repo := newFakeAutomationRepo(&models.Automation{
		ID:                 id,
		Name:               "External Health",
		URLPath:            "external-health",
		Host:               "example.com",
		Port:               80,
		HealthCheckType:    "http",
		HealthCheckURL:     "http://example.com/health",
		ExpectedHTTPStatus: http.StatusOK,
	})
	service := NewService(repo, events.Publisher{})

	result, err := service.RunHealthCheck(id)
	if err != nil {
		t.Fatalf("RunHealthCheck: %v", err)
	}
	if result.Status == "healthy" {
		t.Fatalf("status = %q, want blocked health check failure", result.Status)
	}
	if !strings.Contains(result.FailureReason, "not allowlisted") {
		t.Fatalf("failureReason = %q, want allowlist failure", result.FailureReason)
	}
}

func TestHealthCheckBlocksTCPOutsideAllowlist(t *testing.T) {
	id := uuid.New()
	repo := newFakeAutomationRepo(&models.Automation{
		ID:              id,
		Name:            "External TCP Health",
		URLPath:         "external-tcp-health",
		Host:            "example.com",
		Port:            443,
		HealthCheckType: "tcp",
	})
	service := NewService(repo, events.Publisher{})

	result, err := service.RunHealthCheck(id)
	if err != nil {
		t.Fatalf("RunHealthCheck: %v", err)
	}
	if result.Status == "healthy" {
		t.Fatalf("status = %q, want blocked health check failure", result.Status)
	}
	if !strings.Contains(result.FailureReason, "not allowlisted") {
		t.Fatalf("failureReason = %q, want allowlist failure", result.FailureReason)
	}
}

func TestHealthCheckDoesNotFollowHTTPRedirect(t *testing.T) {
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
		Name:               "Redirect Health",
		URLPath:            "redirect-health",
		Host:               "localhost",
		Port:               8080,
		HealthCheckType:    "http",
		HealthCheckURL:     server.URL,
		ExpectedHTTPStatus: http.StatusOK,
	})
	service := NewService(repo, events.Publisher{})

	result, err := service.RunHealthCheck(id)
	if err != nil {
		t.Fatalf("RunHealthCheck: %v", err)
	}
	if redirectCalled {
		t.Fatalf("redirect target was called; health checks must not follow redirects")
	}
	if result.Status == "healthy" {
		t.Fatalf("status = %q, want failed redirect status", result.Status)
	}
	if !strings.Contains(result.FailureReason, "unexpected HTTP status") {
		t.Fatalf("failureReason = %q, want unexpected HTTP status", result.FailureReason)
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
