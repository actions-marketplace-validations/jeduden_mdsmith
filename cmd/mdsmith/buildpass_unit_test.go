package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/config"
)

// buildPassCfg returns a minimal *config.Config with the given recipe
// YAML snippet (already indented under recipes:) for buildpass unit tests.
func buildPassCfg(recipesYAML string) *config.Config {
	yml := []byte("build:\n  recipes:\n" + recipesYAML)
	cfg, err := config.ParseBytes(yml)
	if err != nil {
		panic("buildPassCfg: " + err.Error())
	}
	return cfg
}

// buildPassDirective returns a minimal Markdown snippet with one
// <?build?> directive referencing the given recipe and output filename.
func buildPassDirective(recipe, output string) string {
	return "# Build\n\n" +
		"<?build\nrecipe: " + recipe + "\n" +
		"outputs:\n  - " + output + "\n?>\n" +
		"[" + output + "](" + output + ")\n" +
		"<?/build?>\n"
}

func TestCollectBuildTargets_NonExistentFile(t *testing.T) {
	root := t.TempDir()
	targets, errs := collectBuildTargets([]string{"/nonexistent/path.md"}, root, "", 0)
	assert.Empty(t, targets)
	assert.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "reading")
}

func TestCollectBuildTargets_FileWithDirective(t *testing.T) {
	root := t.TempDir()
	md := buildPassDirective("cp", "out.txt")
	p := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(p, []byte(md), 0o644))

	targets, errs := collectBuildTargets([]string{p}, root, "", 0)
	assert.Empty(t, errs)
	require.Len(t, targets, 1)
	assert.Equal(t, "cp", targets[0].target.Recipe)
	assert.Equal(t, []string{"out.txt"}, targets[0].target.Outputs)
}

func TestCollectBuildTargets_RecipeFilter(t *testing.T) {
	root := t.TempDir()
	md := buildPassDirective("cp", "out.txt")
	p := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(p, []byte(md), 0o644))

	targets, errs := collectBuildTargets([]string{p}, root, "other", 0)
	assert.Empty(t, errs)
	assert.Empty(t, targets, "recipe filter 'other' should exclude the 'cp' directive")
}

func TestRunBuildPass_DryRun(t *testing.T) {
	root := t.TempDir()
	cfg := buildPassCfg("    cp:\n      command: cp {inputs} {outputs}\n")
	cfgPath := filepath.Join(root, ".mdsmith.yml")

	md := buildPassDirective("cp", "out.txt")
	p := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(p, []byte(md), 0o644))

	var buf strings.Builder
	code := runBuildPass(cfg, cfgPath, []string{p}, buildPassOpts{dryRun: true, timeout: time.Second}, &buf)
	assert.Equal(t, 0, code)
	assert.Contains(t, buf.String(), "DRY-RUN")
}

func TestRunBuildPass_NoTargets(t *testing.T) {
	root := t.TempDir()
	cfg := buildPassCfg("")
	cfgPath := filepath.Join(root, ".mdsmith.yml")

	// File with no <?build?> directive.
	p := filepath.Join(root, "plain.md")
	require.NoError(t, os.WriteFile(p, []byte("# Hello\n"), 0o644))

	var buf strings.Builder
	code := runBuildPass(cfg, cfgPath, []string{p}, buildPassOpts{timeout: time.Second}, &buf)
	assert.Equal(t, 0, code)
	assert.Empty(t, buf.String())
}

func TestRunBuildPass_NoTargetsWithReadError(t *testing.T) {
	root := t.TempDir()
	cfg := buildPassCfg("")
	cfgPath := filepath.Join(root, ".mdsmith.yml")

	var buf strings.Builder
	code := runBuildPass(cfg, cfgPath, []string{"/nonexistent/file.md"}, buildPassOpts{timeout: time.Second}, &buf)
	assert.Equal(t, 2, code)
	assert.Contains(t, buf.String(), "reading")
}

func TestRunBuildPass_IgnoresConfigIgnoredFiles(t *testing.T) {
	root := t.TempDir()
	// Config ignores "fixture/**".
	cfg := buildPassCfg("    cp:\n      command: cp {inputs} {outputs}\n")
	cfg.Ignore = []string{"fixture/**"}
	cfgPath := filepath.Join(root, ".mdsmith.yml")

	fixtureDir := filepath.Join(root, "fixture")
	require.NoError(t, os.MkdirAll(fixtureDir, 0o755))
	md := buildPassDirective("cp", "out.txt")
	p := filepath.Join(fixtureDir, "doc.md")
	require.NoError(t, os.WriteFile(p, []byte(md), 0o644))

	var buf strings.Builder
	code := runBuildPass(cfg, cfgPath, []string{p}, buildPassOpts{timeout: time.Second}, &buf)
	// No targets collected (file was ignored) → exit 0 with no output.
	assert.Equal(t, 0, code)
	assert.Empty(t, buf.String())
}

func TestBuildRecipeSpecs_Empty(t *testing.T) {
	cfg := &config.Config{}
	specs := buildRecipeSpecs(cfg)
	assert.Empty(t, specs)
}

func TestBuildRecipeSpecs_WithRecipe(t *testing.T) {
	cfg := buildPassCfg("    copy:\n      command: cp {inputs} {outputs}\n")
	specs := buildRecipeSpecs(cfg)
	require.Contains(t, specs, "copy")
	assert.Equal(t, "cp {inputs} {outputs}", specs["copy"].Command)
}
