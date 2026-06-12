---
settings:
  hooks-before:
    - command: "make dev-server-start"
      name: "start dev server"
    - command: "scripts/wait-for-port {port}"
      params:
        port: "3000"
  hooks-after:
    - command: "make dev-server-stop"
      name: "stop dev server"
---
# Safe Hooks

Before-hooks that start a dev server and wait for it, plus an after-hook
that stops it. All commands use direct binaries — no shell operators or
reserved placeholders.
