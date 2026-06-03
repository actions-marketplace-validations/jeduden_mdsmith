package mdsmith

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOverlayWorkspaceReadFilePrefersOverlay verifies an
// OverlayWorkspace returns the overlaid (open-buffer) bytes for a
// shadowed path and falls through to disk for everything else. This is
// the LSP's workspace: unsaved-buffer bytes shadow the on-disk file.
func TestOverlayWorkspaceReadFilePrefersOverlay(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.md"), []byte("disk-a"), 0o600); err != nil {
		t.Fatalf("WriteFile a: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.md"), []byte("disk-b"), 0o600); err != nil {
		t.Fatalf("WriteFile b: %v", err)
	}

	ws := NewOverlayWorkspace(root)
	ws.Set("a.md", []byte("buffer-a"))

	gotA, err := ws.ReadFile("a.md")
	if err != nil {
		t.Fatalf("ReadFile a: %v", err)
	}
	if string(gotA) != "buffer-a" {
		t.Fatalf("ReadFile a = %q, want overlay bytes buffer-a", gotA)
	}
	gotB, err := ws.ReadFile("b.md")
	if err != nil {
		t.Fatalf("ReadFile b: %v", err)
	}
	if string(gotB) != "disk-b" {
		t.Fatalf("ReadFile b = %q, want disk fall-through disk-b", gotB)
	}
}

// TestOverlayWorkspaceFSPrefersOverlay verifies the fs.FS view also
// shadows disk with overlay bytes, so cross-file rules (catalog,
// include) reading through FS see the open-buffer content (footgun 3).
func TestOverlayWorkspaceFSPrefersOverlay(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "a.md"), []byte("disk"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ws := NewOverlayWorkspace(root)
	ws.Set("docs/a.md", []byte("overlay"))

	got, err := fs.ReadFile(ws.FS(), "docs/a.md")
	if err != nil {
		t.Fatalf("fs.ReadFile: %v", err)
	}
	if string(got) != "overlay" {
		t.Fatalf("FS ReadFile = %q, want overlay", got)
	}

	// A path with no overlay falls through to disk.
	if err := os.WriteFile(filepath.Join(root, "docs", "c.md"), []byte("only-disk"), 0o600); err != nil {
		t.Fatalf("WriteFile c: %v", err)
	}
	gotC, err := fs.ReadFile(ws.FS(), "docs/c.md")
	if err != nil {
		t.Fatalf("fs.ReadFile c: %v", err)
	}
	if string(gotC) != "only-disk" {
		t.Fatalf("FS ReadFile c = %q, want disk fall-through", gotC)
	}
}

// TestOverlayWorkspaceFSReflectsLatestSet verifies a Set after an FS
// view was taken is visible to a freshly fetched FS — the engine
// fetches a new FS per lint pass, so the overlay edit lands on the next
// Check.
func TestOverlayWorkspaceFSReflectsLatestSet(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.md"), []byte("disk"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	ws := NewOverlayWorkspace(root)

	ws.Set("a.md", []byte("v1"))
	if got, _ := fs.ReadFile(ws.FS(), "a.md"); string(got) != "v1" {
		t.Fatalf("FS after Set v1 = %q, want v1", got)
	}
	ws.Set("a.md", []byte("v2"))
	if got, _ := fs.ReadFile(ws.FS(), "a.md"); string(got) != "v2" {
		t.Fatalf("FS after Set v2 = %q, want v2", got)
	}
	ws.Delete("a.md")
	if got, _ := fs.ReadFile(ws.FS(), "a.md"); string(got) != "disk" {
		t.Fatalf("FS after Delete = %q, want disk fall-through", got)
	}
}

// TestOverlayWorkspaceSatisfiesMutable confirms OverlayWorkspace
// satisfies the mutable overlay interface Session.Invalidate uses, so
// the LSP's buffer bytes reach cross-file rules.
func TestOverlayWorkspaceSatisfiesMutable(t *testing.T) {
	var _ mutableWorkspace = NewOverlayWorkspace(t.TempDir())
	var _ Workspace = NewOverlayWorkspace(t.TempDir())
}

// TestSessionOverlayBufferReachesCrossFileRule is the end-to-end
// footgun-3 acceptance for the LSP scenario: a Session over an
// OverlayWorkspace catalogs a file whose unsaved-buffer summary differs
// from disk. After Invalidate pushes the buffer bytes, the index's
// catalog projects the buffer summary, not the saved one — the open
// document reached the cross-file rule through the session.
func TestSessionOverlayBufferReachesCrossFileRule(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "one.md"),
		[]byte("---\nsummary: Saved\n---\n# One\n\nBody paragraph.\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ws := NewOverlayWorkspace(root)
	s, err := NewSession(SessionOptions{Workspace: ws, Config: ConfigYAML("")})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer s.Dispose()

	index := []byte("# Index\n\n<?catalog\nglob:\n  - \"docs/*.md\"\n" +
		"row: \"- [{summary}](docs/{filename})\"\n?>\n<?/catalog?>\n")

	// Saved state: the catalog projects "Saved".
	res1, err := s.Fix("index.md", index)
	if err != nil {
		t.Fatalf("Fix 1: %v", err)
	}
	if !strings.Contains(res1.Source, "Saved") {
		t.Fatalf("Fix 1: catalog should project the saved summary:\n%s", res1.Source)
	}

	// Push an unsaved buffer edit through Invalidate (the LSP didChange
	// path), then re-fix: the catalog must pick up the buffer summary.
	s.Invalidate("docs/one.md", []byte("---\nsummary: Buffered\n---\n# One\n\nBody paragraph.\n"))
	res2, err := s.Fix("index.md", index)
	if err != nil {
		t.Fatalf("Fix 2: %v", err)
	}
	if !strings.Contains(res2.Source, "Buffered") {
		t.Fatalf("Fix 2: open-buffer summary did not reach the catalog rule:\n%s", res2.Source)
	}
}
