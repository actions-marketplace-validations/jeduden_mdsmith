---
title: Extract Markdown as data
weight: 35
summary: >-
  When a Markdown file's payload is prose, put it in
  the body under H2 sections — not in YAML
  frontmatter. `mdsmith extract` projects body
  structure into a JSON tree the same way it
  projects frontmatter, so the file stays editable
  as Markdown.
---
# Extract Markdown as data

`mdsmith extract` projects a schema-conformant
Markdown file into a JSON / YAML / msgpack tree. Two
parts of the file feed the tree:

- **Frontmatter** — decoded YAML, written under a
  `frontmatter` key.
- **Body sections** — H1 / H2 / H3 headings and the
  content under them, projected as siblings.

This guide is about when to use which. Two modes share
the work: declared schema entries **constrain and
rename** the slice you name, while
[`projection: blocks`](#projecting-a-whole-section-body)
**captures everything else** — a section's whole body,
or, at the schema root, the whole document.

## The principle

Frontmatter is for the file's **metadata**: `title`,
`kind`, `status`, dates, cross-references, the
fields a non-prose tool (a workflow, a release
script, a status badge) reads alongside the prose.

Body sections are for the file's **payload**: the
prose, paragraphs, lists, and code blocks the file
exists to hold. If the value is a sentence or two of
copy, it belongs under a heading — not in a folded
YAML scalar.

The trap is to reach for frontmatter for everything
because it's structured. A 60-character `tagline` in
frontmatter and the same 60 characters in a
`## Tagline` body section project the same string;
only the key moves, from `frontmatter.tagline` to
`tagline.text`. The body version is shorter to edit,
diffs cleanly when wrapped, and is lintable as
Markdown.

## Worked example

A product-copy file at `docs/copy/product.md` with a
tagline, a lead, and one per-surface description.

### Frontmatter-heavy (the trap)

```markdown
---
title: Product copy
tagline: >-
  Mark down your ideas; smith them into shipping
  docs.
lead: >-
  A lint-and-fix tool that keeps your Markdown
  consistent across every surface — READMEs, docs
  site, editor extensions.
vscode-description: >-
  Inline diagnostics, fix-on-save, and instant
  navigation for Markdown in VS Code.
---
# Product copy

This file holds the tagline, lead, and VS Code
description. Edit a field above and re-run the
sync.
```

Three folded scalars (`>-`); the body is bookkeeping;
line breaks inside the scalars are cosmetic
(folded-strip collapses them to spaces); a leading
punctuation character in any value would force
double-quotes.

### Body-structured (the principle)

```markdown
---
title: Product copy
---
# Product copy

## Tagline

Mark down your ideas; smith them into shipping
docs.

## Lead

A lint-and-fix tool that keeps your Markdown
consistent across every surface — READMEs, docs
site, editor extensions.

## VS Code

Inline diagnostics, fix-on-save, and instant
navigation for Markdown in VS Code.
```

With a matching schema and kind assignment in
`.mdsmith.yml`:

```yaml
kinds:
  product-copy:
    schema:
      sections:
        - heading: { regex: '^Tagline$' }
          content:
            - { kind: paragraph }
        - heading: { regex: '^Lead$' }
          content:
            - { kind: paragraph }
        - heading: { regex: '^VS Code$' }
          bind: vscode-description
          content:
            - { kind: paragraph }
kind-assignment:
  - glob: ["docs/copy/product.md"]
    kinds: [product-copy]
```

Each `content:` entry declares the paragraph its
section projects. A section without one projects as
an empty object — the schema, not the body, decides
what `extract` emits.

`mdsmith extract product-copy --format json docs/copy/product.md`
emits:

```json
{
  "frontmatter": { "title": "Product copy" },
  "title": "Product copy",
  "lead": { "text": "A lint-and-fix tool that keeps your Markdown consistent across every surface — READMEs, docs site, editor extensions." },
  "tagline": { "text": "Mark down your ideas; smith them into shipping docs." },
  "vscode-description": { "text": "Inline diagnostics, fix-on-save, and instant navigation for Markdown in VS Code." }
}
```

The H1 `# Product copy` projects as the top-level
`title` string. Keys come out sorted, not in document
order. The consumer reads the same strings the
frontmatter version held, and the body version is the
editable artifact.

## Projecting inline structure

A paragraph projects as plain `text` by default. When
the consumer needs the structure *inside* the
paragraph — which fragment is emphasised, which token
is code, which span is a link — set
`projection: inline` on the content entry. The
paragraph then projects under an `inline` key as a
typed, recursive span list instead of a flat string.

The canonical case is a website headline whose hero
template renders one emphasised word from the data:

```markdown
---
title: Product copy
---
# Product copy

## Headline

Mark*down*, smithed.
```

```yaml
kinds:
  product-copy:
    schema:
      sections:
        - heading: { regex: '^Headline$' }
          content:
            - { kind: paragraph, projection: inline, required: true }
```

`mdsmith extract product-copy --format json docs/copy/product.md`
emits the headline as a span list: text, then the
level-1 emphasis span with its own `children`, then
the trailing text:

```json
{
  "frontmatter": { "title": "Product copy" },
  "headline": {
    "inline": [
      { "span": "text", "value": "Mark" },
      {
        "span": "emphasis", "level": 1,
        "children": [{ "span": "text", "value": "down" }]
      },
      { "span": "text", "value": ", smithed." }
    ]
  }
}
```

Nesting composes through the same shape: a paragraph
``run **`mdsmith fix`** daily`` projects the strong
span with the code span nested in its `children` —
the consumer walks one uniform tree, with no
flat-versus-recursive mode switch.

Leaf spans (text, code, autolink) carry a `value`;
container spans (emphasis, strong, link) carry
`children`. A wrapped line emits a `break` span between
the surrounding text spans (`hard: true` for a backslash
or double-space break). An image, inline raw HTML, or
any node outside that set is a hard error — the same
exit code as a non-conformant file. The full mapping
table is in the [extract reference][extract-inline].

[extract-inline]: ../reference/cli/extract.md#inline-span-projection

## Projecting list structure

A list projects as an array of own-text strings by
default. That flat shape loses nesting and strips a task
checkbox to a literal `[x]` prefix. When the consumer
needs the structure — which items are checked, which
nest children — set `projection: tree` on the list
entry. Each item then projects as an object with its own
`text`, a `checked` bool on task items, and a recursive
`children` array on items that nest a sub-list.

The canonical case is a sprint checklist whose
status tool reads `checked` and walks `children`:

```markdown
---
title: Sprint tasks
---
# Sprint tasks

## Tasks

- [x] done item
- [ ] open item with **bold**
  - nested child
- plain item
```

```yaml
kinds:
  checklist:
    schema:
      sections:
        - heading: { regex: '^Tasks$' }
          content:
            - { kind: list, projection: tree }
kind-assignment:
  - glob: ["tasks.md"]
    kinds: [checklist]
```

`mdsmith extract checklist --format json tasks.md` emits:

```json
{
  "frontmatter": { "title": "Sprint tasks" },
  "tasks": {
    "items": [
      { "checked": true, "text": "done item" },
      {
        "checked": false, "text": "open item with bold",
        "children": [{ "text": "nested child" }]
      },
      { "text": "plain item" }
    ]
  }
}
```

The `[x]` / `[ ]` marker becomes the `checked` bool and
never leaks into `text`; `**bold**` flattens to its
text; `nested child` rides inside its parent's
`children`, not concatenated into the parent string. A
plain item is just `{text}`. Array order is item order;
YAML and msgpack emit the same tree.

## Projecting a table as positional rows

A `kind: table` content entry projects as `rows` (an
array of record objects) by default. Set
`projection: rows` when the consumer needs column order
preserved, tolerates duplicate headers, or works with
positional data (a chart script, a CSV writer).

The `rows` projection injects two sibling keys into the
section object — `columns` (the header array) and `rows`
(string arrays, one per body row). Short rows are padded
with empty strings to the header width.

A benchmark table in a performance section:

```markdown
---
title: Benchmark results
---
# Benchmark results

## Latency

| Operation | p50 ms | p99 ms |
| --------- | ------ | ------ |
| check     | 12     | 45     |
| fix       | 18     | 70     |
```

```yaml
kinds:
  benchmark:
    schema:
      sections:
        - heading: { regex: '^Latency$' }
          content:
            - { kind: table, projection: rows }
kind-assignment:
  - glob: ["docs/benchmarks.md"]
    kinds: [benchmark]
```

`mdsmith extract benchmark --format json docs/benchmarks.md`
emits:

```json
{
  "frontmatter": { "title": "Benchmark results" },
  "latency": {
    "columns": ["Operation", "p50 ms", "p99 ms"],
    "rows": [["check", "12", "45"], ["fix", "18", "70"]]
  }
}
```

A chart script reads `latency.rows[0][1]` by index.

The default `records` projection emits the same
table as an array of objects keyed by column header.
The full projection matrix and duplicate-header
semantics are in the
[extract reference][extract-table].

[extract-table]: ../reference/cli/extract.md#table-projection-modes

## Projecting a whole section body

Everything above names a slice and constrains it. The
opposite need is to capture a section's *whole* body
without listing each node. Set `projection: blocks` on
the scope — or once at the schema root, as the default
for every section. The body projects as a typed,
recursive `blocks` list in document order: paragraphs,
code, lists, tables, quotes, and deeper headings (as
nested `section` blocks).

A schema-level switch projects a whole document in one
line. Declared sections still project as keyed objects,
and gain a `blocks` list. The sections the walker would
skip — wildcard and unlisted headings — now project
too. Each lands under its slug, its heading text in a
`heading` field:

```markdown
---
title: Release notes
---
# Release notes

## Summary

Ships **block projection**.

## Details

Two new switches.
```

```yaml
kinds:
  notebook:
    schema:
      projection: blocks
      sections:
        - heading: { regex: '^Summary$' }
kind-assignment:
  - glob: ["notes.md"]
    kinds: [notebook]
```

`mdsmith extract notebook --format json notes.md`
emits the H1 under `title`, the declared `summary`, and
the unlisted `details` (with its `heading` text):

```json
{
  "frontmatter": { "title": "Release notes" },
  "title": "Release notes",
  "summary": {
    "blocks": [{ "block": "paragraph", "text": "Ships block projection." }]
  },
  "details": {
    "heading": "Details",
    "blocks": [{ "block": "paragraph", "text": "Two new switches." }]
  }
}
```

A paragraph block defaults to flat `text`. Add
`block-paragraphs: inline` beside `projection: blocks`
to project each paragraph's span list under `inline`
instead — the same span shape as above, lenient about
images. The full grammar and its CUE contract are in
the [extract reference][extract-blocks].

[extract-blocks]: ../reference/cli/extract.md#block-projection-whole-body

## When frontmatter is the right call

- **Short scalars where YAML's typing earns its
  keep**: booleans (`draft: true`), dates
  (`published: 2026-05-24`), enums
  (`status: "✅"`), numbers.
- **Metadata other tools read**: `title`, `kind`,
  `weight`, `tags` — anything Hugo's
  frontmatter, a release script, or a status
  dashboard consumes directly.
- **Fields that participate in `<?catalog?>`
  directives**: catalog templating reads
  frontmatter keys (`{title}`, `{summary}`).
- **Strict, machine-controlled values**: a
  generated version stamp, a hash, a per-file
  identifier — values an automated tool writes
  and a human should not edit by hand.

Prose paragraphs, multi-line copy, anything wider
than one line, and anything that benefits from
Markdown formatting (code, emphasis, links) all
belong in the body.

## Frontmatter `title` and the H1

The worked example carries the same string twice:
`title: Product copy` in frontmatter and
`# Product copy` as the H1. Nothing checks the two
against each other by default, so they can drift
apart edit by edit.

The test from the previous section decides it. When
no catalog row, site template, or release script
reads `frontmatter.title`, delete the field; the H1
alone is the title. When a tool does read the
field, keep it and let MDS020 enforce the match.

### H1 title in the projection

When the schema roots at H2 (all inline schemas do),
`mdsmith extract` emits the document H1's plain text
under a reserved `title` key beside `frontmatter` —
the `"title": "Product copy"` line in the
[worked example](#worked-example) output above.
When there is no H1 the key is omitted, and a scope
that resolves to `title` (via slug or `bind:`) is
reported as a collision before any data is emitted;
rename the scope with `bind:` to resolve it.

An `<?include?>` with `extract: title` splices the
H1 plain text directly into a host file, exactly
like the `tagline.text` embed shown in
[Reading a value back](#reading-a-value-back-into-markdown).

### Enforcing H1 ↔ frontmatter consistency

Enforcement requires a file-based schema. An inline
`schema:` starts matching at H2 — the H1 belongs to
[first-line-heading][mds004] — so the kind switches
to a `proto.md` rooted at the `{title}` placeholder.
A `<?content?>` directive row in a section body
declares the same content entry the inline
`content:` list declares, so the worked example's
sections keep their `text` projections instead of
collapsing to empty objects:

```markdown
# {title}

## Tagline

<?content
kind: paragraph
?>

## Lead

<?content
kind: paragraph
?>
```

```yaml
kinds:
  product-copy:
    rules:
      required-structure:
        schema: copy-proto.md
```

The `{title}` row requires the frontmatter field
and checks the H1 text against its value. A drifted
H1 fails `mdsmith check`:

```text
docs/copy/product.md:4:1 MDS020 heading does not match frontmatter: expected "Product copy" (from title), got "Product page copy"
```

The synced H1 also becomes data: `mdsmith extract`
projects the H1 scope under a `title` object, the
captured heading text and each section's paragraph
nested inside it.

One limit remains. Every schema source on a file must
declare the same root level, so an H1-rooted `proto.md`
cannot compose with an H2-rooted inline schema. The H1
sync and the body extraction both live on the
`proto.md`. And mdsmith cannot project the H1 text
without a frontmatter field behind it — a `{title}` row
with no `title` field matches any heading, and `extract`
skips wildcard scopes — so keep the `title` field when
the kind syncs the H1.

[mds004]: ../../internal/rules/MDS004-first-line-heading/README.md

## `bind:` patterns

`bind:` renames the JSON key that a heading or
content entry projects under. Use it when the
human-readable heading and the consumer-friendly key
don't match.

- **Heading-to-key rename.** `## VS Code` slugs to
  `vs-code`; set `bind: vscode-description` so the
  consumer reads the field name its code expects.
- **Collapse a wrapper.** `bind: ""` on a parent scope
  hoists its children into the grandparent — for a
  heading that should not nest in the data tree.
- **Repeating sections.** A `repeat: {min, max}` section
  with a placeholder heading projects as an array;
  `bind:` renames the array key.

See [the section-schema reference][secref] for the
full grammar.

[secref]: ../reference/section-schema.md

## Reading a value back into Markdown

`mdsmith extract` writes the projection out for a
release script, a Hugo data file, or any non-Markdown
consumer. The read-side counterpart lives on the
`<?include?>` directive: its `extract:` parameter walks
the same JSON tree and splices one leaf into the host
file's Markdown body.

Re-using the product-copy example, a README embed reads
the tagline directly:

```markdown
<?include
file: docs/copy/product.md
extract: tagline.text
?>
Mark down your ideas; smith them into shipping docs.
<?/include?>
```

The directive runs the same projection rules, walks the
dotted path, and splices the leaf — no intermediate
"fragment" file to keep in sync. The supported paths,
content-key shortcuts, and lint-time errors are in
[generating-content.md](directives/generating-content.md#include-a-typed-value).

## See also

- [`mdsmith extract`][extract] — the CLI reference,
  including default projection rules per content
  entry type (code → `code`, list → `items`,
  table → `rows`, paragraph → `text` or `inline`).
- [Schemas guide][schemas] — declaring the kind
  schema that doubles as the extraction contract.
- [Generating Content with Directives][gen-content] —
  the `<?include ... extract:?>` read-side
  documentation.

[extract]: ../reference/cli/extract.md
[schemas]: schemas.md
[gen-content]: directives/generating-content.md
