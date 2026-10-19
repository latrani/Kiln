// The page's entry: the store, the app drawn from it, and Kiln booting.
import "./kiln.css";
import { boot } from "./boot.ts";
import { offer } from "./notify.ts";
import { effect, mount } from "./page.tsx";
import { Store } from "./store.ts";
import { webHost } from "./webhost.ts";

const host = webHost();
const store: Store = new Store((e) => effect(store)(e), host);
mount(store);
offer(store).then((notifyOffer) => store.set({ notifyOffer }));
boot(store, host).catch((e) => {
  console.error(e);
  store.set({ phase: "stopped" });
});
