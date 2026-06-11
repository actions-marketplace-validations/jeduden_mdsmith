//go:build tinygo

package githooks

import "os"

// chmodFn is a no-op on tinygo/wasm builds. The wasm host has no POSIX mode
// bits; skipping the chmod degrades nothing the engine reads back.
var chmodFn = func(_ string, _ os.FileMode) error { return nil }

// sameFile returns false on tinygo/wasm builds. The hook-install path is
// not reachable from the wasm surface; a "not equal" result is safe.
func sameFile(_, _ os.FileInfo) bool {
	return false
}
