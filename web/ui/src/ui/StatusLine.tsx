import { loadingText } from "../boot.ts";
import type { State } from "../store.ts";
import * as str from "../str.gen.ts";
import { roleClass } from "../theme.ts";

// StatusLine is the bottom row. It says one thing: that Kiln stopped,
// how the download's going, Kiln's status, or that nothing's being saved.
// Errors interrupt (alert); the rest wait their turn (status).
export function StatusLine({ state }: { state: State }) {
  const err = roleClass("status.error");
  let said;
  if (state.phase === "stopped") {
    said = (
      <>
        <span role="alert" class={err}>{str.webStopped()}</span>
        <button onClick={() => location.reload()}>{str.webReload()}</button>
      </>
    );
  } else if (state.phase === "loading" && state.progress) {
    said = <span role="status">{loadingText(state.progress.got, state.progress.total)}</span>;
  } else if (state.status?.Text) {
    said = state.status.Err
      ? <span role="alert" class={err}>{state.status.Text}</span>
      : <span role="status">{state.status.Text}</span>;
  } else if (!state.saving) {
    said = <span role="status" class={err}>{str.webNotSaving()}</span>;
  } else {
    said = <span role="status" />;
  }
  return <div class={`status ${roleClass("status")}`}>{said}</div>;
}
