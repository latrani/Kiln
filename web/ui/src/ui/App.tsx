// The page: the TUI's five areas in a grid (kiln.css), styled by the
// theme Go sends.
import type { Store } from "../store.ts";
import { sidebarCSS } from "../layout.ts";
import { themeCSS } from "../theme.ts";
import { useEffect } from "preact/hooks";
import { titleFor } from "../title.ts";
import { Announcer } from "./Announcer.tsx";
import { BottomPane } from "./BottomPane.tsx";
import { Scrollback } from "./Scrollback.tsx";
import { Sidebar } from "./Sidebar.tsx";
import { StatusLine } from "./StatusLine.tsx";
import { TopBar } from "./TopBar.tsx";
import { useStore } from "./use.ts";

export function App({ store }: { store: Store }) {
  const s = useStore(store);
  const title = titleFor(s);
  useEffect(() => { document.title = title; }, [title]);
  return (
    <>
      <style dangerouslySetInnerHTML={{ __html: themeCSS(s.theme) }} />
      <div class="kiln" style={{ "--side": sidebarCSS() }}>
        <Sidebar state={s} store={store} />
        <div class="divider" />
        <TopBar state={s} />
        <Scrollback state={s} />
        <BottomPane state={s} store={store} />
        <StatusLine state={s} />
      </div>
      <Announcer store={store} />
    </>
  );
}
