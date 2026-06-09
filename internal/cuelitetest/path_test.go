package cuelitetest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPathOutcome_Equal pins the PathOutcome equality contract.
func TestPathOutcome_Equal(t *testing.T) {
	t.Run("both rejected are equal", func(t *testing.T) {
		assert.True(t, PathOutcome{Accepted: false}.Equal(PathOutcome{Accepted: false}))
	})
	t.Run("accepted vs rejected differs", func(t *testing.T) {
		assert.False(t,
			PathOutcome{Accepted: true, Segments: []string{"a"}}.Equal(PathOutcome{Accepted: false}))
	})
	t.Run("same segments are equal", func(t *testing.T) {
		a := PathOutcome{Accepted: true, Segments: []string{"x", "y"}}
		b := PathOutcome{Accepted: true, Segments: []string{"x", "y"}}
		assert.True(t, a.Equal(b))
	})
	t.Run("different segments differ", func(t *testing.T) {
		a := PathOutcome{Accepted: true, Segments: []string{"x"}}
		b := PathOutcome{Accepted: true, Segments: []string{"y"}}
		assert.False(t, a.Equal(b))
	})
	t.Run("nil segments equals empty segments", func(t *testing.T) {
		// A zero PathOutcome has nil Segments; slices.Equal(nil, []string{})
		// returns true, matching the Path.Segments() nil-for-empty contract.
		a := PathOutcome{Accepted: true, Segments: nil}
		b := PathOutcome{Accepted: true, Segments: []string{}}
		assert.True(t, a.Equal(b))
	})
	t.Run("dropping a segment differs", func(t *testing.T) {
		// The key invariant: an arm that gets the first segment right but
		// drops a later one must not look equal.
		a := PathOutcome{Accepted: true, Segments: []string{"a", "b"}}
		b := PathOutcome{Accepted: true, Segments: []string{"a"}}
		assert.False(t, a.Equal(b))
	})
}

// TestCueLitePathParsePath pins the in-house ParsePath arm directly,
// so its accepted/rejected edges are visible outside the corpus run.
func TestCueLitePathParsePath(t *testing.T) {
	t.Run("accepted ident returns segments", func(t *testing.T) {
		o := CueLitePathParsePath(PathCase{Expr: "a.b"})
		require.True(t, o.Accepted)
		assert.Equal(t, []string{"a", "b"}, o.Segments)
	})
	t.Run("rejected returns not-accepted", func(t *testing.T) {
		o := CueLitePathParsePath(PathCase{Expr: ""})
		assert.False(t, o.Accepted)
		assert.Nil(t, o.Segments)
	})
}

// TestOraclePathParsePath pins the oracle arm's accepted/rejected edges
// and its safeUnquoted panic recovery, so each oracle branch has a
// dedicated unit test apart from the corpus run.
func TestOraclePathParsePath(t *testing.T) {
	t.Run("accepted ident returns segments", func(t *testing.T) {
		o := OraclePathParsePath(PathCase{Expr: "a.b"})
		require.True(t, o.Accepted)
		assert.Equal(t, []string{"a", "b"}, o.Segments)
	})
	t.Run("empty expression is rejected", func(t *testing.T) {
		o := OraclePathParsePath(PathCase{Expr: ""})
		assert.False(t, o.Accepted)
	})
	t.Run("parse error is rejected", func(t *testing.T) {
		o := OraclePathParsePath(PathCase{Expr: "a."})
		assert.False(t, o.Accepted)
	})
	t.Run("hash-prefixed ident safeUnquoted panic is rejected", func(t *testing.T) {
		// "#foo" parses without error in CUE (DefinitionLabel) but
		// Unquoted() panics; safeUnquoted must recover the panic and
		// OraclePathParsePath must report it as rejected.
		o := OraclePathParsePath(PathCase{Expr: "#foo"})
		assert.False(t, o.Accepted)
	})
	t.Run("numeric ident safeUnquoted panic is rejected", func(t *testing.T) {
		// "123" parses without error in CUE (IndexLabel) but Unquoted()
		// panics; same recovery.
		o := OraclePathParsePath(PathCase{Expr: "123"})
		assert.False(t, o.Accepted)
	})
	t.Run("empty unquoted segment is rejected", func(t *testing.T) {
		// `"\ud800"` parses without error but Unquoted() returns "".
		o := OraclePathParsePath(PathCase{Expr: `"\ud800"`})
		assert.False(t, o.Accepted)
	})
}

// TestSafeUnquoted drives the panic-recovery seam via the oracle arm, so
// both the panic path (covered by TestOraclePathParsePath's hash and
// numeric cases) and the non-panic path are exercised with a dedicated
// named test.
func TestSafeUnquoted(t *testing.T) {
	t.Run("string label does not panic", func(t *testing.T) {
		// "a" is a plain ident — Unquoted() returns "a" without panicking.
		// Use the oracle directly; it calls safeUnquoted for each selector.
		o := OraclePathParsePath(PathCase{Expr: "a"})
		require.True(t, o.Accepted)
		assert.Equal(t, []string{"a"}, o.Segments)
	})
}

// TestComparePathOutcomes pins the Compare helper: agreement returns
// true with no failure; disagreement returns false with a recorded failure.
func TestComparePathOutcomes(t *testing.T) {
	t.Run("agreement records no failure", func(t *testing.T) {
		r := &recorder{}
		ok := ComparePathOutcomes(r, CueLitePathParsePath, OraclePathParsePath,
			PathCase{Name: "agree", Expr: "a.b"})
		assert.True(t, ok)
		assert.Empty(t, r.failures)
		assert.Positive(t, r.helperCalls)
	})
	t.Run("disagreement records a failure", func(t *testing.T) {
		r := &recorder{}
		accept := func(PathCase) PathOutcome {
			return PathOutcome{Accepted: true, Segments: []string{"a"}}
		}
		reject := func(PathCase) PathOutcome { return PathOutcome{Accepted: false} }
		ok := ComparePathOutcomes(r, accept, reject, PathCase{Name: "mismatch", Expr: "a"})
		assert.False(t, ok)
		require.Len(t, r.failures, 1)
		assert.Contains(t, r.failures[0], `path case "mismatch"`)
	})
}

// TestRunPath_corpus is the CI-visible differential run: the in-house
// ParsePath and the CUE-backed oracle must agree on every case in the
// representative path corpus.
func TestRunPath_corpus(t *testing.T) {
	RunPath(t, pathCorpus())
}
