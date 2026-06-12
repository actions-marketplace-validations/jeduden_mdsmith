---
settings:
  hooks-before:
    - command: "bash start.sh"
      name: "start server"
diagnostics:
  - line: 1
    column: 1
    message: 'hook "start server": command uses shell interpreter "bash" — use the direct binary'
---

# Hook Shell Interpreter

A before-hook that uses bash as the first token.
