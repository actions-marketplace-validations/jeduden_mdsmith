package build

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPruneOrphanLogs_DeletesLogsWithNoCacheEntry(t *testing.T) {
	root := t.TempDir()
	logsDir := filepath.Join(root, filepath.FromSlash(buildLogsRelDir))
	require.NoError(t, os.MkdirAll(logsDir, 0o755))

	keep := "sha256-keep"
	orphan := "sha256-orphan"
	require.NoError(t, os.WriteFile(filepath.Join(logsDir, logFileName(keep)), []byte("k"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(logsDir, logFileName(orphan)), []byte("o"), 0o644))

	cache := NewCache()
	cache.Put(CacheEntry{
		Outputs:  []OutputHash{{Path: "out.txt", Hash: "sha256-x"}},
		ActionID: keep,
	})

	require.NoError(t, PruneOrphanLogs(root, cache))

	assert.FileExists(t, filepath.Join(logsDir, logFileName(keep)))
	assert.NoFileExists(t, filepath.Join(logsDir, logFileName(orphan)))
}

func TestPruneOrphanLogs_NoLogsDirIsNoop(t *testing.T) {
	root := t.TempDir()
	assert.NoError(t, PruneOrphanLogs(root, NewCache()))
}

func TestPruneOrphanLogs_IgnoresNonLogFiles(t *testing.T) {
	root := t.TempDir()
	logsDir := filepath.Join(root, filepath.FromSlash(buildLogsRelDir))
	require.NoError(t, os.MkdirAll(logsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(logsDir, "README.txt"), []byte("x"), 0o644))

	require.NoError(t, PruneOrphanLogs(root, NewCache()))
	assert.FileExists(t, filepath.Join(logsDir, "README.txt"))
}

// TestPruneOrphanLogs_IgnoresDirectories covers the ent.IsDir() branch: a
// subdirectory inside the logs dir is skipped (not treated as a .log file).
func TestPruneOrphanLogs_IgnoresDirectories(t *testing.T) {
	root := t.TempDir()
	logsDir := filepath.Join(root, filepath.FromSlash(buildLogsRelDir))
	require.NoError(t, os.MkdirAll(logsDir, 0o755))
	// Create a directory whose name ends in .log to verify it is skipped.
	require.NoError(t, os.MkdirAll(filepath.Join(logsDir, "subdir.log"), 0o755))

	// A non-empty directory causes os.Remove to fail; PruneOrphanLogs must
	// skip it because ent.IsDir() is true.
	require.NoError(t, PruneOrphanLogs(root, NewCache()))
}

// TestPruneOrphanLogs_RemoveError covers the os.Remove error path: when a
// file named *.log cannot be removed (parent dir not writable), the function
// returns an error.
func TestPruneOrphanLogs_RemoveError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses directory permission checks")
	}
	root := t.TempDir()
	logsDir := filepath.Join(root, filepath.FromSlash(buildLogsRelDir))
	require.NoError(t, os.MkdirAll(logsDir, 0o755))

	orphanName := logFileName("sha256-orphan")
	require.NoError(t, os.WriteFile(filepath.Join(logsDir, orphanName), []byte("o"), 0o644))

	// Remove write permission from the logs dir so os.Remove fails.
	require.NoError(t, os.Chmod(logsDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(logsDir, 0o755) })

	err := PruneOrphanLogs(root, NewCache())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "removing orphan log")
}

// TestPruneOrphanLogs_ReadDirError covers the os.ReadDir error branch: when
// the logs directory exists but cannot be read, the function returns an error.
func TestPruneOrphanLogs_ReadDirError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses directory permission checks")
	}
	root := t.TempDir()
	logsDir := filepath.Join(root, filepath.FromSlash(buildLogsRelDir))
	require.NoError(t, os.MkdirAll(logsDir, 0o755))

	// Remove all permissions from the logs dir so os.ReadDir fails.
	require.NoError(t, os.Chmod(logsDir, 0o000))
	t.Cleanup(func() { _ = os.Chmod(logsDir, 0o755) })

	err := PruneOrphanLogs(root, NewCache())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading build-logs dir")
}
