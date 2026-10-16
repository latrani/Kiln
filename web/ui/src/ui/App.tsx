// The page: the TUI's five areas in a grid (kiln.css), styled by the
// theme Go sends (or the last one it sent, until it sends one), or while
// another tab has Kiln, the splash.
import { logShows } from "../store.ts";
import type { Store } from "../store.ts";
import { narrowWidth, paneSize, sidebarCSS } from "../layout.ts";
import { recallTheme, rememberTheme, themeCSS } from "../theme.ts";
import { useEffect, useRef, useState } from "preact/hooks";
import { titleFor } from "../title.ts";
import { Announcer } from "./Announcer.tsx";
import { BottomPane } from "./BottomPane.tsx";
import { LogView } from "./LogView.tsx";
import { Scrollback } from "./Scrollback.tsx";
import { Sidebar } from "./Sidebar.tsx";
import { sideShown } from "../sidebar.ts";
import { Splash } from "./Splash.tsx";
import { StatusLine } from "./StatusLine.tsx";
import { TopBar } from "./TopBar.tsx";
import { useStore } from "./use.ts";

export function App({ store }: { store: Store }) {
  const s = useStore(store);
  const title = titleFor(s);
  useEffect(() => { document.title = title; }, [title]);
  const [recalled] = useState(recallTheme);
  useEffect(() => { if (s.theme) rememberTheme(s.theme); }, [s.theme]);
  // Narrow under 64 of the page's own characters across, as the TUI's
  // NarrowWidth: measured in its font, as a media query's ch isn't.
  const kiln = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const el = kiln.current;
    if (!el || typeof ResizeObserver === "undefined") return;
    const check = () => {
      const narrow = paneSize(el).cols < narrowWidth;
      if (narrow !== !!store.state.narrow) store.set({ narrow, narrowOpen: false }); // narrow starts closed
    };
    const ro = new ResizeObserver(check);
    ro.observe(el);
    check();
    return () => ro.disconnect();
  }, [s.phase === "locked"]);
  return (
    <>
      <style dangerouslySetInnerHTML={{ __html: themeCSS(s.theme ?? recalled) }} />
      {s.phase === "locked" ? <Splash state={s} /> : (
        <div ref={kiln} class={s.narrow ? (sideShown(s) ? "kiln narrow open" : "kiln narrow") : sideShown(s) ? "kiln" : "kiln collapsed"} style={{ "--side": sidebarCSS() }}>
          {sideShown(s) && <Sidebar state={s} store={store} />}
          {sideShown(s) && !s.narrow && <div class="divider" />}
          {!(s.narrow && sideShown(s)) && <TopBar state={s} store={store} />}
          {logShows(s, s.active) ? <LogView state={s} store={store} /> : <Scrollback state={s} />}
          <BottomPane state={s} store={store} />
          <StatusLine state={s} store={store} />
        </div>
      )}
      <Announcer store={store} />
    </>
  );
}
