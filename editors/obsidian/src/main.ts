// Entrypoint for the mdsmith Obsidian plugin.
//
// Obsidian loads `dist/main.js` and instantiates the default export as
// a Plugin subclass. This module is the wiring root — task 7 of plan
// 217 fills it in: load the WASM bundle, build one mdsmith.Session over
// the vault, register the CM6 diagnostics extension, the palette
// commands, the diagnostics view, the settings tab, and the vault
// listeners. For now it is a minimal stub so the bundle builds.

import { Plugin } from "obsidian";

export default class MdsmithPlugin extends Plugin {
  override async onload(): Promise<void> {
    // Wiring lands in task 7.
  }

  override onunload(): void {
    // Teardown lands in task 7.
  }
}
