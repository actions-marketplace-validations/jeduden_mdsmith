---
id: 174
title: Expose rename and dependency-graph as CLI subcommands and feature docs
status: "🔳"
model: opus
depends-on: [131, 151, 153]
summary: >-
  Lift the heading / link-reference rename logic out of
  `internal/lsp` into a shared `internal/rename` core, relocate
  the LSP symbol index to `internal/index`, and expose rename
  plus the include/catalog/build/link dependency graph as the
  name-based `mdsmith rename` and `mdsmith deps` subcommands.
  Document every LSP capability as a feature.
---
# Expose rename and dependency-graph as CLI subcommands and feature docs

## Goal

The rename capability (plan 151) ships only over the LSP
wire protocol. So does the call-hierarchy / dependency graph
(plan 131). An agent or script with no editor cannot reach
either. Neither appears in `docs/features/`.

This plan exposes both as CLI subcommands. It also documents
the full LSP surface as features. Domain logic is not
duplicated.

## Background

Today `internal/lsp/rename.go` computes slug remaps and
edits as methods on the LSP `*Server`. It speaks LSP wire
types (`WorkspaceEdit`, `textEdit`). The
[layering map](../docs/development/architecture/index.md)
makes `cmd/mdsmith` and `internal/lsp` sibling entry points.
So the CLI must not import `internal/lsp`.

Plan 153 made `internal/linkgraph` the canonical shared link
extractor. `mdsmith list backlinks` already consumes it with
no `internal/lsp` import. Plan 153 kept the symbol index
LSP-local on purpose.

This plan supersedes that one plan-153 non-goal. The symbol
index moves to a peer `internal/index` package. Both entry
points can then consult it. The
[architecture audit log](../docs/development/architecture-audit.md)
records the supersession.

## Non-Goals

- File rename, `kind:` rename, directive-name rename,
  front-matter-key rename — all out of scope per plan 151.
- New CLI commands for the inherently interactive navigation
  capabilities (definition, references, document-symbol,
  workspace-symbol, implementation, completion). They get
  feature-doc coverage only; `cross-system.md` warns against
  manufacturing CLI surfaces for editor-only features.
- Changing the LSP wire behavior. Plans 131 / 151 test suites
  are the regression gate for the delegation refactor.
- A persistent graph cache. `deps` builds a transient index
  per invocation, like `backlinks`.

## Design

### Package boundaries

- `internal/index` — the relocated symbol / edge index
  (pure `git mv` of `internal/lsp/index`; package name
  unchanged, import path only). Support layer; consumed by
  `internal/lsp`, `internal/rename`, and `cmd/mdsmith`.
  Must not import `internal/lsp` (SRP, DIP).
- `internal/rename` — NEW core. Answers one question:
  "given a workspace and a rename target, what file edits
  perform it, or what typed error?" Depends on
  `internal/linkgraph`, `internal/mdtext`, `internal/index`.
  Returns plain `Edit{File,Start,End,NewText}` and typed
  errors; no LSP types. Must not import `internal/lsp`
  (DIP — high-level surfaces depend on this, not the
  reverse).
- `internal/lsp/rename.go` — thin adapter: LSP params →
  core request → core edits → `WorkspaceEdit`; typed core
  error → `InvalidParams{data.conflict}`. Duplicated
  computation deleted (no half-formed duplicate, per
  `cross-system.md`).
- LSP call-hierarchy handlers — delegate to a shared
  `internal/index` deps query so CLI and LSP share one walk.
- `cmd/mdsmith/rename.go`, `cmd/mdsmith/deps.go` — thin
  handlers (<50 lines each per `go.md`), mirroring
  `cmd/mdsmith/backlinks.go`. Neither imports `internal/lsp`.

### `mdsmith rename` (name-based contract)

```bash
mdsmith rename <file> --heading "Old Title" "New Title"
mdsmith rename <file> --link-ref oldlabel newlabel
```

