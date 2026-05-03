package nounusedlinkdefinitions

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newFile(t *testing.T, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	return f
}

func TestRuleMetadata(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "MDS053", r.ID())
	assert.Equal(t, "no-unused-link-definitions", r.Name())
	assert.Equal(t, "link", r.Category())
	assert.True(t, r.EnabledByDefault())
}

func TestRuleInterfaces(t *testing.T) {
	r := &Rule{}
	var _ rule.Rule = r
	var _ rule.FixableRule = r
	var _ rule.Configurable = r
	var _ rule.Defaultable = r
	var _ rule.ListMerger = r
}

func TestSettingMergeMode(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, rule.MergeReplace, r.SettingMergeMode("ignored-labels"))
	assert.Equal(t, rule.MergeReplace, r.SettingMergeMode("unknown"))
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	assert.Equal(t, []string{}, ds["ignored-labels"])
}

// --- Check tests ---

func TestCheck_UsedDefinition_NoDiagnostic(t *testing.T) {
	f := newFile(t, "See [example][ex].\n\n[ex]: https://example.com\n")
	r := &Rule{}
	assert.Empty(t, r.Check(f))
}

func TestCheck_UsedCollapsedReference_NoDiagnostic(t *testing.T) {
	f := newFile(t, "See [ex][].\n\n[ex]: https://example.com\n")
	r := &Rule{}
	assert.Empty(t, r.Check(f))
}

func TestCheck_UsedShortcutReference_NoDiagnostic(t *testing.T) {
	f := newFile(t, "See [ex].\n\n[ex]: https://example.com\n")
	r := &Rule{}
	assert.Empty(t, r.Check(f))
}

func TestCheck_UsedImage_NoDiagnostic(t *testing.T) {
	f := newFile(t, "![alt][img]\n\n[img]: https://example.com/img.png\n")
	r := &Rule{}
	assert.Empty(t, r.Check(f))
}

func TestCheck_UnusedDefinition_Diagnostic(t *testing.T) {
	f := newFile(t, "# Heading\n\nSome text.\n\n[orphan]: https://example.com\n")
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, 5, diags[0].Line)
	assert.Equal(t, 1, diags[0].Column)
	assert.Equal(t, `unused link reference definition "orphan"`, diags[0].Message)
	assert.Equal(t, "MDS053", diags[0].RuleID)
}

func TestCheck_DuplicateDefinition_FlagsSecond(t *testing.T) {
	src := "# Heading\n\nSee [foo].\n\n[foo]: https://first.com\n\n[foo]: https://second.com\n"
	f := newFile(t, src)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, 7, diags[0].Line)
	assert.Contains(t, diags[0].Message, "duplicate link reference definition")
	assert.Contains(t, diags[0].Message, "first defined on line 5")
}

func TestCheck_DuplicateAndUnused_TwoDiagnostics(t *testing.T) {
	src := "# Heading\n\n[foo]: https://first.com\n\n[foo]: https://second.com\n"
	f := newFile(t, src)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 2)
	// First definition is unused
	assert.Contains(t, diags[0].Message, "unused link reference definition")
	// Second definition is duplicate
	assert.Contains(t, diags[1].Message, "duplicate link reference definition")
}

func TestCheck_CaseFoldNormalization_NoDiagnostic(t *testing.T) {
	// [Foo Bar] definition is consumed by [x][foo bar] (case-insensitive match)
	f := newFile(t, "See [x][foo bar].\n\n[Foo Bar]: https://example.com\n")
	r := &Rule{}
	assert.Empty(t, r.Check(f))
}

func TestCheck_WhitespaceNormalization_NoDiagnostic(t *testing.T) {
	// Multi-word label with internal spaces normalizes to same key
	f := newFile(t, "See [foo  bar].\n\n[foo bar]: https://example.com\n")
	r := &Rule{}
	assert.Empty(t, r.Check(f))
}

