# mdsmith Security Review

- **Target:** jeduden/mdsmith @ `646f04a6fd4630aab79649577f24b5a49555e005`
- **Mode:** audit  
- **Scope:** full repo — all surfaces
- **Date:** 2026-06-09

## Summary

Critical: 0 | High: 0 | Medium: 1 | Low: 0 | Info: 2

| ID | Sev | Conf | Title | Surface | Location |
|----|-----|------|-------|---------|----------|
| S001 | medium | confirmed | LSP lint goroutines have no panic recovery — hostile Markdown crashes the server | lsp | `internal/lsp/server_diagnostics.go:131-156` |
| S002 | info | confirmed | cuetemplate.buildCUESource panics on json.Marshal failure instead of returning an error | directive | `internal/cuetemplate/cuetemplate.go:174` |
| S003 | info | confirmed | convention.go calls yaml.Unmarshal directly instead of yamlutil.UnmarshalNodeSafe | directive | `internal/config/convention.go:206` |

## Findings

### S001 · LSP lint goroutines have no panic recovery — hostile Markdown crashes the server

**Severity:** medium · **Confidence:** confirmed · **Surface:** lsp · **CWE-248**

**Location:** `internal/lsp/server_diagnostics.go:131-156`
- related: `internal/lsp/server_diagnostics.go:82`
- related: `internal/lsp/server_diagnostics.go:240`

**What.** The AfterFunc-based lint goroutine (`runLintIfCurrent` → `runLint` → `sess.CheckVersion`) carries no `recover()` wrapper anywhere in its call chain. In Go, a panic in a goroutine not covered by a recover propagates to the runtime and kills the entire process. The LSP server (`mdsmith lsp`) is a long-lived process: every open editor session runs exactly one. If a rule, the parser, or include/catalog resolution panics while processing an attacker-controlled Markdown file (nil dereference, index out of bounds, type assertion, etc.), the server exits, all squiggles disappear, and VS Code shows a language-server-crashed banner. The same absence of recovery applies to `dispatchRaw` and `dispatch` — a panic handling an LSP message would crash the main loop too.

**Impact.** Denial of service: an attacker who can get the victim to open a crafted Markdown file in VS Code with the mdsmith extension active can silently crash the LSP server. Recovery depends on the editor's auto-restart policy. A crash loop (if the file stays open) gives a persistent DoS. No code execution, no data exfiltration.

**Repro (sketch).** 1. Author a Markdown file whose content triggers an unguarded panic in any lint rule (e.g. a rule that nil-dereferences an AST node on an unusual parse tree). 2. Open the file in VS Code with the extension active. The LSP server crashes and the Output Channel shows the panic stack trace.

**Fix.** Wrap the body of `runLintIfCurrent` (or `runLint`) in a `defer func() { if r := recover(); r != nil { log.Printf("lint panic: %v", r) } }()`. Do the same for `dispatchRaw`. This converts an unrecoverable crash into a logged error; the server stays up, the document's diagnostics are simply absent for that cycle. Then identify and fix the root-cause panic separately.

## Hardening / Informational

### S002 · cuetemplate.buildCUESource panics on json.Marshal failure instead of returning an error

**Severity:** info · **Confidence:** confirmed · **Surface:** directive · **CWE-248**

**Location:** `internal/cuetemplate/cuetemplate.go:174`

**What.** The `buildCUESource` helper (called from `cuetemplate.Template.Render` when evaluating a catalog `row-expr:` directive) calls `json.Marshal(emit)` and panics on error: `panic(fmt.Errorf("cuetemplate: encoding frontmatter: %w", err))`. The `emit` map is built from the Markdown file's front matter — attacker-controlled content. While `json.Marshal` failure on YAML-parsed values is unlikely in practice (all go-yaml v3 scalar types are JSON-marshallable), any future change that introduces a non-serialisable type (e.g. a `time.Duration`, channel, or function) would make this reachable. In the LSP context this panic is not recovered (see S001) and kills the server.

**Impact.** If triggered in the LSP, same DoS as S001. If triggered via `mdsmith fix`, the CLI process terminates with a non-zero exit — the fix pass is aborted but no data is lost.

**Repro (sketch).** Currently not reachable with standard go-yaml v3 output. Would become reachable if a front-matter field ever holds a type that json.Marshal cannot serialise.

**Fix.** Replace the panic with an error return: propagate the error up through `buildCUESource` → `Render`. The function signature change is mechanical; the call site in catalog's `renderTemplate` already handles error returns.

### S003 · convention.go calls yaml.Unmarshal directly instead of yamlutil.UnmarshalNodeSafe

**Severity:** info · **Confidence:** confirmed · **Surface:** directive · **CWE-400**

**Location:** `internal/config/convention.go:206`

**What.** At line 206 in `parseConventionFileBody`, `yaml.Unmarshal(data, &node)` is called with a raw `yaml.Node` destination rather than routing through `yamlutil.UnmarshalNodeSafe`. The in-code comment documents this as intentional: go-yaml v3 does not expand aliases when deserialising into a yaml.Node (aliases become AliasNode references), so billion-laughs is not a risk at this call site. The issue is consistency: every other user-YAML entry point uses the safe wrappers. A future refactor that adds a `.Decode()` call on the resulting node without re-checking would reintroduce the alias-expansion risk.

**Impact.** No current exploitability. Inconsistency with the project's stated policy creates a latent risk if the call site evolves.

**Repro (sketch).** No repro needed; this is a hardening gap.

**Fix.** Replace `yaml.Unmarshal(data, &node)` with `node, err = yamlutil.UnmarshalNodeSafe(data)` to align with project convention. The function already imports the yamlutil package for the pre-check (line 205) so this is a one-line change.

## Coverage

All seven threat-model surfaces reviewed: directive engine (include/catalog/build), CLI core and workspace walk, LSP server, VS Code extension, Obsidian plugin (WASM-based; no file exists), distribution wrappers (npm, GitHub Actions), and Git integration (merge-driver, pre-merge-commit). The Obsidian plugin TypeScript source was not present in the tree at this ref — that surface is therefore inconclusive. All §0 baseline defenses were confirmed to still hold. Two issues were found: one confirmed Medium (LSP panic DoS) and one informational (cuetemplate panic-vs-error).
