package build

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PruneOrphanLogs deletes every .mdsmith/build-logs/<id>.log whose <id>
// matches no cache entry's ActionID. Orphans accumulate from
// --build-no-cache runs (which write a log but no entry) and from cache
// entries replaced by a newer ActionID. A missing logs directory is a
// no-op. Non-.log files are left untouched.
func PruneOrphanLogs(root string, cache *Cache) error {
	logsDir := filepath.Join(root, filepath.FromSlash(buildLogsRelDir))
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading build-logs dir: %w", err)
	}

	live := make(map[string]struct{}, len(cache.Entries))
	for _, e := range cache.Entries {
		live[logFileName(e.ActionID)] = struct{}{}
	}

	for _, ent := range entries {
		name := ent.Name()
		if ent.IsDir() || !strings.HasSuffix(name, ".log") {
			continue
		}
		if _, ok := live[name]; ok {
			continue
		}
		if err := os.Remove(filepath.Join(logsDir, name)); err != nil {
			return fmt.Errorf("removing orphan log %s: %w", name, err)
		}
	}
	return nil
}
