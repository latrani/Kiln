import { togglePanel } from "../filter.ts";
import { findStatus, toggleLog } from "../log.ts";
import type { LogState } from "../log.ts";
import { logShows } from "../store.ts";
import type { State, Store } from "../store.ts";
import * as str from "../str.gen.ts";
import { roleClass } from "../theme.ts";

// Presence states (app.Presence).
const here = 1, away = 2;

// TopBar is the row above the scrollback, as the TUI's: the world and
// character showing (just the world for its overview), in log mode with
// find's status after them; on the right the presence chip, which sets
// you away (or, away, back); then, in log mode, the Filter chip, lit
// while the filter panel's open, which opens and closes it as f does;
// then the Log chip, lit in log mode, which shows and hides it as Ctrl+L
// does.
export function TopBar({ state, store }: { state: State; store: Store }) {
  const c = state.chars[state.active];
  const where = c ? c.World + str.separator() + c.Name : state.active.startsWith("=") ? state.active.slice(1) : "";
  const shows = !!c && logShows(state, state.active);
  const find = shows ? findStatus(state.logs[state.active] as LogState, state.logView[state.active]?.at, state.logLines[state.active]?.lines ?? []) : "";
  const [label, role] = state.presence === here ? [str.viewPresenceHere(), "status.presence.here"]
    : state.presence === away ? [str.viewPresenceAway(), "status.presence.away"]
    : [str.viewPresenceUnknown(), "status.presence.unknown"];
  return (
    <header class={`top ${roleClass("status")}`}>
      <span class="where">{where}{find && "   " + find}</span>
      {c && <button class={roleClass(role)} onClick={() => state.act?.({ Do: state.presence === away ? "here" : "away" })}>{label}</button>}
      {c && <span>{str.separator()}</span>}
      {shows && <button class={roleClass(state.logView[state.active]?.panel ? "status.filter.on" : "status.filter")} onClick={() => togglePanel(store)}>{str.viewFilterButton()}</button>}
      {shows && <span>{str.separator()}</span>}
      {c && <button class={roleClass(shows ? "status.log.on" : "status.log")} onClick={() => toggleLog(store)}>{str.viewLogButton()}</button>}
    </header>
  );
}
