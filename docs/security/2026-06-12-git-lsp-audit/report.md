---
date: '2026-06-12'
scope: 'Git integration and LSP server'
method: 'audit'
title: 'Git integration and LSP server audit'
summary: 'Two low-severity symlink and binary-lookup gaps in merge driver hook install; all critical baseline defenses confirmed intact.'
---
# mdsmith Security Review

- **Target:** jeduden/mdsmith @ `73dbde5dc34fc67f4eaddc2fa3cfb89e15327306`
- **Mode:** audit
- **Scope:** Git integration (merge driver, pre-merge-commit hook) and LSP server
- **Date:** 2026-06-12

## Summary

Critical: 0 | High: 0 | Medium: 0 | Low: 2 | Info: 3

| ID   | Sev  | Conf      | Title                                                                                    | Surface | Location                             |
| ---- | ---- | --------- | ---------------------------------------------------------------------------------------- | ------- | ------------------------------------ |
| S001 | low  | confirmed | Hook-file write and remove lack a prior lstat guard                                      | git     | `cmd/mdsmith/mergedriver.go:728-748` |
| S003 | low  | tentative | GOPATH fallback in resolveInstalledBinary can be steered by environment                  | git     | `cmd/mdsmith/mergedriver.go:818-821` |
| S002 | info | confirmed | shellQuote correctly escapes the merge driver binary path for git config shell expansion | git     | `cmd/mdsmith/mergedriver.go:769-779` |
| S004 | info | confirmed | Build-pass exec paths confirmed CLI-only — no LSP or merge-driver regression             | git     | `internal/build/builder.go:1-9`      |
| S005 | info | confirmed | LSP server: no executeCommand handler, three-layer panic recovery confirmed              | lsp     | `internal/lsp/server.go:343-358`     |

## Findings

### S001 · Hook-file write and remove lack a prior lstat guard

**Severity:** low · **Confidence:** confirmed · **Surface:** git · **CWE-363**

**Location:** `cmd/mdsmith/mergedriver.go:728-748`

- related: `cmd/mdsmith/premergecommit.go:137`
- related: `cmd/mdsmith/premergecommit.go:157`
- related: `cmd/mdsmith/premergecommit.go:186`

**What.** ensurePreMergeCommitHook (mergedriver.go:728) calls os.ReadFile then os.WriteFile on hookPath without a prior os.Lstat guard. runPreMergeCommitUninstall (premergecommit.go:137,157) and runPreMergeCommitStatus (premergecommit.go:186) do the same. If .git/hooks/pre-merge-commit is a symlink, ReadFile/WriteFile/Remove all follow it. WriteGitattributes (githooks.go:703) applies lstatFile before every I/O and uses temp-then-rename so the write cannot follow a late-introduced symlink. The hook paths are missing that pattern.

**Impact.** A hostile repo shipping a symlink at .git/hooks/pre-merge-commit can redirect the WriteFile to an arbitrary path, overwriting a file outside the workspace at install time. Remove can delete an arbitrary file. Triggered only when the user explicitly runs 'mdsmith merge-driver install' or 'mdsmith pre-merge-commit install' — not during a routine git merge.

**Repro (sketch).** Create a repo with .git/hooks/pre-merge-commit as a symlink to ~/target. Run 'mdsmith merge-driver install'. The hook script is written to ~/target.

**Fix.** Apply the WriteGitattributes pattern: lstat before I/O, reject if not a regular file, and write via temp-then-rename instead of os.WriteFile. For Remove, lstat and reject if not a regular file before calling os.Remove.

### S003 · GOPATH fallback in resolveInstalledBinary can be steered by environment

**Severity:** low · **Confidence:** tentative · **Surface:** git · **CWE-426**

**Location:** `cmd/mdsmith/mergedriver.go:818-821`

- related: `cmd/mdsmith/mergedriver.go:859`

**What.** resolveInstalledBinary falls back to $GOPATH/bin/mdsmith (via 'go env GOPATH', line 859) when os.Executable() and exec.LookPath both fail. If $GOPATH is attacker-controlled (poisoned CI, repo .envrc), the fallback resolves an attacker binary. That path is written into git config as the merge driver, so subsequent merges invoke it. The fallback is only reached in abnormal install scenarios where both primary lookups fail.

