// Offline launches: once Kiln is running, the page tells its service
// worker (sw.ts) which files this launch used, and the worker keeps them
// as one set for a launch with no network.

// launchedFiles is the page itself (scope) and the files under scope
// that names (the page's resource entries) list, once each and without
// queries, leaving out the relay and the worker's own script.
export function launchedFiles(names: string[], scope: string): string[] {
  const out = new Set([scope]);
  for (const n of names) {
    const u = new URL(n);
    const key = u.origin + u.pathname;
    if (!key.startsWith(scope)) continue;
    const rel = key.slice(scope.length);
    if (rel === "relay" || rel === "sw.js") continue; // str:ok paths
    out.add(key);
  }
  return [...out];
}

// launched tells the worker this launch is running, once it's ready.
export async function launched(sw: ServiceWorkerContainer | undefined): Promise<void> {
  try {
    if (!sw) return;
    const reg = await sw.ready;
    const scope = new URL(reg.scope).href;
    const names = performance.getEntriesByType("resource").map((e) => e.name);
    reg.active?.postMessage({ t: "launched", files: launchedFiles(names, scope) }); // str:ok message type
  } catch (e) {
    console.error("offline", e); // str:ok log tag
  }
}
