//go:build tinygo

package crossfilereferenceintegrity

// evalSymlinksOSCall is a no-op identity function on tinygo/wasm builds. The
// wasm sandbox has no symlinks; returning the input path unchanged is correct.
func evalSymlinksOSCall(path string) (string, error) {
	return path, nil
}
