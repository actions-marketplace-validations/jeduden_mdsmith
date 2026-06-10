// Thin activation entry for the mdsmith VS Code extension.
//
// This module is the one place that imports the live `vscode` namespace
// and the vscode-languageclient runtime (LanguageClient + TransportKind);
// both require()d `vscode` at load time, which only exists inside the
// editor host. It hands those two boundary objects to the Wiring
// composition root (wiring.ts) and does nothing else, so the extension's
// logic — LSP lifecycle, the .mdsmith.yml watcher, command registration —
// stays in a module that `bun test` can drive with fakes. See
// docs/development/architecture/typescript.md.

import * as vscode from "vscode";
import { LanguageClient, TransportKind } from "vscode-languageclient/node";

import { Wiring, type ClientLike } from "./wiring";

// The single Wiring instance for this activation. activate() builds it;
// deactivate() tears it down.
let wiring: Wiring | undefined;

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  wiring = new Wiring({
    api: vscode,
    // The only spot that needs the languageclient runtime: construct the
    // real LanguageClient. Wiring drives it through the structural
    // ClientLike, so the construction is the entire boundary.
    createClient: (id, name, serverOptions, clientOptions): ClientLike =>
      new LanguageClient(id, name, serverOptions, clientOptions),
    stdioTransport: TransportKind.stdio,
  });
  await wiring.activate(context);
}

export async function deactivate(): Promise<void> {
  await wiring?.deactivate();
  wiring = undefined;
}
