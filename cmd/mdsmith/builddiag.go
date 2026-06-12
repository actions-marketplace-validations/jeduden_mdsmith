package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	buildexec "github.com/jeduden/mdsmith/internal/build"
)

// diagTailLines is the number of trailing stream lines printed in the
// failure and timeout diagnostics.
const diagTailLines = 20

// reportBuildFailure prints the rich diagnostic for a failed recipe. A
// timeout prints the hung-recipe block (last lines of both streams before
// SIGTERM); any other failure prints the six-field block plus the last 20
// lines of stderr.
func reportBuildFailure(bt buildTarget, res targetRunResult, w io.Writer) {
	name := targetName(bt)
	if res.TimedOut {
		reportTimeout(name, res, w)
		return
	}
	_, _ = fmt.Fprintf(w, "FAIL %s (recipe: %s)\n", name, bt.target.Recipe)
	_, _ = fmt.Fprintf(w, "  source:   %s:%d <?build?>\n", bt.file, bt.line)
	_, _ = fmt.Fprintf(w, "  argv:     %s\n", strings.Join(res.Argv, " "))
	_, _ = fmt.Fprintf(w, "  cwd:      %s\n", res.Cwd)
	_, _ = fmt.Fprintf(w, "  exit:     %d\n", res.ExitCode)
	_, _ = fmt.Fprintf(w, "  duration: %s\n", res.Duration.Round(time.Millisecond))
	_, _ = fmt.Fprintf(w, "  log:      %s\n", relLogPath(bt.target.Root, res.LogPath))
	if len(res.StderrTail) == 0 && res.LogPath == "" {
		_, _ = fmt.Fprintf(w, "  error:    %v\n", res.Err)
		return
	}
	_, _ = fmt.Fprintf(w, "  --- last %d lines of stderr ---\n", diagTailLines)
	for _, line := range lastLines(res.StderrTail, diagTailLines) {
		_, _ = fmt.Fprintf(w, "  %s\n", line)
	}
}

// reportTimeout prints the hung-recipe diagnostic before the SIGTERM that
// the context cancellation already sent.
func reportTimeout(name string, res targetRunResult, w io.Writer) {
	_, _ = fmt.Fprintf(w, "TIMEOUT %s after %s\n", name, res.Duration.Round(time.Millisecond))
	_, _ = fmt.Fprintf(w, "  --- last %d lines of stdout ---\n", diagTailLines)
	for _, line := range lastLines(res.StdoutTail, diagTailLines) {
		_, _ = fmt.Fprintf(w, "  %s\n", line)
	}
	_, _ = fmt.Fprintf(w, "  --- last %d lines of stderr ---\n", diagTailLines)
	for _, line := range lastLines(res.StderrTail, diagTailLines) {
		_, _ = fmt.Fprintf(w, "  %s\n", line)
	}
	_, _ = fmt.Fprintf(w, "  sent SIGTERM to process group\n")
}

// lastLines returns the last n elements of lines (or all if fewer).
func lastLines(lines []string, n int) []string {
	if len(lines) <= n {
		return lines
	}
	return lines[len(lines)-n:]
}

// relLogPath returns the log path relative to root for readable output,
// or the original path when it is not under root or is empty.
func relLogPath(root, logPath string) string {
	if logPath == "" {
		return "(no log)"
	}
	if rel, err := filepath.Rel(root, logPath); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return logPath
}

// verifyTarget re-runs the recipe a second time in an independent staging
// dir, diffs the declared output bytes against the first run, and sets
// res.Unstable with a warning when they differ. A mismatch is a warning,
// not a failure: some recipes embed timestamps or random seeds.
func verifyTarget(
	b buildexec.Builder, bt buildTarget, stin buildexec.StalenessInput,
	opts buildPassOpts, timeout time.Duration, res *targetRunResult, w io.Writer,
) {
	first := snapshotOutputs(bt)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	verifyOpts := buildexec.BuildOptions{}
	second := b.BuildWithResult(ctx, bt.target, verifyOpts)
	if second.Err != nil {
		_, _ = fmt.Fprintf(w, "WARN %s: verify re-run failed: %v\n", targetName(bt), second.Err)
		res.Unstable = true
		return
	}
	if !outputsEqual(first, snapshotOutputs(bt)) {
		_, _ = fmt.Fprintf(w,
			"WARN %s: non-deterministic output (two runs differ); marking unstable\n",
			targetName(bt))
		res.Unstable = true
	}
}

// snapshotOutputs reads every declared output of bt from disk into a
// path→bytes map. A missing output maps to nil.
func snapshotOutputs(bt buildTarget) map[string][]byte {
	out := make(map[string][]byte, len(bt.target.Outputs))
	for _, rel := range bt.target.Outputs {
		abs := filepath.Join(bt.target.Root, filepath.FromSlash(rel))
		data, err := os.ReadFile(abs) //nolint:gosec // abs is an in-root declared output
		if err != nil {
			out[rel] = nil
			continue
		}
		out[rel] = data
	}
	return out
}

// outputsEqual reports whether two output snapshots hold identical bytes
// for every key.
func outputsEqual(a, b map[string][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok || string(av) != string(bv) {
			return false
		}
	}
	return true
}
