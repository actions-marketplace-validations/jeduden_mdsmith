package cuelitetest

// path.go — differential harness for surface D: ParsePath.
//
// This file adds a path-comparing arm to the cuelitetest harness. Surface
// D (placeholder paths) uses only ParsePath; the schema/data arms in the
// main harness do not cover it because a path-only case has no schema or
// data document, and appending it to corpus() would agree vacuously —
// empty schema and data classify identically in both arms regardless of
// the path expression.
//
// So surface D has its own:
//   - PathCase — one path expression to parse.
//   - PathOutcome — accepted-with-segments or rejected.
//   - PathPath — a parse strategy (in-house or oracle).
//   - RunPath — the CI-visible runner that compares both arms.
//
// The oracle arm calls cuelang.org/go/cue.ParsePath directly and
// reconstructs the unquoted-segment result the same way ParsePath does
// internally, so the two arms are independent implementations of the
// same contract.

import (
	"slices"
	"testing"

	"cuelang.org/go/cue"

	cuelitepkg "github.com/jeduden/mdsmith/cue/cuelite"
)

// PathCase is one differential-test input: a CUE path expression to
// parse. Name labels the case in failure messages.
type PathCase struct {
	Name string
	Expr string
}

// PathOutcome is the result of parsing a PathCase through one path arm.
// Accepted reports whether the expression parsed successfully. When
// Accepted is true, Segments holds the unquoted per-selector strings
// the parser produced; when false, Segments is nil. Comparing both
// Accepted and Segments ensures the two arms agree not only on
// accept/reject but on the exact decoded content they produce.
type PathOutcome struct {
	Accepted bool
	Segments []string
}

// Equal reports whether two PathOutcomes agree — the same accept/reject
// decision AND the same segment list. A nil Segments equals an empty
// Segments, consistent with how Path.Segments() behaves for a zero Path.
func (o PathOutcome) Equal(other PathOutcome) bool {
	if o.Accepted != other.Accepted {
		return false
	}
	return slices.Equal(o.Segments, other.Segments)
}

// PathPath is a path-parse strategy: it parses a PathCase and returns
// a PathOutcome. The in-house arm and the oracle arm are both PathPaths,
// so RunPath can call either uniformly.
type PathPath func(c PathCase) PathOutcome

// CueLitePathParsePath parses a PathCase through the cue/cuelite
// façade — the in-house path. ParsePath is now the pure-Go in-house
// implementation; this function is the stable binding that RunPath calls.
// A successful parse always yields at least one segment (ParsePath rejects
// zero-segment expressions), so Segments is always non-nil on acceptance.
func CueLitePathParsePath(c PathCase) PathOutcome {
	p, err := cuelitepkg.ParsePath(c.Expr)
	if err != nil {
		return PathOutcome{Accepted: false}
	}
	return PathOutcome{Accepted: true, Segments: p.Segments()}
}

// OraclePathParsePath parses a PathCase directly through
// cuelang.org/go/cue — the oracle the in-house path is measured
// against. It mirrors the in-house path stage for stage:
//
//   - An empty expression is rejected (the in-house path's explicit check).
//   - A cue.ParsePath error is a rejection.
//   - A selector whose Unquoted() panics (e.g. an index or definition
//     label) is treated as a rejected, empty-segment outcome, matching
//     the in-house path's empty-segment check.
//   - An empty Unquoted() string (e.g. the lone-surrogate `"\ud800"`)
//     is a rejection, matching the in-house path's empty-segment check.
//
// Note: cue.ParsePath returns zero selectors only for the empty string,
// which is caught by the empty-expression guard above. For any non-empty
// expression that passes the error check, at least one selector is
// present.
func OraclePathParsePath(c PathCase) PathOutcome {
	if c.Expr == "" {
		return PathOutcome{Accepted: false}
	}
	p := cue.ParsePath(c.Expr)
	if p.Err() != nil {
		return PathOutcome{Accepted: false}
	}
	sels := p.Selectors()
	segs := make([]string, len(sels))
	for i, s := range sels {
		u, panicked := safeUnquoted(s)
		if panicked || u == "" {
			return PathOutcome{Accepted: false}
		}
		segs[i] = u
	}
	return PathOutcome{Accepted: true, Segments: segs}
}

