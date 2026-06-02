package checker_test

import (
	"errors"
	"testing"

	"github.com/jeduden/mdsmith/internal/checker"
	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// plainRule is a minimal non-NodeChecker rule for testing.
type plainRule struct{ id string }

func (r *plainRule) ID() string                           { return r.id }
func (r *plainRule) Name() string                         { return r.id }
func (r *plainRule) Category() string                     { return "test" }
func (r *plainRule) Check(_ *lint.File) []lint.Diagnostic { return nil }

// errConfigurable implements rule.Configurable and always fails ApplySettings.
type errConfigurable struct{ plainRule }

func (r *errConfigurable) ApplySettings(_ map[string]any) error {
	return errors.New("intentional settings error")
}
func (r *errConfigurable) DefaultSettings() map[string]any { return nil }

var _ rule.Configurable = (*errConfigurable)(nil)

func TestConfigureRule_settingsError(t *testing.T) {
	r := &errConfigurable{plainRule: plainRule{id: "TST001"}}
	_, err := checker.ConfigureRule(r, config.RuleCfg{Settings: map[string]any{"x": 1}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "intentional settings error")
}

func newTestFile(t *testing.T, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("doc.md", []byte(src))
	require.NoError(t, err)
	f.RootDir = "."
	f.RunCache = lint.NewRunCache()
	return f
}

func TestCheckRulesWithIntraFile_concurrent(t *testing.T) {
	f := newTestFile(t, "# Hello\n\nParagraph.\n")
	rules := []rule.Rule{
		&plainRule{id: "TST001"},
		&plainRule{id: "TST002"},
	}
	effective := map[string]config.RuleCfg{
		"TST001": {Enabled: true},
		"TST002": {Enabled: true},
	}
	// intraFileCap > 1 exercises the goroutine path in runNonNodeCheckers.
	diags, errs := checker.CheckRulesWithIntraFile(f, rules, effective, true, 2)
	assert.Empty(t, errs)
	assert.Empty(t, diags)
}

func TestCheckRulesWithIntraFile_settingsErrorPropagated(t *testing.T) {
	f := newTestFile(t, "# Hello\n")
	rules := []rule.Rule{&errConfigurable{plainRule: plainRule{id: "TST001"}}}
	effective := map[string]config.RuleCfg{
		"TST001": {Enabled: true, Settings: map[string]any{"x": 1}},
	}
	diags, errs := checker.CheckRulesWithIntraFile(f, rules, effective, true, 1)
	assert.Empty(t, diags)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "intentional settings error")
}

func TestPopulateSourceContext_outOfBoundLine(t *testing.T) {
	f := newTestFile(t, "# Hello\n")
	diags := []lint.Diagnostic{
		{Line: 0, RuleID: "TST001"}, // lineIdx = 0 - 0 - 1 = -1, below bounds
	}
	checker.PopulateSourceContext(f, diags, 2)
	// Out-of-bound lines must not populate SourceLines.
	assert.Nil(t, diags[0].SourceLines)
}

func TestFilterGeneratedDiags_empty(t *testing.T) {
	diags := []lint.Diagnostic{{Line: 5, RuleID: "TST001"}}
	out := checker.FilterGeneratedDiags(diags, nil)
	assert.Equal(t, diags, out)
}
