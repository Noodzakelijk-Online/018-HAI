package agentruntime

import (
	"archive/zip"
	"automation-hub-backend/internal/identity"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	// Ecosystem archives are inspected, not extracted, but their metadata still
	// determines what HAI indexes. Bound it so an uploaded archive cannot create
	// ambiguous paths or consume unbounded parsing resources.
	maxOpenClawZipEntries           = 100_000
	maxOpenClawZipUncompressedBytes = uint64(1 << 30)
	maxOpenClawZipCompressionRatio  = uint64(200)
)

type Handler struct {
	registry *Registry
}

func NewHandler(registry *Registry) *Handler {
	return &Handler{registry: registry}
}

func (h *Handler) Registry(c *gin.Context) {
	c.JSON(http.StatusOK, h.registry.List())
}

func (h *Handler) Health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 12*time.Second)
	defer cancel()
	c.JSON(http.StatusOK, h.registry.Health(ctx))
}

func (h *Handler) Skills(c *gin.Context) {
	runtimeID := strings.TrimSpace(c.Param("id"))
	ctx, cancel := context.WithTimeout(c.Request.Context(), 12*time.Second)
	defer cancel()
	skills, err := h.registry.Skills(ctx, runtimeID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, skills)
}

func (h *Handler) StopTask(c *gin.Context) {
	ownerIdentity, ok := runtimeOwner(c)
	if !ok {
		return
	}
	runtimeID := strings.TrimSpace(c.Param("id"))
	taskID := strings.TrimSpace(c.Param("taskId"))
	ctx, cancel := context.WithTimeout(c.Request.Context(), 12*time.Second)
	defer cancel()
	result := h.registry.StopTask(ctx, runtimeID, taskID, ownerIdentity)
	status := http.StatusOK
	if result.Status == "blocked" {
		status = http.StatusBadRequest
	}
	c.JSON(status, result)
}

type openClawEcosystemRequest struct {
	EcosystemPath string `json:"ecosystemPath"`
}

func (h *Handler) OpenClawEcosystem(c *gin.Context) {
	openClaw, ok := h.registry.OpenClawAdapter()
	if !ok || openClaw == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "openclaw runtime is not registered"})
		return
	}
	c.JSON(http.StatusOK, openClaw.Info())
}

func (h *Handler) SetOpenClawEcosystem(c *gin.Context) {
	if !requireRuntimeOwner(c) {
		return
	}
	var request openClawEcosystemRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body for openclaw ecosystem path"})
		return
	}
	path := strings.TrimSpace(request.EcosystemPath)
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ecosystemPath is required"})
		return
	}
	info, err := h.registry.SetOpenClawEcosystemPath(path)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, info)
}

func (h *Handler) RefreshOpenClawEcosystem(c *gin.Context) {
	if !requireRuntimeOwner(c) {
		return
	}
	info, err := h.registry.RefreshOpenClawEcosystem()
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, info)
}

func (h *Handler) UploadOpenClawEcosystem(c *gin.Context) {
	if !requireRuntimeOwner(c) {
		return
	}
	file, err := c.FormFile("ecosystem")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing ecosystem zip upload field 'ecosystem'"})
		return
	}

	filename := strings.TrimSpace(file.Filename)
	if filename == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing uploaded ecosystem filename"})
		return
	}
	if !strings.EqualFold(filepath.Ext(filename), ".zip") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "openclaw ecosystem upload must be a zip file"})
		return
	}
	if file.Size <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "openclaw ecosystem upload is empty"})
		return
	}
	if file.Size > 750*1024*1024 {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "openclaw ecosystem zip is too large"})
		return
	}

	source, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to open uploaded ecosystem file"})
		return
	}
	defer source.Close()

	f, err := os.CreateTemp("", "openclaw-ecosystem-*.zip")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create temporary ecosystem storage"})
		return
	}
	dest := f.Name()

	copied, copyErr := io.Copy(f, source)
	if closeErr := f.Close(); closeErr != nil && copyErr == nil {
		err = closeErr
	}
	if copyErr != nil {
		_ = os.Remove(dest)
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to persist uploaded ecosystem file"})
		return
	}
	if err != nil {
		_ = os.Remove(dest)
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to persist uploaded ecosystem file"})
		return
	}
	if copied == 0 {
		_ = os.Remove(dest)
		c.JSON(http.StatusBadRequest, gin.H{"error": "openclaw ecosystem upload is empty"})
		return
	}
	if copied > 750*1024*1024 {
		_ = os.Remove(dest)
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "openclaw ecosystem zip is too large"})
		return
	}
	if err := validateOpenClawZip(dest); err != nil {
		_ = os.Remove(dest)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	info, err := h.registry.setUploadedOpenClawEcosystemPath(dest)
	if err != nil {
		_ = os.Remove(dest)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, info)
}

