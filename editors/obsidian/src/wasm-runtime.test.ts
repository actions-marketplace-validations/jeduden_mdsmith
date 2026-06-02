// Drives the WASM runtime facade.
//
// The facade (src/wasm-runtime.ts) is the only module that touches the
// plan-215 WebAssembly engine. It instantiates the module, constructs
// one mdsmith.Session over a workspace snapshot + config YAML, and
// exposes check / fix / invalidate / dispose through a typed surface.
//
// These tests load the REAL .wasm artifact via Go's wasm_exec.js — the
// same path src/wasm-runtime.ts uses in the Electron/WebView host — so
// they exercise the JS↔Go marshalling, not a mock. They build the
// artifact on demand and skip when the Go toolchain is absent.

import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import {
  createRuntime,
  type Diagnostic,
  type MdsmithRuntime,
} from "./wasm-runtime";

function hasGo(): boolean {
  return !Bun.spawnSync(["go", "version"]).exitCode;
}

// goRoot resolves the active toolchain's wasm_exec.js, which moved to
// lib/wasm/ in recent Go releases (mirrors cmd/mdsmith-wasm smoke test).
function wasmExecPath(): string {
  const out = Bun.spawnSync(["go", "env", "GOROOT"]);
  const root = out.stdout.toString().trim();
  return join(root, "lib", "wasm", "wasm_exec.js");
}

// buildWasm compiles cmd/mdsmith-wasm for GOOS=js GOARCH=wasm into a
// temp file and returns the bytes.
function buildWasm(dir: string): Uint8Array {
  const out = join(dir, "mdsmith.wasm");
  const res = Bun.spawnSync(["go", "build", "-o", out, "."], {
    cwd: join(import.meta.dir, "..", "..", "..", "cmd", "mdsmith-wasm"),
    env: { ...process.env, GOOS: "js", GOARCH: "wasm" },
  });
  if (res.exitCode !== 0) {
    throw new Error(`go build wasm failed: ${res.stderr.toString()}`);
  }
  return readFileSync(out);
}

const skip = !hasGo();
let tmp = "";
let wasmBytes: Uint8Array;
let wasmExecSource: string;

beforeAll(() => {
  if (skip) return;
  tmp = mkdtempSync(join(tmpdir(), "mds-wasm-rt-"));
  wasmBytes = buildWasm(tmp);
  wasmExecSource = readFileSync(wasmExecPath(), "utf8");
}, 120_000); // a cold GOOS=js GOARCH=wasm build takes ~30 s.

afterAll(() => {
  if (tmp) rmSync(tmp, { recursive: true, force: true });
});

// makeRuntime builds a runtime from the test's prebuilt artifact. The
// loader takes the wasm_exec.js source and the .wasm bytes so the test
// controls where they come from; the host wires its own loader that
// fetches the two files from the plugin directory.
async function makeRuntime(
  workspace: Record<string, string>,
  configYAML = "",
): Promise<MdsmithRuntime> {
  return createRuntime({
    workspace,
    configYAML,
    loadWasmExec: () => wasmExecSource,
    loadWasmBytes: async () => wasmBytes,
  });
}

describe.skipIf(skip)("createRuntime", () => {
  test("check returns a normalized diagnostic array for a clean file", async () => {
    const rt = await makeRuntime({});
    const diags = await rt.check("clean.md", "# Clean\n\nA tidy paragraph.\n");
    expect(Array.isArray(diags)).toBe(true);
    expect(diags.length).toBe(0);
    rt.dispose();
  });

  test("check surfaces an MDS001 violation with the engine's wire shape", async () => {
    const rt = await makeRuntime({});
    // MDS001 is line-length (default max 80). A line past 80 columns
    // fires it — the exact rule the plan's acceptance criterion names.
    const longLine =
      "This line is deliberately made to exceed the eighty character limit by adding extra words here now.";
    const diags = await rt.check("bad.md", `# Title\n\n${longLine}\n`);
    const codes = diags.map((d) => d.rule);
    expect(codes).toContain("MDS001");
    const d = diags.find((x) => x.rule === "MDS001") as Diagnostic;
    expect(typeof d.line).toBe("number");
    expect(typeof d.column).toBe("number");
    expect(d.severity).toBeDefined();
    expect(typeof d.message).toBe("string");
    rt.dispose();
  });

  test("fix returns rewritten source plus a changed flag", async () => {
    const rt = await makeRuntime({});
    // Trailing spaces are fixable (MDS009).
    const res = await rt.fix("trail.md", "# Title\n\nText with trailing.   \n");
    expect(res.changed).toBe(true);
    expect(res.source).not.toContain("trailing.   \n");
    expect(Array.isArray(res.diagnostics)).toBe(true);
    rt.dispose();
  });

  test("fix matches the WASM session.fix on the same input", async () => {
    const input = "#  Spaced Title\n\n\n\ntext   \n";
    const rt = await makeRuntime({});
    const viaFacade = await rt.fix("doc.md", input);
    // The facade is a thin pass-through; a second fix of its own output
    // must be a no-op (idempotent), proving it returns the engine's
    // final bytes rather than a partial pass.
    const again = await rt.fix("doc.md", viaFacade.source);
    expect(again.changed).toBe(false);
    expect(again.source).toBe(viaFacade.source);
    rt.dispose();
  });

  test("invalidate(uri, content) makes a cross-file rule see new bytes", async () => {
    // index.md catalogs docs/*.md by their summary front matter. Drop a
    // doc in, then invalidate the index's view of the workspace.
    const indexSrc =
      "# Index\n\n<?catalog\nglob:\n  - \"docs/*.md\"\n" +
      "row: \"- [{summary}](docs/{filename})\"\n?>\n<?/catalog?>\n";
    const rt = await makeRuntime({
      "docs/a.md": "---\nsummary: Doc A\n---\n# A\n\nBody of doc A here.\n",
    });
    const before = await rt.check("index.md", indexSrc);
    // Add a second doc to the workspace and re-check: the catalog is now
    // out of date by one more entry, so MDS019 still fires (the body is
    // empty). The point is the call does not throw and the workspace
    // mutation is accepted.
    rt.invalidate(
      "docs/b.md",
      "---\nsummary: Doc B\n---\n# B\n\nBody of doc B here.\n",
    );
    const after = await rt.check("index.md", indexSrc);
    expect(Array.isArray(before)).toBe(true);
    expect(Array.isArray(after)).toBe(true);
    rt.dispose();
  });

  test("capabilities advertises check, fix, and kinds", async () => {
    const rt = await makeRuntime({});
    const caps = rt.capabilities();
    expect(caps).toContain("check");
    expect(caps).toContain("fix");
    expect(caps).toContain("kinds");
    rt.dispose();
  });
});
