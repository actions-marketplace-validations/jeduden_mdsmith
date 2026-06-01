package release

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// relKey is the slash path LoadChannels passes to the extractor.
func relKey(name string) string {
	return filepath.ToSlash(filepath.Join(ChannelDir, name))
}

// mkChannelDoc builds the extract envelope for one channel.
func mkChannelDoc(title, mech, art, cmd, aud string, weight int) channelDoc {
	var d channelDoc
	d.Frontmatter.Title = title
	d.Frontmatter.Mechanism = mech
	d.Frontmatter.Artifact = art
	d.Frontmatter.Command = cmd
	d.Frontmatter.Audience = aud
	d.Frontmatter.Weight = weight
	return d
}

// stubChannelExtractor swaps the shell-out seam for canned JSON and
// restores it when the test ends.
func stubChannelExtractor(t *testing.T, byRel map[string]channelDoc) {
	t.Helper()
	prev := channelExtractor
	t.Cleanup(func() { channelExtractor = prev })
	channelExtractor = func(_, rel string) ([]byte, error) {
		doc, ok := byRel[rel]
		require.Truef(t, ok, "no extractor stub for %s", rel)
		b, err := json.Marshal(doc)
		require.NoError(t, err)
		return b, nil
	}
}

// seedChannelDir creates a repo root with the given channel files.
func seedChannelDir(t *testing.T, files ...string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, filepath.FromSlash(ChannelDir))
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for _, f := range files {
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, f), []byte("# x\n"), 0o644))
	}
	return root
}

func TestLoadChannelsSortsByWeightAndSkipsProto(t *testing.T) {
	root := seedChannelDir(t, "a.md", "b.md", "proto.md")
	stubChannelExtractor(t, map[string]channelDoc{
		relKey("a.md"): mkChannelDoc("A", "push", "cli", "cmd a", "aud a", 5),
		relKey("b.md"): mkChannelDoc("B", "pull", "cli", "cmd b", "aud b", 1),
		// no proto.md stub: channelFiles must exclude it.
	})

	chs, err := LoadChannels(root)
	require.NoError(t, err)
	require.Len(t, chs, 2)
	assert.Equal(t, "B", chs[0].Title, "weight 1 sorts first")
	assert.Equal(t, "A", chs[1].Title)
}

func TestLoadChannelsRejectsEmptyRequiredField(t *testing.T) {
	root := seedChannelDir(t, "a.md")
	stubChannelExtractor(t, map[string]channelDoc{
		relKey("a.md"): mkChannelDoc("A", "push", "cli", "", "aud", 1),
	})

	_, err := LoadChannels(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command")
}

func TestRenderChannelsYAML(t *testing.T) {
	out, err := RenderChannelsYAML([]Channel{{
		Title: "Go", Command: "go install", Mechanism: "toolchain",
		Artifact: "cli", Audience: "devs", Platforms: []string{"go"},
		URL: "https://example.test", Weight: 1,
	}})
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, "do not edit by hand")
	assert.Contains(t, s, "- title: Go")
	assert.Contains(t, s, "command: go install")
	assert.Contains(t, s, "platforms:")
}

func TestSyncChannelsWritesAndCheckDetectsDrift(t *testing.T) {
	root := seedChannelDir(t, "a.md")
	stubChannelExtractor(t, map[string]channelDoc{
		relKey("a.md"): mkChannelDoc("A", "push", "cli", "cmd a", "aud a", 1),
	})
	dataPath := filepath.Join(root, filepath.FromSlash(ChannelsDataFile))

	// Missing data file counts as drift.
	drift, err := CheckChannels(root)
	require.NoError(t, err)
	assert.True(t, drift)

	changed, err := SyncChannels(root)
	require.NoError(t, err)
	assert.True(t, changed)

	data, err := os.ReadFile(dataPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "title: A")

	// Re-sync is a byte-stable no-op; check now passes.
	changed, err = SyncChannels(root)
	require.NoError(t, err)
	assert.False(t, changed)
	drift, err = CheckChannels(root)
	require.NoError(t, err)
	assert.False(t, drift)

	// Hand-editing the data file is drift again.
	require.NoError(t, os.WriteFile(dataPath, []byte("# tampered\n"), 0o644))
	drift, err = CheckChannels(root)
	require.NoError(t, err)
	assert.True(t, drift)
}
