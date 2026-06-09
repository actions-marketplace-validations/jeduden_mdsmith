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
// CompileJSON, the Value methods Unify and Validate, the package-level
// Errors accessor, and the path-tagged PathError. The per-surface
// façade methods (ParsePath, LookupPath, String, Decode, Exists,
// Fields, …) arrive in the phases that migrate each call site.
//
// # Value isolation and the interim context cost
//
// A cue.Value is tied to the *cue.Context it was built in, and CUE
// v0.16.1 documents that values created from the same Context are not
// safe for concurrent use, and that long-lived contexts can grow
// unbounded. So each Compile/CompileJSON builds its own *cue.Context.
// Independently compiled roots are therefore isolated from one another.
// A DERIVED value — the result of Unify — shares the receiver root's
// context, so it is NOT isolated from that root and must not be used
// concurrently with it; under CUE v0.16.1 that concurrency disclaimer
// applies to every value drawn from the same context. Unify keeps a
// single result context by reusing an operand's value when it already
// belongs to the receiver's context and otherwise re-compiling the
// operand's retained source there. This is the honest interim cost of
// the CUE-backed phase: one context per compiled root, and at most one
// re-compile of a cross-context operand per Unify. The in-house engine
// of plan 218 erases both — a flipped Value is a context-free immutable
// struct shareable across goroutines — without changing this API: Value
// stays a value type whose Unify takes and returns a Value, so a bottom
// (⊥) absorbs cleanly in either implementation.
package cuelite

import (
	stderrors "errors"
	"fmt"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/errors"
	cuejson "cuelang.org/go/encoding/json"
)

// Value is an immutable compiled CUE value behind the cuelite façade.
// It is a value type: methods take and return Value by copy, so a
// zero/bottom Value composes without a nil receiver. A Value compiled
// by Compile or CompileJSON carries the compiled cue.Value and its
// original source, so Unify can rebuild it inside another Value's
// context when the two come from different roots. A Value carrying err
// is a bottom (⊥): Unify with it yields a bottom, and Validate returns
// the carried error. The zero Value (no compiled cue.Value, no source,
// no error) is also a bottom: Validate reports it and Unify absorbs it,
// so a caller that forgot to compile never triggers a nil-context
// panic. Once the implementation is flipped to the in-house engine a
// Value becomes a context-free struct; this API does not change.
type Value struct {
	val cue.Value
	src []byte
	err error
}

// errZeroValue is the bottom reason for a zero Value — one that was
// never compiled, so it has neither a usable cue.Value nor source to
// rebuild from. Unify absorbs it and Validate returns it.
var errZeroValue = stderrors.New("uninitialized cuelite.Value")

// bottom builds an error-carrying Value. Unify treats it as ⊥
// (absorbing), and Validate returns its error, so a compile failure or
// a bottom operand propagates through a Unify chain without panicking.
func bottom(err error) Value {
	return Value{err: err}
}

// isBottom reports whether v is a bottom: either it carries an explicit
// error, or it is the zero Value (never compiled, so its cue.Value does
// not exist). It returns the reason to surface so the bottom propagates
// with a message rather than a nil-context panic.
func (v Value) isBottom() (error, bool) {
	if v.err != nil {
		return v.err, true
	}
	if !v.val.Exists() {
		return errZeroValue, true
	}
	return nil, false
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
	return Value{val: val, src: []byte(src)}, nil
}

// CompileJSON compiles a JSON document into a Value. It is the façade
// over the JSON-data lift mdsmith uses to validate marshalled front
// matter against a schema. The input must be strict JSON: arbitrary
// CUE source (an unquoted key, an expression) is rejected, unlike a
// raw CompileBytes. A document that parses as JSON but builds to a
// bottom — a duplicate key, say — also reports an error, matching
// Compile, which surfaces bottoms rather than silently accepting them.
// The returned Value owns a fresh *cue.Context.
func CompileJSON(data []byte) (Value, error) {
	ctx := cuecontext.New()
	val, err := buildJSON(ctx, data)
	if err != nil {
		return bottom(err), err
	}
	return Value{val: val, src: append([]byte(nil), data...)}, nil
}

