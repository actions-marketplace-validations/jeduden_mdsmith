package release

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/rules"
)

// TestRenderCoverageMatrix_TablePerCategory verifies that
// RenderCoverageMatrix groups rules by category, emits one
// five-column table per category that has peer mappings, and
// renders each peer cell with the upstream default-on indicator.
func TestRenderCoverageMatrix_TablePerCategory(t *testing.T) {
	rs := []rules.RuleInfo{
		{
			ID: "MDS001", Name: "line-length", Status: "ready",
			Description: "Line exceeds maximum length.",
			Category:    "line",
			Markdownlint: []rules.RuleMapping{
				{ID: "MD013", Name: "line-length", Default: true},
			},
			Rumdl: []rules.RuleMapping{
				{ID: "MD013", Name: "line-length", Default: true},
			},
			Mado: []rules.RuleMapping{
				{ID: "MD013", Name: "line-length", Default: true},
			},
		},
		{
			ID: "MDS019", Name: "catalog", Status: "ready",
			Description: "Catalog directive.",
			Category:    "directive",
		},
	}
	out := RenderCoverageMatrix(rs)
	assert.Contains(t, out, "## Line length")
	assert.Contains(t, out,
		"[MDS001](../../../internal/rules/MDS001-line-length/README.md) line-length")
	assert.Contains(t, out, "MD013 ✅ line-length")
	assert.Contains(t, out, "## Generated sections (directives) (mdsmith-only)")
	assert.Contains(t, out,
		"[MDS019](../../../internal/rules/MDS019-catalog/README.md) catalog")
	assert.Contains(t, out, "| Catalog directive.")
	// Table rows are padded to the widest cell so output passes
	// MDS025 table-format without a downstream fix pass.
	assert.NotContains(t, out, " | —|", "cells should have a trailing space before the pipe")
	assert.False(t, strings.HasSuffix(out, "\n\n"),
		"file ends with a single trailing newline, not a blank line")
}

// TestRenderCoverageMatrix_PeerDefaults verifies that an entry
// marked default:false renders with the off-by-default marker,
// and that partial coverage suffixes with "(partial)".
func TestRenderCoverageMatrix_PeerDefaults(t *testing.T) {
	rs := []rules.RuleInfo{
		{
			ID: "MDS064", Name: "atx-heading-whitespace", Status: "ready",
			Description: "ATX whitespace.",
			Category:    "heading",
			Markdownlint: []rules.RuleMapping{
				{ID: "MD020", Name: "no-missing-space-closed-atx",
					Default: true, Partial: true},
			},
			Rumdl: []rules.RuleMapping{
				{ID: "MD020", Name: "no-space-closed-atx", Default: false},
			},
		},
	}
	out := RenderCoverageMatrix(rs)
	assert.Contains(t, out, "MD020 ✅ no-missing-space-closed-atx (partial)")
	assert.Contains(t, out, "MD020 ⚪ no-space-closed-atx")
}

// TestRenderCoverageMatrix_DeterministicAcrossRuns verifies
// that two renderings of the same input slice produce byte-
// identical output. Drift checking depends on this property.
func TestRenderCoverageMatrix_DeterministicAcrossRuns(t *testing.T) {
	rs := []rules.RuleInfo{
		{ID: "MDS003", Name: "heading-increment", Status: "ready", Category: "heading"},
		{ID: "MDS001", Name: "line-length", Status: "ready", Category: "line"},
	}
	a := RenderCoverageMatrix(rs)
	b := RenderCoverageMatrix(rs)
	assert.Equal(t, a, b)
}

// TestApplyCoverageMatrix_PropagatesListRulesError verifies the
// stub-listRules-fails branch in ApplyCoverageMatrix. The
// real embed.FS-backed listRules cannot fail in practice, so
// this is the only way to exercise the error-propagation path.
func TestApplyCoverageMatrix_PropagatesListRulesError(t *testing.T) {
	prev := listRules
	t.Cleanup(func() { listRules = prev })
	listRules = func() ([]rules.RuleInfo, error) {
		return nil, errStubListRules
	}
	_, err := ApplyCoverageMatrix(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading rule metadata")
}

// TestCheckCoverageMatrix_PropagatesListRulesError verifies
// the same listRules-fails branch in CheckCoverageMatrix.
func TestCheckCoverageMatrix_PropagatesListRulesError(t *testing.T) {
	prev := listRules
	t.Cleanup(func() { listRules = prev })
	listRules = func() ([]rules.RuleInfo, error) {
		return nil, errStubListRules
	}
	_, err := CheckCoverageMatrix(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading rule metadata")
}

// TestApplyCoverageMatrix_PropagatesReadError drives the
// non-NotExist ReadFile error: a directory at the target file
// path makes os.ReadFile fail with EISDIR, and Apply must
// surface the error rather than treating it as "file absent".
func TestApplyCoverageMatrix_PropagatesReadError(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(
		filepath.Join(root, CoverageMatrixFile), 0o755,
	))
	_, err := ApplyCoverageMatrix(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading existing matrix")
}

// TestApplyCoverageMatrix_PropagatesMkdirError drives the
// MkdirAll error path: make an intermediate directory of the
// target path read-only so the MkdirAll call below it fails
// even though ReadFile still reports IsNotExist for the leaf.
func TestApplyCoverageMatrix_PropagatesMkdirError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("chmod-based readonly test is unreliable as root")
	}
	root := t.TempDir()
	intermediate := filepath.Join(root, "docs", "research")
	require.NoError(t, os.MkdirAll(intermediate, 0o755))
	require.NoError(t, os.Chmod(intermediate, 0o555))
	t.Cleanup(func() { _ = os.Chmod(intermediate, 0o755) })
	_, err := ApplyCoverageMatrix(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "creating output dir")
}

