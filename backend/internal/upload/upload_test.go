package upload

import "testing"

func policy() Policy {
	return Policy{MaxBytes: 1024, AllowedExts: []string{".jpg", ".png"}}
}

func TestValidUploadPasses(t *testing.T) {
	if err := policy().Validate("images/photo.PNG", 500); err != nil {
		t.Fatalf("valid upload rejected: %v", err)
	}
}

func TestRejectsTraversalFilename(t *testing.T) {
	if err := policy().Validate("../secret.png", 10); err == nil {
		t.Fatalf("path traversal filename must be rejected")
	}
}

func TestRejectsBadExtensionAndSize(t *testing.T) {
	if err := policy().Validate("doc.exe", 10); err == nil {
		t.Fatalf("disallowed extension must be rejected")
	}
	if err := policy().Validate("photo.png", 5000); err == nil {
		t.Fatalf("oversized upload must be rejected")
	}
	if err := policy().Validate("photo.png", 0); err == nil {
		t.Fatalf("empty upload must be rejected")
	}
}

func TestNoAllowlistAllowsAnyExtension(t *testing.T) {
	p := Policy{MaxBytes: 1024}
	if err := p.Validate("data.bin", 100); err != nil {
		t.Fatalf("no allowlist should accept any extension: %v", err)
	}
}
