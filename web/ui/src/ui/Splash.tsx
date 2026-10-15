// Splash is the page while another tab has Kiln: just that, and Use
// here with what it costs. Not the five areas, since none of them works
// until this tab has Kiln.
import type { State } from "../store.ts";
import * as str from "../str.gen.ts";
import { roleClass } from "../theme.ts";

export function Splash({ state }: { state: State }) {
  return (
    <main class="splash">
      <h1 class={roleClass("form.title")}>{str.webTitle()}</h1>
      <p>{str.webOtherTab()}</p>
      <p id="use-here-warning" class={roleClass("form.hint")}>{str.webUseHereWarning()}</p>
      <button class={roleClass("form.button")} onClick={state.onUseHere} disabled={!state.onUseHere} aria-describedby="use-here-warning">{str.webUseHere()}</button>
    </main>
  );
}
