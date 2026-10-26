import { useLayoutEffect } from "preact/hooks";
import { choose, pickerRows, selKey } from "../picker.ts";
import type { PickRow } from "../picker.ts";
import { canToggle, collapseButton, sideForced, sidebarRows, toggleSidebar, treeGlyph } from "../sidebar.ts";
import { listing, logShows } from "../store.ts";
import type { State, Store } from "../store.ts";
import * as str from "../str.gen.ts";
import { roleClass } from "../theme.ts";
import { FilterPanel } from "./FilterPanel.tsx";
import { NameField } from "./PickerForm.tsx";

// Session states (session.State).
const connecting = 1, connected = 2;

// Sidebar is the worlds and characters, as the TUI draws them: a world
// opens its overview; a character's row hangs off its world's tree line,
// then has its connection badge (… while connecting; a × that closes it
// once disconnected; else nothing), its name, cut with … when too long,
// and its unread count, after ● when a line was a priority. The active
// row is lit across, past the tree. Last, after a gap,
// "+ Connection" opens the picker. With the picker open, it's the
// picker's list instead; in log mode, the filter panel. Each is titled,
// as the TUI's.
export function Sidebar({ state, store }: { state: State; store: Store }) {
  if (logShows(state, state.active)) return <FilterPanel state={state} store={store} />;
  if (listing(state)) return <PickerList state={state} store={store} />;
  const go = (Key: string) => store.pickFromSidebar(Key);
  return (
    <nav class={`side ${roleClass("sidebar")}`} aria-label={str.webCharacters()}
      onKeyDown={(e) => { if (e.key === "Escape" && state.narrowOpen) { e.preventDefault(); toggleSidebar(store); } }}>
      <SideTitle role="sidebar.title" rule="rule.sidebar" label={str.viewConnectionsTitle()} collapse={{ role: "sidebar.collapse", state, store }} />
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
            <li key={r.key}>
              <Tree role="sidebar.tree" last={r.last} />
              <span class={"crow" + (on ? " " + roleClass("sidebar.active") : "")}>
                <span class={"lead " + (own ?? "")}>
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
              </span>
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
      <SideTitle role="picker.title" rule="rule.picker" label={str.pickerTitle()} collapse={{ role: "picker.collapse", state, store }} />
      <ul role="listbox" id="kiln-picker" aria-label={str.webPicker()}>
        {groups(pickerRows(state.worlds, state.chars, p.filter, store.host.worlds === true)).map(({ world, rows }) => (
          // Each world's options are a group named for it, so a screen
          // reader says which world a character is in; the first is the
          // world's own row, its editor.
          <li key={"=" + world} role="group" aria-label={world}>
            <ul role="none">
              {rows.map((r) => {
                // A world's row is its name, lit in its own role; a
                // character's row hangs off the world's tree line and is
                // lit across past it; "+ Character" is a chip at the
                // line's end, and the name field while adding one.
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
                  return <li key={k} {...here} class="pick naming"><Tree role="picker.tree" last={r.last} /><NameField state={state} store={store} /></li>;
                }
                if (r.kind === "add_char") {
                  return <li {...opts} class="pick branch"><Tree role="picker.tree" last={r.last} /><span class={roleClass(on ? "picker.selected" : "picker.add")}> {str.pickerAddCharacter()} </span></li>;
                }
                return (
                  <li {...opts} class="pick branch"><Tree role="picker.tree" last={r.kind === "char" && r.last} />
                    <span class={"pname" + (on ? " " + roleClass("picker.selected") : "")}>{r.kind === "char" ? r.name : ""}</span></li>
                );
              })}
            </ul>
          </li>
        ))}
        {/* + World ends the list, a chip with no indent, as the TUI's;
            only for a host that can add worlds (picker.ts). */}
        {store.host.worlds === true && (
          <li id="pick-+" role="option" aria-selected={p.sel === "+"} class="pick" onClick={() => choose(store, "+")}>
            <span class={roleClass(p.sel === "+" ? "picker.selected" : "picker.add")}> {str.pickerAddWorld()} </span>
          </li>
        )}
      </ul>
    </nav>
  );
}

// Tree is the line from a world to a row under it, in role, as the
// TUI's: a branch, or the line's end for the world's last.
function Tree({ role, last }: { role: string; last?: boolean }) {
  return <span aria-hidden="true" class={"tree " + roleClass(role)}>{treeGlyph(last)}</span>;
}

// groups splits the picker's rows by world: each world's row, then its
// rows.
function groups(rows: PickRow[]): { world: string; rows: PickRow[] }[] {
  const out: { world: string; rows: PickRow[] }[] = [];
  for (const r of rows) {
    if (r.kind === "world") out.push({ world: r.world, rows: [r] });
    else if (r.kind === "add_world") continue; // the list's own last row
    else out.at(-1)?.rows.push(r);
  }
  return out;
}

// SideTitle is a sidebar list's title, as the TUI's: label over a rule
// across the column in rule's color, in line with the top bar's. A press
// on it does onPress (the filter panel's: focus to it).
export function SideTitle({ role, rule, label, onPress, collapse }: { role: string; rule: string; label: string; onPress?: () => void; collapse?: { role: string; state: State; store: Store } }) {
  return (
    <div class={`stitle ${roleClass(role)}`} style={`--rule:var(--role-${rule.replaceAll(".", "-")}-fg)`}
      onMouseDown={onPress && ((e) => { e.preventDefault(); onPress(); })}>
      <h2 class="stext">{label}</h2>
      {collapse && !sideForced(collapse.state) && canToggle(collapse.state) && (
        <button class={roleClass(collapse.role)} aria-label={str.webCollapseSidebar()} aria-expanded="true" onMouseDown={(e) => e.stopPropagation()}
          onClick={() => toggleSidebar(collapse.store)}>{collapseButton}</button>
      )}
    </div>
  );
}
