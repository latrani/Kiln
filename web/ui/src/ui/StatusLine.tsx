import { loadingText } from "../boot.ts";
import { cursorOf, findStatus } from "../log.ts";
import type { LogState } from "../log.ts";
import { ask, dismiss } from "../notify.ts";
import { historyShows, logShows } from "../store.ts";
import type { State, Store } from "../store.ts";
import * as str from "../str.gen.ts";
import { roleClass } from "../theme.ts";

// stateName is a connection state (session.State) as the status line
// says it.
function stateName(s: number): string {
  return [str.stateDisconnected(), str.stateConnecting(), str.stateConnected(), str.stateFailed()][s] ?? str.stateDisconnected();
}

// logStatus is log mode's status line with no news, as the TUI's
// statusParts: find's status while it's on, then how many lines are
// selected; most specific first.
function logStatus(state: State, g: LogState): string {
  const lines = state.logLines[state.active]?.lines ?? [];
  const find = findStatus(g, cursorOf(g, state.logView[state.active], lines), lines);
  return [find, str.viewSelected(g.Selected)].filter(Boolean).join(str.separator());
}

// StatusLine is the bottom row. Two live regions are always there, so
// screen readers are watching them before anything's said: a polite one
// for news and an urgent one for errors (Kiln stopping, a status error).
// A new status replaces the region's text node (keyed by Gen), so the
// same words said again are read again. Download progress isn't live:
// read chunk by chunk it would be a flood. While the browser won't show
// notifications, a button asks it to, with a × to stop offering. The
// History chip, at the right while there's an input to recall into, as
// the TUI's, opens and closes the history panel; lit while it's up.
export function StatusLine({ state, store }: { state: State; store: Store }) {
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
  // log mode find's status while it's on, then how many lines are
  // selected. (Log mode's messages come as the status: Go moves them
  // there, as the TUI's takeLogStatus.)
  const c = state.chars[state.active];
  const g = c && logShows(state, state.active) ? (state.logs[state.active] as LogState) : undefined;
  const conn = state.phase === "running" && !state.status?.Text && c
    ? (g ? logStatus(state, g) : c.State === 2 ? c.ConnectedSince : stateName(c.State))
    : "";
  const chip = state.phase === "running" && !state.picker && !state.active.startsWith("=") && !g && !state.inputs[state.active]?.NeedPW;
  const on = historyShows(state);
  return (
    <div class={`status ${roleClass("status")}`}>
      {chip && (
        <button class={`chip ${roleClass(on ? "status.history.on" : "status.history")}`} aria-pressed={on}
          onMouseDown={(e) => e.preventDefault()} onClick={() => state.act?.({ Do: "history_toggle" })}>{str.viewHistoryButton()}</button>
      )}
      {loading && <span>{loadingText(state.progress!.got, state.progress!.total)}</span>}
      {conn && <span>{conn}{polite && str.separator()}</span>}
      <span role="status" class={!state.saving && !state.status?.Text ? err : undefined}><span key={gen}>{polite}</span></span>
      <span role="alert" class={err}><span key={gen}>{urgent}</span></span>
      {state.phase === "running" && state.notifyOffer && (
        <span class="offer">
          <button class={roleClass("form.button")} onClick={() => ask(store)}>{str.webNotifyAllow()}</button>
          <button aria-label={str.webNotifyDismiss()} onClick={() => dismiss(store)}>{str.glyph.remove}</button>
        </span>
      )}
      {state.phase === "stopped" && <button class={roleClass("form.button")} onClick={() => location.reload()}>{str.webReload()}</button>}
    </div>
  );
}
