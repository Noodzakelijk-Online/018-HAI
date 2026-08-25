package automation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"automation-hub-backend/internal/agentruntime"
	"automation-hub-backend/internal/config"
	"automation-hub-backend/internal/events"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/safety"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestExecutionAuthorizationProfileUsesBoundedAutomationTarget(t *testing.T) {
	id := uuid.New()
	automation := &models.Automation{
		ID:           id,
		LaunchType:   "api",
		LaunchTarget: "GET https://example.test/health?verbose=true",
	}

	_, _, _, _, _, _, _, target := executionAuthorizationProfile(automation, http.MethodGet)
	if target != "automation:"+id.String() {
		t.Fatalf("authorization target = %q, want bounded automation identity", target)
	}
}

func TestMain(m *testing.M) {
	switch os.Getenv("HAI_TEST_SCRIPT_MODE") {
	case "ok":
		fmt.Println("script-ok")
		os.Exit(0)
	case "clean-environment":
		if os.Getenv("SECRET_TOKEN") != "" {
			fmt.Println("leaked")
		} else {
			fmt.Println("clean")
		}
		os.Exit(0)
	case "redact":
		fmt.Println("token=super-secret-token")
		os.Exit(0)
	case "fail-with-secret":
		fmt.Fprintln(os.Stderr, "token=super-secret-token")
		os.Exit(1)
	}
	os.Exit(m.Run())
}

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
	service := newTestService(repo, events.Publisher{})

	result, err := service.LaunchTask(id, approvedTaskLaunchRequest(t, service, id, TaskLaunchRequest{
		OwnerIdentity: "alice",
	}))
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

