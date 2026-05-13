---
id: 153
title: Unify linkgraph and the LSP symbol index
status: "🔲"
model: opus
depends-on: [151]
summary: >-
  Pick one canonical link-extraction layer between
  internal/linkgraph and internal/lsp/index, then route
  every caller through it. MDS027, the
  `mdsmith list backlinks` CLI, and the LSP rename /
  navigation / call-hierarchy surface today walk
  Markdown links via two parallel implementations with
  subtly different edge models.
---
# Unify linkgraph and the LSP symbol index

## Goal

One source of truth for "what's a Markdown link and what
does it point at." MDS027, `mdsmith list backlinks`, and the
LSP server should all consult the same extractor. They
should also share one edge type. A link the CLI surfaces
as a backlink should be the same link the LSP jumps through
and the lint rule audits.

## Background

The repo currently maintains two extractors:

- [`linkgraph`](../internal/linkgraph/linkgraph.go) exposes
  `ExtractLinks(f *lint.File) []Link`, `ParseTarget`, and
  `NormalizeAnchor`. The MDS027 rule and the
  [`backlinks` CLI](../cmd/mdsmith/backlinks.go) (plan
  138) call it. Each invocation re-walks the workspace
  and re-parses every file.

- [`lsp/index`](../internal/lsp/index/index.go) builds a
  workspace graph. The LSP keeps it warm. It stores
  outgoing edges and symbol tables per file. The
  reverse-edge surface (`IncomingEdges`, `BacklinksFor`)
  reads from those edges. The extractor itself sits in
  [`build.go`](../internal/lsp/index/build.go).
  `collectLinkEdges` and `collectDirectiveEdges` walk the
  same source bytes linkgraph would, but under a different
  `EdgeKind` set: `EdgeAnchorLink`, `EdgeFileLink`,
  `EdgeRefLink`, `EdgeInclude`, `EdgeCatalog`,
  `EdgeBuild`.

The duplication is real:

- Anchor normalization happens twice
  (`linkgraph.NormalizeAnchor` vs
  `mdtext.Slugify` + `decodeAnchor` in the index).
- Empty-`TargetFile` semantics differ: in the index it
  means "same file" for anchor / ref links and
  "placeholder for a `<?catalog?>` directive" for
  catalog edges, which forced
  [`BacklinksFor`](../internal/lsp/index/index.go)
  to special-case `EdgeCatalog` after plan 151
  surfaced phantom self-backlinks.
- Path resolution rules drift:
  `linkgraph.ParseTarget` percent-decodes and validates
  with one set of rules; the index's
  `resolveRelTarget` applies its own
  workspace-escape check.

## Non-Goals

- Replacing the LSP server's *symbol* index — headings,
  link-ref defs, directives, and front-matter keys all
  stay in `internal/lsp/index`. Only the link/edge
  extraction is in scope.
- Changing MDS027's diagnostic shape or message text
  (regressions there are user-visible).
- Changing the `mdsmith list backlinks` output format
  (the JSON / table shapes are documented in
  [`docs/reference/cli/backlinks.md`](../docs/reference/cli/backlinks.md)).
- A persistent on-disk graph cache. The CLI keeps its
  per-invocation walk; the LSP keeps its in-memory
  graph. Only the per-file extractor is unified.

## Design

### Pick the canonical extractor

Adopt `internal/linkgraph` as the canonical layer.
Rationale:

- It already has the wider audience (lint rule + CLI).
- It already exposes URL parsing
  (`ParseTarget`) and anchor normalization
  (`NormalizeAnchor`) as public primitives.
- Its `Link` type is simpler than the index's `Edge`
  (no embedded `Kind` overload for catalog placeholders).

The LSP index keeps owning the graph (the map of files,
the reverse-edge query, the directive-aware
call-hierarchy). It just stops re-implementing link
extraction.

### New shared types

Extend linkgraph with the small surface the index needs
on top of `ExtractLinks`:

- `DirectiveEdge` — a typed record for
  `<?include?>` / `<?build?>` / `<?catalog?>` targets,
  with explicit catalog handling (no empty-`TargetFile`
  placeholder; catalog matches expand inside the
  extractor when a workspace `fs.FS` is supplied, or
  return a typed `Unresolved` marker otherwise).
- `ExtractDirectives(f *lint.File) []DirectiveEdge` to
  match `ExtractLinks`.

The index's `EdgeKind` collapses to
`{LinkAnchor, LinkFile, LinkRef, DirectiveInclude,
DirectiveBuild, DirectiveCatalog}` — the same labels,
just sourced from linkgraph.

### Migration steps

