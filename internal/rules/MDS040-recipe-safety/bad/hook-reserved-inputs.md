---
settings:
  hooks-before:
    - command: "tool {inputs}"
diagnostics:
  - line: 1
    column: 1
    message: 'hook "before[0]": command references {inputs} which is not available in hooks (hooks have no directive context)'
---

# Hook Reserved Inputs

A before-hook that references the reserved {inputs} collective placeholder,
which is only available in recipe directives and has no meaning in hooks.
