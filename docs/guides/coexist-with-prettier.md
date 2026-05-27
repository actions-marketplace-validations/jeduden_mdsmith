---
title: Coexist with Prettier
summary: >-
  If your project already runs Prettier on Markdown,
  mdsmith slots in alongside it: keep your Prettier
  config, run `mdsmith fix` before `prettier --write`
  in the same pre-commit hook, and the two tools
  converge on the same bytes.
---
# Coexist with Prettier

You already run Prettier on your Markdown and do not
want to give it up — mdsmith does not ask you to. Add
`mdsmith fix` to your pre-commit hook *before*
`prettier --write` and the two tools settle on the
same bytes. Prettier keeps the final say on paragraph
wrapping and table layout; mdsmith handles the
formatting rules Prettier does not touch, plus
generated sections, cross-file links, and readability
budgets.

## Quick start

```json
{
  "lint-staged": {
    "*.md": [
      "mdsmith fix",
      "prettier --write"
    ]
  }
}
```

That is the minimum: mdsmith first, Prettier last, in
one `lint-staged` array. A second run of either tool
produces zero diffs. The rest of this page is for
when something looks off — which tool to blame, what
to set in Prettier, and the one edge case around
generated content.

## Which tool owns what

When a check fails or a fixer rewrites something you
did not expect, this table points you at the tool to
configure:

| Concern                              | Owner    |
| ------------------------------------ | -------- |
| Final paragraph wrapping             | Prettier |
| Final table alignment                | Prettier |
| Trailing whitespace, hard tabs       | mdsmith  |
| Heading style (atx vs. setext)       | mdsmith  |
| Fenced-code style and language tag   | mdsmith  |
| Bare URLs                            | mdsmith  |
| Generated sections (catalog, toc)    | mdsmith  |
| Cross-file link and anchor integrity | mdsmith  |
| Readability budgets                  | mdsmith  |

The only place the two overlap is GFM table padding
and list-item indentation — both tools rewrite those
bytes, which is why the ordering rule exists.
Prettier runs last on those constructs and its
formatter happens to produce the same column widths
mdsmith's `table-format` does, so the second pass is
a no-op.

## Do you need to change your Prettier config?

Mostly no. Default Prettier and default mdsmith line
up out of the box: both target 80 columns, both
indent with two spaces, both normalize unordered list
markers to `-`, and Prettier's `proseWrap: "preserve"`
(its default) leaves alone the line breaks mdsmith's
`line-length` rule already sized — `proseWrap`
controls paragraph reflow, not table layout.

Two cases worth checking:

- If you have raised mdsmith's `line-length.max` past
  80, raise Prettier's `printWidth` to the same
  number. Otherwise Prettier will rewrap lines
  mdsmith still considers within budget and you will
  see churn on every commit.
- If you have enabled mdsmith's `list-marker-style`
  (MDS045, opt-in), set `style: dash` so it agrees
  with Prettier's default `-` marker.

## What about generated sections?

Prettier does not parse mdsmith's `<?...?>` directive
markers and may rewrap text inside generated bodies.
If a Prettier-induced rewrap shows up in your diffs,
add the affected files to `.prettierignore`.
Generated bodies regenerate from the directive source
on the next `mdsmith fix`, so the worst case is a
one-commit round-trip. Never hand-edit content
between `<?directive?>` and `<?/directive?>` markers.

## Plain Git hook

If you are not on `husky` / `lint-staged`, the same
ordering works in a hand-written `.husky/pre-commit`:

```sh
list=$(git diff --cached --name-only --diff-filter=ACMR -z -- '*.md')
[ -z "$list" ] && exit 0
printf '%s' "$list" | xargs -0 mdsmith fix --
printf '%s' "$list" | xargs -0 git add --
printf '%s' "$list" | xargs -0 npx prettier --write --
printf '%s' "$list" | xargs -0 git add --
```

POSIX sh; NUL-delimited so filenames with spaces
survive; exits early on an empty stage so neither
tool falls back to a full-repo rewrite.

## CI check

Both tools have read-only modes for CI:

```yaml
- name: prettier check
  run: npx prettier --check '**/*.md'
- name: mdsmith check
  run: mdsmith check .
```

Order does not matter here — both jobs only report
violations. Run them in parallel.

## Could you simplify by dropping one?

You almost certainly want to keep Prettier — it owns
paragraph re-wrap, which mdsmith deliberately does
not do. The real question is whether you need
mdsmith.

Keep mdsmith if your repo relies on any of:

- Generated sections (`<?catalog?>`, `<?include?>`,
  `<?toc?>`, `<?build?>`).
- Cross-file link or anchor integrity checks.
- Per-file kinds, schemas, or readability budgets.
- Release-gating on Markdown metrics.

Drop mdsmith if you have none of those and your only
need is wrap-and-tidy on prose — Prettier alone
covers that.

## See also

- [Auto-fix](../features/auto-fix.md) — what
  `mdsmith fix` rewrites.
- [Migrate from markdownlint](migrate-from-markdownlint.md)
  — if you used markdownlint + Prettier before.
