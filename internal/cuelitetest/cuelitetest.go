// Package cuelitetest is the differential-testing harness behind the
// cue/cuelite façade. It runs one schema-plus-data case through two
// validation paths — the in-house path (the cuelite façade, the
// eventual pure-Go engine) and the CUE-backed oracle path (direct
// cuelang.org/go) — and reports whether the two agree on accept/reject,
// on the stage at which a rejection happened, and on the field path of
// the rejection.
//
// It is module-internal on purpose. The harness imports cuelang.org/go
// from non-test files, which is the dependency plan 218 phase 4 deletes;
// living under internal/ keeps that import invisible outside the module
// (no external user can take a dependency on a package slated for
// deletion) and lets every package in the module reuse the harness in
// its own differential tests.
//
// Phase 0 (plan 236) has no in-house engine yet, so the in-house path is
// itself the CUE-backed cuelite façade. The two paths therefore agree by
// construction and the harness runs green in CI as an ordinary go test.
// The per-surface phases that flip cuelite to the in-house engine reuse
// this harness to prove the flip preserves behaviour against the oracle.
// The Case/Outcome shape is stable but not frozen: surface D (ParsePath)
// adds a path-parse case and surface C (row-expression → string) adds an
// evaluation outcome, so later phases extend these types rather than
// treating them as final.
package cuelitetest

import (
	stderrors "errors"
	"slices"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/errors"
	cuejson "cuelang.org/go/encoding/json"

	"github.com/jeduden/mdsmith/cue/cuelite"
)

// Stage names the point at which a path resolved a Case: where a
// rejection happened, or that the data was accepted. It keeps a parse
// failure (a schema or data the engine could not even compile) from
// masquerading as agreement with an oracle that merely rejected
// conforming-but-failing data — the two are different bugs.
type Stage int

const (
	// StageAccepted means the data satisfied the schema.
	StageAccepted Stage = iota
	// StageCompileSchema means the schema source failed to compile.
	StageCompileSchema
	// StageCompileData means the data document failed to compile.
	StageCompileData
	// StageValidate means schema and data compiled but the data did not
	// satisfy the schema.
	StageValidate
	// StageError means a path returned an error of an unexpected shape —
	// for the cuelite path, a validation error that was not a *PathError.
	// It exists so the harness can record the misbehaviour as a diff
	// instead of panicking.
	StageError
)

// Case is one differential-test input: a CUE schema source and a JSON
// data document to validate against it. Name labels the case in failure
// messages.
type Case struct {
	Name   string
	Schema string
	Data   string
}

// Outcome is the result of validating a Case through one path. Stage
// records where the case resolved. When it resolved at StageValidate,
// Path carries the field path of the rejecting leaf, so the two paths
// can be compared not just on accept/reject but on where they reject.
type Outcome struct {
	Stage Stage
	Path  []string
}

// Accepted reports whether the data satisfied the schema.
func (o Outcome) Accepted() bool { return o.Stage == StageAccepted }

// Equal reports whether two Outcomes agree on the resolution stage and,
// when both rejected at validation, on the rejecting field path. Two
// outcomes that resolved at the same non-validate stage are equal
// regardless of Path, since only a validation rejection locates a leaf.
func (o Outcome) Equal(other Outcome) bool {
	if o.Stage != other.Stage {
		return false
	}
	if o.Stage != StageValidate {
		return true
	}
	return slices.Equal(o.Path, other.Path)
}

// Path is a validation strategy: it validates a Case and reports the
// Outcome. The in-house path and the oracle path are both Paths, so the
// harness can call either uniformly.
type Path func(c Case) Outcome

// CueLitePath validates a Case through the cue/cuelite façade — the
// in-house path. In phase 0 the façade still delegates to CUE; later
// phases flip it to the pure-Go engine without changing this function.
func CueLitePath(c Case) Outcome {
	schema, err := cuelite.Compile(c.Schema)
	if err != nil {
		return Outcome{Stage: StageCompileSchema}
	}
	data, err := cuelite.CompileJSON([]byte(c.Data))
	if err != nil {
		return Outcome{Stage: StageCompileData}
	}
	return validateOutcome(schema.Unify(data).Validate())
}

// validateOutcome maps the result of cuelite.Validate to an Outcome.
// cuelite.Validate documents that it returns a *PathError (bare or
// joined), so errors.As recovers the field path. A nil error is an
// acceptance; any other error shape — which a future engine bug could
// produce — becomes StageError so the harness reports a diff rather than
// panicking on a failed type assertion.
func validateOutcome(verr error) Outcome {
	if verr == nil {
		return Outcome{Stage: StageAccepted}
	}
	var pe *cuelite.PathError
	if !stderrors.As(verr, &pe) {
		return Outcome{Stage: StageError}
	}
	return Outcome{Stage: StageValidate, Path: pe.Path()}
}

// OraclePath validates a Case directly through cuelang.org/go — the
// oracle the in-house path is measured against. It mirrors the cuelite
// path stage for stage: a per-call context (matching cuelite's per-Value
// context), strict JSON extraction for the data arm, and the first CUE
// error's field path on a validation rejection.
func OraclePath(c Case) Outcome {
	ctx := cuecontext.New()
	schema := ctx.CompileString(c.Schema)
	if schema.Err() != nil {
		return Outcome{Stage: StageCompileSchema}
	}
	data := oracleData(ctx, c.Data)
	if data.Err() != nil {
		return Outcome{Stage: StageCompileData}
	}
	verr := schema.Unify(data).Validate(cue.Concrete(true))
	if verr == nil {
		return Outcome{Stage: StageAccepted}
	}
	return Outcome{Stage: StageValidate, Path: errors.Errors(verr)[0].Path()}
}

// oracleData lifts the data document into ctx the same way cuelite's
// CompileJSON does — strict JSON extraction, not CompileBytes — so the
// oracle rejects non-JSON data exactly where the cuelite path does.
func oracleData(ctx *cue.Context, data string) cue.Value {
	expr, err := cuejson.Extract("", []byte(data))
	if err != nil {
		return ctx.CompileString("_|_") // a bottom whose Err() is non-nil
	}
	return ctx.BuildExpr(expr)
}

// Compare runs one Case through both inHouse and oracle and reports a
// failure on t when the two Outcomes disagree. It returns true when they
// agree.
func Compare(t testing.TB, inHouse, oracle Path, c Case) bool {
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
func Run(t testing.TB, cases []Case) {
	t.Helper()
	for _, c := range cases {
		Compare(t, CueLitePath, OraclePath, c)
	}
}
