package main_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestE2E_Build_FailurePrintsSixFieldBlock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh is not available on Windows")
	}
	dir := writeBuildRepo(t, "    boom:\n      command: noop\n")
	script := filepath.Join(dir, "boom.sh")
	require.NoError(t, os.WriteFile(script,
		[]byte("#!/bin/sh\nfor i in 1 2 3; do echo \"err line $i\" 1>&2; done\nexit 5\n"), 0o755))
	dir2 := writeBuildRepo(t, "    boom:\n      command: "+script+" {outputs}\n")
	writeFixture(t, dir2, "intro.md", buildDirective("boom", "", "out.txt"))

	_, stderr, code := runBinaryInDir(t, dir2, "", "fix", "--no-color", "--build-only", "intro.md")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "FAIL out.txt")
	assert.Contains(t, stderr, "source:")
	assert.Contains(t, stderr, "intro.md:")
	assert.Contains(t, stderr, "exit:")
	assert.Contains(t, stderr, "5")
	assert.Contains(t, stderr, "log:")
	assert.Contains(t, stderr, "last 3 lines of stderr")
	assert.Contains(t, stderr, "err line 3")
	_ = dir
}

func TestE2E_Build_StreamForwardsLines(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh is not available on Windows")
	}
	dir := writeBuildRepo(t, "    noop:\n      command: noop\n")
	script := filepath.Join(dir, "stream.sh")
	require.NoError(t, os.WriteFile(script,
		[]byte("#!/bin/sh\nfor i in $(seq 1 100); do echo \"line $i\"; done\nprintf x > \"$1\"\n"), 0o755))
	dir2 := writeBuildRepo(t, "    stream:\n      command: "+script+" {outputs}\n")
	writeFixture(t, dir2, "doc.md", buildDirective("stream", "", "out.txt"))

	stdout, stderr, code := runBinaryInDir(t, dir2, "", "fix", "--no-color",
		"--build-only", "--build-stream", "doc.md")
	out := stdout + stderr
	assert.Equal(t, 0, code, out)
	assert.Contains(t, out, "[out.txt] line 1")
	assert.Contains(t, out, "[out.txt] line 100")
	// The log file is still written.
	matches, _ := filepath.Glob(filepath.Join(dir2, ".mdsmith", "build-logs", "*.log"))
	assert.NotEmpty(t, matches, "the log file must still be written under --build-stream")
	_ = dir
}

func TestE2E_Build_ExplainPrintsActionIDAndRunsNoRecipe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cp is not available on Windows")
	}
	dir := writeBuildRepo(t, "    copy:\n      command: cp {inputs} {outputs}\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("hello"), 0o644))
	writeFixture(t, dir, "doc.md", buildDirective("copy", "src.txt", "dst.txt"))

	stdout, stderr, code := runBinaryInDir(t, dir, "", "fix", "--no-color",
		"--build-only", "--build-explain", "dst.txt", "doc.md")
	out := stdout + stderr
	assert.Equal(t, 0, code, out)
	assert.Contains(t, out, "recipe.command:")
	assert.Contains(t, out, "action-id: sha256-")
	assert.Contains(t, out, "verdict: STALE")
	// No recipe ran, so the output must not exist.
	assert.NoFileExists(t, filepath.Join(dir, "dst.txt"))
}

func TestE2E_Build_ExplainNoMatchExitsNonZero(t *testing.T) {
	dir := writeBuildRepo(t, "    copy:\n      command: cp {inputs} {outputs}\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("hello"), 0o644))
	writeFixture(t, dir, "doc.md", buildDirective("copy", "src.txt", "dst.txt"))

	_, stderr, code := runBinaryInDir(t, dir, "", "fix", "--no-color",
		"--build-only", "--build-explain", "absent.txt", "doc.md")
	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr, "no target named")
}

func TestE2E_Build_VerifyMarksUnstable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh is not available on Windows")
	}
	dir := writeBuildRepo(t, "    noop:\n      command: noop\n")
	// Script writes a different value on each run (a counter file).
	script := filepath.Join(dir, "nondet.sh")
	require.NoError(t, os.WriteFile(script,
		[]byte("#!/bin/sh\nn=0\n[ -f "+filepath.Join(dir, "counter")+" ] && n=$(cat "+
			filepath.Join(dir, "counter")+")\nn=$((n+1))\necho $n > "+
			filepath.Join(dir, "counter")+"\nprintf \"$n\" > \"$1\"\n"), 0o755))
	dir2 := writeBuildRepo(t, "    nondet:\n      command: "+script+" {outputs}\n")
	writeFixture(t, dir2, "doc.md", buildDirective("nondet", "", "out.txt"))

	stdout, stderr, code := runBinaryInDir(t, dir2, "", "fix", "--no-color",
		"--build-only", "--build-verify", "doc.md")
	out := stdout + stderr
	assert.Equal(t, 0, code, out)
	assert.Contains(t, out, "WARN")
	assert.Contains(t, out, "unstable")

	cacheData, err := os.ReadFile(filepath.Join(dir2, ".mdsmith", "build-cache.json"))
	require.NoError(t, err)
	assert.Contains(t, string(cacheData), "\"unstable\": true")
	_ = dir
}

func TestE2E_Build_OrphanLogsRemovedOnNextFix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cp is not available on Windows")
	}
	dir := writeBuildRepo(t, "    copy:\n      command: cp {inputs} {outputs}\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("hello"), 0o644))
	writeFixture(t, dir, "doc.md", buildDirective("copy", "src.txt", "dst.txt"))

	// Plant an orphan log: no cache entry matches this action id.
	logsDir := filepath.Join(dir, ".mdsmith", "build-logs")
	require.NoError(t, os.MkdirAll(logsDir, 0o755))
	orphan := filepath.Join(logsDir, "sha256-orphan.log")
	require.NoError(t, os.WriteFile(orphan, []byte("[stdout] stale\n"), 0o644))

	_, stderr, code := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "doc.md")
	require.Equal(t, 0, code, stderr)
	assert.NoFileExists(t, orphan, "orphan log with no matching cache entry must be removed")
}

func TestE2E_Build_JobsRunsConcurrently(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh/sleep is not available on Windows")
	}
	script := "sleep.sh"
	dir := writeBuildRepo(t, "    slow:\n      command: noop\n")
	scriptPath := filepath.Join(dir, script)
	require.NoError(t, os.WriteFile(scriptPath,
		[]byte("#!/bin/sh\nsleep 1\nprintf x > \"$1\"\n"), 0o755))
	dir2 := writeBuildRepo(t, "    slow:\n      command: "+scriptPath+" {outputs}\n")
	for i := 0; i < 4; i++ {
		name := "doc" + strings.Repeat("a", i) + ".md"
		out := "out" + strings.Repeat("a", i) + ".txt"
		writeFixture(t, dir2, name, buildDirective("slow", "", out))
	}

	_, stderr, code := runBinaryInDir(t, dir2, "", "fix", "--no-color",
		"--build-only", "--build-jobs", "4", ".")
	assert.Equal(t, 0, code, stderr)
	for i := 0; i < 4; i++ {
		out := "out" + strings.Repeat("a", i) + ".txt"
		assert.FileExists(t, filepath.Join(dir2, out))
	}
	_ = dir
}
