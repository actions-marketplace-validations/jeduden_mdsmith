---
id: 237
title: "cuelite phase 1 — surface D (placeholder paths)"
status: "✅"
model: sonnet
summary: >-
  Move internal/fieldinterp onto cue/cuelite's ParsePath (still
  delegating to CUE, so green), then flip ParsePath to the
  in-house parser, checked against the CUE-backed path. Surface
  D is the smallest surface and proves the adopt-then-flip
  cadence end to end.
depends-on: [236]
---
# cuelite phase 1 — surface D (placeholder paths)

## Goal

Move placeholder-path parsing onto `cue/cuelite`, then flip
`ParsePath` to the in-house parser.

## Context

Phase 1 of [plan 218](218_wasm-size-reduction.md). Surface D
uses only `cue.ParsePath`, to parse paths like `{a.b.c}` and
`{"my-key".sub}`; resolution is already hand-rolled. It is the
smallest surface, so it proves the cadence before the larger
flips.

## Tasks

1. Add `ParsePath` to the `cue/cuelite` façade, delegating to
   `cue.ParsePath`.
2. Move [fieldinterp](../internal/fieldinterp/fieldinterp.go)
   onto `cuelite.ParsePath`; drop its `cuelang.org/go` import.
   The suite stays green, because behaviour is unchanged.
3. Flip `cuelite.ParsePath` to an in-house path parser, with
   red/green unit tests and the differential harness checking
   it against the CUE-backed path.

## Acceptance Criteria

- [x] `internal/fieldinterp` imports `cue/cuelite`, not
      `cuelang.org/go`.
- [x] `cuelite.ParsePath` is in-house and the harness shows it
      matches CUE on the path corpus.
- [x] `cue/cuelite` path code keeps 100 % statement and branch
      coverage.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues.

## Deviations

- **Tasks 1–3 were collapsed; the adopt-then-flip cadence was NOT
  demonstrated.** No CUE-delegating intermediate `ParsePath` was ever
  committed: `fieldinterp` moved onto `cuelite.ParsePath` and that
  `ParsePath` was written in-house in a single pass. The two-step
  "adopt the façade green, then flip the implementation" cadence this
  plan set out to prove was therefore skipped. The substitute safety is
  the differential harness plus `FuzzParsePath` in
  `internal/cuelitetest`, which compares the in-house parser against the
  CUE-backed oracle on every input — both a corpus run (one case per
  divergence class) and a fuzz run (`go test -fuzz=FuzzParsePath`) with
  zero divergences.
- **First in-house cut diverged systematically from CUE; corrected in
  review round 1.** The committed parser (`3969eed`) used an ASCII-only
  ident class and `strconv.Unquote`, validated only against a curated
  corpus (`ef7626d`) chosen to avoid known divergences. Fuzzing against
  `cue.ParsePath` found systematic disagreement. The realignment:
  - identifiers accept Unicode letters/digits and `$` (CUE's class), not
    just `[a-zA-Z]`;
  - `true`/`false`/`null` are rejected as the leading selector,
    `if`/`for`/`let`/`in` are ordinary identifiers;
  - a CUE-compatible unquoter (`cue/cuelite/unquote.go`) replaces
    `strconv.Unquote`: it accepts `\/` and `\uXXXX`/`\UXXXXXXXX`, rejects
    Go-only `\xNN`/octal `\NNN` and raw NUL/newline/CR in quotes;
  - whitespace (space/tab/CR) and trailing newlines/line-comments around
    tokens are tolerated, matching `cue.ParsePath`.
- **Behavior change vs the old CUE-backed `fieldinterp`.** The previous
  `fieldinterp` called `cue.ParsePath` then `Selector.Unquoted()`
  unguarded, so an INDEX or DEFINITION selector (`{123}`, `a[0]`, `#foo`)
  **panicked** — `cue.ParsePath` parses those as valid non-string labels
  and `Unquoted()` panics on a non-string selector. A HIDDEN label (`_foo`)
  did NOT panic: `cue.ParsePath` itself REJECTS it ("hidden label `_foo`
  not allowed"), so the old `ParseCUEPath` already returned nil gracefully
  there. The string-label-only `cuelite.ParsePath` now rejects all three
  kinds with a clear kind-naming error, which `ParseCUEPath` maps to "no
  path" — a graceful diagnostic in place of the panic on the index and
  definition cases. For every string-label path the two agree (verified by
  the harness), so the realignment is parity-preserving where it mattered.
- **`cuelite.ParsePath` is string-label-only by design.** The hidden-label
  rejection (`_foo`) is PARITY with CUE — `cue.ParsePath` rejects it too
  ("hidden label `_foo` not allowed"), so it is not a valid CUE path. The
  index- and definition-label rejections are the deliberate STRING-LABEL
  NARROWING: CUE accepts those as valid paths, but `cuelite.Path` is
  `[]string`-backed and the phase-2 consumers (`fieldinterp`, `query`) need
  only string labels, so `ParsePath` documents and rejects them rather than
  representing them.
- **The `len(segs) == 0` dead branch in `ParseCUEPath` was removed.**
  `cuelite.ParsePath` never returns a nil error with zero segments (it
  rejects the empty and whitespace-only expression), so the guard was
  unreachable. (The earlier plan text claimed this was already done
  during the flip — it was not; it is removed now.)
- **`ParsePath` returns a plain error, not a `*PathError`.** A
  path-expression syntax error has no data-tree field path to tag, so a
  `*PathError` with a nil path would add nothing; `PathError` stays the
  type for per-leaf validation failures only.
- **Harness adds `PathCase`/`PathOutcome` in a separate file**
  (`internal/cuelitetest/path.go`), not by extending the existing
  `Case`/`Outcome` types, so the schema/data `Case` does not carry a
  path field irrelevant to most tests. The oracle classifies each CUE
  selector by `LabelType` (string label → segment; index/definition/
  hidden → the documented rejection) and wraps `cue.ParsePath` in a
  panic guard, because `cue.ParsePath("a...")` nil-derefs inside
  cuelang v0.16.1.

## Implementation notes

- **`Path` is not opaque — both directions are real consumers.**
  [`fieldinterp.ParseCUEPath`](../internal/fieldinterp/fieldinterp.go)
  reads the unquoted per-selector strings back OUT of a parsed path
  (the harness compares `[][]string`), so the surface-D `Path` API
  must expose a `Segments()` accessor.
  [`query.collectPaths`](../internal/query/query.go) builds paths IN
  programmatically via `cue.MakePath` + `iter.Selector()`, so the
  API must also offer a constructor-from-segments (the in-house
  `MakePath` equivalent). Adopt both when adding `ParsePath`, so the
  flip in task 3 does not change the API.
- **Extend the harness TYPES, but add a separate path arm — don't
  append to the corpus.** Surface D extends `internal/cuelitetest`'s
  `Case` and `Outcome` (a new case field plus a stage or payload as
  the shape needs), not a parallel structure. But `Run` hardcodes
  the schema/data arms (`CueLitePath`/`OraclePath`), and a path-only
  `Case` in the existing `corpus()` slice would agree VACUOUSLY —
  an empty schema and data classify identically in both arms
  regardless of the path. So surface D adds its OWN path-comparing
  arm/runner (parse via cuelite vs `cue.ParsePath`, compare
  segments) rather than appending path cases to the existing corpus
  slice. `Outcome.Equal` already compares `Paths` at every stage, so
  a parsed-segment payload is differentially checked there.

## See also

- [Plan 218 — in-house CUE-subset engine](218_wasm-size-reduction.md)
