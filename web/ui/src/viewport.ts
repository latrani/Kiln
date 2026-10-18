// Fitting the page to what the on-screen keyboard leaves. iOS Safari
// doesn't shrink the page for its keyboard (interactive-widget isn't in
// a shipped Safari yet): it lays the page out under the keyboard and
// pans to the focused field, so the bottom pane is scrolled away
// instead of riding above the keyboard. The visual viewport is what
// shows, so the grid takes its height and follows its pan.

// Viewportish is the part of a VisualViewport fit reads.
export interface Viewportish {
  height: number;
  offsetTop: number;
  scale: number;
}

// fit is the grid's height and how far down it moves, in px, or
// undefined to leave it at 100dvh: zoomed in (the layout stays put
// under a pinch) or nothing covering the page. layout is the layout
// viewport's height. With nothing covering, there's no pan either: iOS
// 26 can leave offsetTop set after the keyboard goes.
export function fit(vv: Viewportish, layout: number): { height: number; top: number } | undefined {
  if (Math.abs(vv.scale - 1) > 0.01 || vv.height >= layout - 1) return undefined;
  return { height: Math.round(vv.height), top: Math.round(vv.offsetTop) };
}

// watchViewport keeps --vvh and --vvtop on root (kiln.css reads them)
// to fit, and --vvbottom at 0 while it applies (the keyboard covers the
// home indicator), as the visual viewport changes. It returns the stop.
export function watchViewport(vv: (Viewportish & EventTarget) | null | undefined, root: HTMLElement): () => void {
  if (!vv) return () => {};
  const update = () => {
    const f = fit(vv, root.clientHeight);
    if (f) {
      root.style.setProperty("--vvh", f.height + "px");
      root.style.setProperty("--vvtop", f.top + "px");
      root.style.setProperty("--vvbottom", "0px");
    } else {
      root.style.removeProperty("--vvh");
      root.style.removeProperty("--vvtop");
      root.style.removeProperty("--vvbottom");
    }
  };
  update();
  vv.addEventListener("resize", update);
  vv.addEventListener("scroll", update);
  return () => {
    vv.removeEventListener("resize", update);
    vv.removeEventListener("scroll", update);
  };
}
