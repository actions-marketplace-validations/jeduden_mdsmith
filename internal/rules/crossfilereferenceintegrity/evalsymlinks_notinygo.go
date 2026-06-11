//go:build !tinygo

package crossfilereferenceintegrity

import "path/filepath"

// evalSymlinksOSCall resolves symlinks in path. On non-tinygo builds this
// calls filepath.EvalSymlinks.
func evalSymlinksOSCall(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}
