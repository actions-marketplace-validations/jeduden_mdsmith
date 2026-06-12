---
date: "2026-06-12"
scope: "full repo — all seven threat-model surfaces"
method: "audit"
title: "mdsmith security audit — 2026-06-12"
summary: "Full-repo audit at 73dbde5. One medium finding (komac download without checksum in release.yml), four low findings (MDS040 gate bypass, hook-file lstat gap, GOPATH fallback, rename symlink write-through), and two informational. Prior S001/S002/S003 findings confirmed fixed."
---
# mdsmith Security Review

- **Target:** jeduden/mdsmith @ `73dbde5dc34fc67f4eaddc2fa3cfb89e15327306`
- **Mode:** audit
- **Scope:** full repo — all seven threat-model surfaces
- **Date:** 2026-06-12

## Summary

Critical: 0 | High: 0 | Medium: 1 | Low: 4 | Info: 2

| ID   | Sev    | Conf      | Title                                                                                           | Surface     | Location                                                                  |
| ---- | ------ | --------- | ----------------------------------------------------------------------------------------------- | ----------- | ------------------------------------------------------------------------- |
| S001 | medium | confirmed | komac binary downloaded and executed without SHA256 checksum verification                       | supplychain | `.github/workflows/release.yml:1015-1022`                                 |
| S002 | low    | confirmed | MDS040 (recipe-safety) gate passes when recipe-safety is disabled by attacker-controlled config | cli         | `cmd/mdsmith/buildpass.go:50-84`                                          |
| S003 | low    | confirmed | Hook-file write and remove lack a prior lstat guard — symlink redirects write to arbitrary path | git         | `cmd/mdsmith/mergedriver.go:728-748`                                      |
| S004 | low    | tentative | GOPATH fallback in resolveInstalledBinary can be steered by attacker-controlled environment     | git         | `cmd/mdsmith/mergedriver.go:818-821`                                      |
| S007 | low    | confirmed | rename command writes through symlinks when follow-symlinks is enabled                          | cli         | `cmd/mdsmith/rename.go:356-362`                                           |
| S006 | info   | confirmed | Threat model §0 baseline statement 'recipes are not executed by mdsmith' is incorrect           | cli         | `.claude/skills/mdsmith-security-review/references/threat-model.md:29-32` |
| S005 | info   | tentative | Obsidian plugin configPath passed to vault adapter without path-traversal validation            | obsidian    | `editors/obsidian/src/main.ts:210-223`                                    |

## Findings

### S001 · komac binary downloaded and executed without SHA256 checksum verification

**Severity:** medium · **Confidence:** confirmed · **Surface:** supplychain · **CWE-494**

**Location:** `.github/workflows/release.yml:1015-1022`

**What.** The winget-submit job downloads the komac binary from GitHub Releases with curl and immediately marks it executable and runs it, with WINGET_PR_TOKEN in scope, without any SHA256 or signature check after the download.
Every other binary download in the repository (tinygo in ci.yml:563-568, VHS/ttyd/ffmpeg in record-demo.yml, mdsmith itself in setup-mdsmith-pinned-version/action.yml) has a hardcoded SHA256 verified with `sha256sum -c` before execution.
The winget-submit job runs inside the `release` environment, which requires human approval, but the binary is fetched and executed within that environment's privileged step.

**Impact.** A MitM attacker or a compromised GitHub release asset for the pinned komac version could substitute a malicious binary that runs with the WINGET_PR_TOKEN. The token has fork+PR rights on microsoft/winget-pkgs. Exploitation requires breaking TLS or compromising the GitHub release asset at the specific version tag.

**Repro (sketch).** Pin a hostile binary at the expected komac release URL (or intercept TLS). Run a release dispatch. The substituted binary executes with WINGET_PR_TOKEN in scope — no manual step can detect the substitution since no checksum is verified.

**Fix.** Add a hardcoded SHA256 and a `sha256sum -c` check immediately after the curl line, matching the pattern used by tinygo in ci.yml:563-568: `echo "<sha256>  /usr/local/bin/komac" | sha256sum -c`. Optionally cosign-verify the release asset as well.

### S002 · MDS040 (recipe-safety) gate passes when recipe-safety is disabled by attacker-controlled config

**Severity:** low · **Confidence:** confirmed · **Surface:** cli · **CWE-693**

**Location:** `cmd/mdsmith/buildpass.go:50-84`

