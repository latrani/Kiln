// The TUI's pager and pill (internal/ui/scrollback.go's Pause), over the
// lines' offsets and heights in pixels rather than wrapped rows.

// pause is where the view stops when the lines not yet seen (from
// firstUnseen on) are more than fit in view: the scrollTop putting the
// first of them at the top. undefined means stay live, at the bottom.
export function pause(tops: number[], heights: number[], firstUnseen: number, view: number): number | undefined {
  if (firstUnseen < 0 || firstUnseen >= tops.length) return undefined;
  const end = tops.at(-1)! + heights.at(-1)!;
  return end - tops[firstUnseen] <= view ? undefined : tops[firstUnseen];
}

// below is how many lines lie wholly below the view: the pill's count.
export function below(tops: number[], scrollTop: number, view: number): number {
  return tops.filter((t) => t >= scrollTop + view).length;
}
