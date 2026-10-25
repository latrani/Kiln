import { Fragment } from "preact";
import { tagsFor } from "../look.ts";
import type { State } from "../store.ts";
import * as str from "../str.gen.ts";
import { roleClass } from "../theme.ts";
import { Line } from "./Line.tsx";

// overviewLines is how many of each character's last lines show.
const overviewLines = 5;

// Overview is a world's overview, as the TUI draws it: each of its open
// characters, when its last line came, and its last few lines cut to a
// row each, a rule after each character. A character's block switches to
// it (a link in it only opens); its name is the button that does it.
export function Overview({ state, world }: { state: State; world: string }) {
  const keys = state.order.filter((k) => state.chars[k]?.World === world);
  return (
    <>
      {keys.map((k) => {
        const c = state.chars[k];
        const go = () => state.act?.({ Do: "switch", Key: k });
        const tags = tagsFor(state.theme, k);
        return (
          <Fragment key={k}>
            <div class="ov" onClick={(e) => { if (!(e.target as Element).closest("a")) go(); }}>
              <div class="line">
                <button class={roleClass("scrollback.overview")} onClick={(e) => { e.stopPropagation(); go(); }}>{c.Name}</button>
                <span class={roleClass("scrollback.sys")}>{str.separator() + (c.LastSeen || str.viewOverviewQuiet())}</span>
              </div>
              {(state.lines[k]?.lines ?? []).slice(-overviewLines).map((l) => <Line key={l.Seq} line={l} tags={tags} />)}
            </div>
            <hr class={"ov-rule " + roleClass("scrollback.rule")} />
          </Fragment>
        );
      })}
    </>
  );
}
