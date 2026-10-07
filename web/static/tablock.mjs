// tablock: one tab owns Kiln. Two tabs saving their own memfs would
// overwrite each other's files, and two live Kilns would connect the same
// characters twice. The owner holds the Web Lock "kiln" for its lifetime;
// another tab can ask for it over BroadcastChannel ("release"), and takes
// it by force if the owner doesn't answer (a frozen background tab).

const NAME = "kiln";

export function createTabLock({ locks, openChannel, onLost, setTimer = setTimeout, clearTimer = clearTimeout, stealAfter = 3000 }) {
  let release = null;
  // hold is the lock callback: it reports whether the lock was granted,
  // and keeps it until release() is called.
  const hold = (got) => (lock) => {
    got(!!lock);
    if (!lock) return undefined;
    return new Promise((r) => { release = r; });
  };
  const lost = (e) => {
    if (e?.name !== "AbortError") return;
    release = null; // stolen: no longer ours to flush or let go of
    onLost();
  };

  return {
    // tryTake resolves true when this tab now owns Kiln.
    tryTake() {
      return new Promise((ok) => { locks.request(NAME, { ifAvailable: true }, hold(ok)).catch(lost); });
    },
    // serve answers other tabs' requests: beforeRelease (save everything),
    // then let go of the lock, then afterRelease (reload, which shows this
    // tab the lock screen).
    serve(beforeRelease, afterRelease) {
      const ch = openChannel();
      ch.onmessage = async (e) => {
        if (e.data !== "release" || !release) return;
        const r = release;
        release = null; // before awaiting, so a second message does nothing
        await beforeRelease();
        r();
        afterRelease();
      };
    },
    // takeOver asks the owner to let go, and steals the lock if it hasn't
    // after stealAfter ms. It resolves once this tab owns Kiln.
    takeOver() {
      return new Promise((ok) => {
        const ac = new AbortController();
        const timer = setTimer(() => {
          ac.abort();
          locks.request(NAME, { steal: true }, hold(() => ok())).catch(lost);
        }, stealAfter);
        let got = false;
        locks.request(NAME, { signal: ac.signal }, (lock) => {
          got = true;
          clearTimer(timer);
          return hold(() => ok())(lock);
        }).catch((e) => { if (got) lost(e); }); // before the grant: our own steal's abort
        openChannel().postMessage("release");
      });
    },
  };
}
