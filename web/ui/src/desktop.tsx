// The desktop page's entry (desktop/, a Wails app): the page, with Go
// native in the app behind the Kiln service's bindings in place of the
// wasm. Only build.mjs's desktop build makes it. The Wails runtime is
// bundled: the app serves only the page's own files.
import "./kiln.css";
import { Events } from "@wailsio/runtime";
import * as Kiln from "./bindings/github.com/latrani/Kiln/desktop/kiln.ts";
import { desktopHost, openLinks, run } from "./desktophost.ts";
import { offer } from "./notify.ts";
import { effect, mount } from "./page.tsx";
import { Store } from "./store.ts";

const host = desktopHost(Kiln);
const store: Store = new Store((e) => effect(store)(e), host);
mount(store);
offer(store).then((notifyOffer) => store.set({ notifyOffer }));
openLinks(document, Kiln.OpenURL);
run(store, Kiln, (name, f) => Events.On(name, (e) => f(e.data)));