func TestLaunchBlocksMutatingAPIWithoutApprovalBeforeNetworkAccess(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	id := uuid.New()
	repo := newFakeAutomationRepo(&models.Automation{
		ID:           id,
		Name:         "Unapproved API Automation",
		URLPath:      "unapproved-api-automation",
		LaunchType:   "api",
		LaunchTarget: "POST " + server.URL + "/mutate",
	})
	service := newTestService(repo, events.Publisher{})

	result, err := service.LaunchTask(id, TaskLaunchRequest{OwnerIdentity: "alice"})
	if err != nil {
		t.Fatalf("LaunchTask: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("mutating API received %d calls without approval, want zero", calls.Load())
	}
	assertLauncherApprovalBlocked(t, result, repo, "action-bound approval proof rejected before network access")
}

func TestLaunchRejectsApprovalAfterAutomationTargetChangesBeforeNetworkAccess(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	id := uuid.New()
	repo := newFakeAutomationRepo(&models.Automation{
		ID:           id,
		Name:         "Digest-bound API Automation",
		URLPath:      "digest-bound-api-automation",
		LaunchType:   "api",
		LaunchTarget: "POST " + server.URL + "/approved-target",
	})
	service := newTestService(repo, events.Publisher{})
	request := approvedTaskLaunchRequest(t, service, id, TaskLaunchRequest{
		OwnerIdentity: "alice",
		Task:          "Perform the reviewed mutation.",
		ProjectKey:    "018-hai",
	})

	repo.automation.LaunchTarget = "POST " + server.URL + "/changed-after-review"
	result, err := service.LaunchTask(id, request)
	if err != nil {
		t.Fatalf("LaunchTask: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("changed action received %d network calls, want zero", calls.Load())
	}
	assertLauncherApprovalBlocked(t, result, repo, "action-bound approval proof rejected before network access")
	if !strings.Contains(result.Message, "action digest mismatch") {
		t.Fatalf("blocked message = %q, want action digest mismatch", result.Message)
	}
}

func TestLaunchRejectsMismatchedApprovalBindingBeforeNetworkAccess(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	tests := []struct {
		name          string
		mutate        func(*TaskLaunchRequest)
		expectedError string
	}{
		{name: "owner", mutate: func(request *TaskLaunchRequest) { request.OwnerIdentity = "bob" }, expectedError: "owner mismatch"},
		{name: "task", mutate: func(request *TaskLaunchRequest) { request.Task = "Different action" }, expectedError: "action digest mismatch"},
		{name: "review item", mutate: func(request *TaskLaunchRequest) { request.ApprovalSourceID = "task-review:" + uuid.NewString() }, expectedError: "approval source mismatch"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			id := uuid.New()
			repo := newFakeAutomationRepo(&models.Automation{
				ID:           id,
				Name:         "Bound API Automation",
				URLPath:      "bound-api-automation",
				LaunchType:   "api",
				LaunchTarget: "POST " + server.URL + "/mutate",
			})
			service := newTestService(repo, events.Publisher{})
			request := approvedTaskLaunchRequest(t, service, id, TaskLaunchRequest{
				OwnerIdentity: "alice",
				Task:          "Perform the reviewed action.",
				ProjectKey:    "018-hai",
			})
			test.mutate(&request)

			result, err := service.LaunchTask(id, request)
			if err != nil {
				t.Fatalf("LaunchTask: %v", err)
			}
			if result.Status != "blocked" || !strings.Contains(result.Message, test.expectedError) {
				t.Fatalf("mismatch result = %#v, want %q block", result, test.expectedError)
			}
			if len(repo.launchEvents) != 1 || repo.launchEvents[0].Status != "blocked" {
				t.Fatalf("mismatch denial was not audited: %#v", repo.launchEvents)
			}
		})
	}
	if calls.Load() != 0 {
		t.Fatalf("mismatched proofs caused %d network calls, want zero", calls.Load())
	}
}

func TestLaunchConsumesApprovalProofOnceBeforeMutatingAPI(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	id := uuid.New()
	repo := newFakeAutomationRepo(&models.Automation{
		ID:                 id,
		Name:               "Single-use API Automation",
		URLPath:            "single-use-api-automation",
		LaunchType:         "api",
		LaunchTarget:       "POST " + server.URL + "/mutate",
		ExpectedHTTPStatus: http.StatusNoContent,
	})
	service := newTestService(repo, events.Publisher{})
	request := approvedTaskLaunchRequest(t, service, id, TaskLaunchRequest{
		OwnerIdentity: "alice",
		Task:          "Perform this mutation once.",
		ProjectKey:    "018-hai",
	})

	first, err := service.LaunchTask(id, request)
	if err != nil {
		t.Fatalf("first LaunchTask: %v", err)
	}
	if first.Status != "completed" || calls.Load() != 1 {
		t.Fatalf("first result = %#v calls=%d, want one completed mutation", first, calls.Load())
	}
	second, err := service.LaunchTask(id, request)
	if err != nil {
		t.Fatalf("second LaunchTask: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("replayed proof caused %d network calls, want one total", calls.Load())
	}
	if second.Status != "blocked" || !strings.Contains(second.Message, ErrApprovalProofConsumed.Error()) {
		t.Fatalf("replay result = %#v, want consumed-proof block", second)
	}
	if len(repo.launchEvents) != 2 || repo.launchEvents[1].Status != "blocked" {
		t.Fatalf("replay denial was not audited: %#v", repo.launchEvents)
	}
}

func TestLaunchRejectsExpiredApprovalBeforeMutatingAPI(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	now := time.Date(2026, time.July, 30, 10, 0, 0, 0, time.UTC)
	proofService := newApprovalProofTestService(t, func() time.Time { return now })
	id := uuid.New()
	repo := newFakeAutomationRepo(&models.Automation{
		ID:           id,
		Name:         "Expiring API Automation",
		URLPath:      "expiring-api-automation",
		LaunchType:   "api",
		LaunchTarget: "POST " + server.URL + "/mutate",
	})
	service := newTestServiceWithRuntimeRegistryAndApprovalProofs(
		repo,
		events.Publisher{},
		agentruntime.DefaultRegistry(),
		proofService,
	)
	request := approvedTaskLaunchRequest(t, service, id, TaskLaunchRequest{
		OwnerIdentity: "alice",
		Task:          "Perform the reviewed mutation.",
	})
	now = now.Add(defaultApprovalProofTTL)

	result, err := service.LaunchTask(id, request)
	if err != nil {
		t.Fatalf("LaunchTask: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("expired proof caused %d network calls, want zero", calls.Load())
	}
	if result.Status != "blocked" || !strings.Contains(result.Message, ErrApprovalProofExpired.Error()) {
		t.Fatalf("expired result = %#v, want expiry block", result)
	}
}

func TestLaunchAllowsReadOnlyAPIProbeWithoutApproval(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != method {
					t.Errorf("method = %s, want %s", r.Method, method)
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			id := uuid.New()
			repo := newFakeAutomationRepo(&models.Automation{
				ID:                 id,
				Name:               "Read-only API Probe",
				URLPath:            "read-only-api-probe",
				LaunchType:         "api",
				LaunchTarget:       method + " " + server.URL + "/health",
				ExpectedHTTPStatus: http.StatusOK,
			})
			service := newTestService(repo, events.Publisher{})

			result, err := service.LaunchTask(id, TaskLaunchRequest{OwnerIdentity: "alice"})
			if err != nil {
				t.Fatalf("LaunchTask: %v", err)
			}
			if calls.Load() != 1 {
				t.Fatalf("read-only API received %d calls, want one", calls.Load())
			}
			if result.Status != "completed" || result.RequiresApproval {
				t.Fatalf("read-only result = %#v, want completed without approval", result)
			}
			if !containsAuditFragment(
				result.AuditEvents,
				"unified execution authorization receipt",
			) {
				t.Fatalf("read-only authorization receipt missing from audit: %#v", result.AuditEvents)
			}
		})
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
	service := newTestService(repo, events.Publisher{})

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
	service := newTestService(repo, events.Publisher{})

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
	service := newTestService(repo, events.Publisher{})

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
	service := newTestService(repo, events.Publisher{})

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
	service := newTestService(repo, events.Publisher{})

	result, err := service.LaunchTask(id, approvedTaskLaunchRequest(t, service, id, TaskLaunchRequest{}))
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
	target := writeExecutableScriptFixture(t, dir, "ok")
	t.Setenv("AUTOMATION_SCRIPT_EXECUTION_ENABLED", "true")
	t.Setenv("AUTOMATION_SCRIPT_DIR", dir)
	t.Setenv("AUTOMATION_SCRIPT_SHA256_ALLOWLIST", scriptPin(t, filepath.Join(dir, target)))

	id := uuid.New()
	repo := newFakeAutomationRepo(&models.Automation{
		ID:           id,
		Name:         "Script Automation",
		URLPath:      "script-automation",
		Host:         "localhost",
		Port:         8080,
		LaunchType:   "script",
		LaunchTarget: target,
	})
	service := newTestService(repo, events.Publisher{})

	result, err := service.LaunchTask(id, approvedTaskLaunchRequest(t, service, id, TaskLaunchRequest{}))
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

func TestLaunchBlocksScriptChangedDuringFinalAuthorization(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "launch.sh")
	marker := filepath.Join(dir, "changed-script-ran.txt")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'reviewed-script'\n"), 0755); err != nil {
		t.Fatalf("WriteFile reviewed script: %v", err)
	}
	t.Setenv("AUTOMATION_SCRIPT_EXECUTION_ENABLED", "true")
	t.Setenv("AUTOMATION_SCRIPT_DIR", dir)
	t.Setenv("AUTOMATION_SCRIPT_SHA256_ALLOWLIST", scriptPin(t, script))

	id := uuid.New()
	repo := newFakeAutomationRepo(&models.Automation{
		ID:           id,
		Name:         "Pinned Script Automation",
		URLPath:      "pinned-script-automation",
		LaunchType:   "script",
		LaunchTarget: filepath.Base(script),
	})
	authorizer := &recordingExecutionAuthorizer{
		onCall: func() {
			if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch "+filepath.Base(marker)+"\n"), 0755); err != nil {
				t.Fatalf("replace script during authorization: %v", err)
			}
		},
	}
	service := NewServiceWithRuntimeRegistryApprovalProofsAndExecutionAuthorization(
		repo,
		events.Publisher{},
		agentruntime.DefaultRegistry(),
		newUnitTestApprovalProofService(),
		authorizer,
	)

	result, err := service.LaunchTask(id, approvedTaskLaunchRequest(t, service, id, TaskLaunchRequest{}))
	if err != nil {
		t.Fatalf("LaunchTask: %v", err)
	}
	if result.Status != "blocked" || !strings.Contains(result.Message, "SHA-256 does not match") {
		t.Fatalf("result = %#v, want changed script blocked by the final pin check", result)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("replacement script executed or stat returned unexpected error: %v", err)
	}
	if authorizer.calls.Load() != 1 {
		t.Fatalf("authorization calls = %d, want one", authorizer.calls.Load())
	}
	if len(repo.launchEvents) != 1 || !containsString(repo.launchEvents[0].AuditEvents, "script hash pin rejected after execution authorization") {
		t.Fatalf("launch audit = %#v, want final pin rejection", repo.launchEvents)
	}
}

