package automation

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"automation-hub-backend/internal/agentruntime"
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

	result, err := service.LaunchTask(id, TaskLaunchRequest{OwnerIdentity: "alice"})
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
	if result.LaunchEventID == uuid.Nil || result.LaunchEventID != repo.launchEvents[0].ID {
		t.Fatalf("launch event id was not returned with launch result: result=%s event=%s", result.LaunchEventID, repo.launchEvents[0].ID)
	}
	if repo.launchEvents[0].OwnerIdentity != "alice" {
		t.Fatalf("launch event owner = %q, want alice", repo.launchEvents[0].OwnerIdentity)
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

func TestLaunchRedactsScriptOutputSecrets(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "leak.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho 'token=super-secret-token'\n"), 0755); err != nil {
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
		LaunchTarget: "leak.sh",
	})
	service := NewService(repo, events.Publisher{})

	result, err := service.Launch(id)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if strings.Contains(result.Output, "super-secret-token") {
		t.Fatalf("script output leaked secret: %s", result.Output)
	}
	if len(repo.launchEvents) != 1 || strings.Contains(repo.launchEvents[0].Output, "super-secret-token") {
		t.Fatalf("launch event leaked secret: %#v", repo.launchEvents)
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

func TestLaunchBlocksWhenEmergencyStopActive(t *testing.T) {
	t.Setenv("HAI_EMERGENCY_STOP", "true")

	id := uuid.New()
	repo := newFakeAutomationRepo(&models.Automation{
		ID:           id,
		Name:         "Emergency Automation",
		URLPath:      "emergency-automation",
		Host:         "localhost",
		Port:         8080,
		LaunchType:   "browser_url",
		LaunchTarget: "http://localhost:8080",
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
		t.Fatalf("emergency stop block should require review")
	}
	if len(repo.launchEvents) != 1 || repo.launchEvents[0].Status != "blocked" {
		t.Fatalf("expected blocked launch event, got %#v", repo.launchEvents)
	}
}

func TestAgentRuntimeLaunchRequiresApprovalAndReceivesTask(t *testing.T) {
	id := uuid.New()
	adapter := &fakeAgentRuntimeAdapter{id: "hermes"}
	registry := agentruntime.NewRegistry(adapter)
	repo := newFakeAutomationRepo(&models.Automation{
		ID:           id,
		Name:         "Hermes Runtime",
		URLPath:      "hermes-runtime",
		Host:         "localhost",
		Port:         8080,
		LaunchType:   "agent_runtime",
		RuntimeType:  "hermes",
		LaunchTarget: "runtime://hermes",
	})
	service := NewServiceWithRuntimeRegistry(repo, events.Publisher{}, registry)

	blocked, err := service.Launch(id)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if blocked.Status != "blocked" || adapter.called {
		t.Fatalf("direct launch bypassed approval: %#v", blocked)
	}
	if len(repo.launchEvents) != 1 || !containsString(repo.launchEvents[0].AuditEvents, "agent approval gate blocked execution") {
		t.Fatalf("blocked runtime audit was not persisted: %#v", repo.launchEvents)
	}

	completed, err := service.LaunchTask(id, TaskLaunchRequest{
		Task:          "Inspect the project and report verified completion.",
		ProjectKey:    "018-hai",
		HumanApproved: true,
	})
	if err != nil {
		t.Fatalf("LaunchTask: %v", err)
	}
	if completed.Status != "completed" || !adapter.called {
		t.Fatalf("approved task did not run: %#v", completed)
	}
	if completed.RuntimeTaskID != id.String() {
		t.Fatalf("runtime task id = %q, want automation id %s", completed.RuntimeTaskID, id)
	}
	if adapter.task.Prompt != "Inspect the project and report verified completion." || adapter.task.ProjectKey != "018-hai" {
		t.Fatalf("task context was not propagated: %#v", adapter.task)
	}
	if adapter.task.ID != id.String() {
		t.Fatalf("adapter task id = %q, want %s", adapter.task.ID, id)
	}
	if len(repo.launchEvents) != 2 || !containsString(repo.launchEvents[1].AuditEvents, "fake runtime executed under approval") {
		t.Fatalf("completed runtime audit was not persisted: %#v", repo.launchEvents)
	}
	if repo.launchEvents[1].RuntimeTaskID != id.String() {
		t.Fatalf("launch event runtime task id = %q, want %s", repo.launchEvents[1].RuntimeTaskID, id)
	}
}

func TestStopRuntimeTaskUsesAutomationRuntimeAndTaskID(t *testing.T) {
	id := uuid.New()
	adapter := &fakeAgentRuntimeAdapter{id: "openclaw"}
	registry := agentruntime.NewRegistry(adapter)
	repo := newFakeAutomationRepo(&models.Automation{
		ID:           id,
		Name:         "OpenClaw Runtime",
		URLPath:      "openclaw-runtime",
		Host:         "localhost",
		Port:         8080,
		LaunchType:   "agent_runtime",
		RuntimeType:  "openclaw",
		LaunchTarget: "runtime://openclaw",
	})
	service := NewServiceWithRuntimeRegistry(repo, events.Publisher{}, registry)

	result, err := service.StopRuntimeTask(id)
	if err != nil {
		t.Fatalf("StopRuntimeTask: %v", err)
	}
	if result.RuntimeID != "openclaw" || result.TaskID != id.String() || result.Status != "stopped" {
		t.Fatalf("stop result = %#v", result)
	}
	if result.EvidenceURI == "" {
		t.Fatalf("stop result missing evidence URI: %#v", result)
	}
	if len(repo.launchEvents) != 1 {
		t.Fatalf("expected runtime stop launch event, got %#v", repo.launchEvents)
	}
	event := repo.launchEvents[0]
	if event.LaunchType != "agent_runtime_stop" || event.RuntimeTaskID != id.String() || event.Status != "stopped" {
		t.Fatalf("runtime stop event = %#v", event)
	}
	if result.EvidenceURI != "automation-launch://"+event.ID.String() {
		t.Fatalf("evidence URI = %q, want event %s", result.EvidenceURI, event.ID)
	}
	if !containsString(event.AuditEvents, "runtime stop requested") {
		t.Fatalf("runtime stop audit missing: %#v", event.AuditEvents)
	}
}

func TestAgentRuntimeLaunchAllowsOpenClaw(t *testing.T) {
	id := uuid.New()
	adapter := &fakeAgentRuntimeAdapter{id: "openclaw"}
	registry := agentruntime.NewRegistry(adapter)
	repo := newFakeAutomationRepo(&models.Automation{
		ID:           id,
		Name:         "OpenClaw Runtime",
		URLPath:      "openclaw-runtime",
		Host:         "localhost",
		Port:         8080,
		LaunchType:   "agent_runtime",
		RuntimeType:  "openclaw",
		LaunchTarget: "runtime://openclaw",
	})
	service := NewServiceWithRuntimeRegistry(repo, events.Publisher{}, registry)

	completed, err := service.LaunchTask(id, TaskLaunchRequest{
		Task:          "Move approved OpenClaw work forward safely.",
		ProjectKey:    "018-hai",
		HumanApproved: true,
	})
	if err != nil {
		t.Fatalf("LaunchTask: %v", err)
	}
	if completed.Status != "completed" || !adapter.called {
		t.Fatalf("approved OpenClaw task did not run: %#v", completed)
	}
	if adapter.task.Prompt != "Move approved OpenClaw work forward safely." {
		t.Fatalf("task context was not propagated: %#v", adapter.task)
	}
	if completed.RuntimeRouteTrace == nil || completed.RuntimeRouteTrace.RuntimeID != "openclaw" || !containsString(completed.RuntimeRouteTrace.RecommendedSkills, "autoreview") {
		t.Fatalf("runtime route trace missing from launch result: %#v", completed.RuntimeRouteTrace)
	}
	if len(repo.launchEvents) != 1 || repo.launchEvents[0].RuntimeRouteTrace == nil || !containsString(repo.launchEvents[0].RuntimeRouteTrace.VisibleTools, "browser") {
		t.Fatalf("runtime route trace missing from launch event: %#v", repo.launchEvents)
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

func TestLaunchRedactsAPITargetAndResponseSecrets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("password=hunter2"))
	}))
	defer server.Close()
	t.Setenv("AUTOMATION_API_ALLOWED_HOSTS", "127.0.0.1")

	id := uuid.New()
	repo := newFakeAutomationRepo(&models.Automation{
		ID:           id,
		Name:         "API Automation",
		URLPath:      "api-automation",
		Host:         "localhost",
		Port:         8080,
		LaunchType:   "api",
		LaunchTarget: "POST " + server.URL + "/start?token=super-secret-token",
	})
	service := NewService(repo, events.Publisher{})

	result, err := service.Launch(id)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	for _, value := range []string{result.Target, result.Message, result.Output, repo.launchEvents[0].Target, repo.launchEvents[0].Message, repo.launchEvents[0].Output} {
		if strings.Contains(value, "super-secret-token") || strings.Contains(value, "hunter2") {
			t.Fatalf("launch leaked secret in %q", value)
		}
	}
}

