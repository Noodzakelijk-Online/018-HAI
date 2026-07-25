package agentruntime

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
)

func TestOpenClawEcosystemHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)

	root := t.TempDir()
	initialPath := filepath.Join(root, "openclaw-main.zip")
	updatedPath := filepath.Join(root, "openclaw-next.zip")
	if err := writeMinimalOpenClawZip(initialPath); err != nil {
		t.Fatalf("create initial ecosystem file: %v", err)
	}
	if err := writeMinimalOpenClawZip(updatedPath); err != nil {
		t.Fatalf("create updated ecosystem file: %v", err)
	}

	adapter := &openClawAdapter{
		enabled:         true,
		executable:      "openclaw",
		workspace:       root,
		workspaceRoot:   root,
		ecosystemPath:   initialPath,
		agentCLIEnabled: true,
		allowedHost:     map[string]bool{"127.0.0.1": true},
	}
	registry := NewRegistry(adapter)
	handler := NewHandler(registry)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "alice")
		c.Next()
	})
	r.GET("/agent-runtimes/openclaw/ecosystem", handler.OpenClawEcosystem)
	r.PATCH("/agent-runtimes/openclaw/ecosystem", handler.SetOpenClawEcosystem)
	r.POST("/agent-runtimes/openclaw/ecosystem/refresh", handler.RefreshOpenClawEcosystem)
	r.POST("/agent-runtimes/openclaw/ecosystem/upload", handler.UploadOpenClawEcosystem)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/agent-runtimes/openclaw/ecosystem", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /openclaw/ecosystem status = %d", w.Code)
	}
	var info Info
	if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil {
		t.Fatalf("decode openclaw ecosystem response: %v", err)
	}
	if info.ID != "openclaw" {
		t.Fatalf("expected openclaw runtime info, got %q", info.ID)
	}

	body, err := json.Marshal(map[string]string{"ecosystemPath": updatedPath})
	if err != nil {
		t.Fatalf("marshal patch body: %v", err)
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/agent-runtimes/openclaw/ecosystem", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH /openclaw/ecosystem status = %d, body=%s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil {
		t.Fatalf("decode updated openclaw runtime response: %v", err)
	}
	if info.EcosystemPath != updatedPath {
		t.Fatalf("expected ecosystemPath %q, got %q", updatedPath, info.EcosystemPath)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/agent-runtimes/openclaw/ecosystem", bytes.NewBufferString(`{"ecosystemPath":""}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request for empty path, got %d", w.Code)
	}

	randomPath := filepath.Join(root, "not-openclaw.txt")
	if err := os.WriteFile(randomPath, []byte("not an ecosystem"), 0o644); err != nil {
		t.Fatalf("create random ecosystem file: %v", err)
	}
	body, err = json.Marshal(map[string]string{"ecosystemPath": randomPath})
	if err != nil {
		t.Fatalf("marshal invalid patch body: %v", err)
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/agent-runtimes/openclaw/ecosystem", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request for non-zip ecosystem file, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/agent-runtimes/openclaw/ecosystem/refresh", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /openclaw/ecosystem/refresh status = %d", w.Code)
	}

	invalidBody := &bytes.Buffer{}
	invalidWriter := multipart.NewWriter(invalidBody)
	part, err := invalidWriter.CreateFormFile("ecosystem", "openclaw-main.zip")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte("PK\x03\x04fake-zip-content")); err != nil {
		t.Fatalf("write upload content: %v", err)
	}
	if err := invalidWriter.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req = httptest.NewRequest(http.MethodPost, "/agent-runtimes/openclaw/ecosystem/upload", invalidBody)
	req.Header.Set("Content-Type", invalidWriter.FormDataContentType())
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid zip upload status 400, got %d", w.Code)
	}

	validZipPath := filepath.Join(root, "openclaw-valid.zip")
	validFile, err := os.Create(validZipPath)
	if err != nil {
		t.Fatalf("create valid zip file: %v", err)
	}
	writerZip := zip.NewWriter(validFile)
	entry, err := writerZip.Create("openclaw-main/package.json")
	if err != nil {
		t.Fatalf("zip entry create: %v", err)
	}
	if _, err := entry.Write([]byte(`{"name":"openclaw","version":"0.0.1"}`)); err != nil {
		t.Fatalf("zip payload write: %v", err)
	}
	if err := writerZip.Close(); err != nil {
		t.Fatalf("close zip file writer: %v", err)
	}
	if err := validFile.Close(); err != nil {
		t.Fatalf("close valid zip file: %v", err)
	}
	zipPayload, err := os.ReadFile(validZipPath)
	if err != nil {
		t.Fatalf("create valid zip payload: %v", err)
	}
	zipBody := &bytes.Buffer{}
	zipWriter := multipart.NewWriter(zipBody)
	part, err = zipWriter.CreateFormFile("ecosystem", filepath.Base(validZipPath))
	if err != nil {
		t.Fatalf("create valid form file: %v", err)
	}
	if _, err := part.Write(zipPayload); err != nil {
		t.Fatalf("write valid zip upload content: %v", err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req = httptest.NewRequest(http.MethodPost, "/agent-runtimes/openclaw/ecosystem/upload", zipBody)
	req.Header.Set("Content-Type", zipWriter.FormDataContentType())
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected valid zip upload status 200, got %d, body=%s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	if info.EcosystemPath == "" {
		t.Fatalf("expected upload to update ecosystem path")
	}
}

func TestAgentRuntimeSkillsAndStopHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adapter := &fakeAdapter{info: Info{ID: "openclaw", Name: "OpenClaw", Enabled: true, Configured: true, ExecutionEnabled: true}}
	handler := NewHandler(NewRegistry(adapter))

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "alice")
		c.Next()
	})
	r.GET("/agent-runtimes/:id/skills", handler.Skills)
	r.POST("/agent-runtimes/:id/tasks/:taskId/stop", handler.StopTask)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/agent-runtimes/openclaw/skills", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /agent-runtimes/openclaw/skills status = %d, body=%s", w.Code, w.Body.String())
	}
	var skills []Skill
	if err := json.Unmarshal(w.Body.Bytes(), &skills); err != nil {
		t.Fatalf("decode runtime skills: %v", err)
	}
	if len(skills) != 1 || skills[0].RuntimeID != "openclaw" || skills[0].Name != "test" {
		t.Fatalf("skills = %#v", skills)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/agent-runtimes/openclaw/tasks/task-123/stop", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST stop task status = %d, body=%s", w.Code, w.Body.String())
	}
	var stop StopResult
	if err := json.Unmarshal(w.Body.Bytes(), &stop); err != nil {
		t.Fatalf("decode stop result: %v", err)
	}
	if stop.RuntimeID != "openclaw" || stop.TaskID != "task-123" || stop.Status != "stopped" {
		t.Fatalf("stop result = %#v", stop)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/agent-runtimes/missing/skills", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing runtime skills should return 404, got %d", w.Code)
	}
}