**What.** checkMDS040Gate (buildpass.go:50-84) returns true (gate open, build pass proceeds) when `recipe-safety` is absent from cfg.Rules or is disabled: `if !ok || !rc.Enabled { return true }`.
An attacker-controlled .mdsmith.yml can set `recipe-safety: false`, causing the gate to open with no validation of the recipe commands. The gate is the sole runtime check before executing recipes.
When disabled, recipes with shell interpreters (sh, bash, etc.) — normally blocked by MDS040 — will execute via exec.CommandContext during `mdsmith fix`.
The build pass is explicitly CLI-only (not LSP/WASM/merge-driver/hook), so this requires the user to run `mdsmith fix` on an untrusted repo. The pre-merge-commit hook passes --no-build, so git merge is not affected.

**Impact.** A user who runs `mdsmith fix .` on a freshly cloned hostile repo executes recipe commands even when those commands would violate MDS040's shell-safety rules. Analogous to `npm install` running postinstall scripts in a hostile package, but the user must explicitly run `fix` not `check`.

**Repro (sketch).** Create a .mdsmith.yml with `recipe-safety: false` and a recipe whose command is `sh -c 'touch /tmp/pwned'`. Add a <?build?> directive to a Markdown file referencing that recipe. Run `mdsmith fix`. The command executes.

**Fix.** Option A: make the MDS040 gate non-bypassable — when recipes are defined, always run the shell-safety check regardless of the recipe-safety rule toggle. Option B: document the build pass behavior prominently in a user-facing warning when recipes are present in an unfamiliar repo.

### S003 · Hook-file write and remove lack a prior lstat guard — symlink redirects write to arbitrary path

**Severity:** low · **Confidence:** confirmed · **Surface:** git · **CWE-363**

**Location:** `cmd/mdsmith/mergedriver.go:728-748`

- related: `cmd/mdsmith/premergecommit.go:137`
- related: `cmd/mdsmith/premergecommit.go:157`
- related: `cmd/mdsmith/premergecommit.go:186`

**What.** ensurePreMergeCommitHook (mergedriver.go:728) calls os.ReadFile then os.WriteFile on hookPath without a prior os.Lstat guard that rejects symlinks. runPreMergeCommitUninstall (premergecommit.go:137,157) and runPreMergeCommitStatus (premergecommit.go:186) do the same.
If .git/hooks/pre-merge-commit is a symlink placed by a hostile repo, ReadFile follows it to read arbitrary content, and WriteFile overwrites the symlink target.
WriteGitattributes (internal/githooks/githooks.go:703) applies an lstatFile guard before every I/O and uses temp-then-rename — the hook paths are missing that pattern.
Triggered only when the user explicitly runs `mdsmith merge-driver install` or `mdsmith pre-merge-commit install`, not during a routine `git merge`.

**Impact.** A hostile repo with a symlink at .git/hooks/pre-merge-commit can cause `mdsmith merge-driver install` to overwrite an arbitrary file on the user's system (the symlink target) with the hook script content. Remove can delete an arbitrary file.

**Repro (sketch).** Create a repo with `.git/hooks/pre-merge-commit` as a symlink to `~/victim-file`. Run `mdsmith merge-driver install`. The hook script is written to `~/victim-file`.

**Fix.** Apply the WriteGitattributes pattern: call os.Lstat before every read/write/remove on hookPath; if the path exists and is not a regular file, return an error. For WriteFile, use a temp-file-then-rename approach as WriteGitattributes does.

### S004 · GOPATH fallback in resolveInstalledBinary can be steered by attacker-controlled environment

**Severity:** low · **Confidence:** tentative · **Surface:** git · **CWE-426**

**Location:** `cmd/mdsmith/mergedriver.go:818-821`

- related: `cmd/mdsmith/mergedriver.go:859`

**What.** resolveInstalledBinary (mergedriver.go:797) falls back to $GOPATH/bin/mdsmith (via `go env GOPATH`) when both os.Executable() and exec.LookPath('mdsmith') fail. If $GOPATH is attacker-controlled — via a poisoned CI environment, a hostile `.envrc`, or a compromised tool that sets $GOPATH — the fallback resolves and registers a malicious binary as the git merge driver. The path written to git config is then used on every future `git merge` in the repo. The fallback is reached only in abnormal install scenarios where both primary lookups fail.

**Impact.** If triggered, every subsequent `git merge` in the repository invokes the attacker-controlled binary as the merge driver — persistent RCE on routine merges.

**Repro (sketch).** Set GOPATH to a directory containing a malicious mdsmith binary; arrange that os.Executable() returns a transient-looking path and 'mdsmith' is absent from PATH. Run `mdsmith merge-driver install`. The malicious binary is registered in git config.

**Fix.** Remove the GOPATH fallback entirely; os.Executable() and exec.LookPath cover all practical deployment scenarios. If the fallback must remain, validate that the resolved path's directory matches the directory of os.Executable() before accepting it.

### S007 · rename command writes through symlinks when follow-symlinks is enabled

**Severity:** low · **Confidence:** confirmed · **Surface:** cli · **CWE-363**