// safeUnquoted calls s.Unquoted() and recovers from the panic
// cuelang.org/go v0.16.1 raises when Unquoted is invoked on a
// non-string label (an index label like "123" or a definition label
// like "#foo"). It returns the unquoted string and whether a panic
// occurred, so the caller can treat the panic case as a rejection.
func safeUnquoted(s cue.Selector) (u string, panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			panicked = true
		}
	}()
	return s.Unquoted(), false
}

// ComparePathOutcomes runs one PathCase through both inHouse and oracle
// and reports a failure on t when the two PathOutcomes disagree. It
// returns true when they agree. The test name includes the expression so
// a failure message names the disagreeing input.
func ComparePathOutcomes(t testing.TB, inHouse, oracle PathPath, c PathCase) bool {
	t.Helper()
	got := inHouse(c)
	want := oracle(c)
	if got.Equal(want) {
		return true
	}
	t.Errorf("path case %q expr=%q: in-house %+v disagrees with oracle %+v",
		c.Name, c.Expr, got, want)
	return false
}

// RunPath compares every PathCase in cases through the in-house and
// oracle path arms, reporting each disagreement on t. It is the entry
// point a phase's differential path test calls over its corpus.
func RunPath(t testing.TB, cases []PathCase) {
	t.Helper()
	for _, c := range cases {
		ComparePathOutcomes(t, CueLitePathParsePath, OraclePathParsePath, c)
	}
}

// pathCorpus returns the representative set of path expressions for the
// differential path arm. It spans:
//   - accepted inputs: simple idents, dotted idents, quoted keys (with
//     dots, escaped quotes, unicode, numeric-looking content), mixed
//     ident+quoted;
//   - rejected inputs: empty string, leading/trailing dots, empty quoted
//     segment, malformed quote suffix, underscore-prefixed idents (CUE
//     hidden labels), hash-prefixed idents (CUE definition labels),
//     bare numeric idents (CUE index labels), whitespace-only, whitespace
//     mid-expression, unterminated quotes, and invalid escape sequences.
func pathCorpus() []PathCase {
	return []PathCase{
		// Accepted: simple identifiers.
		{Name: "simple ident", Expr: "title"},
		{Name: "ident with digits", Expr: "abc123"},
		{Name: "ident with underscore", Expr: "a_b"},
		{Name: "upper-case ident", Expr: "Title"},

		// Accepted: dotted identifiers.
		{Name: "dotted idents", Expr: "a.b.c"},
		{Name: "two-segment dotted", Expr: "params.subtitle"},

		// Accepted: quoted keys.
		{Name: "quoted key with hyphen", Expr: `"my-key"`},
		{Name: "quoted key with dot inside", Expr: `"a.b"`},
		{Name: "quoted key with escaped quote", Expr: `"a\"b"`},
		{Name: "quoted key with unicode", Expr: `"über"`},
		{Name: "numeric-looking quoted segment", Expr: `"123"`},

		// Accepted: mixed ident and quoted.
		{Name: "quoted then ident", Expr: `"my-key".sub`},
		{Name: "ident then quoted", Expr: `params."a.b"`},

		// Rejected: empty expression.
		{Name: "empty string", Expr: ""},

		// Rejected: leading/trailing dot.
		{Name: "trailing dot", Expr: "a."},
		{Name: "leading dot", Expr: ".a"},
		{Name: "quoted trailing dot", Expr: `"a".`},

		// Rejected: empty quoted segment (both arms reject empty unquoted).
		{Name: "empty quoted segment", Expr: `""`},

		// Rejected: malformed — text after closing quote without dot.
		{Name: "missing dot after quoted", Expr: `"a"b`},

		// Rejected: underscore-prefixed idents (CUE hidden labels).
		{Name: "underscore-prefixed ident", Expr: "_foo"},
		{Name: "double-underscore ident", Expr: "__"},

		// Rejected: hash-prefixed idents (CUE definition labels, Unquoted panics).
		{Name: "hash-prefixed ident", Expr: "#foo"},

		// Rejected: bare numeric — CUE index label, not a field name.
		{Name: "bare numeric segment", Expr: "123"},

		// Rejected: whitespace-only (both arms reject).
		{Name: "whitespace only", Expr: " "},

		// Rejected: whitespace mid-expression (both arms reject).
		{Name: "whitespace mid ident", Expr: "a b"},

		// Rejected: unterminated quoted segment.
		{Name: "unterminated quoted", Expr: `"unterminated`},

		// Rejected: invalid escape sequence (lone surrogate).
		{Name: "lone-surrogate escape", Expr: `"\ud800"`},
	}
}
