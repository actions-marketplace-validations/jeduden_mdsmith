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
