package main

import (
	"bytes"
	"compress/gzip"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Size budgets for the shipping standard-Go WASM artifact, built with
// the same -trimpath -ldflags="-s -w" flags as build.sh.
//
// With cuelang.org/go removed (plan 218/240 — the in-house cue/cuelite
// engine replaced it), the artifact dropped from ~37.9 MB raw to the
// sizes below. The plan-215 standard-Go target was ≤ 18 MB; the
// artifact now clears it comfortably. These ceilings are REGRESSION
// GUARDS set just above the measured size, so an accidental dependency
// bloat is caught in CI.
//
// Measured (Go 1.25, stripped): ~11.2 MB raw / ~2.8 MB gzipped. The
// ceilings leave headroom for toolchain-version drift while staying
// under the 18 MB plan-215 budget.
const (
	maxWASMRawBytes  = 14 * 1024 * 1024 // 14 MiB (< 18 MiB plan-215 budget)
	maxWASMGzipBytes = 4 * 1024 * 1024  // 4 MiB
)

// TestWASMArtifactSizeBudget builds the shipping WASM artifact with the
// same flags as build.sh and asserts it stays within the
// regression-guard ceilings. It needs only the standard Go wasm
// toolchain (no node), so it gates on every host that runs the wasm
// job — and in the main test job's `go test ./...` too.
func TestWASMArtifactSizeBudget(t *testing.T) {
	out := filepath.Join(t.TempDir(), "mdsmith.wasm")
	// Mirror build.sh's `go` target: -trimpath for reproducibility,
	// -ldflags="-s -w" to strip the symbol table and DWARF.
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", out, ".")
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building wasm artifact: %v\n%s", err, b)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading wasm artifact: %v", err)
	}
	raw := len(data)

	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	gz := buf.Len()

	const mib = 1024 * 1024
	t.Logf("wasm artifact: raw=%d bytes (%.1f MiB), gzip=%d bytes (%.1f MiB)",
		raw, float64(raw)/mib, gz, float64(gz)/mib)

	if raw > maxWASMRawBytes {
		t.Errorf("wasm raw size %d bytes exceeds budget %d (%.1f MiB > %.1f MiB)",
			raw, maxWASMRawBytes, float64(raw)/mib, float64(maxWASMRawBytes)/mib)
	}
	if gz > maxWASMGzipBytes {
		t.Errorf("wasm gzip size %d bytes exceeds budget %d (%.1f MiB > %.1f MiB)",
			gz, maxWASMGzipBytes, float64(gz)/mib, float64(maxWASMGzipBytes)/mib)
	}
}

// TinyGo WASM size budgets. The production build uses -no-debug, which
// strips DWARF info and can halve the raw output vs. a debug build.
//
// Raw budget: guards tinygo heap and segment fit (plan 215/247 ceiling).
// Gzip budget: guards mobile transfer cost — browsers and CDNs deliver
// the .wasm file compressed. Measured baseline with -no-debug: ~3.5 MiB
// raw / ~1.6 MiB gzipped. Ceilings leave 10 %+ headroom for
// toolchain-version drift.
const (
	maxTinyGoWASMRawBytes  = 8 * 1024 * 1024 // 8 MiB raw (plan-215/247 ceiling)
	maxTinyGoWASMGzipBytes = 3 * 1024 * 1024 // 3 MiB gzip (mobile transfer guard)
)

// tinygoFlags are the build flags used by both the production build
// script and this test, so they cannot drift apart.
var tinygoFlags = []string{"build", "-target", "wasm", "-no-debug"}

// TestTinyGoWASMArtifactSizeBudget builds the engine with tinygo using
// the same -no-debug flag as the production build and asserts the
// artifact stays within the plan-215/247 raw budget and the gzip
// transfer budget. It skips when tinygo is not installed (the offline
// dev container and the standard-Go test job); the dedicated tinygo-wasm
// CI job always has tinygo available and will not skip.
func TestTinyGoWASMArtifactSizeBudget(t *testing.T) {
	if _, err := exec.LookPath("tinygo"); err != nil {
		t.Skip("tinygo not installed; skipping tinygo size budget check")
	}
	out := filepath.Join(t.TempDir(), "mdsmith-tinygo.wasm")
	args := append(tinygoFlags, "-o", out, ".")
	cmd := exec.Command("tinygo", args...)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tinygo wasm build failed: %v\n%s", err, b)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading tinygo wasm artifact: %v", err)
	}
	raw := len(data)

	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	gz := buf.Len()

	const mib = 1024 * 1024
	t.Logf("tinygo wasm artifact: raw=%d bytes (%.1f MiB), gzip=%d bytes (%.1f MiB)",
		raw, float64(raw)/mib, gz, float64(gz)/mib)

	if raw > maxTinyGoWASMRawBytes {
		t.Errorf("tinygo wasm raw size %d bytes exceeds budget %d (%.1f MiB > %.1f MiB)",
			raw, maxTinyGoWASMRawBytes, float64(raw)/mib, float64(maxTinyGoWASMRawBytes)/mib)
	}
	if gz > maxTinyGoWASMGzipBytes {
		t.Errorf("tinygo wasm gzip size %d bytes exceeds budget %d (%.1f MiB > %.1f MiB)",
			gz, maxTinyGoWASMGzipBytes, float64(gz)/mib, float64(maxTinyGoWASMGzipBytes)/mib)
	}
}
