// Package cuelite is the public, versioned façade mdsmith uses for the
// small CUE subset it depends on — schema constraints, query filters,
// placeholder paths, and catalog templates. Like
// github.com/jeduden/mdsmith/pkg/markdown it is an exported public
// surface, imported under github.com/jeduden/mdsmith/cue/cuelite, and
// it sits at the bottom of the layering map: it imports no internal
// mdsmith package.
//
// # Strategy: CUE-backed façade, then in-house flip
//
// The package lands first as a thin wrapper over cuelang.org/go. Every
// method delegates to CUE, so mdsmith call sites can move onto the
// façade with behaviour unchanged. Only afterward is the implementation
// flipped, method by method, to a small in-house pure-Go engine behind
// this same stable API. Throughout that flip the CUE-backed path stays
// available as the differential oracle: the harness in
// internal/cuelitetest runs a value through both the in-house path and
// the CUE-backed path and asserts identical accept/reject outcomes and
// identical error field paths. Until a method is flipped, both paths
// are the same CUE-backed code, so the harness is a green scaffold the
// later phases extend.
//
// Phase 0 (plan 236) exposes only the minimal surface the delegation
// pattern, the differential harness, and the benchmark need: Compile,
// CompileJSON, and the Value methods Unify and Validate, plus the
// path-tagged PathError. The per-surface façade methods (ParsePath,
// LookupPath, String, Decode, Exists, Fields, …) arrive in the phases
// that migrate each call site.
//
// # Value isolation and the interim context cost
//
// A cue.Value is tied to the *cue.Context it was built in, and CUE
// v0.16.1 documents that values created from the same Context are not
// safe for concurrent use, and that long-lived contexts can grow
// unbounded. So each Compile/CompileJSON builds its own *cue.Context
// and keeps the original source alongside the compiled value. Unify
// rebuilds the operand's source inside the receiver's context, so
// unification stays single-context and two distinct Values never share
// mutable CUE state. This is the honest interim cost of the CUE-backed
// phase: one context per compiled Value, and one re-compile of the
// operand per Unify. The in-house engine of plan 218 erases both — a
// flipped Value is a context-free immutable struct — without changing
// this API: Value stays a value type whose Unify takes and returns a
// Value, so a bottom (⊥) absorbs cleanly in either implementation.
package cuelite

import (
	stderrors "errors"
	"fmt"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/errors"
	cuejson "cuelang.org/go/encoding/json"
)

// srcKind distinguishes how a Value's source must be rebuilt inside
// another Value's context during Unify: as CUE source or as strict
// JSON data.
type srcKind int

const (
	srcCUE srcKind = iota
	srcJSON
)

// Value is an immutable compiled CUE value behind the cuelite façade.
// It is a value type: methods take and return Value by copy, so a
// zero/bottom Value composes without a nil receiver. In the phase-0
// CUE-backed implementation it carries its own *cue.Context, the
// compiled cue.Value, and the original source, so Unify can rebuild an
// operand inside the receiver's context. A Value carrying err is a
// bottom (⊥): Unify with it yields a bottom, and Validate returns the
// carried error. Once the implementation is flipped to the in-house
// engine a Value becomes a context-free struct; this API does not
// change.
type Value struct {
	ctx  *cue.Context
	val  cue.Value
	kind srcKind
	src  []byte
	err  error
}

// bottom builds an error-carrying Value. Unify treats it as ⊥
// (absorbing), and Validate returns its error, so a compile failure or
// a bottom operand propagates through a Unify chain without panicking.
func bottom(err error) Value {
	return Value{err: err}
}

// Compile compiles a CUE source string into a Value. It is the façade
// over cue.Context.CompileString. A syntactically invalid source or a
// bottom value reports an error. The returned Value owns a fresh
// *cue.Context.
func Compile(src string) (Value, error) {
	ctx := cuecontext.New()
	val := ctx.CompileString(src)
	if err := val.Err(); err != nil {
		return bottom(err), err
	}
	return Value{ctx: ctx, val: val, kind: srcCUE, src: []byte(src)}, nil
}

