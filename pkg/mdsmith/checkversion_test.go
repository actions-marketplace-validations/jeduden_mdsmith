package mdsmith

import "testing"

// TestCheckVersionReflectsNewText is the LSP-facing contract: a
// version-aware Check parses (and caches by version), and a later Check
// at a new version on edited bytes reflects the post-edit text — the
// per-keystroke path plan 216's parse cache backs.
func TestCheckVersionReflectsNewText(t *testing.T) {
	s := newTestSession(t, "", nil)

	clean := []byte("# Hi\n\nclean line\n")
	if diags, err := s.CheckVersion("a.md", clean, 1); err != nil {
		t.Fatalf("CheckVersion v1: %v", err)
	} else if hasRule(diags, "MDS006") {
		t.Fatalf("CheckVersion v1: clean source should have no MDS006, got %+v", diags)
	}

	dirty := []byte("# Hi\n\ndirty line   \n")
	diags, err := s.CheckVersion("a.md", dirty, 2)
	if err != nil {
		t.Fatalf("CheckVersion v2: %v", err)
	}
	if !hasRule(diags, "MDS006") {
		t.Fatalf("CheckVersion v2: expected MDS006 for trailing space, got %+v", diags)
	}
}

// TestCheckVersionReusesParseAtSameVersion verifies the version-keyed
// parse cache: a second CheckVersion at the same (uri, version) reuses
// the parsed file rather than re-parsing. Observed via the engine
// ParseCache the session installs — a hit means the second call did not
// allocate a fresh parse.
func TestCheckVersionReusesParseAtSameVersion(t *testing.T) {
	s := newTestSession(t, "", nil)
	src := []byte("# Hi\n\nsome body line here\n")

	if _, err := s.CheckVersion("a.md", src, 1); err != nil {
		t.Fatalf("CheckVersion 1: %v", err)
	}
	hitsBefore := s.parseCacheHits()
	if _, err := s.CheckVersion("a.md", src, 1); err != nil {
		t.Fatalf("CheckVersion 2: %v", err)
	}
	if s.parseCacheHits() <= hitsBefore {
		t.Fatalf("expected a parse-cache hit on the second CheckVersion at the same version")
	}
}

// TestCheckVersionCrossFileSeesOverlay verifies the version path reads
// cross-file content through the session workspace: a catalog over a
// MemWorkspace file projects that file's summary. This is the seam the
// LSP buffer overlay rides on (footgun 3).
func TestCheckVersionCrossFileSeesOverlay(t *testing.T) {
	files := map[string][]byte{
		"docs/one.md": []byte("---\nsummary: First\n---\n# One\n\nBody paragraph.\n"),
	}
	s := newTestSession(t, "", files)
	// An index whose catalog row would be stale if the body did not list
	// the projected summary. Check returns the catalog diagnostic (the
	// body is empty and out of date), proving the cross-file read ran.
	index := []byte("# Index\n\n<?catalog\nglob:\n  - \"docs/*.md\"\n" +
		"row: \"- [{summary}](docs/{filename})\"\n?>\n<?/catalog?>\n")

	diags, err := s.CheckVersion("index.md", index, 1)
	if err != nil {
		t.Fatalf("CheckVersion: %v", err)
	}
	if !hasRule(diags, "MDS019") {
		t.Fatalf("CheckVersion: expected MDS019 (stale catalog) proving cross-file read, got %+v", diags)
	}
}

func hasRule(diags []Diagnostic, rule string) bool {
	for _, d := range diags {
		if d.Rule == rule {
			return true
		}
	}
	return false
}
