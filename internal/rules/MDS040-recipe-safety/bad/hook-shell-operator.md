---
settings:
  hooks-before:
    - command: "make start && make wait"
      name: "start and wait"
diagnostics:
  - line: 1
    column: 1
    message: 'hook "start and wait": command contains shell operator "&&" — use a wrapper script'
---

# Hook Shell Operator

A before-hook whose command contains a shell operator.
