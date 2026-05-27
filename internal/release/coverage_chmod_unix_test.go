//go:build !windows

package release

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The two tests below drive ApplyCoverageMatrix's MkdirAll
// and WriteFile error branches by chmod-ing intermediate
// paths read-only. They live in a Unix-tagged file because
// os.Geteuid and the relevant chmod permission semantics
// are not portable to Windows builds.

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
