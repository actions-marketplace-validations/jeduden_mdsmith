//go:build !windows

package release

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestScanTestLayersWalkError drives scanTestLayers' walk-error
// branch by chmod-ing a subdirectory unreadable so WalkDir's
// ReadDir fails. It mirrors channels_chmod_unix_test.go: the chmod
// permission semantics are not portable to Windows, and root
// bypasses them, so the file is Unix-tagged and the test skips when
// running as root.
func TestScanTestLayersWalkError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("chmod-based readonly test is unreliable as root")
	}
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"),
		[]byte("module example.com/m\n"), 0o644))
	locked := filepath.Join(root, "locked")
	require.NoError(t, os.MkdirAll(locked, 0o755))
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	_, err := scanTestLayers(root)
	assert.Error(t, err)
}
