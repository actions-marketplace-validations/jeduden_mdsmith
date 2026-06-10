package cuelitetest

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// These unit tests drive the surface-C harness's own branches — its
// rejection paths, its agreement/disagreement reporting, and its CUE-source
// reconstruction — to full coverage, independent of the corpus (which only
// exercises the agreement path).

func TestExprOutcome_Equal(t *testing.T) {
	assert.True(t, ExprOutcome{Accepted: true, Result: "x"}.Equal(ExprOutcome{Accepted: true, Result: "x"}))
	// Accept/reject mismatch differs.
	assert.False(t, ExprOutcome{Accepted: true}.Equal(ExprOutcome{}))
	// Same accept, different result differs.
	assert.False(t, ExprOutcome{Accepted: true, Result: "x"}.Equal(ExprOutcome{Accepted: true, Result: "y"}))
}

func TestCueLiteExprPath_RejectsBadScope(t *testing.T) {
	// A scope that is not a JSON object is rejected before evaluation.
	got := CueLiteExprPath(ExprCase{Expr: `"x"`, ScopeJSON: `[1,2]`})
	assert.False(t, got.Accepted)
}

func TestCueLiteExprPath_RejectsSyntaxError(t *testing.T) {
	got := CueLiteExprPath(ExprCase{Expr: `strings.Join([for x in`, ScopeJSON: ``})
	assert.False(t, got.Accepted)
}

func TestCueLiteExprPath_RejectsRenderError(t *testing.T) {
	got := CueLiteExprPath(ExprCase{Expr: `"\(missing)"`, ScopeJSON: `{}`})
	assert.False(t, got.Accepted)
}

func TestOracleExprPath_RejectsBadScope(t *testing.T) {
	got := OracleExprPath(ExprCase{Expr: `"x"`, ScopeJSON: `not json`})
	assert.False(t, got.Accepted)
}

func TestOracleExprPath_RejectsCompileError(t *testing.T) {
	got := OracleExprPath(ExprCase{Expr: `strings.Join([for x in`, ScopeJSON: ``})
	assert.False(t, got.Accepted)
}

func TestOracleExprPath_RejectsNonStringResult(t *testing.T) {
	got := OracleExprPath(ExprCase{Expr: `42`, ScopeJSON: ``})
	assert.False(t, got.Accepted)
}

func TestOracleSource_SkipsScaffoldingKeys(t *testing.T) {
	// A front-matter key colliding with the scaffolding (fm / out field) is
	// dropped from the emitted JSON, so it cannot shadow the renderer's own
	// fields. The expression still renders from the surviving key.
	got := OracleExprPath(ExprCase{
		Expr:      `"\(id)"`,
		ScopeJSON: `{"id":"A","fm":"shadow","mdsmith_template_out":"shadow"}`,
	})
	assert.True(t, got.Accepted)
	assert.Equal(t, "A", got.Result)
	// The in-house arm agrees.
	inHouse := CueLiteExprPath(ExprCase{
		Expr:      `"\(id)"`,
		ScopeJSON: `{"id":"A","fm":"shadow","mdsmith_template_out":"shadow"}`,
	})
	assert.True(t, inHouse.Equal(got))
}

func TestDecodeScope_RejectsTrailingContent(t *testing.T) {
	_, ok := decodeScope(`{"a":1} {"b":2}`)
	assert.False(t, ok)
}

func TestDecodeScope_RejectsInvalidJSON(t *testing.T) {
	_, ok := decodeScope(`{bad`)
	assert.False(t, ok)
}

func TestCompareExprOutcomes_AgreementReturnsTrue(t *testing.T) {
	r := &recorder{}
	ok := CompareExprOutcomes(r, CueLiteExprPath, OracleExprPath,
		ExprCase{Name: "lit", Expr: `"x"`, ScopeJSON: ``})
	assert.True(t, ok)
	assert.Empty(t, r.failures)
}

func TestCompareExprOutcomes_DisagreementReportsFailure(t *testing.T) {
	// Force a disagreement with two stub arms: one accepts, one rejects.
	accept := func(ExprCase) ExprOutcome { return ExprOutcome{Accepted: true, Result: "a"} }
	reject := func(ExprCase) ExprOutcome { return ExprOutcome{} }
	r := &recorder{}
	ok := CompareExprOutcomes(r, accept, reject, ExprCase{Name: "stub"})
	assert.False(t, ok)
	assert.Len(t, r.failures, 1)
}

// TestHatchedDivergence covers the two divergence-scoped tolerance classes and
// the cases each must NOT mask, so the hatch's signature stays tight.
func TestHatchedDivergence(t *testing.T) {
	// float-display: both accept, results differ only in an equal-ish numeric
	// substring.
	assert.True(t, HatchedDivergence(
		ExprOutcome{Accepted: true, Result: "w=1.5"},
		ExprOutcome{Accepted: true, Result: "w=1.50"}))
	// float-display must NOT mask a genuine value difference.
	assert.False(t, HatchedDivergence(
		ExprOutcome{Accepted: true, Result: "w=1.5"},
		ExprOutcome{Accepted: true, Result: "w=2.5"}))
	// float-display must NOT mask a non-numeric difference.
	assert.False(t, HatchedDivergence(
		ExprOutcome{Accepted: true, Result: "a1.5"},
		ExprOutcome{Accepted: true, Result: "b1.5"}))
	// unsupported-construct: in-house rejects with the "unsupported" wording,
	// oracle accepts.
	assert.True(t, HatchedDivergence(
		ExprOutcome{Reason: "cuelite: unsupported float arithmetic"},
		ExprOutcome{Accepted: true, Result: "0.3"}))
	// A genuine in-house parse/reference rejection (not "unsupported") is NOT
	// hatched even when the oracle accepts.
	assert.False(t, HatchedDivergence(
		ExprOutcome{Reason: `cuelite: reference "x" not found`},
		ExprOutcome{Accepted: true, Result: "y"}))
	// In-house accepts but oracle rejects: never hatched.
	assert.False(t, HatchedDivergence(
		ExprOutcome{Accepted: true, Result: "v"},
		ExprOutcome{Reason: "oracle rejected"}))
	// Both accept with identical results is not a divergence the hatch is asked
	// about, but floatDisplayDivergence returns true on equal strings; that is
	// harmless (the caller only consults the hatch after Equal already failed).
	assert.True(t, HatchedDivergence(
		ExprOutcome{Accepted: true, Result: "x"},
		ExprOutcome{Accepted: true, Result: "x"}))
}

// TestNumericallyEquivalent exercises leadingNumber's exponent, sign, and
// non-number branches and floatEqualish's tolerance.
func TestNumericallyEquivalent(t *testing.T) {
	assert.True(t, numericallyEquivalent("1.50", "1.5"))
	assert.True(t, numericallyEquivalent("a-b", "a-b"))       // hyphen is punctuation, not a sign
	assert.True(t, numericallyEquivalent("1e3", "1000"))      // exponent vs plain
	assert.True(t, numericallyEquivalent("-1.5", "-1.50"))    // leading sign
	assert.False(t, numericallyEquivalent("1.5", "1.5x"))     // trailing length differs
	assert.False(t, numericallyEquivalent("1.5", "9.9"))      // value differs
	assert.False(t, numericallyEquivalent("ab", "ba"))        // non-numeric byte differs
	assert.True(t, numericallyEquivalent("1.5e", "1.5e"))     // 'e' with no digits is literal text, matches verbatim
	assert.True(t, numericallyEquivalent("v1.5e+", "v1.5e+")) // dangling exponent text matches verbatim
}
