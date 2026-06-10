// MdsmithErrorHandler replaces vscode-languageclient's default
// ErrorHandler. The default's "5 closes in 180 seconds → stop" rule is
// hostile during local development (rebuild loops, editor reloads,
// transient ENOENT while iterating on the binary path) — once it trips,
// the only recovery is a window reload. This handler:
//
//  - Always returns ErrorAction.Continue on RPC errors. Errors don't
//    kill the process, so there's nothing useful to do other than keep
//    going.
//  - Allows up to maxRestarts close events per windowMs of wallclock
//    time before falling back to DoNotRestart, which is significantly
//    more permissive than the default.
//  - On the falling-back path, invokes the injected onCapExceeded
//    callback so the caller can surface a recovery notification ("Restart
//    Language Server" / "Show Output") instead of silently disabling the
//    extension.
//  - Suppresses the restart entirely once the server announces it was
//    superseded (markSuperseded).
//
// The restart decision itself lives in decideClose (wiring.ts) so it can
// be unit-tested without vscode; the prompt is injected so this class is
// likewise testable without the editor runtime.

import type {
  CloseAction,
  CloseHandlerResult,
  ErrorAction,
  ErrorHandler,
  ErrorHandlerResult,
  Message,
} from "vscode-languageclient/node";

import { decideClose, type RestartPolicyState } from "../wiring";

// Wire values of the vscode-languageclient enums we return. Imported as
// type-only above so this module — like wiring.ts — never pulls the
// vscode-languageclient runtime (which does require("vscode") at load
// time) into a plain `bun test`. The numbers mirror
// vscode-languageclient/lib/common/client.js; the run-mode-style
// contract test below pins them so a package bump that renumbered the
// enum fails loudly. The `as` casts re-attach the nominal enum type.
const ERROR_ACTION_CONTINUE = 1 as ErrorAction;
const CLOSE_ACTION_DO_NOT_RESTART = 1 as CloseAction;
const CLOSE_ACTION_RESTART = 2 as CloseAction;

export class MdsmithErrorHandler implements ErrorHandler {
  private static readonly maxRestarts = 25;
  private static readonly windowMs = 3 * 60 * 1000;
  private state: RestartPolicyState = { restarts: [], superseded: false };

  // onCapExceeded is invoked (at most once per cap breach) when the
  // restart-rate ceiling is hit and the handler has decided to stop
  // restarting. The caller wires it to a user-facing recovery prompt.
  // Injected rather than calling vscode.window directly so the handler
  // stays unit-testable without the editor host.
  constructor(private readonly onCapExceeded: () => void) {}

  // markSuperseded records that the server told us (via the
  // mdsmith/superseded notification) that a newer instance for this
  // workspace has taken over. The imminent connection close is then
  // expected and intentional, so closed() must NOT restart — otherwise
  // this now-orphaned editor host would respawn a server the newer one
  // supersedes again, the exact restart loop a leaked extension host
  // used to sustain.
  markSuperseded(): void {
    this.state.superseded = true;
  }

  error(_error: Error, _message: Message | undefined, _count: number | undefined): ErrorHandlerResult {
    return { action: ERROR_ACTION_CONTINUE };
  }

  closed(): CloseHandlerResult {
    const { restart, capExceeded } = decideClose(
      this.state,
      Date.now(),
      MdsmithErrorHandler.maxRestarts,
      MdsmithErrorHandler.windowMs
    );
    if (capExceeded) {
      this.onCapExceeded();
    }
    return { action: restart ? CLOSE_ACTION_RESTART : CLOSE_ACTION_DO_NOT_RESTART };
  }
}
