package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	buildexec "github.com/jeduden/mdsmith/internal/build"
)

func TestExplainTarget_PrintsActionIDInputsAndVerdict(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "src.txt"), []byte("content"), 0o644))
	cfg := buildPassCfg("    cp:\n      command: cp {inputs} {outputs}\n")
	bt := buildTarget{
		file: "doc.md",
		line: 1,
		target: buildexec.Target{
			Recipe:  "cp",
			Root:    root,
			Inputs:  []string{"src.txt"},
			Outputs: []string{"out.txt"},
		},
	}
	cache := buildexec.NewCache()
	var buf strings.Builder
	code := explainTarget([]buildTarget{bt}, "out.txt", cfg, cache, &buf)
	require.Equal(t, 0, code)
	out := buf.String()
	assert.Contains(t, out, "recipe.command:")
	assert.Contains(t, out, "cp {inputs} {outputs}")
	assert.Contains(t, out, "inputs:")
	assert.Contains(t, out, "src.txt")
	assert.Contains(t, out, "outputs:")
	assert.Contains(t, out, "out.txt")
	assert.Contains(t, out, "cache.version:")
	assert.Contains(t, out, "action-id:")
	assert.Contains(t, out, "sha256-")
	assert.Contains(t, out, "verdict:")
	assert.Contains(t, out, "STALE")
}

func TestExplainTarget_NoMatchExitsNonZero(t *testing.T) {
	cfg := buildPassCfg("    cp:\n      command: cp {inputs} {outputs}\n")
	bt := buildTarget{
		file: "doc.md",
		line: 1,
		target: buildexec.Target{
			Recipe:  "cp",
			Root:    t.TempDir(),
			Outputs: []string{"out.txt"},
		},
	}
	var buf strings.Builder
	code := explainTarget([]buildTarget{bt}, "nope.txt", cfg, buildexec.NewCache(), &buf)
	assert.NotEqual(t, 0, code)
	assert.Contains(t, buf.String(), "no target named")
}
