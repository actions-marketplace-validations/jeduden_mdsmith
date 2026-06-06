package release

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// hyperfineExport is the slice of a hyperfine `--export-json` file the
// regression check reads: the per-command median wall times.
type hyperfineExport struct {
	Results []struct {
		Command string  `json:"command"`
		Median  float64 `json:"median"`
	} `json:"results"`
}

// medianFor returns the median wall time hyperfine recorded for command
// in the export at path. A missing file, unparseable JSON, or absent
// command is an error rather than a silent zero, so a renamed tool or a
// truncated upload trips the check loudly instead of reading as "fast".
func medianFor(path, command string) (float64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", path, err)
	}
	var ex hyperfineExport
	if err := json.Unmarshal(data, &ex); err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}
	for _, r := range ex.Results {
		if r.Command == command {
			return r.Median, nil
		}
	}
	return 0, fmt.Errorf("%s: no %q result", path, command)
}

// ratioFor reads the tool/baseline median ratio from one hyperfine
// export. The ratio (not the absolute median) is the comparable
// quantity across runs: both tools walk the same corpus on the same
// runner, so the machine speed and the corpus size cancel.
func ratioFor(path, tool, baseline string) (float64, error) {
	t, err := medianFor(path, tool)
	if err != nil {
		return 0, err
	}
	b, err := medianFor(path, baseline)
	if err != nil {
		return 0, err
	}
	if b <= 0 {
		return 0, fmt.Errorf("%s: non-positive %q median %g", path, baseline, b)
	}
	return t / b, nil
}

// BenchCheckConfig tunes BenchCheck. The zero value applies the
// defaults documented on each field.
type BenchCheckConfig struct {
	// Tool is the tool under test (default "mdsmith").
	Tool string
	// BaselineTool is the stable reference the ratio is taken against
	// (default "mado" — the leanest, most run-to-run stable peer).
	BaselineTool string
	// Tolerance is the fractional worsening of the ratio allowed before
	// it counts as a regression. A nil pointer applies the default 0.15
	// (15%); a non-nil value is used as-is, so an explicit 0 is a strict
	// zero-tolerance gate — which a plain float64 could not tell apart
	// from "unset".
	Tolerance *float64
	// Corpora are the export file names compared (default
	// corpus_repo.json and corpus_neutral.json).
	Corpora []string
}

func (c *BenchCheckConfig) applyDefaults() {
	if c.Tool == "" {
		c.Tool = "mdsmith"
	}
	if c.BaselineTool == "" {
		c.BaselineTool = "mado"
	}
	if c.Tolerance == nil {
		d := 0.15
		c.Tolerance = &d
	}
	if len(c.Corpora) == 0 {
		c.Corpora = []string{"corpus_repo.json", "corpus_neutral.json"}
	}
}

// BenchCheck fails when the tool regressed relative to the baseline
// tool between two benchmark snapshots. For each corpus it compares the
// within-run tool/baseline median ratio in freshDir against the same
// ratio in baselineDir; a ratio that worsened beyond Tolerance is a
// regression. Comparing the ratio rather than the absolute time is what
// makes this the machine-independent "same code, same factor" signal:
// a slower runner or a grown corpus moves both tools together and does
// not trip it. A human-readable line per corpus is written to w.
func BenchCheck(w io.Writer, baselineDir, freshDir string, cfg BenchCheckConfig) error {
	cfg.applyDefaults()
	_, _ = fmt.Fprintf(w, "perf regression check: %s vs %s, tolerance %.0f%%\n",
		cfg.Tool, cfg.BaselineTool, *cfg.Tolerance*100)
	var regressed []string
	for _, corpus := range cfg.Corpora {
		base, err := ratioFor(filepath.Join(baselineDir, corpus), cfg.Tool, cfg.BaselineTool)
		if err != nil {
			return err
		}
		fresh, err := ratioFor(filepath.Join(freshDir, corpus), cfg.Tool, cfg.BaselineTool)
		if err != nil {
			return err
		}
		delta := (fresh - base) / base
		verdict := "ok"
		if fresh > base*(1+*cfg.Tolerance) {
			verdict = "REGRESSION"
			regressed = append(regressed, corpus)
		}
		_, _ = fmt.Fprintf(w, "  %-18s baseline %.2fx -> fresh %.2fx (%+.0f%%) %s\n",
			corpus, base, fresh, delta*100, verdict)
	}
	if len(regressed) > 0 {
		return fmt.Errorf("perf regression: %s slowed relative to %s beyond %.0f%% on %v",
			cfg.Tool, cfg.BaselineTool, *cfg.Tolerance*100, regressed)
	}
	_, _ = fmt.Fprintln(w, "no perf regression")
	return nil
}