func TestLaunchBlocksScriptWithoutApprovalBeforeProcessExecution(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "launch.sh")
	marker := filepath.Join(dir, "executed.txt")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch executed.txt\n"), 0755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("AUTOMATION_SCRIPT_EXECUTION_ENABLED", "true")
	t.Setenv("AUTOMATION_SCRIPT_DIR", dir)

	id := uuid.New()
	repo := newFakeAutomationRepo(&models.Automation{
		ID:           id,
		Name:         "Unapproved Script Automation",
		URLPath:      "unapproved-script-automation",
		LaunchType:   "script",
		LaunchTarget: "launch.sh",
	})
	service := newTestService(repo, events.Publisher{})

	result, err := service.LaunchTask(id, TaskLaunchRequest{OwnerIdentity: "alice"})
	if err != nil {
		t.Fatalf("LaunchTask: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("script side-effect marker exists or stat returned unexpected error: %v", err)
	}
	assertLauncherApprovalBlocked(t, result, repo, "action-bound approval proof rejected before process or filesystem access")
}

func TestLaunchRunsScriptWithMinimalEnvironment(t *testing.T) {
	dir := t.TempDir()
	target := writeExecutableScriptFixture(t, dir, "clean-environment")
	t.Setenv("AUTOMATION_SCRIPT_EXECUTION_ENABLED", "true")
	t.Setenv("AUTOMATION_SCRIPT_DIR", dir)
	t.Setenv("AUTOMATION_SCRIPT_SHA256_ALLOWLIST", scriptPin(t, filepath.Join(dir, target)))
	t.Setenv("SECRET_TOKEN", "must-not-leak")

	id := uuid.New()
	repo := newFakeAutomationRepo(&models.Automation{
		ID:           id,
		Name:         "Script Automation",
		URLPath:      "script-automation",
		Host:         "localhost",
		Port:         8080,
		LaunchType:   "script",
		LaunchTarget: target,
	})
	service := newTestService(repo, events.Publisher{})

	result, err := service.LaunchTask(id, approvedTaskLaunchRequest(t, service, id, TaskLaunchRequest{}))
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
	target := writeExecutableScriptFixture(t, dir, "redact")
	t.Setenv("AUTOMATION_SCRIPT_EXECUTION_ENABLED", "true")
	t.Setenv("AUTOMATION_SCRIPT_DIR", dir)
	t.Setenv("AUTOMATION_SCRIPT_SHA256_ALLOWLIST", scriptPin(t, filepath.Join(dir, target)))

	id := uuid.New()
	repo := newFakeAutomationRepo(&models.Automation{
		ID:           id,
		Name:         "Script Automation",
		URLPath:      "script-automation",
		Host:         "localhost",
		Port:         8080,
		LaunchType:   "script",
		LaunchTarget: target,
	})
	service := newTestService(repo, events.Publisher{})

	result, err := service.LaunchTask(id, approvedTaskLaunchRequest(t, service, id, TaskLaunchRequest{}))
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("status = %q, want completed: %s", result.Status, result.Message)
	}
	if strings.Contains(result.Output, "super-secret-token") {
		t.Fatalf("script output leaked secret: %s", result.Output)
	}
	if len(repo.launchEvents) != 1 || strings.Contains(repo.launchEvents[0].Output, "super-secret-token") {
		t.Fatalf("launch event leaked secret: %#v", repo.launchEvents)
	}
}