// TestApplyCoverageMatrix_PropagatesWriteError drives the
// WriteFile error path: pre-create the target file as
// read-only so ReadFile succeeds (returning stale content
// distinct from the generator output) and the subsequent
// WriteFile cannot overwrite it.
func TestApplyCoverageMatrix_PropagatesWriteError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("chmod-based readonly test is unreliable as root")
	}
	root := t.TempDir()
	path := filepath.Join(root, CoverageMatrixFile)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("stale\n"), 0o444))
	_, err := ApplyCoverageMatrix(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writing coverage matrix")
}

// TestFormatCoverageDrift_HaveLongerThanWant drives the n =
// len(wLines) branch (when on-disk has more lines than the
// generator's output and every overlapping line matches).
func TestFormatCoverageDrift_HaveLongerThanWant(t *testing.T) {
	msg := formatCoverageDrift("a\nb\nc", "a\nb")
	assert.Contains(t, msg, "file has 3 lines, expected 2")
}

// errStubListRules is the sentinel returned by the listRules
// seam in the error-path tests above so the assertions can
// identify it without coupling to the wrapping message.
var errStubListRules = errors.New("stub listRules failure")

// TestApplyCoverageMatrix_WritesWhenMissing verifies that a
// fresh run writes the generated file and returns changed=true.
func TestApplyCoverageMatrix_WritesWhenMissing(t *testing.T) {
	root := t.TempDir()
	changed, err := ApplyCoverageMatrix(root)
	require.NoError(t, err)
	assert.True(t, changed)
	path := filepath.Join(root, CoverageMatrixFile)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(data), "---\n"))
}

// TestApplyCoverageMatrix_IdempotentAfterFirstWrite verifies
// that a re-run on an already-generated file is a no-op.
func TestApplyCoverageMatrix_IdempotentAfterFirstWrite(t *testing.T) {
	root := t.TempDir()
	_, err := ApplyCoverageMatrix(root)
	require.NoError(t, err)
	changed, err := ApplyCoverageMatrix(root)
	require.NoError(t, err)
	assert.False(t, changed)
}

// TestRenderCoverageMatrix_UnknownCategoryFallback verifies
// that a rule whose `category:` value is missing from the
// canonical categoryTitle map still renders — the section
// title falls back to a title-cased form, and orderedCategories
// places the bucket at the end. Drives the "extras" branch in
// orderedCategories plus the title-fallback branch in
// RenderCoverageMatrix.
func TestRenderCoverageMatrix_UnknownCategoryFallback(t *testing.T) {
	rs := []rules.RuleInfo{
		{
			ID: "MDS900", Name: "experimental", Status: "ready",
			Description: "Experimental.",
			Category:    "experimental",
		},
	}
	out := RenderCoverageMatrix(rs)
	assert.Contains(t, out, "## Experimental")
}

// TestCheckCoverageMatrix_ReturnsEmptyWhenInSync verifies the
// happy path: when on-disk matches the generator, the check
// reports no drift (empty message, no error).
func TestCheckCoverageMatrix_ReturnsEmptyWhenInSync(t *testing.T) {
	root := t.TempDir()
	_, err := ApplyCoverageMatrix(root)
	require.NoError(t, err)
	msg, err := CheckCoverageMatrix(root)
	require.NoError(t, err)
	assert.Empty(t, msg)
}

// TestCheckCoverageMatrix_PropagatesReadError verifies that a
// non-NotExist read failure (here: a directory where a file is
// expected) surfaces as an error rather than a drift message.
func TestCheckCoverageMatrix_PropagatesReadError(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(
		filepath.Join(root, CoverageMatrixFile),
		0o755,
	))
	msg, err := CheckCoverageMatrix(root)
	require.Error(t, err)
	assert.Empty(t, msg)
}

// TestFormatCoverageDrift_FileLengthsDiffer drives the fallback
// branch in formatCoverageDrift: when every overlapping line
// matches but the files have different total lengths, the
// formatter reports the length mismatch rather than a per-line
// diff.
func TestFormatCoverageDrift_FileLengthsDiffer(t *testing.T) {
	// Two strings without a trailing newline; every overlapping
	// line matches and want is exactly one line longer.
	msg := formatCoverageDrift("a\nb", "a\nb\nc")
	assert.Contains(t, msg, "file has 2 lines, expected 3")
	assert.Contains(t, msg,
		"run `mdsmith-release sync-coverage-matrix` to regenerate")
}

// TestCheckCoverageMatrix_DetectsDrift verifies that a
// manually edited coverage file surfaces a drift message with
// the offending line number.
func TestCheckCoverageMatrix_DetectsDrift(t *testing.T) {
	root := t.TempDir()
	_, err := ApplyCoverageMatrix(root)
	require.NoError(t, err)
	path := filepath.Join(root, CoverageMatrixFile)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	tampered := strings.Replace(string(data),
		"# Peer-linter coverage matrix",
		"# Hand-edited title",
		1)
	require.NoError(t, os.WriteFile(path, []byte(tampered), 0o644))
	msg, err := CheckCoverageMatrix(root)
	require.NoError(t, err)
	assert.Contains(t, msg, "drift at line")
	assert.Contains(t, msg, "Hand-edited title")
}
