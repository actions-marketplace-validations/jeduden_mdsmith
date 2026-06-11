package build

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadCache_MissingFileReturnsEmpty(t *testing.T) {
	root := t.TempDir()
	c, err := LoadCache(root)
	require.NoError(t, err)
	assert.Equal(t, CacheVersion, c.Version)
	assert.Empty(t, c.Entries)
}

func TestCache_SaveLoadRoundTrip(t *testing.T) {
	root := t.TempDir()
	c := NewCache()
	c.Put(CacheEntry{
		Outputs:  []OutputHash{{Path: "out.txt", Hash: "sha256-abc"}},
		Inputs:   []string{"src.txt"},
		ActionID: "sha256-deadbeef",
		Recipe:   "copy",
		BuiltAt:  "2026-06-11T00:00:00Z",
	})
	require.NoError(t, c.Save(root))

	got, err := LoadCache(root)
	require.NoError(t, err)
	require.Len(t, got.Entries, 1)
	e := got.Entries[0]
	assert.Equal(t, "sha256-deadbeef", e.ActionID)
	assert.Equal(t, "copy", e.Recipe)
	require.Len(t, e.Outputs, 1)
	assert.Equal(t, "out.txt", e.Outputs[0].Path)
	assert.Equal(t, "sha256-abc", e.Outputs[0].Hash)

	// File lives at .mdsmith/build-cache.json.
	_, statErr := os.Stat(filepath.Join(root, ".mdsmith", "build-cache.json"))
	assert.NoError(t, statErr)
}

func TestCache_LookupBySortedOutputSet(t *testing.T) {
	c := NewCache()
	c.Put(CacheEntry{
		Outputs:  []OutputHash{{Path: "b.txt", Hash: "h2"}, {Path: "a.txt", Hash: "h1"}},
		ActionID: "sha256-id",
	})
	// Lookup with a differently-ordered output set must still match.
	e, ok := c.Lookup([]string{"a.txt", "b.txt"})
	require.True(t, ok)
	assert.Equal(t, "sha256-id", e.ActionID)

	_, ok = c.Lookup([]string{"a.txt"})
	assert.False(t, ok)
}

func TestCache_PutReplacesByOutputSet(t *testing.T) {
	c := NewCache()
	c.Put(CacheEntry{
		Outputs:  []OutputHash{{Path: "a.txt", Hash: "h1"}},
		ActionID: "sha256-old",
	})
	c.Put(CacheEntry{
		Outputs:  []OutputHash{{Path: "a.txt", Hash: "h2"}},
		ActionID: "sha256-new",
	})
	assert.Len(t, c.Entries, 1)
	e, ok := c.Lookup([]string{"a.txt"})
	require.True(t, ok)
	assert.Equal(t, "sha256-new", e.ActionID)
}

func TestLoadCache_CorruptFileReturnsError(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".mdsmith"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, ".mdsmith", "build-cache.json"), []byte("{not json"), 0o644))
	_, err := LoadCache(root)
	require.Error(t, err)
}

func TestCache_SaveIsAtomic_NoTempLeftBehind(t *testing.T) {
	root := t.TempDir()
	c := NewCache()
	c.Put(CacheEntry{Outputs: []OutputHash{{Path: "a", Hash: "h"}}, ActionID: "id"})
	require.NoError(t, c.Save(root))

	entries, err := os.ReadDir(filepath.Join(root, ".mdsmith"))
	require.NoError(t, err)
	for _, e := range entries {
		assert.Equal(t, "build-cache.json", e.Name(), "no temp file should remain")
	}
}

func TestLoadCache_VersionZeroNormalized(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".mdsmith"), 0o755))
	// Write a cache JSON with version: 0 — should be normalised to CacheVersion on load.
	raw := `{"version":0,"entries":[]}`
	require.NoError(t, os.WriteFile(
		filepath.Join(root, ".mdsmith", "build-cache.json"), []byte(raw), 0o644))
	c, err := LoadCache(root)
	require.NoError(t, err)
	assert.Equal(t, CacheVersion, c.Version)
}

func TestLoadCache_UnreadableFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	root := t.TempDir()
	dir := filepath.Join(root, ".mdsmith")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	p := filepath.Join(dir, "build-cache.json")
	require.NoError(t, os.WriteFile(p, []byte(`{"version":1}`), 0o000))
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })

	_, err := LoadCache(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading build cache")
}

func TestCache_Save_UnwritableDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	root := t.TempDir()
	dir := filepath.Join(root, ".mdsmith")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	c := NewCache()
	err := c.Save(root)
	require.Error(t, err)
}
