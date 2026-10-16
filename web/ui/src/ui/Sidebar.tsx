import { useLayoutEffect } from "preact/hooks";
import { choose, pickerRows, selKey } from "../picker.ts";
import type { PickRow } from "../picker.ts";
import { sidebarRows } from "../sidebar.ts";
import { listing, logShows } from "../store.ts";
import type { State, Store } from "../store.ts";
import { titled } from "../layout.ts";
import * as str from "../str.gen.ts";
import { roleClass } from "../theme.ts";
import { FilterPanel } from "./FilterPanel.tsx";
import { NameField } from "./PickerForm.tsx";

// Session states (session.State).
const connecting = 1, connected = 2;

// Sidebar is the worlds and characters, as the TUI draws them: a world
// opens its overview; a character's row has its connection badge (… while
// connecting; a × that closes it once disconnected; else a space), its
// name, cut with … when too long, and its unread count, after ● when a
// line was a priority. The active row is lit across. Last, after a gap,
// "+ Connection" opens the picker. With the picker open, it's the
// picker's list instead; in log mode, the filter panel. Each is titled,
// as the TUI's.
export function Sidebar({ state, store }: { state: State; store: Store }) {
  if (logShows(state, state.active)) return <FilterPanel state={state} store={store} />;
  if (listing(state)) return <PickerList state={state} store={store} />;
  const go = (Key: string) => state.act?.({ Do: "switch", Key });
  return (
    <nav class={`side ${roleClass("sidebar")}`} aria-label={str.webCharacters()}>
      <h2 class={`stitle ${roleClass("sidebar.title")}`}>{titled(str.viewConnectionsTitle())}</h2>
      <ul>
        {sidebarRows(state.order, state.chars).map((r) => {
          if (r.kind === "gap") return <li key="gap" aria-hidden="true"> </li>;
          if (r.kind === "add") {
            return (
              <li key="add">
                <button class={"chip " + roleClass("sidebar.add")} onClick={() => store.openPicker()}>{str.sidebarOpenConnection()}</button>
              </li>
            );
          }
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
          const priority = c.Priority && unread > 0;
          return (
            <li key={r.key} class={on ? roleClass("sidebar.active") : undefined}>
              <span class={"lead " + (own ?? "")}>
                {" "}
                {c.State === connecting && <>{str.glyph.connecting}{" "}</>}
                {closable && <><button class={own} aria-label={str.webCloseChar(c.Name)} onClick={() => state.act?.({ Do: "close", Key: r.key })}>{str.glyph.remove}</button>{" "}</>}
              </span>
              <button class={"name " + (own ?? "")} aria-current={on ? "true" : undefined}
                aria-label={unread > 0 ? (priority ? str.webCharPriority(c.Name, unread) : str.webCharUnread(unread, c.Name)) : undefined}
                onClick={() => go(r.key)}>{c.Name}</button>
              {unread > 0 && (
                <span class="activity" aria-hidden="true">
                  {" "}
                  {priority && <><span class={roleClass("sidebar.priority")}>{str.glyph.priority}</span>{" "}</>}
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

// PickerList is the picker in the sidebar, as the TUI's: worlds, the
// characters to open under them, and "+ Character" ending each; the
// highlighted row lit. It's the list the picker's filter (PickerForm)
// controls, so a screen reader hears the highlighted row as you move.
// A click does what Enter does.
function PickerList({ state, store }: { state: State; store: Store }) {
  const p = state.picker!;
  useLayoutEffect(() => {
    document.getElementById("pick-" + p.sel)?.scrollIntoView?.({ block: "nearest" });
  }, [p.sel]);
  return (
    <nav class={`side ${roleClass("picker")}`} aria-label={str.webCharacters()}>
      <h2 class={`stitle ${roleClass("picker.title")}`}>{titled(str.pickerTitle())}</h2>
      <ul role="listbox" id="kiln-picker" aria-label={str.webPicker()}>
        {groups(pickerRows(state.worlds, state.chars, p.filter)).map(({ world, rows }) => (
          // Each world's options are a group named for it, so a screen
          // reader says which world a character is in; the first is the
          // world's own row, its editor.
          <li key={"=" + world} role="group" aria-label={world}>
            <ul role="none">
              {rows.map((r) => {
                // A world's row is its name, lit in its own role; a
                // character's row is lit across, one space in; "+
                // Character" is a chip, one space in, and the name
                // field while adding one.
                const k = selKey(r);
                const on = k === p.sel;
                const opts = { key: k, id: "pick-" + k, role: "option" as const, "aria-selected": on, onClick: () => choose(store, k) };
                if (r.kind === "world") {
                  return <li {...opts} class={"pick " + roleClass(on ? "picker.world.selected" : "picker.world")}>{r.world}</li>;
                }
                if (r.kind === "add_char" && p.adding === r.world) {
                  // The name being added, in the add row's place. No
                  // click: that would start the name over. Its key is
                  // its own, not spread: a row added above it (the new
                  // character, as Go saves it) mustn't remount it before
                  // it hears Go's answer.
                  const { key: _k, onClick: _c, ...here } = opts;
                  return <li key={k} {...here} class="pick naming"><NameField state={state} store={store} /></li>;
                }
                if (r.kind === "add_char") {
                  return <li {...opts} class="pick"> <span class={roleClass(on ? "picker.selected" : "picker.add")}> {str.pickerAddCharacter()} </span></li>;
                }
                return <li {...opts} class={"pick " + (on ? roleClass("picker.selected") : "")}> {r.kind === "char" ? r.name : ""}</li>;
              })}
            </ul>
          </li>
        ))}
        {/* + World, in no world: a chip at the left edge, as the TUI's. */}
        <li key="+" id="pick-+" role="option" aria-selected={p.sel === "+"} class="pick" onClick={() => choose(store, "+")}>
          <span class={roleClass(p.sel === "+" ? "picker.selected" : "picker.add")}> {str.pickerAddWorld()} </span>
        </li>
      </ul>
    </nav>
  );
}

// groups splits the picker's rows by world (each world's row, then its
// rows), leaving + World, which is in no world, for after them.
function groups(rows: PickRow[]): { world: string; rows: PickRow[] }[] {
  const out: { world: string; rows: PickRow[] }[] = [];
  for (const r of rows) {
    if (r.kind === "world") out.push({ world: r.world, rows: [r] });
    else if (r.kind !== "add_world") out.at(-1)?.rows.push(r);
  }
  return out;
}