func TestLaunchRedactsScriptFailureSecrets(t *testing.T) {
	dir := t.TempDir()
	target := writeExecutableScriptFixture(t, dir, "fail-with-secret")
	t.Setenv("AUTOMATION_SCRIPT_EXECUTION_ENABLED", "true")
	t.Setenv("AUTOMATION_SCRIPT_DIR", dir)
	t.Setenv("AUTOMATION_SCRIPT_SHA256_ALLOWLIST", scriptPin(t, filepath.Join(dir, target)))

	id := uuid.New()
	repo := newFakeAutomationRepo(&models.Automation{
		ID:           id,
		Name:         "Failing Script Automation",
		URLPath:      "failing-script-automation",
		LaunchType:   "script",
		LaunchTarget: target,
	})
	service := newTestService(repo, events.Publisher{})

	result, err := service.LaunchTask(id, approvedTaskLaunchRequest(t, service, id, TaskLaunchRequest{}))
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if result.Status != "failed" {
		t.Fatalf("status = %q, want failed", result.Status)
	}
	if strings.Contains(result.Output, "super-secret-token") || strings.Contains(result.Message, "super-secret-token") {
		t.Fatalf("failed script result leaked a secret: %#v", result)
	}
	if len(repo.launchEvents) != 1 || strings.Contains(repo.launchEvents[0].Output, "super-secret-token") || strings.Contains(repo.launchEvents[0].Message, "super-secret-token") {
		t.Fatalf("launch event leaked a secret: %#v", repo.launchEvents)
	}
}

func writeExecutableScriptFixture(t *testing.T, dir, mode string) string {
	t.Helper()
	t.Setenv("HAI_TEST_SCRIPT_MODE", mode)
	t.Setenv("AUTOMATION_SCRIPT_ENV_ALLOWLIST", "HAI_TEST_SCRIPT_MODE")
	if runtime.GOOS == "windows" {
		source, err := os.Executable()
		if err != nil {
			t.Fatalf("locate test executable: %v", err)
		}
		target := filepath.Join(dir, "hai-script-test-helper.exe")
		payload, err := os.ReadFile(source)
		if err != nil {
			t.Fatalf("read test executable: %v", err)
		}
		if err := os.WriteFile(target, payload, 0755); err != nil {
			t.Fatalf("write test executable fixture: %v", err)
		}
		return filepath.Base(target)
	}

	target := filepath.Join(dir, "hai-script-test-helper.sh")
	var body string
	switch mode {
	case "ok":
		body = "#!/bin/sh\necho script-ok\n"
	case "clean-environment":
		body = "#!/bin/sh\nif [ -n \"$SECRET_TOKEN\" ]; then echo leaked; else echo clean; fi\n"
	case "redact":
		body = "#!/bin/sh\necho 'token=super-secret-token'\n"
	case "fail-with-secret":
		body = "#!/bin/sh\necho 'token=super-secret-token' >&2\nexit 1\n"
	default:
		t.Fatalf("unsupported script fixture mode %q", mode)
	}
	if err := os.WriteFile(target, []byte(body), 0755); err != nil {
		t.Fatalf("write script fixture: %v", err)
	}
	return filepath.Base(target)
}

