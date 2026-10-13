import type { State } from "../store.ts";
import * as str from "../str.gen.ts";

// Scrollback is the active character's lines (step 3), read aloud as
// they come. While another tab has Kiln, it says so here.
export function Scrollback({ state }: { state: State }) {
  return (
    <main class="body" role="log">
      {state.phase === "locked" && (
        <div class="lock">
          <p>{str.webOtherTab()}</p>
          <button onClick={state.onUseHere} disabled={!state.onUseHere}>{str.webUseHere()}</button>
        </div>
      )}
    </main>
  );
}
