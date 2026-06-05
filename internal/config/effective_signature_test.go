package config_test

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/stretchr/testify/assert"
)

func sigTestConfig() *config.Config {
	return &config.Config{
		Rules: map[string]config.RuleCfg{
			"line-length":  {Enabled: true, Settings: map[string]any{"max": 80}},
			"no-bare-urls": {Enabled: true},
		},
		Overrides: []config.Override{
			{Glob: []string{"docs/**"}, Rules: map[string]config.RuleCfg{"line-length": {Enabled: false}}},
		},
	}
}

func TestEffectiveSignature_SharesKeyWhenInputsMatch(t *testing.T) {
	cfg := sigTestConfig()
	// Two paths, neither matching docs/**, no kinds: identical inputs to
	// effectiveRules ⇒ one signature, so the engine memo serves both.
	k1, kinds1 := config.EffectiveSignature(cfg, "src/a.md", nil, nil)
	k2, kinds2 := config.EffectiveSignature(cfg, "lib/b.md", nil, nil)
	assert.Equal(t, k1, k2)
	assert.Empty(t, kinds1)
	assert.Empty(t, kinds2)
}

func TestEffectiveSignature_OverrideMatchChangesKey(t *testing.T) {
	cfg := sigTestConfig()
	kNoMatch, _ := config.EffectiveSignature(cfg, "src/a.md", nil, nil)
	kMatch, _ := config.EffectiveSignature(cfg, "docs/a.md", nil, nil)
	assert.NotEqual(t, kNoMatch, kMatch, "a matching override must change the signature")
}

func TestEffectiveSignature_KindsDimension(t *testing.T) {
	cfg := sigTestConfig()
	// Files outside docs/** share override matches, so only the resolved
	// kinds distinguish them: different kinds ⇒ different keys.
	ka, kindsA := config.EffectiveSignature(cfg, "src/x.md", []string{"a"}, nil)
	kb, _ := config.EffectiveSignature(cfg, "src/y.md", []string{"b"}, nil)
	assert.Equal(t, []string{"a"}, kindsA)
	assert.NotEqual(t, ka, kb, "different resolved kinds must produce different signatures")

	// Same kinds + same (no) override match ⇒ one shared key, so the
	// engine memo serves both files from a single resolution.
	ka2, _ := config.EffectiveSignature(cfg, "lib/z.md", []string{"a"}, nil)
	assert.Equal(t, ka, ka2, "same kinds + same overrides must share a signature")
}

func TestEffectiveAllForKinds_MatchesEffectiveAll(t *testing.T) {
	cfg := sigTestConfig()
	for _, path := range []string{"src/a.md", "docs/a.md", "docs/sub/c.md"} {
		wantR, wantC, wantE := config.EffectiveAll(cfg, path, nil, nil)
		_, kinds := config.EffectiveSignature(cfg, path, nil, nil)
		gotR, gotC, gotE := config.EffectiveAllForKinds(cfg, path, kinds)
		assert.Equal(t, wantR, gotR, "rules mismatch for %s", path)
		assert.Equal(t, wantC, gotC, "categories mismatch for %s", path)
		assert.Equal(t, wantE, gotE, "explicit mismatch for %s", path)
	}
}
