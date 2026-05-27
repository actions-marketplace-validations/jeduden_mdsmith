---
title: Coexist with Prettier
summary: >-
  Prettier owns paragraph wrapping and final table
  layout; mdsmith owns lint, generated sections, and
  cross-file checks. Run `mdsmith fix` first and
  `prettier --write` last so a second pass produces
  zero diffs.
---
# Coexist with Prettier

Prettier and mdsmith both rewrite GFM table padding and
list-item indentation — run them in the wrong order
and each pass undoes the other's column alignment.
Pin a fixed order in one pre-commit hook: `mdsmith
fix` first (it owns whitespace, heading style, code
fences, bare URLs, generated sections, and cross-file
checks), then `prettier --write` last to settle
paragraph wrapping and the final table layout. Once
that is wired up, a second run of either tool produces
zero diffs.

## Who owns what

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

Under Prettier's default `--prose-wrap preserve`,
Prettier leaves an mdsmith-aligned table's column
widths intact. Running mdsmith first, Prettier last
converges in one round.

## Prettier config

Default Prettier and default mdsmith line up for the
common settings:

- Prettier's `proseWrap: "preserve"` leaves the line
  breaks mdsmith's `line-length` rule already sized
  (both default to 80 columns).
- Prettier's `tabWidth: 2` matches `list-indent`'s
  default of `spaces: 2`.
- Prettier's `-` unordered-list marker matches
  `list-marker-style: dash`.

If you raise mdsmith's `line-length.max`, raise
Prettier's `printWidth` to the same number — otherwise
Prettier will rewrap lines mdsmith still considers
within budget.

## Generated sections

Prettier does not parse mdsmith's `<?...?>` directive
markers and may rewrap text inside generated bodies.
If that shows up in diffs, add the affected files to
`.prettierignore`. Generated content always
regenerates from the directive source on the next
`mdsmith fix`, so the worst case is a one-commit
round-trip — never hand-edit generated bodies.

## Pre-commit hook

With `lint-staged` and `husky`:

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

With a plain Git hook (`.husky/pre-commit`):

```sh
list=$(git diff --cached --name-only --diff-filter=ACMR -z -- '*.md')
[ -z "$list" ] && exit 0
printf '%s' "$list" | xargs -0 mdsmith fix --
printf '%s' "$list" | xargs -0 git add --
printf '%s' "$list" | xargs -0 npx prettier --write --
printf '%s' "$list" | xargs -0 git add --
```

The script is POSIX sh, uses NUL-delimited file lists
so filenames with spaces survive, and exits early on
an empty stage so neither tool falls back to a
full-repo rewrite.

## CI check

```yaml
- name: prettier check
  run: npx prettier --check '**/*.md'
- name: mdsmith check
  run: mdsmith check .
```

Order does not matter on read-only CI — both jobs only
report violations. Run them in parallel.

## When to drop a tool

- Drop **Prettier** if you do not need paragraph
  re-wrap and are willing to let mdsmith's
  `line-length` rule flag (not rewrite) over-long
  lines.
- Drop **mdsmith** if your repo has no generated
  sections, no cross-file link integrity needs, and no
  readability budgets — Prettier alone covers
  formatting for prose-only repos.

## See also

- [Auto-fix](../features/auto-fix.md) — what
  `mdsmith fix` rewrites.
- [Migrate from markdownlint](migrate-from-markdownlint.md)
  — if you used markdownlint + Prettier before.