func TestRuntimeMutationHandlersRequireVerifiedOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adapter := &fakeAdapter{info: Info{ID: "openclaw", Enabled: true, Configured: true, ExecutionEnabled: true}}
	handler := NewHandler(NewRegistry(adapter))
	r := gin.New()
	r.POST("/agent-runtimes/:id/tasks/:taskId/stop", handler.StopTask)
	r.POST("/agent-runtimes/openclaw/ecosystem/refresh", handler.RefreshOpenClawEcosystem)

	for _, path := range []string{
		"/agent-runtimes/openclaw/tasks/task-123/stop",
		"/agent-runtimes/openclaw/ecosystem/refresh",
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, nil))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s status = %d, want %d: %s", path, w.Code, http.StatusUnauthorized, w.Body.String())
		}
	}
}

func TestValidateOpenClawZipRejectsUnsafeEntryPath(t *testing.T) {
	root := t.TempDir()
	zipPath := filepath.Join(root, "openclaw-unsafe.zip")
	if err := writeZipEntries(zipPath, map[string]string{
		"../escape/package.json": `{"name":"openclaw"}`,
	}); err != nil {
		t.Fatalf("write unsafe zip: %v", err)
	}
	if err := validateOpenClawZip(zipPath); err == nil {
		t.Fatalf("expected unsafe zip path to be rejected")
	}
}