**Impact.** If triggered, every subsequent git merge in the repository runs the attacker's binary as the merge driver — RCE on routine merges.

**Repro (sketch).** Set GOPATH=/attacker-dir with a malicious binary at /attacker-dir/bin/mdsmith; arrange os.Executable() to return a non-existent path and 'mdsmith' to be absent from PATH. Run 'mdsmith merge-driver install'. The malicious binary is registered.

**Fix.** Remove the GOPATH fallback; os.Executable() and LookPath cover all normal deployments. If the fallback is kept, verify the resolved path resides in the same directory as the running binary before accepting it.

## Hardening / Informational

### S002 · shellQuote correctly escapes the merge driver binary path for git config shell expansion

**Severity:** info · **Confidence:** confirmed · **Surface:** git · **CWE-78**

**Location:** `cmd/mdsmith/mergedriver.go:769-779`

**What.** registerMergeDriver builds 'shellQuote(exe) + " merge-driver run %O %A %B %P"' and writes it to git config. git later passes this to /bin/sh. shellQuote (lines 853-855) wraps the path in single quotes and escapes embedded single quotes as '\'''. The escaping is correct for all representable POSIX paths. This is an informational confirmation that the load-bearing control has been reviewed.

**Impact.** None — the shellQuote is correct and prevents injection.

**Repro (sketch).** N/A — not a defect.

**Fix.** No action required. Consider a unit test that asserts shellQuote of a path with single quotes, spaces, and dollar signs produces a string that /bin/sh eval yields the original path.

### S004 · Build-pass exec paths confirmed CLI-only — no LSP or merge-driver regression

**Severity:** info · **Confidence:** confirmed · **Surface:** git

**Location:** `internal/build/builder.go:1-9`

- related: `internal/build/builder.go:254`
- related: `internal/build/hooks.go:90`
- related: `cmd/mdsmith/buildpass.go:23`

**What.** internal/build exec.CommandContext calls (builder.go:254, hooks.go:90) run user-declared recipes and hooks. The package doc states the build pass is CLI-only and excluded from LSP and merge-driver paths. Verified: the only callers of NewCustomBuilder and RunHooks/RunAfterHooks are in cmd/mdsmith/buildpass.go. The LSP fix-on-save path calls sess.Fix (pkg/mdsmith Session), which does not import internal/build. The merge driver calls fixpkg.Source directly — also no exec. The threat-model baseline holds.

**Impact.** None — baseline confirmed intact.

**Repro (sketch).** N/A — not a defect.

**Fix.** No action required. Consider an integration test asserting the LSP Session.Fix call chain contains no internal/build import.

### S005 · LSP server: no executeCommand handler, three-layer panic recovery confirmed

**Severity:** info · **Confidence:** confirmed · **Surface:** lsp

**Location:** `internal/lsp/server.go:343-358`

**What.** workspace/executeCommand is absent from the dispatch table (server.go:548-559); unrecognised methods return method-not-found. Code actions (quickFix, source.fixAll.mdsmith) return WorkspaceEdit in-band via sess.Fix — in-process only. dispatchRaw (server.go:343) defers a recover() per frame, logging via window/logMessage and settling pending calls. Lint goroutines and fetchClientSettings have their own recover() guards. A rule panic on attacker content cannot crash or hang the server.

**Impact.** None — positive findings confirming expected security properties.

**Repro (sketch).** N/A — not a defect.

**Fix.** No action required.

## Coverage

Reviewed: cmd/mdsmith/mergedriver.go, cmd/mdsmith/premergecommit.go, internal/build/builder.go, internal/build/hooks.go, cmd/mdsmith/buildpass.go, internal/githooks/githooks.go, and the full internal/lsp package (server.go, server_lifecycle.go, server_documents.go, server_codeaction.go). Grepped all exec.Command calls in internal/ and cmd/. Not covered: VS Code extension (2026-06-12-editor-extensions-audit), Obsidian plugin, distribution wrappers, CUE/YAML evaluation, workspace-walk symlink deny, rename write-safety, ReDoS in rules.
