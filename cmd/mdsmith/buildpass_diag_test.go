package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	buildexec "github.com/jeduden/mdsmith/internal/build"
)

// writeShScript writes an executable shell script and returns its path.
func writeShScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755))
	return p
}

func TestDispatchOne_FailingRecipePrintsSixFieldBlock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh is not available on Windows")
	}
	root := t.TempDir()
	bindir := t.TempDir()
	script := writeShScript(t, bindir, "boom.sh", `echo "boom error 1" 1>&2; echo "boom error 2" 1>&2; exit 4`)

	cfg := buildPassCfg("    boom:\n      command: " + script + " {outputs}\n")
	bt := buildTarget{
		file: "chapters/intro.md",
		line: 12,
		target: buildexec.Target{
			Recipe:  "boom",
			Root:    root,
			Outputs: []string{"out.txt"},
		},
	}
	builder := buildexec.NewCustomBuilder(map[string]buildexec.RecipeSpec{
		"boom": {Command: script + " {outputs}"},
	})

	cache := buildexec.NewCache()
	var buf strings.Builder
	outcome := dispatchOne(builder, bt, cfg, buildPassOpts{}, cache, time.Second, &buf)
	require.Equal(t, outcomeFailed, outcome)
	out := buf.String()
	assert.Contains(t, out, "FAIL out.txt")
	assert.Contains(t, out, "source:")
	assert.Contains(t, out, "chapters/intro.md:12")
	assert.Contains(t, out, "argv:")
	assert.Contains(t, out, "cwd:")
	assert.Contains(t, out, "exit:")
	assert.Contains(t, out, "4")
	assert.Contains(t, out, "duration:")
	assert.Contains(t, out, "log:")
	assert.Contains(t, out, "last 20 lines of stderr")
	assert.Contains(t, out, "boom error 2")
}

func TestDispatchOne_TimeoutPrintsDiagnosticBlock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh is not available on Windows")
	}
	root := t.TempDir()
	bindir := t.TempDir()
	script := writeShScript(t, bindir, "hang.sh", `echo "warming up" 1>&2; echo "ready" ; sleep 30`)

	cfg := buildPassCfg("    hang:\n      command: " + script + " {outputs}\n")
	bt := buildTarget{
		file: "doc.md",
		line: 1,
		target: buildexec.Target{
			Recipe:  "hang",
			Root:    root,
			Outputs: []string{"book.html"},
		},
	}
	builder := buildexec.NewCustomBuilder(map[string]buildexec.RecipeSpec{
		"hang": {Command: script + " {outputs}"},
	})

	cache := buildexec.NewCache()
	var buf strings.Builder
	outcome := dispatchOne(builder, bt, cfg, buildPassOpts{}, cache, 200*time.Millisecond, &buf)
	require.Equal(t, outcomeFailed, outcome)
	out := buf.String()
	assert.Contains(t, out, "TIMEOUT book.html")
	assert.Contains(t, out, "last 20 lines of stdout")
	assert.Contains(t, out, "last 20 lines of stderr")
	assert.Contains(t, out, "SIGTERM")
}
