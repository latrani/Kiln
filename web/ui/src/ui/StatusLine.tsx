import { loadingText } from "../boot.ts";
import type { State } from "../store.ts";
import * as str from "../str.gen.ts";
import { roleClass } from "../theme.ts";

// StatusLine is the bottom row. Two live regions are always there, so
// screen readers are watching them before anything's said: a polite one
// for news and an urgent one for errors (Kiln stopping, a status error).
// A new status replaces the region's text node (keyed by Gen), so the
// same words said again are read again. Download progress isn't live:
// read chunk by chunk it would be a flood.
export function StatusLine({ state }: { state: State }) {
  const err = roleClass("status.error");
  let polite = "", urgent = "", gen = 0;
  if (state.phase === "stopped") urgent = str.webStopped();
  else if (state.status?.Text) {
    gen = state.status.Gen;
    if (state.status.Err) urgent = state.status.Text;
    else polite = state.status.Text;
  } else if (!state.saving) polite = str.webNotSaving();
  const loading = state.phase === "loading" && state.progress;
  return (
    <div class={`status ${roleClass("status")}`}>
      {loading && <span>{loadingText(state.progress!.got, state.progress!.total)}</span>}
      <span role="status" class={!state.saving && !state.status?.Text ? err : undefined}><span key={gen}>{polite}</span></span>
      <span role="alert" class={err}><span key={gen}>{urgent}</span></span>
      {state.phase === "stopped" && <button onClick={() => location.reload()}>{str.webReload()}</button>}
    </div>
  );
}
