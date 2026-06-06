package integration

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/convention"
	"github.com/stretchr/testify/require"
)

// slopPatternsPath is the docs-author catalog that is the source of
// truth for the no-llm-tells convention's curated lists.
const slopPatternsPath = "../../.claude/skills/docs-author/slop-patterns.md"

// TestNoLLMTellsConventionMatchesSlopCatalog asserts that every entry
// in the no-llm-tells convention's forbidden lists still appears in
// the docs-author slop-patterns catalog. It fails CI when an item is
// removed from the catalog without being removed from the convention,
// or vice versa, so the parallel lists cannot silently diverge.
func TestNoLLMTellsConventionMatchesSlopCatalog(t *testing.T) {
	catalog := readSlopCatalog(t)

	conv, err := convention.Lookup("no-llm-tells", nil)
	require.NoError(t, err)

	// MDS056 contains: holds vocabulary tells followed by phrasal tells.
	contains := conventionStringList(t, conv, "forbidden-text", "contains")
	vocabulary := catalog["Vocabulary tells"]
	phrases := catalog["Phrasal tells"]
	for _, item := range contains {
		if vocabulary[item] || phrases[item] {
			continue
		}
		t.Errorf(
			"forbidden-text contains %q is not in slop-patterns.md "+
				"Vocabulary tells or Phrasal tells", item,
		)
	}

	// MDS055 starts: holds the banned sentence openers.
	starts := conventionStringList(t, conv, "forbidden-paragraph-starts", "starts")
	openers := catalog["Sentence openers"]
	for _, item := range starts {
		if openers[item] {
			continue
		}
		t.Errorf(
			"forbidden-paragraph-starts starts %q is not in "+
				"slop-patterns.md Sentence openers", item,
		)
	}
}

// conventionStringList returns the named list setting of the named rule
// from a convention, as a []string. It fails the test if the setting is
// missing or not a list of strings.
func conventionStringList(
	t *testing.T, conv convention.Convention, ruleName, key string,
) []string {
	t.Helper()
	preset, ok := conv.Rules[ruleName]
	require.True(t, ok, "convention must preset %s", ruleName)
	raw, ok := preset.Settings[key]
	require.True(t, ok, "%s must set %s", ruleName, key)
	list, ok := raw.([]any)
	require.True(t, ok, "%s.%s must be a list", ruleName, key)
	out := make([]string, 0, len(list))
	for _, v := range list {
		s, ok := v.(string)
		require.True(t, ok, "%s.%s entries must be strings", ruleName, key)
		out = append(out, s)
	}
	return out
}

// readSlopCatalog parses slop-patterns.md into a map from section
// heading ("Vocabulary tells", "Phrasal tells", "Sentence openers") to
// a set of normalized catalog items. Vocabulary bullets may list
// several comma-separated words and carry a "(figurative)"-style tag,
// which is stripped. Phrasal bullets are wrapped in double quotes,
// which are stripped. Sentence-opener bullets are taken verbatim
// (including the trailing comma).
func readSlopCatalog(t *testing.T) map[string]map[string]bool {
	t.Helper()
	path, err := filepath.Abs(slopPatternsPath)
	require.NoError(t, err)
	data, err := os.ReadFile(path) //nolint:gosec // fixed in-repo path
	require.NoError(t, err)

	want := map[string]bool{
		"Vocabulary tells": true,
		"Phrasal tells":    true,
		"Sentence openers": true,
	}
	out := map[string]map[string]bool{}
	for s := range want {
		out[s] = map[string]bool{}
	}

	var section string
	var bullet string
	flush := func() {
		if bullet == "" || !want[section] {
			bullet = ""
			return
		}
		item := strings.TrimSpace(strings.TrimPrefix(bullet, "- "))
		switch section {
		case "Vocabulary tells":
			for _, word := range strings.Split(item, ",") {
				out[section][normalizeVocab(word)] = true
			}
		case "Phrasal tells":
			out[section][strings.Trim(item, `"`)] = true
		case "Sentence openers":
			out[section][item] = true
		}
		bullet = ""
	}

	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "## ") {
			flush()
			section = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			continue
		}
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "- "):
			// New bullet: emit the previous one, start this one.
			flush()
			bullet = trimmed
		case trimmed == "":
			flush()
		case bullet != "":
			// Continuation of a wrapped bullet.
			bullet += " " + trimmed
		}
	}
	flush()
	require.NoError(t, sc.Err())
	return out
}

// normalizeVocab strips a parenthetical sense tag (e.g.
// "landscape (figurative)" -> "landscape") and surrounding whitespace
// from a vocabulary catalog word.
func normalizeVocab(word string) string {
	if i := strings.IndexByte(word, '('); i >= 0 {
		word = word[:i]
	}
	return strings.TrimSpace(word)
}
