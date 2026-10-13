import { sidebarRows } from "../sidebar.ts";
import type { State } from "../store.ts";
import * as str from "../str.gen.ts";
import { roleClass } from "../theme.ts";

// Session states (session.State).
const connecting = 1, connected = 2;

// Sidebar is the worlds and characters, as the TUI draws them: a world
// opens its overview; a character's row has its connection badge (… while
// connecting; a × that closes it once disconnected; else a space), its
// name, cut with … when too long, and its unread count, after ● when a
// line wanted you. The active row is lit across. Later, in log mode, the
// filter panel; with the picker open, the picker.
export function Sidebar({ state }: { state: State }) {
  const go = (Key: string) => state.act?.({ Do: "switch", Key });
  return (
    <nav class={`side ${roleClass("sidebar")}`} aria-label={str.webCharacters()}>
      <ul>
        {sidebarRows(state.order, state.chars).map((r) => {
          if (r.kind === "world") {
            const on = state.active === "=" + r.world;
            return (
              <li key={"=" + r.world}>
                <button class={"name " + roleClass(on ? "sidebar.active" : "sidebar.world")} aria-current={on ? "true" : undefined} onClick={() => go("=" + r.world)}>{r.world}</button>
              </li>
            );
          }
          const c = state.chars[r.key];
          const on = state.active === r.key;
          const closable = c.State !== connecting && c.State !== connected;
          const lead = roleClass(c.State === connecting ? "sidebar.connecting" : closable ? "sidebar.disconnected" : "sidebar.char");
          const own = on ? undefined : lead; // the active row's parts draw plain inside it
          const unread: number = c.Unread ?? 0;
          const wants = c.Attention && unread > 0;
          return (
            <li key={r.key} class={on ? roleClass("sidebar.active") : undefined}>
              <span class={"lead " + (own ?? "")}>
                {" "}
                {c.State === connecting && <><span class={own}>{str.glyph.connecting}</span>{" "}</>}
                {closable && <><button class={own} aria-label={str.webCloseChar(c.Name)} onClick={() => state.act?.({ Do: "close", Key: r.key })}>{str.glyph.remove}</button>{" "}</>}
              </span>
              <button class={"name " + (own ?? "")} aria-current={on ? "true" : undefined}
                aria-label={unread > 0 ? (wants ? str.webCharAttention(unread, c.Name) : str.webCharUnread(unread, c.Name)) : undefined}
                onClick={() => go(r.key)}>{c.Name}</button>
              {unread > 0 && (
                <span class="activity" aria-hidden="true">
                  {" "}
                  {wants && <><span class={roleClass("sidebar.attention")}>{str.glyph.attention}</span>{" "}</>}
                  <span class={on ? undefined : roleClass("sidebar.unread")}>{unread}</span>
                </span>
              )}
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