**Location:** `cmd/mdsmith/rename.go:356-362`

**What.** `writeFilePreservingMode` (rename.go:356) calls `os.WriteFile(path, data, mode)`, which on POSIX follows symlinks.
When `--follow-symlinks` is enabled (or `follow-symlinks: true` in config), workspace discovery includes symlinks to regular files.
If a symlinked workspace file points outside the workspace, `mdsmith rename` overwrites the external file.
The `fix` command avoids this by writing to a temp file and calling `os.Rename` (`atomicWriteFile` in fix.go) — `os.Rename` on a symlink replaces the symlink itself on POSIX, not the target.
The `rename` command is missing the same pattern.

**Impact.** When `--follow-symlinks` is enabled and a workspace symlink points outside the workspace, `mdsmith rename` overwrites the symlink target. Requires explicit opt-in (non-default) and a crafted symlink in the workspace.

**Repro (sketch).** Enable `follow-symlinks`. Place a workspace symlink pointing to `~/victim-file`. Run `mdsmith rename` on a heading in the symlinked file. The file at `~/victim-file` is overwritten.

**Fix.** Replace `os.WriteFile` in `writeFilePreservingMode` with the temp-file-then-rename pattern used by `atomicWriteFile` in fix.go.

## Hardening / Informational

### S006 · Threat model §0 baseline statement 'recipes are not executed by mdsmith' is incorrect

**Severity:** info · **Confidence:** confirmed · **Surface:** cli

**Location:** `.claude/skills/mdsmith-security-review/references/threat-model.md:29-32`

**What.** The §0 baseline states 'Recipes are not executed by mdsmith. `<?build?>` renders a body template; the recipe command is run by external tooling (CI/Make), never by mdsmith.'
This is incorrect for current code. Recipes are executed by `mdsmith fix` CLI via exec.CommandContext in internal/build/builder.go:254 and hooks.go:90.
The execution IS intentionally constrained: CLI-only (not in LSP, WASM, merge driver, or pre-merge-commit hook — which passes --no-build).
Future reviewers relying on the §0 baseline will waste time looking for a 'regression' that is actually the documented design.

**Impact.** Process risk only: a reviewer following this document will incorrectly classify the exec paths in internal/build/ as a regression rather than intended behavior, and may miss the actual gap (MDS040 gate bypassed by disabling recipe-safety in config).

**Repro (sketch).** N/A — documentation issue.

**Fix.** Update the §0 baseline to: 'Recipes are executed by `mdsmith fix` CLI via exec.Command (no shell). They are NOT executed in the LSP, WASM bindings, merge driver, or pre-merge-commit hook (--no-build). The fix is that mdsmith does execute recipes in the CLI — the constraint is that this does not happen in zero-interaction paths.'

### S005 · Obsidian plugin configPath passed to vault adapter without path-traversal validation

**Severity:** info · **Confidence:** tentative · **Surface:** obsidian · **CWE-22**

**Location:** `editors/obsidian/src/main.ts:210-223`

**What.** loadConfigYAML() passes the user-configured configPath directly to this.app.vault.adapter.read(p) without normalizing or checking for `..` traversal segments. The plugin relies on Obsidian's DataAdapter to confine reads to the vault root. Obsidian's desktop DataAdapter does internally join the vault root with the supplied path, which resolves traversal. However, no independent validation is present in the plugin, so a path like `../../etc/passwd` would reach the adapter's own normalization rather than being caught early.

**Impact.** If Obsidian's DataAdapter normalization is incomplete or behaves differently on a future platform, the plugin could read a file outside the vault to use as mdsmith config. No code execution, no write outside vault.

**Repro (sketch).** Configure configPath to `../../etc/passwd` in the plugin settings. The read behavior depends on Obsidian's adapter implementation.

**Fix.** Add a simple guard before the adapter.read: reject paths that begin with `..` or are absolute, using `!path.normalize(p).startsWith('..')` and `!path.isAbsolute(p)`. This makes the constraint explicit and independent of the adapter.

## Coverage

All seven surfaces reviewed: directive engine (include/catalog/build), CLI core and workspace walk, LSP server, VS Code extension, Obsidian plugin, distribution wrappers (npm, PyPI, GitHub Actions), and Git integration (merge driver, pre-merge-commit hook).
The prior findings S001 (LSP panic), S002 (cuetemplate panic), and S003 (convention.go YAML) from the 2026-06-09 full-repo audit are all confirmed fixed or addressed.
The CLI-core sub-agent did not return direct findings but direct inspection confirmed: symlink default-deny holds (bytelimit.ReadFileLimited and ResolvePathInRoot use EvalSymlinks), Go RE2 precludes ReDoS, and file-size limits are enforced throughout.
