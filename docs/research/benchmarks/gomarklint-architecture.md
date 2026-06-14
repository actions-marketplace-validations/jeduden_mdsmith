---
summary: >-
  Why mdsmith-parity trails gomarklint on benchmark 2: gomarklint
  is a line scanner that never builds an AST, while 27 of
  parity's 30 rules force mdsmith's CommonMark parse — the
  ~35% of parity wall time that is the whole gap. Reviews
  gomarklint's architecture and records the optimization
  levers and their ceilings.
---
# gomarklint architecture and the parity gap

This page answers a single question: on benchmark 2 (the neutral
corpus — 234 Rust Book + Reference files), why does
`mdsmith-parity` run at roughly 1.8x gomarklint's wall time, and
what can close that gap?

It is a research note, not a tuning changelog. The headline
finding is architectural and does not move with micro-optimization:
**gomarklint never parses Markdown, and 27 of parity's 30 rules
force mdsmith to.**

## gomarklint in one paragraph

gomarklint (`shinagawa-web/gomarklint`, v3.2.3 — the pinned
benchmark binary) is a line scanner. `collectErrors` strips front
matter, runs `lines := strings.Split(body, "\n")` once, and hands
that `[]string` to every rule. Each rule is a plain function with
the shape:

```go
func CheckMaxLineLength(path string, lines []string, offset int, ...) []LintError
```

There is no CommonMark parse, no AST, and no node tree — ever.
Fenced-code state, heading levels, and list markers are tracked by
walking the lines with byte comparisons. A cheap prefilter,
`firstNonSpaceByte`, finds the first non-space byte of a line so
`strings.TrimSpace` only runs on lines that could match a rule.
Rules reach for `strings.HasPrefix` / `bytes.IndexByte` /
direct byte indexing rather than `regexp` in their hot paths.

Concurrency is one goroutine per file (`go func(p string)` in a
loop over the deduped path set), with a single mutex guarding result
aggregation. The external-link checker — the one rule that would
dominate — is off by default, so the default run is pure in-process
line scanning. There is no on-disk cache, which is why the benchmark
gives gomarklint no `--no-cache` flag.

That is the entire performance story: split into lines once, scan
the lines with byte ops, fan out per file. It is fast because it does
structurally less than any AST linter can.

## The measured difference

Wall-clock medians on the real 234-file neutral corpus. The
absolute numbers below are from a 4-core dev box and run higher than
the published page (different hardware); the **ratios and the
profile percentages are what transfer**, and they match the
published `gomarklint 18 ms / parity 31 ms / full 81 ms`.

| Run                                    | median  | vs gomarklint |
| -------------------------------------- | ------- | ------------- |
| gomarklint                             | ~40 ms  | 1.0x          |
| mdsmith-parity (`-c parity`)           | ~74 ms  | ~1.8x         |
| mdsmith default                        | ~105 ms | ~2.6x         |
| mdsmith repo-config (published "full") | ~170 ms | ~4.0x         |

CPU profile of the **parity** run (the apples-to-apples comparison),
share of total samples:

| Bucket                | share     | what it is                            |
| --------------------- | --------- | ------------------------------------- |
| `goldmark` parse      | ~35%      | block + inline CommonMark parse       |
| rules                 | ~36%      | the 30 enabled structural rules       |
| read + per-file setup | ~10%      | file I/O, front matter, FS, gitignore |
| merge / sort / walk   | remainder | result assembly, workspace walk       |

The single biggest cost in the parity run is the parse, and
gomarklint pays none of it. Within the parse, block parsing
(`parseBlocks` → `openBlocks`/`closeBlocks`) is ~23% and inline
parsing (`walkBlock`) is ~11%. Individual rules are each cheap —
the costliest, `atx-heading-whitespace` (MDS064), is ~7%, and most
of that is the shared code-block-line walk it happens to trigger
first, not the rule's own line scan.

## Why parity cannot skip the parse

The obvious idea — parse lazily, and skip goldmark entirely for the
cheap structural rules the way gomarklint does — does not help the
parity config. **27 of parity's 30 active rules require the AST.**
Only three are pure line scanners (`single-trailing-newline`,
`unique-frontmatter`, `no-trailing-punctuation-in-heading`).

