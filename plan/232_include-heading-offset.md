---
id: 232
title: include heading-offset parameter
status: "✅"
summary: >-
  Add a `heading-offset` parameter to the `<?include?>`
  directive (MDS021) that shifts every heading in the
  embedded content by a signed integer. It complements the
  context-relative `heading-level: "absolute"` and works
  even when no heading precedes the marker.
model: opus
depends-on: []
---
# include heading-offset parameter

## Goal

Give [`<?include?>`][include] a way to shift the headings in
an embedded file by a fixed amount. The content then nests
at the depth you pick. It works even with no heading before
the marker.

## Context

`<?include?>` already adjusts headings with
`heading-level: "absolute"`. That mode shifts the embedded
headings so the shallowest one sits one level below the
nearest preceding heading. The math lives in
[`adjustHeadings`][headings].

The mode is context-relative: it reads the parent heading
level from the host document. When the marker sits at the
document root, there is no parent. `adjustHeadings` then
returns the content unchanged, so the embedded file keeps
its source levels.

That leaves a gap. Take a README whose visual title is a
logo rather than an H1. A file embedded at the top of it
cannot have its headings demoted: the source `# Title`
stays an H1 and outranks the host's own `##` sections. The
author needs a way to say "shift these down by one" that
does not depend on a parent heading.

## Design

### New parameter: `heading-offset`

A signed integer. It shifts every heading in the embedded
content by that amount. The idea matches Pandoc's
`--shift-heading-level-by` and AsciiDoc's `leveloffset`.

```text
<?include
file: features.md
heading-offset: "1"
?>
## Was An H1 In The Source
<?/include?>
```

| Value  | Effect                                |
| ------ | ------------------------------------- |
| `"1"`  | every heading one level deeper, H1→H2 |
| `"-1"` | every heading one level shallower     |
| `"0"`  | no change                             |

Levels are clamped to the 1–6 range after the shift, the
same cap `absolute` already applies. The sign may carry an
explicit `+`, as in `"+1"`.

### Semantics next to `heading-level`

`heading-level: "absolute"` is relative to the host: the
shift depends on the parent heading. `heading-offset` is
relative to the source: the shift is fixed and applies even
at the document root. They are two strategies for one goal,
so they are mutually exclusive. Setting both is a lint
error.

`heading-offset` shifts every heading by the same amount,
upward too for a negative value. So it reuses the existing
[`applyShift`][headings] helper directly. It skips the
"no shift when the result is ≤ 0" guard that `adjustHeadings`
needs for parent-relative nesting.

### Validation

- The value must parse as an integer from −6 to 6. Anything
  else is a lint error.
- `heading-offset` cannot pair with `heading-level`, since
  the two are rival strategies.
- `heading-offset` cannot pair with `extract:`, which
  returns one scalar and has no headings. This mirrors the
  rule `heading-level` already follows.

### Out of scope

- An absolute base form that pins the shallowest heading to
  level N. It overlaps `heading-offset` for single-rooted
  content, so it waits for a real need.
- Stacking `heading-offset` on `absolute`. The two stay
  mutually exclusive to keep the model simple.
- Using the new syntax in any tracked Markdown file. Per
  [the directive-syntax process][adopt], the pinned CI
  baseline must ship the feature first, or
  `mdsmith-fixed-version` breaks. This plan adds the
  capability only. Adoption is a later, separate change.

[include]: ../internal/rules/MDS021-include/README.md
[headings]: ../internal/rules/include/headings.go
[adopt]: ../docs/development/adopt-new-directive-syntax.md

## Tasks

1. [x] Add `adjustHeadingsByOffset(content, offset)` to
   [`headings.go`][headings], reusing `applyShift`. Unit
   tests for ATX, setext, code fences, clamping, and the
   `offset == 0` no-op land in `headings_test.go` first,
   red.
2. [x] Apply `heading-offset` in `processIncludedContent`
   in [`rule.go`][rule]: call `adjustHeadingsByOffset` when
   the parameter is present.
3. [x] Extend `validateIncludeDirective`: reject a
   non-integer or out-of-range value, reject the pairing
   with `heading-level`, and reject the pairing with
   `extract`. A unit test pins each branch.
4. [x] Add fixtures under [MDS021-include][include-dir]: a
   `good/` plus matching `fixed/` pair that exercises
   `heading-offset`, and a `bad/` case with a stale body.
5. [x] Update the rule README ([MDS021-include][include]):
   the parameter table, a heading-offset section, an
   example, and the new diagnostics rows.
6. [x] Update the directive guide
   ([generating content][guide]) and the built-in help doc
   [`internal/directives/generating-content.md`][help] to
   cover `heading-offset`.
7. [x] Run `go test ./...`, `go tool golangci-lint run`,
   the allocation-budget test, and `mdsmith check .`.

[rule]: ../internal/rules/include/rule.go
[include-dir]: ../internal/rules/MDS021-include/
[guide]: ../docs/guides/directives/generating-content.md
[help]: ../internal/directives/generating-content.md

## Acceptance Criteria

- [x] `heading-offset: "1"` demotes every embedded heading
      one level on `mdsmith fix`, even with no heading
      before the marker
- [x] `heading-offset: "-1"` promotes every embedded
      heading one level, clamped at H1
- [x] A stale `heading-offset` body reports the MDS021
      "generated section is out of date" diagnostic on
      `check`
- [x] Pairing `heading-offset` with `heading-level` or
      `extract` is a lint error with a clear message
- [x] A non-integer or out-of-range `heading-offset` is a
      lint error
- [x] No tracked Markdown uses the new syntax, so
      `mdsmith-fixed-version` stays green
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
