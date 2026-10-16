import { useLayoutEffect, useRef } from "preact/hooks";
import { filterKey, fixSel, glyphClick, panelAct, panelClick, panelRows } from "../filter.ts";
import type { PanelKey } from "../filter.ts";
import { logAct, logKey } from "../log.ts";
import type { LogKey } from "../log.ts";
import type { State, Store } from "../store.ts";
import { titled } from "../layout.ts";
import * as str from "../str.gen.ts";
import { roleClass } from "../theme.ts";

// FilterPanel is log mode's filter panel in the sidebar, as the TUI's:
// each entry its name (indented by depth; ▼ > on a group, × on a text
// row) and its Hide and Only, a blank row after each top-level group, +
// Text last. Hide and Only are toggle buttons; each entry holds a group
// named by its entry, so they're read with it. A group's ▼ > is its fold
// button, named by the entry too, as it's the TUI's fold control. The
// highlight is the
// roving focus: ↑/↓ move it, and only its entry's controls are in the
// tab order. Its keys work while it has focus (filter.ts), and so do the
// log's that the panel doesn't share (s, /, g, c, e, n, N). Under a
// title; at the bottom its footer: its keys while it has focus, those
// that reach it from the log while that has, as the TUI's.
export function FilterPanel({ state, store }: { state: State; store: Store }) {
  const k = state.active;
  const v = state.logView[k];
  const p = v?.panel ?? { sel: "" }; // first entry until one's highlighted
  const rows = panelRows(state.filters[k]?.Entries ?? []);
  const sel = fixSel(rows, p.sel);
  const nav = useRef<HTMLElement>(null);

  // Focus goes to the highlight when asked (opening, a key's move), as
  // the TUI's follow scrolls to it; never on a draw Go caused.
  useLayoutEffect(() => {
    if (!p.follow) return;
    nav.current?.querySelector<HTMLElement>(".fsel")?.focus();
    store.setLogView(k, { panel: { sel, follow: false } });
  });

  const onKeyDown = (e: KeyboardEvent) => {
    const does = filterKey(e);
    if (!does) {
      const l = logKey(e); // the log's own keys reach it from here too
      if (l && !sharedLogKeys.has(l)) {
        e.preventDefault();
        logAct(store, l);
      }
      return;
    }
    // Enter or Space on Hide, Only or a fold glyph presses it, as a
    // button's does; they're the panel's only on its highlighted row.
    if ((does === "enter" || does === "toggle") && !(e.target as HTMLElement).classList.contains("fsel")) return;
    e.preventDefault();
    panelAct(store, does);
  };

  return (
    <nav ref={nav} class={`side filter-panel ${roleClass("filter")}`} aria-label={str.webFilter()} onKeyDown={onKeyDown}
      onFocusIn={() => { if (v?.focus !== "panel") store.setLogView(k, { focus: "panel" }); }}
      onFocusOut={(e) => { if (!nav.current?.contains(e.relatedTarget as Node)) store.setLogView(k, { focus: undefined }); }}>
      <h2 class={`stitle ${roleClass("filter.title")}`}>{titled(str.filterTitle())}</h2>
      <ul>
        {/* One flat keyed list, not an array per entry: Preact matches
            arrays by position, so a row Go adds or drops above the
            highlight would remount it and lose focus. */}
        {rows.flatMap((r, i) => {
          const on = r.key === sel;
          const tab = on ? 0 : -1;
          if (r.kind === "add") {
            return [
              <li key="+" class="fentry">
                {" "}
                <button class={`fname ${on ? "fsel " : ""}${roleClass(on ? "filter.selected" : "filter.add")}`} tabindex={tab}
                  onClick={() => panelClick(store, "+", "add")}>{str.filterAddText()}</button>
              </li>,
            ];
          }
          const { e } = r;
          const pad = " ".repeat(1 + 2 * e.Depth);
          const id = `flt-${i}`;
          // The glyph and • light with the name, as the TUI's.
          const look = roleClass(on ? "filter.selected" : "filter.item");
          return [
            // The group sits inside the list item, so the list keeps its
            // items.
            <li key={r.key} class="fentry"><div role="group" aria-labelledby={id}>
              <div class="frow">
                {pad}
                {/* A click focuses the button, as not every browser does. */}
                {e.Parent
                  ? <button class={`fglyph ${look}`} tabindex={tab} aria-expanded={!e.Folded} aria-labelledby={id}
                      onClick={(ev) => { ev.currentTarget.focus(); glyphClick(store, r.key); }}>{r.glyph + " "}</button>
                  : r.glyph && <span aria-hidden="true" class={`fglyph ${look}`} onClick={() => glyphClick(store, r.key)}>{r.glyph + " "}</span>}
                <button id={id} class={`fname ${on ? "fsel " : ""}${look}`} tabindex={tab}
                  onClick={() => panelClick(store, r.key)}>{r.name}</button>
                {r.after && <span aria-hidden="true" class={look}>{" " + r.after}</span>}
              </div>
              <div class="frow">
                {pad + " "}
                <button class={roleClass(e.Hidden ? "filter.button.on" : "filter.button")} tabindex={tab} aria-pressed={e.Hidden}
                  onClick={() => panelClick(store, r.key, "hide")}>{str.filterHide()}</button>
                {str.separator()}
                <button class={roleClass(e.Only ? "filter.button.on" : "filter.button")} tabindex={tab} aria-pressed={e.Only}
                  onClick={() => panelClick(store, r.key, "only")}>{str.filterOnly()}</button>
              </div>
            </div></li>,
            ...(r.endsGroup ? [<li key={r.key + "\n"} aria-hidden="true"> </li>] : []),
          ];
        })}
      </ul>
      <div class={`ffoot ${roleClass("filter.hints")}`}>
        {(v?.focus === "panel" ? str.filterHints() : str.filterHintsAway()).split(str.separator()).map((h, i) => <span key={i}>{i > 0 && str.separator()}{h}</span>)}
      </div>
    </nav>
  );
}

// sharedLogKeys are the log's keys the panel has its own use for: from
// the panel they're the panel's.
const sharedLogKeys = new Set<LogKey>(["up", "down", "exclude", "back"]);

// panelOnlyKeys are the panel's keys the log doesn't use: they work from
// the log too.
export const panelOnlyKeys = new Set<PanelKey>(["hide", "only", "shut", "open", "remove"]);
