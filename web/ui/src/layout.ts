// Sizes in characters, as the TUI counts them.
import * as str from "./str.gen.ts";

const len = (s: string) => [...s].length;

// chipsWidth is the narrowest the sidebar gets: wide enough for its add
// chips, padded a space either side (the TUI's SidebarWidth).
export function chipsWidth(): number {
  return 2 + Math.max(len(str.sidebarOpenConnection()), 1 + len(str.pickerAddCharacter()), len(str.pickerAddWorld()));
}

// sidebarCSS is the sidebar's width as the TUI works it out, in whole
// characters: a fifth of the window, at least chipsWidth, at most 22.
export function sidebarCSS(): string {
  return `min(22ch, max(${chipsWidth()}ch, round(down, 20vw, 1ch)))`;
}

// paneSize is how many characters fit across el and how many lines down,
// measured with a hidden box one character wide and one line high.
export function paneSize(el: HTMLElement): { cols: number; rows: number } {
  const probe = document.createElement("div");
  probe.style.cssText = "position:absolute;visibility:hidden;width:1ch;height:1lh";
  el.append(probe);
  const { width, height } = probe.getBoundingClientRect();
  probe.remove();
  if (!width || !height) return { cols: 0, rows: 0 };
  return { cols: Math.floor(el.clientWidth / width), rows: Math.floor(el.clientHeight / height) };
}
