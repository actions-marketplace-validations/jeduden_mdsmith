package release

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeExport writes a hyperfine-shaped export with the given per-tool
// median wall times (seconds) into dir/name.
func writeExport(t *testing.T, dir, name string, medians map[string]float64) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	var b bytes.Buffer
	b.WriteString(`{"results":[`)
	first := true
	for cmd, med := range medians {
		if !first {
			b.WriteString(",")
		}
		first = false
		fmt.Fprintf(&b, `{"command":%q,"median":%g}`, cmd, med)
	}
	b.WriteString("]}")
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), b.Bytes(), 0o644))
}

// stageCorpora writes both default corpora into a fresh baseline and
// fresh dir from per-corpus {tool: median} maps, returning the dirs.
func stageCorpora(t *testing.T, base, fresh map[string]map[string]float64) (string, string) {
	t.Helper()
	bDir, fDir := t.TempDir(), t.TempDir()
	for corpus, m := range base {
		writeExport(t, bDir, corpus, m)
	}
	for corpus, m := range fresh {
		writeExport(t, fDir, corpus, m)
	}
	return bDir, fDir
}

func TestBenchCheck(t *testing.T) {
	// Helper: both corpora at the same ratio, derived from the medians.
	both := func(mdsmith, mado float64) map[string]map[string]float64 {
		one := map[string]float64{"mdsmith": mdsmith, "mado": mado}
		return map[string]map[string]float64{
			"corpus_repo.json":    one,
			"corpus_neutral.json": one,
		}
	}

	t.Run("ratio within tolerance is not a regression", func(t *testing.T) {
		// baseline 5.0x, fresh 5.4x (+8%) < 15%.
		bDir, fDir := stageCorpora(t, both(0.25, 0.05), both(0.27, 0.05))
		var w bytes.Buffer
		require.NoError(t, BenchCheck(&w, bDir, fDir, BenchCheckConfig{}))
		assert.Contains(t, w.String(), "no perf regression")
		assert.Contains(t, w.String(), "ok")
	})

	t.Run("ratio worsening beyond tolerance is a regression", func(t *testing.T) {
		// baseline 5.0x, fresh 6.0x (+20%) > 15%.
		bDir, fDir := stageCorpora(t, both(0.25, 0.05), both(0.30, 0.05))
		var w bytes.Buffer
		err := BenchCheck(&w, bDir, fDir, BenchCheckConfig{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "perf regression")
		assert.Contains(t, err.Error(), "corpus_repo.json")
		assert.Contains(t, w.String(), "REGRESSION")
	})

	t.Run("getting faster is never a regression", func(t *testing.T) {
		bDir, fDir := stageCorpora(t, both(0.25, 0.05), both(0.20, 0.05)) // 5.0x -> 4.0x
		require.NoError(t, BenchCheck(&bytes.Buffer{}, bDir, fDir, BenchCheckConfig{}))
	})

	t.Run("corpus growth alone does not trip it", func(t *testing.T) {
		// The corpus doubled: both tools' absolute time doubled, so the
		// ratio is unchanged. This is the property the ratio buys us.
		bDir, fDir := stageCorpora(t, both(0.25, 0.05), both(0.50, 0.10)) // both 5.0x
		require.NoError(t, BenchCheck(&bytes.Buffer{}, bDir, fDir, BenchCheckConfig{}))
	})

	t.Run("custom config overrides the defaults", func(t *testing.T) {
		// rumdl vs mado, single corpus, 50% tolerance: 2.0x -> 2.8x
		// (+40%) is within 50%, so not a regression.
		one := map[string]float64{"rumdl": 0.10, "mado": 0.05}
		freshOne := map[string]float64{"rumdl": 0.14, "mado": 0.05}
		bDir, fDir := stageCorpora(t,
			map[string]map[string]float64{"corpus_repo.json": one},
			map[string]map[string]float64{"corpus_repo.json": freshOne})
		require.NoError(t, BenchCheck(&bytes.Buffer{}, bDir, fDir, BenchCheckConfig{
			Tool: "rumdl", BaselineTool: "mado", Tolerance: 0.5,
			Corpora: []string{"corpus_repo.json"},
		}))
	})

	t.Run("missing fresh export errors", func(t *testing.T) {
		bDir, _ := stageCorpora(t, both(0.25, 0.05), nil)
		err := BenchCheck(&bytes.Buffer{}, bDir, t.TempDir(), BenchCheckConfig{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read ")
	})

	t.Run("missing baseline export errors", func(t *testing.T) {
		_, fDir := stageCorpora(t, nil, both(0.25, 0.05))
		err := BenchCheck(&bytes.Buffer{}, t.TempDir(), fDir, BenchCheckConfig{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read ")
	})
}

func TestMedianFor(t *testing.T) {
	dir := t.TempDir()
	writeExport(t, dir, "c.json", map[string]float64{"mado": 0.05})

	t.Run("reads the recorded median", func(t *testing.T) {
		got, err := medianFor(filepath.Join(dir, "c.json"), "mado")
		require.NoError(t, err)
		assert.InDelta(t, 0.05, got, 1e-9)
	})

	t.Run("missing file errors", func(t *testing.T) {
		_, err := medianFor(filepath.Join(dir, "absent.json"), "mado")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read ")
	})

	t.Run("unparseable JSON errors", func(t *testing.T) {
		bad := filepath.Join(dir, "bad.json")
		require.NoError(t, os.WriteFile(bad, []byte("not json"), 0o644))
		_, err := medianFor(bad, "mado")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse ")
	})

	t.Run("absent command errors", func(t *testing.T) {
		_, err := medianFor(filepath.Join(dir, "c.json"), "mdsmith")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `no "mdsmith" result`)
	})
}

func TestRatioFor(t *testing.T) {
	dir := t.TempDir()

	t.Run("non-positive baseline median errors", func(t *testing.T) {
		writeExport(t, dir, "zero.json", map[string]float64{"mdsmith": 0.25, "mado": 0})
		_, err := ratioFor(filepath.Join(dir, "zero.json"), "mdsmith", "mado")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "non-positive")
	})

	t.Run("missing tool median errors before the baseline is read", func(t *testing.T) {
		writeExport(t, dir, "noml.json", map[string]float64{"mado": 0.05})
		_, err := ratioFor(filepath.Join(dir, "noml.json"), "mdsmith", "mado")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `no "mdsmith" result`)
	})

	t.Run("missing baseline median errors", func(t *testing.T) {
		writeExport(t, dir, "nomado.json", map[string]float64{"mdsmith": 0.25})
		_, err := ratioFor(filepath.Join(dir, "nomado.json"), "mdsmith", "mado")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `no "mado" result`)
	})
}