func requireRuntimeOwner(c *gin.Context) bool {
	_, ok := runtimeOwner(c)
	return ok
}

func runtimeOwner(c *gin.Context) (string, bool) {
	value, ok := c.Get(identity.ContextSubjectKey)
	if ok {
		if owner, ok := value.(string); ok && strings.TrimSpace(owner) != "" {
			return strings.TrimSpace(owner), true
		}
	}
	c.JSON(http.StatusUnauthorized, gin.H{"error": "an authenticated owner session is required for runtime control actions"})
	return "", false
}

func validateOpenClawZip(path string) error {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer reader.Close()
	if len(reader.File) == 0 {
		return errors.New("openclaw ecosystem zip is empty")
	}
	if len(reader.File) > maxOpenClawZipEntries {
		return fmt.Errorf("openclaw ecosystem zip has too many entries (maximum %d)", maxOpenClawZipEntries)
	}
	hasOpenClawName := false
	hasSkillMarker := false
	seenPaths := make(map[string]struct{}, len(reader.File))
	var totalUncompressed uint64
	for _, file := range reader.File {
		normalized, ok := safeZipEntryName(file.Name)
		if !ok {
			return errors.New("openclaw ecosystem zip contains an unsafe path")
		}
		// Windows is case-insensitive by default. Treat case-only aliases as
		// duplicates too, so an uploaded archive cannot resolve differently on
		// the host than it did during validation.
		pathKey := strings.ToLower(normalized)
		if _, duplicate := seenPaths[pathKey]; duplicate {
			return fmt.Errorf("openclaw ecosystem zip contains a duplicate entry: %s", normalized)
		}
		seenPaths[pathKey] = struct{}{}
		if file.UncompressedSize64 > maxOpenClawZipUncompressedBytes-totalUncompressed {
			return fmt.Errorf("openclaw ecosystem zip expands beyond the %d byte inspection limit", maxOpenClawZipUncompressedBytes)
		}
		totalUncompressed += file.UncompressedSize64
		if file.UncompressedSize64 > 0 && file.CompressedSize64 > 0 && file.UncompressedSize64 > file.CompressedSize64*maxOpenClawZipCompressionRatio {
			return fmt.Errorf("openclaw ecosystem zip entry is compressed beyond the %d:1 inspection limit: %s", maxOpenClawZipCompressionRatio, normalized)
		}
		parts := strings.Split(normalized, "/")
		if isOpenClawRootPackage(parts) {
			if packageLooksLikeOpenClaw(file) {
				hasOpenClawName = true
			}
		}
		if openClawZipEntryLooksLikeSkill(parts) {
			hasSkillMarker = true
		}
	}
	if !hasOpenClawName && !hasSkillMarker {
		return errors.New("openclaw ecosystem zip does not look like an OpenClaw checkout")
	}
	return nil
}

func safeZipEntryName(name string) (string, bool) {
	normalized := strings.ReplaceAll(strings.TrimSpace(name), "\\", "/")
	if normalized == "" || strings.Contains(normalized, "\x00") {
		return "", false
	}
	if strings.HasPrefix(normalized, "/") || len(normalized) >= 2 && normalized[1] == ':' {
		return "", false
	}
	cleaned := path.Clean(normalized)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	return cleaned, true
}

func packageLooksLikeOpenClaw(file *zip.File) bool {
	reader, err := file.Open()
	if err != nil {
		return false
	}
	defer reader.Close()
	var metadata struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(io.LimitReader(reader, 128*1024)).Decode(&metadata); err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(strings.TrimSpace(metadata.Name)), "openclaw")
}

func openClawZipEntryLooksLikeSkill(parts []string) bool {
	if len(parts) < 3 {
		return false
	}
	if parts[len(parts)-1] != "SKILL.md" {
		return false
	}
	for index := 0; index < len(parts)-2; index++ {
		if parts[index] == "skills" && parts[index+1] != "" {
			return true
		}
	}
	return false
}
