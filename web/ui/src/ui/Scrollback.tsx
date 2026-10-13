import type { State } from "../store.ts";
import * as str from "../str.gen.ts";
import { roleClass } from "../theme.ts";

// Scrollback is the active character's lines (step 3), read aloud as
// they come: the page's main landmark, with the log inside it (a role on
// <main> itself would hide the landmark). While another tab has Kiln, it says so here.
export function Scrollback({ state }: { state: State }) {
  return (
    <main class="body">
      <div role="log">
        {state.phase === "locked" && (
          <div class="lock">
            <p>{str.webOtherTab()}</p>
            <button class={roleClass("form.button")} onClick={state.onUseHere} disabled={!state.onUseHere}>{str.webUseHere()}</button>
          </div>
        )}
      </div>
    </main>
  );
}