func TestCheck_IgnoredLabel_NoDiagnostic(t *testing.T) {
	f := newFile(t, "# Heading\n\nSome text.\n\n[kept]: https://example.com\n")
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{
		"ignored-labels": []any{"kept"},
	}))
	assert.Empty(t, r.Check(f))
}

func TestCheck_IgnoredLabel_CaseInsensitive(t *testing.T) {
	f := newFile(t, "# Heading\n\nSome text.\n\n[Kept]: https://example.com\n")
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{
		"ignored-labels": []any{"kept"},
	}))
	assert.Empty(t, r.Check(f))
}

func TestCheck_DefinitionInCodeBlock_NoDiagnostic(t *testing.T) {
	src := "# Heading\n\n```\n[orphan]: https://example.com\n```\n"
	f := newFile(t, src)
	r := &Rule{}
	assert.Empty(t, r.Check(f))
}

func TestCheck_NoDefinitions_NoDiagnostic(t *testing.T) {
	f := newFile(t, "# Heading\n\nSome text.\n")
	r := &Rule{}
	assert.Empty(t, r.Check(f))
}

func TestCheck_MultipleUnused(t *testing.T) {
	src := "# Heading\n\n[a]: https://a.com\n[b]: https://b.com\n"
	f := newFile(t, src)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 2)
}

// --- Fix tests ---

func TestFix_UnusedDefinition_Removed(t *testing.T) {
	src := "# Heading\n\nSome text.\n\n[orphan]: https://example.com\n"
	f := newFile(t, src)
	r := &Rule{}
	got := string(r.Fix(f))
	assert.Equal(t, "# Heading\n\nSome text.\n", got)
}

func TestFix_UsedDefinition_Preserved(t *testing.T) {
	src := "See [example][ex].\n\n[ex]: https://example.com\n"
	f := newFile(t, src)
	r := &Rule{}
	got := string(r.Fix(f))
	assert.Equal(t, src, got)
}

func TestFix_DuplicateDefinition_RemovesSecond(t *testing.T) {
	src := "See [foo].\n\n[foo]: https://first.com\n\n[foo]: https://second.com\n"
	f := newFile(t, src)
	r := &Rule{}
	got := string(r.Fix(f))
	assert.Equal(t, "See [foo].\n\n[foo]: https://first.com\n", got)
}

func TestFix_UnusedBetweenBlanks_CollapsesBlanks(t *testing.T) {
	// Blank line before and after the definition: fix should not leave
	// a double-blank behind.
	src := "# Heading\n\nSome text.\n\n[orphan]: https://example.com\n\nMore text.\n"
	f := newFile(t, src)
	r := &Rule{}
	got := string(r.Fix(f))
	assert.Equal(t, "# Heading\n\nSome text.\n\nMore text.\n", got)
}

func TestFix_IgnoredLabel_Preserved(t *testing.T) {
	src := "# Heading\n\nSome text.\n\n[kept]: https://example.com\n"
	f := newFile(t, src)
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{
		"ignored-labels": []any{"kept"},
	}))
	got := string(r.Fix(f))
	assert.Equal(t, src, got)
}

func TestFix_NoDefs_Unchanged(t *testing.T) {
	src := "# Heading\n\nSome text.\n"
	f := newFile(t, src)
	r := &Rule{}
	got := string(r.Fix(f))
	assert.Equal(t, src, got)
}

// --- ApplySettings tests ---

func TestApplySettings_SliceAny(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{
		"ignored-labels": []any{"foo", "BAR"},
	}))
	assert.True(t, r.ignoredLabels["foo"])
	assert.True(t, r.ignoredLabels["bar"])
}

func TestApplySettings_SliceString(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{
		"ignored-labels": []string{"Baz"},
	}))
	assert.True(t, r.ignoredLabels["baz"])
}

func TestApplySettings_Empty(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{}))
	assert.Empty(t, r.ignoredLabels)
}
