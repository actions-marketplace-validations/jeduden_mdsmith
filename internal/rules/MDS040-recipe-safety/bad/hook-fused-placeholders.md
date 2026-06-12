---
settings:
  hooks-after:
    - command: "tool {a}{b}"
      name: "stop server"
      params:
        a: "x"
        b: "y"
diagnostics:
  - line: 1
    column: 1
    message: 'hook "stop server": command contains fused placeholders "{a}{b}" — separate with a delimiter'
---

# Hook Fused Placeholders

An after-hook whose command contains two adjacent placeholders without a delimiter.