func TestValidateOpenClawZipRejectsNonOpenClawArchive(t *testing.T) {
	root := t.TempDir()
	zipPath := filepath.Join(root, "not-openclaw.zip")
	if err := writeZipEntries(zipPath, map[string]string{
		"random-project/package.json":   `{"name":"random-project"}`,
		"random-project/docs/readme.md": "not openclaw",
	}); err != nil {
		t.Fatalf("write non-openclaw zip: %v", err)
	}
	if err := validateOpenClawZip(zipPath); err == nil {
		t.Fatalf("expected non-OpenClaw zip to be rejected")
	}
}

func TestValidateOpenClawZipRejectsDuplicateEntries(t *testing.T) {
	root := t.TempDir()
	zipPath := filepath.Join(root, "openclaw-duplicate.zip")
	if err := writeZipEntryList(zipPath, []zipEntry{
		{name: "openclaw-main/package.json", content: `{"name":"openclaw"}`},
		{name: "openclaw-main/PACKAGE.json", content: `{"name":"not-openclaw"}`},
	}); err != nil {
		t.Fatalf("write duplicate zip: %v", err)
	}
	if err := validateOpenClawZip(zipPath); err == nil {
		t.Fatalf("expected duplicate zip entries to be rejected")
	}
}

func TestValidateOpenClawZipAcceptsSkillsOnlyArchive(t *testing.T) {
	root := t.TempDir()
	zipPath := filepath.Join(root, "openclaw-main.zip")
	if err := writeZipEntries(zipPath, map[string]string{
		"openclaw-main/.agents/skills/agent-transcript/SKILL.md":                              "transcript skill",
		"openclaw-main/.agents/skills/claw-score/references/completeness/openclaw-app-sdk.md": "reference",
	}); err != nil {
		t.Fatalf("write skills-only zip: %v", err)
	}
	if err := validateOpenClawZip(zipPath); err != nil {
		t.Fatalf("expected skills-only OpenClaw zip to be accepted: %v", err)
	}
}

func TestValidateOpenClawEcosystemPathRejectsRandomDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"random-project"}`), 0o644); err != nil {
		t.Fatalf("write random package: %v", err)
	}
	if err := validateOpenClawEcosystemPath(root); err == nil {
		t.Fatalf("expected random directory to be rejected")
	}
}

func TestValidateOpenClawEcosystemPathAcceptsSkillsOnlyCheckout(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".agents", "skills", "agent-transcript"), 0o755); err != nil {
		t.Fatalf("create skill path: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".agents", "skills", "agent-transcript", "SKILL.md"), []byte("transcript skill"), 0o644); err != nil {
		t.Fatalf("write skill file: %v", err)
	}
	if err := validateOpenClawEcosystemPath(root); err != nil {
		t.Fatalf("expected skills-only checkout to be accepted: %v", err)
	}
}

func TestValidateOpenClawEcosystemPathAcceptsExtractedCheckout(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".agents", "skills"), 0o755); err != nil {
		t.Fatalf("create skill marker: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"openclaw"}`), 0o644); err != nil {
		t.Fatalf("write openclaw package: %v", err)
	}
	if err := validateOpenClawEcosystemPath(root); err != nil {
		t.Fatalf("expected extracted checkout to be accepted: %v", err)
	}
}

func writeMinimalOpenClawZip(path string) error {
	return writeZipEntries(path, map[string]string{
		"openclaw-main/package.json":                    `{"name":"openclaw","version":"0.0.1"}`,
		"openclaw-main/.agents/skills/example/SKILL.md": "example skill",
	})
}

func writeZipEntries(path string, entries map[string]string) error {
	ordered := make([]zipEntry, 0, len(entries))
	for name, content := range entries {
		ordered = append(ordered, zipEntry{name: name, content: content})
	}
	return writeZipEntryList(path, ordered)
}

type zipEntry struct {
	name    string
	content string
}

func writeZipEntryList(path string, entries []zipEntry) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	writer := zip.NewWriter(file)
	for _, item := range entries {
		entry, err := writer.Create(item.name)
		if err != nil {
			_ = writer.Close()
			_ = file.Close()
			return err
		}
		if _, err := entry.Write([]byte(item.content)); err != nil {
			_ = writer.Close()
			_ = file.Close()
			return err
		}
	}
	if err := writer.Close(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