The other 27 either implement `rule.NodeChecker` (driven by the
shared AST walk) or read `f.AST` / link references / code-block line
sets directly: `line-length` skips fenced code via the AST,
`no-bare-urls` and `link-validity` need parsed links,
`no-unused-link-definitions` and `no-undefined-reference-labels`
need goldmark's link-reference map, `list-marker-space` and
`blockquote-whitespace` walk nodes, and so on. A lazy AST is built
the moment any one of them runs — and in parity, they all run.

So the parse is not incidental overhead that better engineering can
remove. It is load-bearing for the rules parity keeps, and it is the
foundation for everything mdsmith does that gomarklint cannot:
cross-file link integrity, generated sections, schemas, rename, and
markdown-as-data. The ~35% parse cost is the architectural price of
that model.

### A note on the "full" benchmark number

The published `mdsmith = 81 ms` is partly a methodology artifact,
not a pure measure of mdsmith's defaults. The harness invokes
`mdsmith check $corpus` from the repository root, so config discovery
walks up and finds mdsmith's own `.mdsmith.yml` and applies it to the
neutral corpus — including the opt-in, Punkt-segmenter-heavy MDS024
`paragraph-structure`, which mdsmith's defaults leave **off**
precisely because the trained sentence tokenizer costs ~20% of wall
time on prose. Every other tool in the comparison runs with its own
defaults. A defaults-vs-defaults run drops mdsmith's number
substantially (~81 → ~50 ms estimated) with no code change. This is
a fairness gap in the comparison, not a regression in mdsmith;
`mdsmith-parity` already sidesteps it by selecting an explicit config.

## Optimization levers and their ceilings

What actually moves the parity number, ranked by realism.

### Allocation tuning — done, but marginal for wall time

The parse's allocation pressure (`mallocgc` + zeroing is ~20% of
CPU) suggested an allocation win. The per-parse slab arena
(`pkg/goldmark/arena`, plans 197/198) already absorbs Text,
Paragraph, Segments, CodeSpan, Link, and Emphasis nodes but **not**
Heading or ListItem — which the heading- and list-dense Rust corpus
allocated on the heap, ~8,200 objects per run.

Extending the arena to `Heading` and `ListItem` (this change)
removes those allocations cleanly: `ast.NewHeading` and
`ast.NewListItem` no longer appear in the allocation profile, the
arena-vs-non-arena equivalence gate still passes, and the per-file
alloc budget drops. But wall time barely moved (within run-to-run
noise). The lesson is decisive: **parity's wall time is dominated by
parse and rule computation, not by allocation.** Small structural
nodes are cheap to allocate; cutting them helps GC pressure under the
concurrent file pool but is not a path to gomarklint's number.

### Faster goldmark parse — high risk, low confidence

The remaining parse cost is goldmark's own block and inline parsing
loops. The top allocators inside it are on the link path
(`blockReader.Value`'s per-link byte copy, Unicode case folding of
reference labels) — both load-bearing for correctness and shared by
every inline parser, so making them zero-copy risks aliasing the
source buffer that callers expect to own. This is deep surgery on a
vendored fork that 15+ prior performance plans have already tuned;
it is not a safe single-session change.

### Line-scan fast-path — large project, and it skips parity anyway

The only design that fully closes the gap is gomarklint's: run the
structural rules as line scanners and build the AST lazily, only when
an AST-requiring rule is enabled. It would help a default-style run
that enables mostly line rules — but it **does not help parity**,
whose 27 AST-requiring rules force the parse regardless. Pursuing it
is a multi-PR architectural effort justified by the broad rule set,
not by this benchmark.

## Conclusion

`mdsmith-parity` trails gomarklint because it builds a CommonMark AST
that gomarklint never builds, and parity's rules require that AST.
The parse is ~35% of parity's wall time and is irreducible for the
parity rule set; the rest is spread thinly across already-tuned
rules. Allocation tuning (the arena extension that ships with this
note) is correct and worth keeping for GC pressure, but it does not
close a wall-time gap that is fundamentally compute, not garbage.

The honest target is therefore not "parity == gomarklint" — that
asks an AST linter to match a line scanner on the line scanner's
terms — but "keep parity in the same class as the Rust markdownlint
ports (mado, rumdl) while doing strictly more," which it already
does.
