---
id: 238
title: "cuelite phase 2 — surfaces A + B (schema, query)"
status: "🔲"
model: opus
summary: >-
  Move internal/schema, requiredstructure, and internal/query
  onto cue/cuelite's compile/unify/validate façade (green),
  then flip to the in-house value model and validator —
  validating front matter directly without the JSON round-trip,
  preserving per-field MDS020 diagnostics, checked against the
  CUE oracle across the whole corpus.
depends-on: [237]
---
# cuelite phase 2 — surfaces A + B (schema, query)

## Goal

Move schema validation (MDS020) and `query`/`where:` onto
`cue/cuelite`, then flip them to the in-house value model and
validator.

## Context

Phase 2 of [plan 218](218_wasm-size-reduction.md), the largest
migration. Surface A is
[internal/schema](../internal/schema/validate.go) and
[requiredstructure](../internal/rules/requiredstructure/rule.go);
surface B is [internal/query](../internal/query/query.go), a
strict subset of A. See plan 218 for the value model and the
unification rules.

## Tasks

1. Adopt the compile/unify/validate façade already shipped by
   phase 0 ([plan 236](236_cuelite-package-harness.md)):
   `Compile`, `CompileJSON`, `Value.Unify`, `Value.Validate`,
   and the `Errors` accessor. Extend it with the per-surface
   methods these call sites need (`LookupPath`, `Decode`,
   `Fields`, …), still delegating to CUE. The phase-0 interim
   has two costs to retire here. First, each `Compile`/
   `CompileJSON` owns a fresh `*cue.Context`, and a cross-context
   `Unify` rebuilds whichever side retains source into the other
   side's context — one rebuild per such `Unify`, leaving the
   result in (and mutating) the non-rebuilt side's context.
   Second, the schema path still marshals front matter to JSON
   and `CompileJSON` parses it back, so validating one file pays
   three JSON traversals — the marshal, `CompileJSON`'s
   duplicate-key scan, and `cuejson.Extract`'s own parse. The
   duplicate scan is interim-only: the post-flip hot path
   validates the `map[string]any` directly and bypasses
   `CompileJSON` entirely, so the scan disappears with the round
   trip. All three blow the ≤ 10 allocs/op budget on the hot
   path, so the budget is met only by the flip in task 3 — the
   in-house engine validates a `map[string]any` directly (plan
   218), with no JSON round-trip and no per-`Value` context — not
   by the façade adoption in this task.
2. Move [internal/schema](../internal/schema),
   [requiredstructure](../internal/rules/requiredstructure/rule.go),
   and [internal/query](../internal/query/query.go) onto the
   façade. The suite stays green.
3. Flip to the in-house value model, `Unify`, and `Validate` —
   red/green per rule and per ⊥/error path. Validate
   front-matter maps directly, with no JSON marshal.
4. Hold the differential harness green across every
   `frontmatter:` constraint, the
   [file-kinds conflict table](../docs/guides/file-kinds.md),
   and the query/`where:` examples.

## Acceptance Criteria

- [ ] `internal/schema`, `requiredstructure`, and
      `internal/query` import `cue/cuelite`, not `cuelang.org/go`.
- [ ] Front-matter validation skips the JSON round-trip and
      stays within the ≤ 10 allocs/op budget.
- [ ] MDS020 diagnostics stay actionable and navigable (plan
      147 / plan 230 behavior preserved).
- [ ] The harness shows in-house and CUE agree on the full
      corpus; `cue/cuelite` keeps 100 % statement and branch
      coverage.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues.

## Implementation notes

- **Build paths from data keys via `MakePath`, never `ParsePath`.**
  `query.collectPaths` and any other consumer that derives a path from a
  `map[string]any` key (e.g. iterating `Value.Fields()`) must use
  `cuelite.MakePath(segs...)`, which stores raw segments — including empty,
  dotted, `true`-headed, or quote-needing keys that `ParsePath` (the narrower
  string-label EXPRESSION grammar) cannot parse back. Reserve `ParsePath`
  for user-typed path expressions. There is deliberately no `Path` render
  today (`PathError.Error()`'s dot-join is lossy and display-only); a future
  `Path.String()` must quote any segment that is not a bare identifier.
- **LookupPath provenance.** A derived `Value` (a `Unify` result)
  is context-pinned and retains no source, so it cannot be rebuilt
  into another context. `LookupPath` on such a value inherits that
  limit: a section-level lookup against a cached schema must keep
  rebuildable provenance — for example the root source plus the
  looked-up path — or the cached schema either races (each lookup
  mutates its shared context) or forfeits caching (every lookup
  recompiles). Choose the provenance representation when adding
  `LookupPath`, not after.
- **Call-site operand order.** Today's adopters call
  `schemaVal.Unify(dataVal)` in
  [validate.go](../internal/schema/validate.go) and
  `m.schema.Unify(dataVal)` in
  [query.go](../internal/query/query.go) — the SHARED schema is the
  receiver, so the cross-context rebuild puts the schema's context
  in the mutated (non-rebuilt) position. Phase 2 must pick the
  operand order and locking deliberately: a schema cached and
  reused across files or goroutines cannot sit in the mutated
  position without synchronization.
- **RunCache / CachedCompile shape (task 2).** The cache's contract
  — one schema compile per Run, shared across parallel workers via
  [`CompiledCUE.Ctx`](../internal/schema/compile_cache.go) — has NO
  façade equivalent, because cuelite hides the context. Neither
  operand order resolves it under the cache: `schemaVal.Unify(
  dataVal)` rebuilds the data into the shared schema's context and
  MUTATES it (a data race under `-race` CI when workers share the
  cached schema), while `dataVal.Unify(schemaVal)` rebuilds the
  schema per file and recompiles it each call (the cache becomes a
  no-op and the compile-once assertions in
  [validate_runcache_test.go](../internal/schema/validate_runcache_test.go)
  break). So task 2 must redesign the cache shape, not just adopt
  the façade — cache the SOURCE and compile per worker, or guard the
  shared `Value` with a lock, or defer caching to the in-house flip
  in task 3 (where a context-free `Value` is shareable). Name the
  affected files when scheduling: `compile_cache.go` and
  `validate_runcache_test.go`.

## See also

- [Plan 218 — in-house CUE-subset engine](218_wasm-size-reduction.md)
