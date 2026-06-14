---
id: 2606141903
title: "Lazy parse: BlockSpan seam for block NodeCheckers"
status: "🔲"
summary: >-
  Run the block-kind NodeChecker rules over the Layer 0
  BlockSpan model instead of the heap AST, so parity's
  structural rules no longer force the parse. Breaks the
  one coupling that ties those rules to the tree: the
  `n.(*ast.Heading)` assertion in CheckNode.
model: opus
depends-on: [2606141902]
---
# Lazy parse: BlockSpan seam for block NodeCheckers

## Goal

Serve the block-kind NodeChecker rules from Layer 0. They
should read a block's kind and line span without a node
tree. Parity's structural rules then stop forcing the
parse.

## Background

See the [seam audit][research]. All 25 NodeChecker rules
are `KindScopedChecker`. Each declares the node kinds it
reacts to. About 15 scope to block kinds.

The block CheckNodes read very little. A typical one
narrows the node with `heading, ok := n.(*ast.Heading)`.
Then it reads the heading's line and works on `f.Lines`.
That type assertion is the only tie to the heap tree.

Present each block as a flat `BlockSpan`. It carries the
kind, the line range, and the nesting. Adapt the block
CheckNodes to read from it. The change is mechanical:
they already read only kind and position.

## Tasks

1. Define the `BlockSpan` view (kind, line range,
   nesting) over the Layer 0 scan.
2. Add a shared dispatch that drives `KindScopedChecker`
   rules over `BlockSpan` for block kinds.
3. Migrate the block-kind CheckNodes off
   `n.(*ast.Heading)` and onto `BlockSpan`.
4. Extend the parse-skip gate to cover these rules.

## Acceptance Criteria

- [ ] Parity's block NodeCheckers run with no parse when
      no Layer 1 or Layer 2 rule is enabled.
- [ ] Each migrated rule's diagnostics are byte-identical
      to its AST output across the corpus and fixtures.
- [ ] All existing rule fixtures pass unchanged.
- [ ] The equivalence gate is green.
- [ ] All tests pass: `go test ./...`

[research]: ../docs/research/benchmarks/lazy-parse-architecture.md
