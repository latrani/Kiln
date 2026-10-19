// Splash is the page while another tab has Kiln: just that, and Use
// here (taking over keeps every character connected). Not the five
// areas, since none of them works until this tab has Kiln.
import type { State, Store } from "../store.ts";
import * as str from "../str.gen.ts";
import { roleClass } from "../theme.ts";

export function Splash({ state, store }: { state: State; store: Store }) {
  const take = store.host.useHere;
  // One click is enough.
  const useHere = () => {
    if (state.handingOff) return;
    store.set({ handingOff: true });
    take?.().catch((e) => console.error("use here", e));
  };
  return (
    <main class="splash">
      <h1 class={roleClass("form.title")}>{str.webTitle()}</h1>
      <p>{str.webOtherTab()}</p>
      {take && <button class={roleClass("form.button")} onClick={useHere} disabled={state.handingOff}>{str.webUseHere()}</button>}
    </main>
  );
}
