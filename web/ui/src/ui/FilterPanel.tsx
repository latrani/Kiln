import { useLayoutEffect, useRef } from "preact/hooks";
import { filterKey, fixSel, glyphClick, panelAct, panelRows } from "../filter.ts";
import type { State, Store } from "../store.ts";
import * as str from "../str.gen.ts";
import { roleClass } from "../theme.ts";

// FilterPanel is log mode's filter panel in the sidebar, as the TUI's:
// each entry its name (indented by depth; ▼ ► on a group, × on a text
// row) and its Hide and Only, a blank row after each top-level group, +
// Text last. Hide and Only are toggle buttons; each entry is a group
// named by its entry, so they're read with it. The highlight is the
// roving focus: ↑/↓ move it, and only its entry's controls are in the
// tab order. Its keys work while it has focus (filter.ts).
export function FilterPanel({ state, store }: { state: State; store: Store }) {
  const k = state.active;
  const p = state.logView[k]!.panel!;
  const rows = panelRows(state.filters[k]?.Entries ?? []);
  const sel = fixSel(rows, p.sel);
  const nav = useRef<HTMLElement>(null);
  const act = state.act ?? (() => {});

  // Focus goes to the highlight when asked (opening, a key's move), as
  // the TUI's follow scrolls to it; never on a draw Go caused.
  useLayoutEffect(() => {
    if (!p.follow) return;
    nav.current?.querySelector<HTMLElement>(".fsel")?.focus();
    store.setLogView(k, { panel: { sel, follow: false } });
  });

  const pick = (key: string) => store.setLogView(k, { panel: { sel: key } });
  const onKeyDown = (e: KeyboardEvent) => {
    const does = filterKey(e);
    if (!does) return;
    // Enter on Hide or Only presses it, as a button's Enter does; Enter
    // is the panel's only on its highlighted row.
    if (does === "add" && !(e.target as HTMLElement).classList.contains("fsel")) return;
    e.preventDefault();
    panelAct(store, does);
  };

  return (
    <nav ref={nav} class={`side filter-panel ${roleClass("filter")}`} aria-label={str.webFilter()} onKeyDown={onKeyDown}>
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
                  onClick={() => { pick("+"); store.setLogView(k, { prompt: { kind: "filter", value: "" } }); }}>{str.filterAddText()}</button>
              </li>,
            ];
          }
          const { e } = r;
          const pad = " ".repeat(1 + 2 * e.Depth);
          const id = `flt-${i}`;
          return [
            <li key={r.key} class="fentry" role="group" aria-labelledby={id}>
              <div class="frow">
                {pad}
                {r.glyph && <><span aria-hidden="true" class="fglyph" onClick={() => glyphClick(store, r.key)}>{r.glyph}</span>{" "}</>}
                <button id={id} class={`fname ${on ? "fsel " : ""}${roleClass(on ? "filter.selected" : "filter.item")}`} tabindex={tab}
                  aria-expanded={e.Parent ? !e.Folded : undefined} onClick={() => pick(r.key)}>{r.name}</button>
                {r.after && <span aria-hidden="true">{" " + r.after}</span>}
              </div>
              <div class="frow">
                {pad + " "}
                <button class={roleClass(e.Hidden ? "filter.button.on" : "filter.button")} tabindex={tab} aria-pressed={e.Hidden}
                  onClick={() => { pick(r.key); act({ Do: "hide", Key: k, Item: e.Item }); }}>{str.filterHide()}</button>
                {" "}
                <button class={roleClass(e.Only ? "filter.button.on" : "filter.button")} tabindex={tab} aria-pressed={e.Only}
                  onClick={() => { pick(r.key); act({ Do: "only", Key: k, Item: e.Item }); }}>{str.filterOnly()}</button>
              </div>
            </li>,
            ...(r.endsGroup ? [<li key={r.key + "\n"} aria-hidden="true"> </li>] : []),
          ];
        })}
      </ul>
    </nav>
  );
}
