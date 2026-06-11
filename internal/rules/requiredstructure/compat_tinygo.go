//go:build tinygo

package requiredstructure

import "os"

// sameFile compares files by their absolute cleaned paths on tinygo/wasm
// builds. The wasm sandbox has no hard links or symlinks; path equality
// is an accurate substitute for os.SameFile there.
func sameFile(_, _ os.FileInfo) bool {
	// The wasm sandbox has no hard links; the caller falls back to the
	// cleaned absolute path comparison in the else branch of isSchemaFile.
	return false
}
