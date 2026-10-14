import { loadingText } from "../boot.ts";
import type { LogState } from "../log.ts";
import { logShows } from "../store.ts";
import type { State } from "../store.ts";
import * as str from "../str.gen.ts";
import { roleClass } from "../theme.ts";

// stateName is a connection state (session.State) as the status line
// says it.
function stateName(s: number): string {
  return [str.stateDisconnected(), str.stateConnecting(), str.stateConnected(), str.stateFailed()][s] ?? str.stateDisconnected();
}

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
  // With no news, the character showing's connection, as the TUI's; in
  // log mode its status (an error read as an alert), else how many lines
  // are selected, as the TUI's takeLogStatus.
  const c = state.chars[state.active];
  const g = c && logShows(state, state.active) ? (state.logs[state.active] as LogState) : undefined;
  if (g && !state.status?.Text && g.Status) {
    if (g.StatusErr) urgent = g.Status;
    else polite = g.Status;
  }
  const conn = state.phase === "running" && !state.status?.Text && c
    ? (g ? (g.Status ? "" : str.viewSelected(g.Selected)) : c.State === 2 ? c.ConnectedSince : stateName(c.State))
    : "";
  return (
    <div class={`status ${roleClass("status")}`}>
      {loading && <span>{loadingText(state.progress!.got, state.progress!.total)}</span>}
      {conn && <span>{conn}{polite && str.separator()}</span>}
      <span role="status" class={!state.saving && !state.status?.Text ? err : undefined}><span key={gen}>{polite}</span></span>
      <span role="alert" class={err}><span key={gen}>{urgent}</span></span>
      {state.phase === "stopped" && <button class={roleClass("form.button")} onClick={() => location.reload()}>{str.webReload()}</button>}
    </div>
  );
}
