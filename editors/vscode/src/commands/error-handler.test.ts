import { describe, expect, test } from "bun:test";

import { MdsmithErrorHandler } from "./error-handler";

// ErrorAction.Continue / CloseAction.{Restart,DoNotRestart} are numeric
// enums in vscode-languageclient. error-handler.ts imports them
// type-only and returns the wire values directly, so the module never
// pulls the languageclient runtime into bun test. Pin those wire values
// here (mirroring vscode-languageclient/lib/common/client.js) so the
// handler's mapping cannot silently drift.
const ErrorActionContinue = 1;
const CloseActionDoNotRestart = 1;
const CloseActionRestart = 2;

describe("MdsmithErrorHandler", () => {
  test("error() always continues without restarting the process", () => {
    // RPC errors don't kill the server, so the only sensible action is
    // to keep going. The handler must never tear down on an error.
    const handler = new MdsmithErrorHandler(() => {});
    const result = handler.error(new Error("boom"), undefined, 1);
    expect(result.action).toBe(ErrorActionContinue);
  });

  test("closed() restarts a normal close under the cap", () => {
    const handler = new MdsmithErrorHandler(() => {});
    const result = handler.closed();
    expect(result.action).toBe(CloseActionRestart);
  });

  test("closed() stops restarting and prompts once the cap is exceeded", () => {
    // The default cap is 25 restarts inside the window. The 26th close
    // must fall back to DoNotRestart and fire the recovery prompt once.
    let prompts = 0;
    const handler = new MdsmithErrorHandler(() => {
      prompts++;
    });
    let last = handler.closed();
    for (let i = 0; i < 25; i++) {
      last = handler.closed();
    }
    expect(last.action).toBe(CloseActionDoNotRestart);
    expect(prompts).toBe(1);
  });

  test("closed() does not restart and does not prompt once superseded", () => {
    // A superseded close is expected — a newer server for this
    // workspace has taken over. Restarting would respawn a server the
    // newer one immediately supersedes again, so closed() must stop
    // without prompting the user.
    let prompts = 0;
    const handler = new MdsmithErrorHandler(() => {
      prompts++;
    });
    handler.markSuperseded();
    const result = handler.closed();
    expect(result.action).toBe(CloseActionDoNotRestart);
    expect(prompts).toBe(0);
  });
});
