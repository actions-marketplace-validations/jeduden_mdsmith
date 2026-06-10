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
- [x] `cue/cuelite` and `internal/cuelitetest` keep 100 %
      STATEMENT coverage (`go tool cover -func`), and the path
      parser is checked end-to-end by the differential corpus plus
      `FuzzParsePath`. Branch coverage is reported by
      `go tool gobco -branch`, not asserted at 100 %: as of round 5
      `cue/cuelite` sits at 394/396 conditions (up from round 4's
      342/344 as the multiline CR-family and escaped-newline fixes
      added live, covered branches). The two remaining gaps are the
      SAME two structural ones, not "defensive conditions outside the
      path code" (round 3's claim was wrong for three of its four):
      `path.go`'s `kind == sepBracket` is one arm of an exhaustive
      `sepKind` switch, and `multiline.go`'s `for i > 0` walk-back
      bound cannot underflow because the multiline opener's `"`
      is a non-space sentinel that always breaks the loop first.
      The three conditions round 3 called defensive in
      `value.go`'s `scanDuplicateJSONKeys` (the array-close arm and
      its parent-restore and array-element-scalar branches) are LIVE
      and now have dedicated array-shaped unit tests
      (`{"a":[1],"a":2}`, `[{"a":1},{"a":1}]`, `[1]`).
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
  CUE-backed oracle on every input — a corpus run (one case per divergence
  class) and a fuzz run (`go test -fuzz=FuzzParsePath`). "Zero divergences"
  is bounded by what the fuzzer reaches: its mutations are seeded from the
  corpus, so it explores the byte-space AROUND known classes but does not
  exhaustively reach every composition. Review round 3 found a class the
  fuzzer had not reached — raw-string × surrogate-escape pairing, whose
  inputs splice a hash-level escape introducer (`\#u`) onto a surrogate
  pair (`#"\#uD800\#uDC00"#`). Three defects there were corrected: a
  reachable out-of-bounds panic on a high half before the closing
  delimiter, a valid `\#u`+`\#u` pair wrongly rejected, and an invalid
  `\#u`+plain-`\u` pair wrongly accepted. Each is now pinned in BOTH the
  corpus and the unit tables and seeded into `FuzzParsePath`, and a 300 s
  deep fuzz run after the fix reported no further divergence.
- **Review round 4 found two more grammar-divergence classes and a CR
  edge inside one of them.** (1) The single-line raw-string close scan used
  a blind `strings.Index`, so an escaped quote followed by a hash run
  (`#"\#"#"#`, body `\#"#`, decoding to `"#`) was read as a false
  terminator — CUE accepts these; the in-house parser rejected them. The
  scan is now escape-aware (`rawStringCloseIndex`/`multilineCloseIndex` skip
  `\`+N`#`+selector before matching the close). (2) Multiline string labels
  (`"""…`/`#"""…`, plain and raw) were rejected wholesale; CUE accepts them
  as head and bracket operands (never after a dot). They are now implemented
  in-house (`multiline.go`) porting CUE's `cue/literal` semantics for the
  string-label subset: the opener must be followed by a newline, the closing
  line's leading whitespace is the indentation stripped from every content
  line, the final newline before the close is excluded, and escapes follow
  the single/raw dialect with surrogate pairing. A malformed multiline whose
  CUE `Unquoted()` is `""` (bad opener/indent/close, escaped final newline,
  lone surrogate) decodes to `""` here and is rejected by the empty-segment
  check — the same outcome as the oracle. While probing, the harness caught a
  CR bug: CUE's scanner strips every `\r` from a multiline token
  (`scanner.stripCR`), so `unquoteMultiline` strips CR up front, matching
  `cue.ParsePath` on CRLF and bare-CR content. Every probed shape (including
  CUE's error cases) is pinned in BOTH the corpus and the unit tables and
  seeded into `FuzzParsePath`. A 300 s deep fuzz run after the fixes reported
  no further divergence.
- **Review round 4's CR handling was wrong; round 5 corrected the CR
  family and uncovered an escaped-newline asymmetry.** Round 4's
  `unquoteMultiline` stripped every `\r` from the whole token BEFORE lexing.
  CUE v0.16.1 does the opposite: its scanner runs `stripCR` on the final
  literal only AFTER scanning (`scanner.scanString`, stripCR at 429/453),
  while the opener-newline check (a multiline opener must be followed by a lone
  `\n` or exactly one `\r\n` — scanner 813–829) and `scanEscape` (the escape
  selector and any `\u`/`\U` hex digits — scanner 352–413) run on the RAW
  bytes. So three inputs the in-house parser accepted are CUE scan errors:
  a CR run at the opener (`"""\r\r\n0\n"""`, fuzz-minimized); a raw CR between
  the backslash and the escape selector (→ "unknown escape sequence"); and a
  raw CR among the `\u` hex digits (→ "illegal character U+000D in escape
  sequence"). The fix validates the
  opener-newline and the escapes on the RAW token (`rawMultilineOK`/
  `rawEscapesOK`/`rawEscapeScan`), stripping CR only for value assembly, so the
  benign CR cases still decode CR-free — including the `\`+CR+`#` level-1 case,
  which the raw scanner reads as a literal backslash (the oracle agrees). A
  300 s fuzz run after that fix then minimized a fourth input,
  `#"""\n\`+CR+`#\n0\n"""#`, which exposed a deeper, pre-existing gap: CUE's
  SCANNER and its `literal.Unquote` disagree on `\#`+newline. The scanner
  rejects it as an unknown escape, but `literal.Unquote` treats it as an
  ESCAPED NEWLINE (a line continuation that elides the newline —
  `escapedNewline`, errEscapedLastNewline for the final one). `cue.ParsePath`
  runs both, so the only way to reach the escaped-newline value path is via the
  `\`+CR+`#` fusion: the scanner accepts the literal backslash, stripCR fuses
  `\#`+newline, and `literal.Unquote` then elides it. `decodeMultilineBody` now
  ports that escaped-newline handling (`escapedNewlineTail`), so the in-house
  value decode matches `literal.Unquote` for line continuations, the final-
  newline rejection, and the bad-next-line-indent rejection. All four inputs
  (plus the benign CR shapes, the `\U` multiline escape, the truncated-hex-at-
  close case, the full non-unicode escape-selector set, and the escaped-newline
  join/keep-indent/last-newline/bad-indent shapes) are pinned in BOTH the
  corpus and the unit tables and seeded into `FuzzParsePath`; the minimized
  `\`+CR+`#` input is also committed as a `testdata/fuzz/FuzzParsePath` seed.
  Branch coverage rose from 342/344 to 394/396 (only the two known structural
  conditions remain). A final 300 s deep fuzz run (~2.12M execs) after the
  round-5 fixes reported no further divergence.
- **Item 7 (fieldinterp boundary error passthrough) was SKIPPED.**
  `fieldinterp.ParseCUEPath` returns `[]string` (nil on error) and is used by
  seven call sites (`catalog/rule.go`, `schema/matcher.go`, `schema/
  matchtree.go`, `requiredstructure/rule.go`, …), each relying on the
  `[]string`/nil contract. Surfacing `cuelite.ParsePath`'s precise error would
  change the hard-coded "non-identifier keys must be quoted" hint those callers
  emit — a user-visible message pinned by `schema/plan156_acceptance_test.go`
  (and embedded in three production sites). That is a message-contract change,
  not a small/clean one, and the round-5 brief forbids changing pinned messages
  without deliberately updating the tests. Phase 2 can revisit it (e.g. a
  parallel `ParseCUEPathErr` that threads the underlying error) alongside a
  deliberate message-contract update.
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