Rewrites the heading / def plus every dependent edit across
the workspace in place. `--format text|json` summarizes the
files touched. Slug collision / label conflict / empty /
invalid-char fail with exit 2 and a message naming the
conflict (the CLI mirror of plan 151's `data.conflict`).

### `mdsmith deps` (dependency graph)

```bash
mdsmith deps <file>              # outgoing: includes/catalogs/builds/links
mdsmith deps <file> --incoming   # files that depend on <file>
```

`--format text|json`. Builds a transient `internal/index`
over discovered files and queries `OutgoingEdges` /
`IncomingEdges` / `BacklinksFor`.

### Feature docs

- New `docs/features/rename.md`, `docs/features/dependency-graph.md`.
- Expand `docs/features/live-diagnostics.md` so the remaining
  navigation suite is actually described (combined, per the
  scope decision).
- Add cards to `docs/features/index.md`; add
  `docs/reference/cli/rename.md` and `cli/deps.md` (catalog
  auto-regenerates).

## Tasks

1. [x] Create this plan; `mdsmith fix PLAN.md`.
2. [x] Relocate `internal/lsp/index` → `internal/index`
   (`git mv`; rewrite import paths in `internal/lsp/*.go` +
   tests + every `*.md` repo-path reference). Layer move
   only; `go build/test` + existing index/lsp tests are the
   regression gate.
3. [x] Update architecture docs (`go.md` SRP list + DIP
   arrows, `index.md` layering map) and append the plan-153
   supersession to the audit log. (`cross-system.md`
   boundaries/versioning deferred to task 8 with the CLI
   surface.)
4. [ ] TDD `internal/rename` core. **In progress —**
   **done:** the
   link-reference engine plus the shared body/offset
   helpers are lifted out of `internal/lsp/rename.go` into
   `internal/rename` (neutral `Edit`/`Position`/`Range`,
   typed `ErrEmptyLabel` / `InvalidLabelRuneError` /
   `LabelConflictError`), behavior-tested at **100%**
   statement coverage, building and linting clean.
   **Remaining:** the heading engine (`computeSlugRemap`,
   `assignSlugs`, anchor/ref-def-dest edits) still lives in
   `internal/lsp/rename.go` because the heading and
   link-ref paths share helpers now in `internal/rename`;
   moving it is the next slice of this task.
5. [ ] Refactor `internal/lsp/rename.go` to delegate to
   `internal/rename`; delete duplicated computation. Plans
   151/131 + `cmd/mdsmith/lsp_rename_test.go` stay green.
6. [ ] TDD `cmd/mdsmith/rename.go` (unit + e2e), register in
   `main.go` dispatch + `usageText`.
7. [x] TDD `cmd/mdsmith/deps.go` (unit + e2e); register
   in `main.go`. The LSP call-hierarchy and `deps` both
   consult `internal/index` for the edge graph, so the
   walk is already shared — no separate query-extraction
   refactor was needed.
8. [x] Feature + reference docs; `mdsmith fix .` to
   regenerate catalogs (CLAUDE.md, cli.md, README grid,
   CLAUDE.md include mirrors); `mdsmith check .`.
   `cross-system.md` left as-is: the CLI-surface row
   already covers subcommands generically, and "adding a
   flag/command is minor" is stated there.
9. [ ] Final gate + flip status to ✅; push. **Remaining
   before ✅: tasks 4–6 (the `internal/rename` core, the
   LSP delegation, and the `mdsmith rename` CLI).**

## Acceptance Criteria

- [x] `internal/index` exists; no production file imports
      `internal/lsp/index`; `grep -r internal/lsp/index`
      finds nothing (SRP / DIP — package answers one
      question, CLI no longer reaches the editor layer).
- [ ] `internal/rename` returns plain edits and typed errors,
      imports neither `internal/lsp` nor any LSP wire type
      (DIP — surfaces depend on the core).
- [ ] `internal/lsp/rename.go` contains no slug / edit
      computation; it delegates to `internal/rename` (no
      duplicated logic across surfaces — `cross-system.md`).
- [x] Plans 131/151 LSP test suites and
      `cmd/mdsmith/lsp_rename_test.go` pass unchanged
      (Liskov — the relocation is behavior-preserving; the
      delegation refactor in task 5 must keep them green).
- [ ] `mdsmith rename f.md --heading "A" "B"` rewrites the
      heading and every workspace anchor link; `--link-ref`
      rewrites def + uses; collisions exit 2 naming the
      conflict.
- [x] `mdsmith deps f.md` and `--incoming` emit the
      dependency edges in text and json.
- [ ] CLI rename + deps contracts locked by e2e tests in
      `cmd/mdsmith/` (cross-system contract test). deps
      done (`e2e_deps_test.go`); rename pending task 6.
- [x] Every new production function has a dedicated unit
      test (`TestFoo` / `TestReceiver_Foo`). Holds for the
      shipped `deps` code; re-verify when the rename core
      lands.
- [x] `docs/features/` documents rename, the dependency
      graph, and the full navigation suite;
      `docs/features/index.md` and the CLI reference list the
      new commands.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues.
- [x] `mdsmith check .` passes.

## Open Questions

- **`deps` command name.** `deps` chosen over `graph` /
  `list deps` for parity with the top-level `rename`
  subcommand and the "what does this depend on" framing.
  Easy to rename before the contract test locks if a
  reviewer prefers otherwise.

## ...

<?allow-empty-section?>
