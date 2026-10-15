// The page: the TUI's five areas in a grid (kiln.css), styled by the
// theme Go sends (or the last one it sent, until it sends one), or while
// another tab has Kiln, the splash.
import { logShows } from "../store.ts";
import type { Store } from "../store.ts";
import { sidebarCSS } from "../layout.ts";
import { recallTheme, rememberTheme, themeCSS } from "../theme.ts";
import { useEffect, useState } from "preact/hooks";
import { titleFor } from "../title.ts";
import { Announcer } from "./Announcer.tsx";
import { BottomPane } from "./BottomPane.tsx";
import { LogView } from "./LogView.tsx";
import { Scrollback } from "./Scrollback.tsx";
import { Sidebar } from "./Sidebar.tsx";
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
  return (
    <>
      <style dangerouslySetInnerHTML={{ __html: themeCSS(s.theme ?? recalled) }} />
      {s.phase === "locked" ? <Splash state={s} /> : (
        <div class="kiln" style={{ "--side": sidebarCSS() }}>
          <Sidebar state={s} store={store} />
          <div class="divider" />
          <TopBar state={s} store={store} />
          {logShows(s, s.active) ? <LogView state={s} store={store} /> : <Scrollback state={s} />}
          <BottomPane state={s} store={store} />
          <StatusLine state={s} store={store} />
        </div>
      )}
      <Announcer store={store} />
    </>
  );
}