type fakeAutomationRepo struct {
	automation   *models.Automation
	launchEvents []models.AutomationLaunchEvent
	healthEvents []models.AutomationHealthEvent
}

type fakeAgentRuntimeAdapter struct {
	id     string
	called bool
	task   agentruntime.Task
}

func (a *fakeAgentRuntimeAdapter) Info() agentruntime.Info {
	id := firstNonEmptyString(a.id, "hermes")
	return agentruntime.Info{
		ID:               id,
		Name:             id,
		Enabled:          true,
		Configured:       true,
		ExecutionEnabled: true,
		RequiresApproval: true,
	}
}

func (a *fakeAgentRuntimeAdapter) HealthCheck(context.Context) agentruntime.Health {
	return agentruntime.Health{RuntimeID: firstNonEmptyString(a.id, "hermes"), Status: "ready"}
}

func (a *fakeAgentRuntimeAdapter) ListSkills(context.Context) []agentruntime.Skill {
	id := firstNonEmptyString(a.id, "hermes")
	return []agentruntime.Skill{{
		ID:               id + ":skill:test",
		RuntimeID:        id,
		Name:             "test",
		Category:         "skill",
		RiskLevel:        "low",
		ApprovalRequired: false,
		ExecutionMode:    "test",
	}}
}

func (a *fakeAgentRuntimeAdapter) ExecuteTask(_ context.Context, task agentruntime.Task) agentruntime.Result {
	a.called = true
	a.task = task
	return agentruntime.Result{
		RuntimeID: firstNonEmptyString(a.id, "hermes"),
		Status:    "completed",
		Output:    "verified runtime output",
		RouteTrace: &agentruntime.RouteTrace{
			RuntimeID:         firstNonEmptyString(a.id, "hermes"),
			Intent:            "software engineering and repository workflow",
			ExecutionMode:     "read-only planning plus approved low-risk local actions",
			RiskLevel:         "medium",
			RecommendedSkills: []string{"autoreview", "gitcrawl"},
			VisibleProviders:  []string{"ollama"},
			VisibleTools:      []string{"browser"},
			BlockedSurfaces:   []string{"outbound message sending"},
		},
		AuditEvents: []string{"fake runtime executed under approval"},
	}
}

func (a *fakeAgentRuntimeAdapter) StopTask(_ context.Context, taskID string) agentruntime.StopResult {
	return agentruntime.StopResult{RuntimeID: firstNonEmptyString(a.id, "hermes"), TaskID: taskID, Status: "stopped"}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
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
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	r.launchEvents = append(r.launchEvents, *event)
	return nil
}

func (r *fakeAutomationRepo) FindLaunchEvents(automationID uuid.UUID, limit int) ([]models.AutomationLaunchEvent, error) {
	return r.launchEvents, nil
}
