package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/config"
)

// chdirToRemoved changes into a fresh temp dir and then deletes it, so the
// process has no valid working directory and os.Getwd fails. t.Chdir
// restores the original directory at cleanup. This drives the Getwd-error
// fallbacks that a normal filesystem never reaches.
func chdirToRemoved(t *testing.T) {
	t.Helper()
	dir, err := os.MkdirTemp("", "mdsmith-nowd-*")
	require.NoError(t, err)
	t.Chdir(dir)
	require.NoError(t, os.Remove(dir))
}

func TestReportFlagParseErr_NilReturnsContinue(t *testing.T) {
	// A nil parse error means "no error": the caller should continue,
	// signalled by -1.
	assert.Equal(t, -1, reportFlagParseErr(nil, os.Stderr, "mdsmith: x"))
}

func TestDiscoverConfigPath_GetwdError(t *testing.T) {
	chdirToRemoved(t)
	// With no cwd, discoverConfigPath falls back to the empty-rooted default.
	assert.Equal(t, config.DefaultConfigPath(""), discoverConfigPath(""))
}

func TestRootDirFromConfig_GetwdError(t *testing.T) {
	chdirToRemoved(t)
	// An empty cfgPath with no cwd yields an empty root.
	assert.Equal(t, "", rootDirFromConfig(""))
}

func TestLoadConfigRaw_GetwdErrorFallsBackToDefaults(t *testing.T) {
	chdirToRemoved(t)
	cfg, path, err := loadConfigRaw("")
	require.NoError(t, err)
	assert.Equal(t, "", path, "no config discovered without a cwd")
	require.NotNil(t, cfg)
}

func TestRunQuery_LoadConfigError(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.yml")
	require.NoError(t, os.WriteFile(bad, []byte("this: : : not yaml\n"), 0o644))
	// ResolveFilesWithOpts(["."]) succeeds, then loadConfig parses the bad
	// config and fails — the runQuery config-error branch.
	code := runQuery([]string{"-c", bad, "status: \"x\"", "."})
	assert.Equal(t, 2, code)
}

func TestRunQuery_ResolveFilesError(t *testing.T) {
	// An explicit, nonexistent file argument makes file resolution fail
	// before any config load.
	code := runQuery([]string{"status: \"x\"", filepath.Join(t.TempDir(), "nope.md")})
	assert.Equal(t, 2, code)
}

func TestExecuteMetricsRank_ResolveFilesError(t *testing.T) {
	// Metric selection and config load succeed, but an explicit nonexistent
	// file makes resolveRankFiles fail — the rank file-resolution branch.
	code := runMetricsRank([]string{filepath.Join(t.TempDir(), "missing.md")})
	assert.Equal(t, 2, code)
}
