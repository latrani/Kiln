import { toggleLog } from "../log.ts";
import { sideShown, toggleSidebar } from "../sidebar.ts";
import { logShows } from "../store.ts";
import type { State, Store } from "../store.ts";
import * as str from "../str.gen.ts";
import { roleClass } from "../theme.ts";

// Presence states (app.Presence).
const here = 1, away = 2;

// TopBar is the row above the scrollback, as the TUI's: the world and
// character showing (just the world for its overview); on the right the presence chip, which sets
// you away (or, away, back); then the Log chip, lit in log mode, which
// shows and hides it as Ctrl+L does.
export function TopBar({ state, store }: { state: State; store: Store }) {
  const c = state.chars[state.active];
  const where = c ? c.World + str.separator() + c.Name : state.active.startsWith("=") ? state.active.slice(1) : "";
  const shows = !!c && logShows(state, state.active);
  const [label, role] = state.presence === here ? [str.viewPresenceHere(), "status.presence.here"]
    : state.presence === away ? [str.viewPresenceAway(), "status.presence.away"]
    : [str.viewPresenceUnknown(), "status.presence.unknown"];
  return (
    <header class={`top ${roleClass("status")}`}>
      {!sideShown(state) && <Menu state={state} store={store} />}
      <span class="where">{where}</span>
      {c && <button class={roleClass(role)} onClick={() => state.act?.({ Do: state.presence === away ? "here" : "away" })}>{label}</button>}
      {c && <span>{str.separator()}</span>}
      {c && <button class={roleClass(shows ? "status.log.on" : "status.log")} onClick={() => toggleLog(store)}>{str.viewLogButton()}</button>}
    </header>
  );
}

// Menu is the collapsed sidebar's ≡, at the top bar's start, as the
// TUI's: beside the connections' (not log mode's filters'), the rows'
// activity summed, as each row shows its own: ● if any has an unread
// priority line, then their unread counts' total. A click brings the
// sidebar back. Its name says the activity for screen readers.
function Menu({ state, store }: { state: State; store: Store }) {
  const sb = state.sidebar;
  const badge = !logShows(state, state.active) && sb && sb.Unread > 0;
  const name = [str.webSidebarMenu(), ...(badge ? [str.webSidebarUnread(sb.Unread), ...(sb.Priority ? [str.webSidebarPriority()] : [])] : [])].join(str.separator());
  return (
    <>
      <button class={`menu ${roleClass("status.menu")}`} aria-expanded="false" aria-label={name} onClick={() => toggleSidebar(store)}>
        {str.glyph.menu}{badge && <>{" "}{sb.Priority && <span class={roleClass("sidebar.priority")}>{str.glyph.priority}</span>}<span class={roleClass("sidebar.unread")}>{sb.Unread}</span></>}
      </button>
      <span> </span>
    </>
  );
}