// buildJSON parses strict JSON into a cue.Value inside ctx. It rejects
// any input that is not valid JSON (so CUE source cannot slip through):
// Extract reports a malformed document before any value is built. A
// document that extracts but builds to a bottom (⊥) — a duplicate key,
// for instance — is returned as that bottom's Err(), so CompileJSON
// surfaces it as a Go error exactly as Compile surfaces a CUE bottom.
func buildJSON(ctx *cue.Context, data []byte) (cue.Value, error) {
	expr, err := cuejson.Extract("", data)
	if err != nil {
		return cue.Value{}, err
	}
	val := ctx.BuildExpr(expr)
	if err := val.Err(); err != nil {
		return cue.Value{}, err
	}
	return val, nil
}

// rebuild returns o as a cue.Value living in ctx, so an operand can be
// unified inside another Value's context. It is general for every
// Value. When o's compiled value already belongs to ctx — a Unify
// result re-unified against its own root — it is returned directly,
// with no recompile. A cross-context operand re-compiles from its
// retained source; strict JSON is a syntactic subset of CUE, so one
// CompileString path rebuilds either a CUE or a JSON source. An operand
// with neither a value in ctx nor source to rebuild from (a derived
// Unify result carried into a foreign context) cannot be reconstructed,
// so ok is false and the caller absorbs it as bottom — never as top.
func (o Value) rebuild(ctx *cue.Context) (cue.Value, bool) {
	if o.val.Context() == ctx {
		return o.val, true
	}
	if o.src != nil {
		return ctx.CompileString(string(o.src)), true
	}
	return cue.Value{}, false
}

// Unify returns the meet of v and o — the value satisfying both. It is
// the façade over cue.Value.Unify. A bottom (⊥) operand absorbs: if
// either v or o is a bottom (an error-carrying or zero Value), the
// result carries that bottom, so a compile failure or an uninitialized
// operand flows through a Unify chain instead of panicking. Otherwise o
// is rebuilt inside v's context and the two are unified there, keeping
// the result single-context. A derived operand that cannot be rebuilt
// in v's context absorbs as a bottom rather than vanishing into an
// empty struct that would silently drop its constraints.
func (v Value) Unify(o Value) Value {
	if err, ok := v.isBottom(); ok {
		return bottom(err)
	}
	if err, ok := o.isBottom(); ok {
		return bottom(err)
	}
	rebuilt, ok := o.rebuild(v.val.Context())
	if !ok {
		return bottom(stderrors.New("cannot unify a derived value across contexts"))
	}
	// The merged value carries v's context. A re-Unify of the result is
	// not part of the phase-0 surface; the result is consumed only by
	// Validate, which reads val, so no source is retained.
	return Value{val: v.val.Unify(rebuilt)}
}

// Validate reports whether the value is concrete and free of conflicts,
// mirroring cue.Value.Validate(cue.Concrete(true)). A bottom Value — an
// error-carrying or zero Value — returns its error rather than panicking
// on a missing context. On a validation failure it returns one
// *PathError per offending leaf, tagged with that leaf's field path,
// joined with errors.Join — so callers (the internal/schema validator
// emits one MDS020 diagnostic per failing field) and the differential
// harness see every rejection, not only the first. Consumers enumerate
// the per-field failures with the package-level Errors accessor. A value
// that satisfies the schema returns nil.
func (v Value) Validate() error {
	if err, ok := v.isBottom(); ok {
		return err
	}
	verr := v.val.Validate(cue.Concrete(true))
	if verr == nil {
		return nil
	}
	leaves := errors.Errors(verr)
	// errors.Errors returns a non-empty list for any non-nil CUE
	// validation error, so leaves is never empty here. The
	// internal/schema validator relies on the same invariant.
	joined := make([]error, len(leaves))
	for i, leaf := range leaves {
		joined[i] = pathErrorOf(leaf)
	}
	if len(joined) == 1 {
		return joined[0]
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
