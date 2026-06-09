// Package difftest is the differential-testing harness behind the
// cue/cuelite façade. It runs one schema-plus-data case through two
// validation paths — the in-house path (the cuelite façade, the
// eventual pure-Go engine) and the CUE-backed oracle path (direct
// cuelang.org/go) — and reports whether the two agree on accept/reject
// and on the field path of the first rejection.
//
// Phase 0 (plan 236) has no in-house engine yet, so the in-house path
// is itself the CUE-backed cuelite façade. The two paths therefore
// agree by construction and the harness runs green in CI as an
// ordinary go test. The per-surface phases that flip cuelite to the
// in-house engine reuse this harness to prove the flip preserves
// behaviour against the oracle, so its shape is fixed now.
package difftest

import (
	"reflect"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/errors"

	"github.com/jeduden/mdsmith/cue/cuelite"
)

// Case is one differential-test input: a CUE schema source and a JSON
// data document to validate against it. Name labels the case in
// failure messages.
type Case struct {
	Name   string
	Schema string
	Data   string
}

// Outcome is the result of validating a Case through one path. Accepted
// reports whether the data satisfied the schema. When it did not, Path
// carries the field path of the first rejecting leaf, so the two paths
// can be compared not just on accept/reject but on where they reject.
type Outcome struct {
	Accepted bool
	Path     []string
}

// Equal reports whether two Outcomes agree on both acceptance and, when
// both reject, on the rejecting field path. Two accepting Outcomes are
// equal regardless of Path, since an accepting path has no rejection to
// locate.
func (o Outcome) Equal(other Outcome) bool {
	if o.Accepted != other.Accepted {
		return false
	}
	if o.Accepted {
		return true
	}
	return reflect.DeepEqual(o.Path, other.Path)
}

// Path is a validation strategy: it validates a Case and reports the
// Outcome. The in-house path and the oracle path are both Paths, so the
// harness can call either uniformly. A compile error on either the
// schema or the data counts as a rejection with a nil field path.
type Path func(c Case) Outcome

// CueLitePath validates a Case through the cue/cuelite façade — the
// in-house path. In phase 0 the façade still delegates to CUE; later
// phases flip it to the pure-Go engine without changing this function.
func CueLitePath(c Case) Outcome {
	schema, err := cuelite.Compile(c.Schema)
	if err != nil {
		return Outcome{Accepted: false}
	}
	data, err := cuelite.CompileJSON([]byte(c.Data))
	if err != nil {
		return Outcome{Accepted: false}
	}
	verr := schema.Unify(data).Validate()
	if verr == nil {
		return Outcome{Accepted: true}
	}
	// cuelite.Validate returns a non-nil error only as a *PathError, so
	// the rejection always carries a field path (possibly nil for an
	// error not tied to a leaf).
	return Outcome{Accepted: false, Path: verr.(*cuelite.PathError).Path()}
}

// OraclePath validates a Case directly through cuelang.org/go — the
// oracle the in-house path is measured against. It compiles the schema
// and data on one shared context, unifies them, and validates for
// concreteness, reading the rejecting field path from the first CUE
// error.
func OraclePath(c Case) Outcome {
	ctx := cuecontext.New()
	schema := ctx.CompileString(c.Schema)
	if schema.Err() != nil {
		return Outcome{Accepted: false}
	}
	data := ctx.CompileBytes([]byte(c.Data))
	if data.Err() != nil {
		return Outcome{Accepted: false}
	}
	verr := schema.Unify(data).Validate(cue.Concrete(true))
	if verr == nil {
		return Outcome{Accepted: true}
	}
	return Outcome{Accepted: false, Path: errors.Errors(verr)[0].Path()}
}

// TestingT is the slice of *testing.T the harness needs. Taking the
// interface rather than the concrete type keeps the harness unit-
// testable: a test can pass a recorder that captures failures instead
// of failing the real test run.
type TestingT interface {
	Helper()
	Errorf(format string, args ...any)
}

// Compare runs one Case through both inHouse and oracle and reports a
// failure on t when the two Outcomes disagree on acceptance or on the
// rejecting field path. It returns true when the paths agree.
func Compare(t TestingT, inHouse, oracle Path, c Case) bool {
	t.Helper()
	got := inHouse(c)
	want := oracle(c)
	if got.Equal(want) {
		return true
	}
	t.Errorf("case %q: in-house path %+v disagrees with oracle %+v", c.Name, got, want)
	return false
}

// Run compares every Case in cases through the in-house and oracle
// paths, reporting each disagreement on t. It is the entry point a
// phase's differential test calls over its corpus.
func Run(t TestingT, cases []Case) {
	t.Helper()
	for _, c := range cases {
		Compare(t, CueLitePath, OraclePath, c)
	}
}