func scriptPin(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	sum := sha256.Sum256(contents)
	return filepath.Base(path) + "=" + hex.EncodeToString(sum[:])
}

func TestBoundedOutputCapsCombinedProcessOutput(t *testing.T) {
	output := newBoundedOutput(8)
	if _, err := output.Write([]byte("12345")); err != nil {
		t.Fatal(err)
	}
	if _, err := output.Write([]byte("67890")); err != nil {
		t.Fatal(err)
	}
	if got := string(output.Bytes()); got != "12345678" {
		t.Fatalf("bounded output = %q, want %q", got, "12345678")
	}
	if !output.Truncated() {
		t.Fatal("overflow was not recorded")
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
	service := newTestService(repo, events.Publisher{})

	result, err := service.LaunchTask(id, approvedTaskLaunchRequest(t, service, id, TaskLaunchRequest{}))
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
	service := newTestService(repo, events.Publisher{})

	result, err := service.LaunchTask(id, approvedTaskLaunchRequest(t, service, id, TaskLaunchRequest{}))
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
	service := newTestService(repo, events.Publisher{})

	result, err := service.LaunchTask(id, approvedTaskLaunchRequest(t, service, id, TaskLaunchRequest{}))
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
	service := newTestService(repo, events.Publisher{})

	result, err := service.LaunchTask(id, approvedTaskLaunchRequest(t, service, id, TaskLaunchRequest{}))
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
	service := newTestService(repo, events.Publisher{})

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

func TestLaunchBlocksWhenPersistedEmergencyStopActive(t *testing.T) {
	restore := safety.SetEmergencyStopProvider(safety.EmergencyStopProviderFunc(func() (bool, string, error) {
		return true, "operator paused execution", nil
	}))
	defer restore()

	id := uuid.New()
	repo := newFakeAutomationRepo(&models.Automation{
		ID:           id,
		Name:         "Persisted Stop Automation",
		URLPath:      "persisted-stop-automation",
		Host:         "localhost",
		Port:         8080,
		LaunchType:   "browser_url",
		LaunchTarget: "http://localhost:8080",
	})
	service := newTestService(repo, events.Publisher{})
	result, err := service.Launch(id)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if result.Status != "blocked" || result.Message != "operator paused execution" {
		t.Fatalf("persisted stop did not block automation launch: %#v", result)
	}
}

func TestAgentRuntimeLaunchRequiresApprovalAndReceivesTask(t *testing.T) {
	id := uuid.New()
	adapter := &fakeAgentRuntimeAdapter{id: "hermes"}
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
	service := newTestServiceWithAuthorizedRuntime(
		t,
		repo,
		events.Publisher{},
		adapter,
	)

	blocked, err := service.LaunchTask(
		id,
		TaskLaunchRequest{
			OwnerIdentity: "alice",
			Task:          "Inspect the project without an approval.",
		},
	)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if blocked.Status != "blocked" || adapter.called {
		t.Fatalf("direct launch bypassed approval: %#v", blocked)
	}
	if len(repo.launchEvents) != 1 ||
		!containsString(
			repo.launchEvents[0].AuditEvents,
			"action-bound approval proof rejected before agent runtime access",
		) {
		t.Fatalf("blocked runtime audit was not persisted: %#v", repo.launchEvents)
	}

	completed, err := service.LaunchTask(id, approvedTaskLaunchRequest(t, service, id, TaskLaunchRequest{
		Task:       "Inspect the project and report verified completion.",
		ProjectKey: "018-hai",
	}))
	if err != nil {
		t.Fatalf("LaunchTask: %v", err)
	}
	if completed.Status != "completed" || !adapter.called {
		t.Fatalf("approved task did not run: %#v", completed)
	}
	if !strings.HasPrefix(completed.RuntimeTaskID, "automation:"+id.String()+":intent:") {
		t.Fatalf("runtime task id = %q, want launch-scoped id for automation %s", completed.RuntimeTaskID, id)
	}
	if adapter.task.Prompt != "Inspect the project and report verified completion." || adapter.task.ProjectKey != "018-hai" {
		t.Fatalf("task context was not propagated: %#v", adapter.task)
	}
	if adapter.task.ID != completed.RuntimeTaskID {
		t.Fatalf("adapter task id = %q, want launch result id %s", adapter.task.ID, completed.RuntimeTaskID)
	}
	if len(repo.launchEvents) != 2 || !containsString(repo.launchEvents[1].AuditEvents, "fake runtime executed under approval") {
		t.Fatalf("completed runtime audit was not persisted: %#v", repo.launchEvents)
	}
	if repo.launchEvents[1].RuntimeTaskID != completed.RuntimeTaskID {
		t.Fatalf("launch event runtime task id = %q, want %s", repo.launchEvents[1].RuntimeTaskID, completed.RuntimeTaskID)
	}
}

func TestStopRuntimeTaskUsesVerifiedOwnerAutomationRuntimeAndTaskID(t *testing.T) {
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
	service := newTestServiceWithRuntimeRegistry(repo, events.Publisher{}, registry)

	result, err := service.StopRuntimeTaskForOwner(id, "alice")
	if err != nil {
		t.Fatalf("StopRuntimeTaskForOwner: %v", err)
	}
	if result.RuntimeID != "openclaw" || result.TaskID != id.String() || result.Status != "blocked" ||
		!strings.Contains(result.Message, "owner-bound") {
		t.Fatalf("stop result = %#v", result)
	}
	if result.EvidenceURI == "" {
		t.Fatalf("stop result missing evidence URI: %#v", result)
	}
	if len(repo.launchIntents) != 1 || repo.launchIntents[0].OwnerIdentity != "alice" {
		t.Fatalf("expected owner-bound runtime stop intent, got %#v", repo.launchIntents)
	}
	if len(repo.launchEvents) != 1 {
		t.Fatalf("expected runtime stop launch event, got %#v", repo.launchEvents)
	}
	event := repo.launchEvents[0]
	if event.LaunchType != "agent_runtime_stop" || event.RuntimeTaskID != id.String() || event.Status != "blocked" ||
		event.OwnerIdentity != "alice" {
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
	service := newTestServiceWithAuthorizedRuntime(
		t,
		repo,
		events.Publisher{},
		adapter,
	)

	completed, err := service.LaunchTask(id, approvedTaskLaunchRequest(t, service, id, TaskLaunchRequest{
		Task:       "Move approved OpenClaw work forward safely.",
		ProjectKey: "018-hai",
	}))
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

func TestAgentRuntimeLaunchAllowsDeepSeekHarness(t *testing.T) {
	id := uuid.New()
	adapter := &fakeAgentRuntimeAdapter{id: "deepseek-harness"}
	repo := newFakeAutomationRepo(&models.Automation{
		ID:           id,
		Name:         "DeepSeek Harness Runtime",
		URLPath:      "deepseek-harness-runtime",
		Host:         "localhost",
		Port:         8080,
		LaunchType:   "agent_runtime",
		RuntimeType:  "deepseek-harness",
		LaunchTarget: "runtime://deepseek-harness",
	})
	service := newTestServiceWithAuthorizedRuntime(t, repo, events.Publisher{}, adapter)

	completed, err := service.LaunchTask(id, approvedTaskLaunchRequest(t, service, id, TaskLaunchRequest{
		Task:       "Inspect the approved workspace and report evidence.",
		ProjectKey: "018-hai",
	}))
	if err != nil {
		t.Fatalf("LaunchTask: %v", err)
	}
	if completed.Status != "completed" || !adapter.called {
		t.Fatalf("approved DeepSeek Harness task did not run: %#v", completed)
	}
	if adapter.task.Prompt != "Inspect the approved workspace and report evidence." || adapter.task.ProjectKey != "018-hai" {
		t.Fatalf("task context was not propagated: %#v", adapter.task)
	}
}

func TestLaunchBlocksDockerWithoutApprovalBeforeSocketAccess(t *testing.T) {
	socketPath, calls := startDockerTestServer(t)
	t.Setenv("AUTOMATION_DOCKER_CONTROL_ENABLED", "true")
	t.Setenv("AUTOMATION_DOCKER_ALLOWED_CONTAINERS", "safe-container")
	t.Setenv("AUTOMATION_DOCKER_SOCKET", socketPath)

	id := uuid.New()
	repo := newFakeAutomationRepo(&models.Automation{
		ID:           id,
		Name:         "Unapproved Docker Automation",
		URLPath:      "unapproved-docker-automation",
		LaunchType:   "docker_service",
		LaunchTarget: "safe-container",
	})
	service := newTestService(repo, events.Publisher{})

	result, err := service.LaunchTask(id, TaskLaunchRequest{OwnerIdentity: "alice"})
	if err != nil {
		t.Fatalf("LaunchTask: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("Docker API received %d calls without approval, want zero", calls.Load())
	}
	assertLauncherApprovalBlocked(t, result, repo, "action-bound approval proof rejected before docker socket access")
}

func TestLaunchAllowsDockerWithApproval(t *testing.T) {
	socketPath, calls := startDockerTestServer(t)
	t.Setenv("AUTOMATION_DOCKER_CONTROL_ENABLED", "true")
	t.Setenv("AUTOMATION_DOCKER_ALLOWED_CONTAINERS", "safe-container")
	t.Setenv("AUTOMATION_DOCKER_SOCKET", socketPath)

	id := uuid.New()
	repo := newFakeAutomationRepo(&models.Automation{
		ID:           id,
		Name:         "Approved Docker Automation",
		URLPath:      "approved-docker-automation",
		LaunchType:   "docker_service",
		LaunchTarget: "safe-container",
	})
	service := newTestService(repo, events.Publisher{})

	result, err := service.LaunchTask(id, approvedTaskLaunchRequest(t, service, id, TaskLaunchRequest{
		OwnerIdentity: "alice",
	}))
	if err != nil {
		t.Fatalf("LaunchTask: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("Docker API received %d calls, want one", calls.Load())
	}
	if result.Status != "completed" || result.ExitCode != http.StatusNoContent {
		t.Fatalf("approved Docker result = %#v, want completed HTTP %d", result, http.StatusNoContent)
	}
	if len(repo.launchEvents) != 1 || repo.launchEvents[0].Status != "completed" {
		t.Fatalf("approved Docker launch was not audited: %#v", repo.launchEvents)
	}
}

func TestLaunchRedactsDockerFailureOutput(t *testing.T) {
	socketPath, calls := startDockerTestServerWithResponse(t, http.StatusInternalServerError, "token=super-secret-token")
	t.Setenv("AUTOMATION_DOCKER_CONTROL_ENABLED", "true")
	t.Setenv("AUTOMATION_DOCKER_ALLOWED_CONTAINERS", "safe-container")
	t.Setenv("AUTOMATION_DOCKER_SOCKET", socketPath)

	id := uuid.New()
	repo := newFakeAutomationRepo(&models.Automation{
		ID:           id,
		Name:         "Docker failure redaction",
		URLPath:      "docker-failure-redaction",
		LaunchType:   "docker_service",
		LaunchTarget: "safe-container",
	})
	service := newTestService(repo, events.Publisher{})

	result, err := service.LaunchTask(id, approvedTaskLaunchRequest(t, service, id, TaskLaunchRequest{OwnerIdentity: "alice"}))
	if err != nil {
		t.Fatalf("LaunchTask: %v", err)
	}
	if calls.Load() != 1 || result.Status != "failed" {
		t.Fatalf("Docker launch result = %#v calls=%d, want one failed launch", result, calls.Load())
	}
	if strings.Contains(result.Output, "super-secret-token") {
		t.Fatalf("Docker failure output leaked a secret: %#v", result)
	}
	if len(repo.launchEvents) != 1 || strings.Contains(repo.launchEvents[0].Output, "super-secret-token") {
		t.Fatalf("Docker launch event leaked a secret: %#v", repo.launchEvents)
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
	service := newTestService(repo, events.Publisher{})

	result, err := service.LaunchTask(id, approvedTaskLaunchRequest(t, service, id, TaskLaunchRequest{}))
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
	service := newTestService(repo, events.Publisher{})

	result, err := service.LaunchTask(id, approvedTaskLaunchRequest(t, service, id, TaskLaunchRequest{}))
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
	service := newTestService(repo, events.Publisher{})

	result, err := service.LaunchTask(id, approvedTaskLaunchRequest(t, service, id, TaskLaunchRequest{}))
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
	service := newTestService(repo, events.Publisher{})

	result, err := service.LaunchTask(id, approvedTaskLaunchRequest(t, service, id, TaskLaunchRequest{}))
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	for _, value := range []string{result.Target, result.Message, result.Output, repo.launchEvents[0].Target, repo.launchEvents[0].Message, repo.launchEvents[0].Output} {
		if strings.Contains(value, "super-secret-token") || strings.Contains(value, "hunter2") {
			t.Fatalf("launch leaked secret in %q", value)
		}
	}
}

func assertLauncherApprovalBlocked(
	t *testing.T,
	result *LaunchResult,
	repo *fakeAutomationRepo,
	expectedAudit string,
) {
	t.Helper()
	if result == nil || result.Status != "blocked" || !result.RequiresApproval {
		t.Fatalf("launcher result = %#v, want blocked and approval-required", result)
	}
	if result.Output != "" {
		t.Fatalf("blocked launch output = %q, want empty", result.Output)
	}
	if len(repo.launchEvents) != 1 {
		t.Fatalf("blocked launch event count = %d, want one", len(repo.launchEvents))
	}
	event := repo.launchEvents[0]
	if event.Status != "blocked" || !containsString(event.AuditEvents, expectedAudit) {
		t.Fatalf("blocked launch audit = %#v, want %q", event, expectedAudit)
	}
	if result.LaunchEventID == uuid.Nil || result.LaunchEventID != event.ID {
		t.Fatalf("blocked launch event id = %s, want %s", result.LaunchEventID, event.ID)
	}
}

func approvedTaskLaunchRequest(
	t *testing.T,
	service Service,
	id uuid.UUID,
	request TaskLaunchRequest,
) TaskLaunchRequest {
	t.Helper()
	if strings.TrimSpace(request.OwnerIdentity) == "" {
		request.OwnerIdentity = "alice"
	}
	if strings.TrimSpace(request.ApprovalSourceID) == "" {
		request.ApprovalSourceID = "task-review:" + uuid.NewString()
	}
	recorder, ok := service.(ApprovalDecisionRecorder)
	if !ok {
		t.Fatalf("automation service does not expose the trusted approval decision recorder")
	}
	if err := recorder.RecordApprovalDecision(id, TaskApprovalDecisionRequest{
		OwnerIdentity:    request.OwnerIdentity,
		Task:             request.Task,
		ProjectKey:       request.ProjectKey,
		ApprovalSourceID: request.ApprovalSourceID,
		ApprovedAt:       time.Now().UTC(),
	}); err != nil {
		t.Fatalf("RecordApprovalDecision: %v", err)
	}
	issuer, ok := service.(ApprovalProofIssuer)
	if !ok {
		t.Fatalf("automation service does not expose the trusted approval proof issuer")
	}
	proof, err := issuer.IssueApprovalProof(id, TaskApprovalProofRequest{
		OwnerIdentity:    request.OwnerIdentity,
		Task:             request.Task,
		ProjectKey:       request.ProjectKey,
		ApprovalSourceID: request.ApprovalSourceID,
	})
	if err != nil {
		t.Fatalf("IssueApprovalProof: %v", err)
	}
	request.ApprovalProof = proof
	request.ApprovalBindingDigest = proof.ActionDigest
	return request
}

func startDockerTestServer(t *testing.T) (string, *atomic.Int32) {
	return startDockerTestServerWithResponse(t, http.StatusNoContent, "")
}

func startDockerTestServerWithResponse(t *testing.T, status int, body string) (string, *atomic.Int32) {
	t.Helper()
	socketPath := filepath.Join(os.TempDir(), "hai-"+uuid.NewString()+".sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Skipf("Unix sockets are unavailable on this platform: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(socketPath)
	})
	calls := &atomic.Int32{}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})}
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		_ = server.Shutdown(context.Background())
		_ = listener.Close()
	})
	return socketPath, calls
}

type fakeAutomationRepo struct {
	automation        *models.Automation
	launchIntents     []models.AutomationLaunchEvent
	launchEvents      []models.AutomationLaunchEvent
	healthEvents      []models.AutomationHealthEvent
	approvalDecisions map[string]ApprovalDecisionRecord
	saveIntentErr     error
	saveLaunchErr     error
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

func containsAuditFragment(values []string, fragment string) bool {
	for _, value := range values {
		if strings.Contains(value, fragment) {
			return true
		}
	}
	return false
}

func newFakeAutomationRepo(automation *models.Automation) *fakeAutomationRepo {
	return &fakeAutomationRepo{
		automation:        automation,
		approvalDecisions: map[string]ApprovalDecisionRecord{},
	}
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
	if r.saveLaunchErr != nil {
		return r.saveLaunchErr
	}
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	r.launchEvents = append(r.launchEvents, *event)
	return nil
}

func (r *fakeAutomationRepo) SaveLaunchIntent(event *models.AutomationLaunchEvent) error {
	if r.saveIntentErr != nil {
		return r.saveIntentErr
	}
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	r.launchIntents = append(r.launchIntents, *event)
	return nil
}

func (r *fakeAutomationRepo) FindLaunchEvents(automationID uuid.UUID, limit int) ([]models.AutomationLaunchEvent, error) {
	return r.launchEvents, nil
}

func (r *fakeAutomationRepo) FindLaunchEventByExecutionReference(reference string) (*models.AutomationLaunchEvent, error) {
	for index := len(r.launchEvents) - 1; index >= 0; index-- {
		event := r.launchEvents[index]
		if event.ExecutionReference == reference {
			return &event, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *fakeAutomationRepo) SaveApprovalDecision(record *ApprovalDecisionRecord) error {
	if err := validateApprovalDecisionRecord(record); err != nil {
		return err
	}
	if existing, ok := r.approvalDecisions[record.SourceID]; ok {
		if sameApprovalDecision(&existing, record) {
			return nil
		}
		return fmt.Errorf("approval decision conflicts with the recorded action binding")
	}
	r.approvalDecisions[record.SourceID] = *record
	return nil
}

func (r *fakeAutomationRepo) FindApprovalDecision(sourceID string) (*ApprovalDecisionRecord, error) {
	record, ok := r.approvalDecisions[sourceID]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return &record, nil
}
