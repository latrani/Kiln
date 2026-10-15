// /reset against the tab lock: a wipe and a handoff to another tab
// (Use here) must not cross. A handoff waits out a wipe already running,
// so the other tab loads after the clear; a reset asked once a handoff
// has begun does nothing, since Kiln is the other tab's now.
export function resetter(o: { wipe: () => Promise<void>; forget: () => void; reload: () => void }) {
  let wiping: Promise<void> | undefined;
  let leaving = false;
  return {
    // reset wipes what the browser saved, forgets what's kept beside it
    // (the notifications offer's answer, the last theme), and reloads; it rejects, reloading nothing, if the
    // wipe fails.
    async reset(): Promise<void> {
      if (leaving) return;
      wiping = o.wipe();
      await wiping;
      o.forget();
      o.reload();
    },
    // leave is the handoff's first step: no reset after it, and none
    // still running.
    async leave(): Promise<void> {
      leaving = true;
      await wiping?.catch(() => {});
    },
  };
}
