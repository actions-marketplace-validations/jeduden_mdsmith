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

// TestPrintExplanation_ExplainError covers the err != nil branch in
// printExplanation (line ~52 in builddiag.go): when Explain returns an
// error, printExplanation writes the error and returns 2.
func TestPrintExplanation_ExplainError(t *testing.T) {
	root := t.TempDir()
	// "absent.txt" does not exist, so Explain's resolveInputs call fails.
	bt := buildTarget{
		file: "doc.md",
		line: 1,
		target: buildexec.Target{
			Recipe:  "cp",
			Root:    root,
			Inputs:  []string{"absent.txt"},
			Outputs: []string{"out.txt"},
		},
	}
	cfg := buildPassCfg("    cp:\n      command: cp {inputs} {outputs}\n")
	cache := buildexec.NewCache()
	var buf strings.Builder
	code := printExplanation(bt, cfg, cache, &buf)
	assert.Equal(t, 2, code)
	assert.Contains(t, buf.String(), "mdsmith:")
}

// TestPrintExplanation_WithParams covers the params loop body in printExplanation:
// when a target has Params, each key: value pair must appear in the output.
func TestPrintExplanation_WithParams(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "src.txt"), []byte("data"), 0o644))
	cfg := buildPassCfg("    vhs:\n      command: vhs {tape}\n      params:\n        optional: [tape, theme]\n")
	bt := buildTarget{
		file: "doc.md",
		line: 1,
		target: buildexec.Target{
			Recipe:  "vhs",
			Root:    root,
			Inputs:  []string{"src.txt"},
			Outputs: []string{"out.gif"},
			Params:  map[string]string{"tape": "demo.tape", "theme": "dark"},
		},
	}
	cache := buildexec.NewCache()
	var buf strings.Builder
	code := printExplanation(bt, cfg, cache, &buf)
	require.Equal(t, 0, code)
	out := buf.String()
	assert.Contains(t, out, "params:")
	assert.Contains(t, out, "tape:")
	assert.Contains(t, out, "theme:")
}

// TestExplainTarget_EmptyOutputsTargetSkipped covers the len(bt.target.Outputs)==0
// branch in explainTarget — a target with no outputs is skipped.
func TestExplainTarget_EmptyOutputsTargetSkipped(t *testing.T) {
	root := t.TempDir()
	cfg := buildPassCfg("    cp:\n      command: cp {inputs} {outputs}\n")
	// Target with no outputs: must be skipped, so the "no target named" message fires.
	bt := buildTarget{
		file: "doc.md",
		line: 1,
		target: buildexec.Target{
			Recipe:  "cp",
			Root:    root,
			Outputs: nil,
		},
	}
	cache := buildexec.NewCache()
	var buf strings.Builder
	code := explainTarget([]buildTarget{bt}, "out.txt", cfg, cache, &buf)
	assert.Equal(t, 2, code)
	assert.Contains(t, buf.String(), "no target named")
}
