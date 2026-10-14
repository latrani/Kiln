// The page's entry: the store, the app drawn from it, and Kiln booting.
import "./kiln.css";
import { boot } from "./boot.ts";
import { offer } from "./notify.ts";
import { effect, mount } from "./page.tsx";
import { Store } from "./store.ts";

const store: Store = new Store((e) => effect(store)(e));
store.set({ notifyOffer: offer() });
mount(store);
boot(store).catch((e) => {
  console.error(e);
  store.set({ phase: "stopped" });
});
