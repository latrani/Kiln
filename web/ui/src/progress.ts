// How far Kiln has got loading, in the status line's words.
import * as str from "./str.gen.ts";

const mb = (n: number) => (n / 1e6).toFixed(1);

// loadingText is the status line's words while the wasm downloads.
export function loadingText(got: number, total?: number): string {
  return total ? str.webLoadingSize(mb(got), mb(total)) : str.webLoading(mb(got));
}
