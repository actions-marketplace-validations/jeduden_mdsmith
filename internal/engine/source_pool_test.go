package engine_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/engine"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/require"

	_ "github.com/jeduden/mdsmith/internal/rules/all"
)

// TestRunner_PooledSourceReuseDoesNotCorrupt is the correctness guard for
// the pooled per-file source read. lintFile draws one *[]byte from a
// sync.Pool, reads each file into it via bytelimit.ReadFileLimitedInto,
// and returns it from release(). With a single serial worker that buffer
// is reused across every file, so the read must reslice to the new file's
// exact length: a large file followed by a small one must not leak the
// large file's trailing bytes into the small file's Source/Lines.
//
// The corpus alternates a long document and a short one. If the pooled
// buffer were resliced to the wrong length (or not at all), the short
// file's parse would see stale tail bytes and its diagnostics would
// diverge from a fresh, un-pooled read. The test pins that the serial
// (pooled-reuse) run is byte-for-byte identical to a high-concurrency run
// whose per-worker buffers each touch fewer files.
func TestRunner_PooledSourceReuseDoesNotCorrupt(t *testing.T) {
	dir := t.TempDir()
	// A long file then a short file, repeated, so a serial worker reuses
	// one grown buffer for a short file right after a long one.
	longBytes := []byte(buildBadDoc(60))
	shortBytes := []byte(buildBadDoc(3))
	paths := make([]string, 0, 20)
	for i := 0; i < 10; i++ {
		lp := filepath.Join(dir, fmt.Sprintf("long%02d.md", i))
		sp := filepath.Join(dir, fmt.Sprintf("short%02d.md", i))
		require.NoError(t, os.WriteFile(lp, longBytes, 0o644))
		require.NoError(t, os.WriteFile(sp, shortBytes, 0o644))
		paths = append(paths, lp, sp)
	}

	run := func(concurrency int) []lint.Diagnostic {
		r := &engine.Runner{
			Config:           config.Defaults(),
			Rules:            rule.All(),
			StripFrontMatter: true,
			RootDir:          dir,
			Concurrency:      concurrency,
		}
		return r.Run(paths).Diagnostics
	}

	// Serial run: one pooled buffer reused across all 20 files.
	serial := run(1)
	// Parallel run: distributes files across workers, a different reuse
	// pattern. Both must agree if the reslice is correct.
	parallel := run(8)

	require.NotEmpty(t, serial, "expected diagnostics from the bad corpus, got none")
	require.Len(t, parallel, len(serial),
		"serial run produced %d diagnostics, parallel %d — pooled buffer reslice bug?",
		len(serial), len(parallel))
	for i := range serial {
		sk, pk := diagKey(serial[i]), diagKey(parallel[i])
		require.Equal(t, sk, pk,
			"diagnostic %d differs between serial and parallel runs: %q vs %q "+
				"— stale bytes leaking through the pooled source buffer?",
			i, sk, pk)
	}
}

// diagKey reduces a diagnostic to the position+message tuple a stale-byte
// reslice bug would perturb, so the serial/parallel comparison ignores
// incidental fields (e.g. unset SourceLines) that the slice-containing
// struct cannot compare with ==.
func diagKey(d lint.Diagnostic) string {
	return fmt.Sprintf("%s:%d:%d:%s:%s", d.File, d.Line, d.Column, d.RuleID, d.Message)
}

// buildBadDoc emits a Markdown document with trailing-whitespace and
// long-line violations so the default rule set produces diagnostics whose
// count and positions depend on the exact source bytes — making a stale-
// byte reslice bug observable as a diagnostic mismatch.
func buildBadDoc(lines int) string {
	var b strings.Builder
	b.WriteString("# Document   \n\n")
	for i := 0; i < lines; i++ {
		// Trailing spaces (MDS) and a long line.
		fmt.Fprintf(&b, "Line %d with trailing spaces and a deliberately long tail that "+
			"runs past the configured maximum line length to trip the rule.   \n\n", i)
	}
	return b.String()
}