1. Move the index's `parseLinkTarget`, `decodeAnchor`,
   and `resolveRelTarget` helpers into linkgraph. Or
   replace them with the existing linkgraph equivalents.
   Drop the duplicates from `internal/lsp/index/build.go`.
2. Have `collectLinkEdges` call `linkgraph.ExtractLinks(f)`
   and map the result to `Edge` records. The LSP index
   and MDS027 now walk the same bytes through the same
   parser.
3. Add `ExtractDirectives` to linkgraph. Route the
   index's `collectDirectiveEdges` through it.
4. Drop `BacklinksFor`'s special-case `EdgeCatalog`
   filter. Catalog edges now carry a real `TargetFile`
   (or a typed `Unresolved` sentinel the helper can
   skip generically).
5. Switch `cmd/mdsmith/backlinks.go` to either:

  - reuse the index for `mdsmith list backlinks` by
     building a transient index over the discovered
     files, or
  - keep the per-invocation walk but use the unified
     extractor so its output stays bit-for-bit
     compatible.

   Pick whichever keeps the CLI output identical; the
   existing E2E test in
   [`cmd/mdsmith/e2e_backlinks_test.go`](../cmd/mdsmith/e2e_backlinks_test.go)
   is the regression gate.

### Backwards compatibility

- MDS027's diagnostic messages and ranges must stay
  byte-identical. Test the rule against its fixture
  set before and after.
- `mdsmith list backlinks --format=json` must produce
  the same key set in the same order.
- The LSP wire surface (rename, references, call
  hierarchy) keeps its current behavior; the only
  internal change is which package extracts the
  edges.

## Tasks

1. Audit `internal/lsp/index/build.go` and
   `internal/linkgraph/linkgraph.go` side by side.
   Produce a table of every parsing rule each applies
   (escape handling, percent-decoding, angle-bracket
   destinations, code-block exclusion, …) and resolve
   each difference as either "promote linkgraph's
   behavior" or "promote the index's behavior" with
   tests for both directions.
2. Add `linkgraph.ExtractDirectives` and the
   `DirectiveEdge` type. Cover include / build /
   catalog with a fixture in
   [linkgraph_test.go](../internal/linkgraph/linkgraph_test.go).
3. Replace `collectLinkEdges` and
   `collectDirectiveEdges` in the index with calls
   through linkgraph. Adjust `Edge` ↔ `Link` mapping.
4. Drop the `EdgeCatalog`-specific filter in
   `BacklinksFor` and update its test to reflect the
   unified catalog handling.
5. Verify MDS027 fixtures still pass without
   modification. If any diagnostic shifts, write a
   plan-text justification and bake the shift into
   the rule's own tests, not the index's.
6. Verify
   [`cmd/mdsmith/e2e_backlinks_test.go`](../cmd/mdsmith/e2e_backlinks_test.go)
   still passes byte-for-byte. If output drift is
   unavoidable, document it in
   [`docs/reference/cli/backlinks.md`](../docs/reference/cli/backlinks.md)
   and adjust the test fixtures in the same commit.
7. Remove the now-dead extractor helpers from the
   index package.

## Acceptance Criteria

- [ ] `internal/lsp/index/build.go` no longer contains
      a Markdown link parser; all link / directive
      extraction goes through `internal/linkgraph`.
- [ ] `linkgraph.ExtractDirectives` exists and is
      covered by `internal/linkgraph/linkgraph_test.go`.
- [ ] `BacklinksFor` returns the same results before
      and after the change for the fixtures in
      `internal/lsp/index/index_test.go`.
- [ ] MDS027 fixtures and unit tests pass without
      diagnostic-message edits.
- [ ] `mdsmith list backlinks` E2E test passes
      byte-for-byte against the pre-change fixture set.
- [ ] All tests pass: `go test ./...`.
- [ ] `go tool golangci-lint run` reports no issues.
- [ ] `mdsmith check .` passes.

## Open Questions

- **Catalog edge representation.** Today the LSP index
  emits one `EdgeCatalog` per directive with empty
  `TargetFile` so call-hierarchy can show "this file
  uses a catalog" without exploding large globs. Plan
  151's `BacklinksFor` filtered them out. Should the
  unified extractor expand the glob (cheap when an
  `fs.FS` is in hand, expensive in the CLI),
  emit a typed `Unresolved` sentinel, or keep the
  current placeholder shape? Decide before step 2.
- **`mdsmith list backlinks` performance.** If the CLI
  builds a transient index per invocation, the
  build cost is paid once instead of per-link.
  Benchmark on a 1 000-file workspace before committing
  to that route.

## ...

<?allow-empty-section?>