// CompileJSON compiles a JSON document into a Value. It is the façade
// over the JSON-data lift mdsmith uses to validate marshalled front
// matter against a schema. The input must be strict JSON: arbitrary
// CUE source (an unquoted key, an expression) is rejected, unlike a
// raw CompileBytes. The returned Value owns a fresh *cue.Context.
func CompileJSON(data []byte) (Value, error) {
	ctx := cuecontext.New()
	val, err := buildJSON(ctx, data)
	if err != nil {
		return bottom(err), err
	}
	return Value{ctx: ctx, val: val, kind: srcJSON, src: append([]byte(nil), data...)}, nil
}

// buildJSON parses strict JSON into a cue.Value inside ctx. It rejects
// any input that is not valid JSON (so CUE source cannot slip through):
// the only failure path is the JSON parse, which Extract reports before
// any value is built. A built value that is itself a bottom (⊥) is
// returned as-is so Validate can surface it; this matches CompileString,
// which also returns bottoms rather than Go errors.
func buildJSON(ctx *cue.Context, data []byte) (cue.Value, error) {
	expr, err := cuejson.Extract("", data)
	if err != nil {
		return cue.Value{}, err
	}
	return ctx.BuildExpr(expr), nil
}

// rebuild reconstructs v's source as a cue.Value inside ctx, so an
// operand compiled in its own context can be unified inside another
// Value's context. v's source already compiled cleanly when v was
// built, so re-deriving it is total: a JSON source re-extracts and a CUE
// source re-compiles to the same value, only bound to ctx instead.
func (v Value) rebuild(ctx *cue.Context) cue.Value {
	if v.kind == srcJSON {
		// v was built by CompileJSON, so its bytes already passed strict
		// JSON extraction; re-extraction into ctx cannot newly fail.
		val, _ := buildJSON(ctx, v.src)
		return val
	}
	return ctx.CompileString(string(v.src))
}

// Unify returns the meet of v and o — the value satisfying both. It is
// the façade over cue.Value.Unify. A bottom (⊥) operand absorbs: if
// either v or o carries an error, the result carries it, so a compile
// failure flows through a Unify chain instead of panicking. Otherwise
// o's source is rebuilt inside v's context and the two are unified
// there, keeping the result single-context.
func (v Value) Unify(o Value) Value {
	if v.err != nil {
		return v
	}
	if o.err != nil {
		return o
	}
	merged := v.val.Unify(o.rebuild(v.ctx))
	// The merged value carries v's context. A re-Unify of the result is
	// not part of the phase-0 surface; the result is consumed only by
	// Validate, which reads merged.val, so no source is retained.
	return Value{ctx: v.ctx, val: merged, kind: srcCUE}
}

// Validate reports whether the value is concrete and free of conflicts,
// mirroring cue.Value.Validate(cue.Concrete(true)). A value carrying a
// compile-time or bottom error returns that error. On a validation
// failure it returns one *PathError per offending leaf, tagged with
// that leaf's field path, joined with errors.Join — so callers (the
// internal/schema validator emits one MDS020 diagnostic per failing
// field) and the differential harness see every rejection, not only the
// first. A value that satisfies the schema returns nil.
func (v Value) Validate() error {
	if v.err != nil {
		return v.err
	}
	verr := v.val.Validate(cue.Concrete(true))
	if verr == nil {
		return nil
	}
	leaves := errors.Errors(verr)
	// errors.Errors returns a non-empty list for any non-nil CUE
	// validation error, so leaves is never empty here. The
	// internal/schema validator relies on the same invariant.
	if len(leaves) == 1 {
		return pathErrorOf(leaves[0])
	}
	joined := make([]error, len(leaves))
	for i, leaf := range leaves {
		joined[i] = pathErrorOf(leaf)
	}
	return stderrors.Join(joined...)
}

// pathErrorOf converts a single CUE error into a *PathError. It uses
// the CUE error's path-free message (Msg, not Error, which prepends the
// dotted path) so PathError.Error() prints the path exactly once.
func pathErrorOf(e errors.Error) *PathError {
	format, args := e.Msg()
	return newPathError(e.Path(), fmt.Sprintf(format, args...))
}
