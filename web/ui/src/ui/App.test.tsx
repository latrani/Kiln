import { test } from "node:test";
import assert from "node:assert/strict";
import { render } from "preact-render-to-string";
import { App } from "./App.tsx";
import { Store } from "../store.ts";
import type { Event, State } from "../store.ts";
import { loadingText } from "../boot.ts";
import { sidebarCSS } from "../layout.ts";
import { roleClass } from "../theme.ts";
import * as str from "../str.gen.ts";

const page = (patch: Partial<State> = {}, batch: Event[] = []) => {
  const s = new Store();
  s.set(patch);
  s.apply(batch);
  return render(<App store={s} />);
};

test("five areas, with landmarks", () => {
  const html = page({ phase: "running" });
  for (const want of ["<header", "<nav", 'class="bottom', 'role="status"']) assert.ok(html.includes(want), want);
  assert.ok(html.includes(`aria-label="${str.webCharacters()}"`));
  assert.match(html, /<main(?![^>]*role=)[^>]*>.*<div[^>]*role="log"[^>]*>.*<\/main>/s, "the scrollback is the main landmark, with the log inside it");
});

test("buttons look like the TUI's", () => {
  for (const patch of [{ phase: "locked", onUseHere: () => {} }, { phase: "stopped" }] as Partial<State>[]) {
    assert.match(page(patch), /<button[^>]*class="[^"]*r-form-button/, JSON.stringify(patch.phase));
  }
});

test("loading shows progress in the status line", () => {
  assert.ok(page({ phase: "loading", progress: { got: 2_100_000, total: 6_400_000 } }).includes(loadingText(2_100_000, 6_400_000)));
});

test("another tab: the message and Use here, in the scrollback", () => {
  const html = page({ phase: "locked" });
  assert.ok(html.includes(str.webOtherTab()));
  assert.ok(html.includes(`>${str.webUseHere()}</button>`));
});

test("stopped: Reload", () => {
  assert.ok(page({ phase: "stopped" }).includes(`>${str.webReload()}</button>`));
});

// top and status are the top bar and the status line.
const top = (html: string) => html.slice(html.indexOf("<header"), html.indexOf("</header>"));
const status = (html: string) => html.slice(html.indexOf('class="status '), html.indexOf("</div>", html.indexOf('class="status ')));

// liveRegions is each live region's role and text, in order.
const liveRegions = (html: string) => [...html.matchAll(/<(\w+)[^>]*role="(status|alert)"[^>]*>(.*?)<\/\1>/g)].map((m) => [m[2], m[3].replace(/<[^>]*>/g, "")]);

test("the status line keeps one polite and one urgent region, always", () => {
  for (const patch of [{ phase: "running" }, { phase: "loading", progress: { got: 1, total: 2 } }, { phase: "stopped" }] as Partial<State>[]) {
    assert.deepEqual(liveRegions(status(page(patch))).map((r) => r[0]), ["status", "alert"], JSON.stringify(patch));
  }
});

test("errors go in the urgent region, the rest in the polite one", () => {
  assert.deepEqual(liveRegions(status(page({ phase: "running" }, [{ type: "Status", Text: "boom", Err: true, Gen: 1 }]))), [["status", ""], ["alert", "boom"]]);
  assert.deepEqual(liveRegions(status(page({ phase: "running" }, [{ type: "Status", Text: "hi", Err: false, Gen: 1 }]))), [["status", "hi"], ["alert", ""]]);
  assert.deepEqual(liveRegions(status(page({ phase: "running", saving: false }))), [["status", str.webNotSaving()], ["alert", ""]]);
  assert.deepEqual(liveRegions(status(page({ phase: "stopped" }))), [["status", ""], ["alert", str.webStopped()]]);
});

test("download progress isn't read aloud chunk by chunk", () => {
  const html = page({ phase: "loading", progress: { got: 2_100_000, total: 6_400_000 } });
  assert.ok(html.includes(loadingText(2_100_000, 6_400_000)));
  assert.ok(!liveRegions(html).some((r) => r[1].includes(loadingText(2_100_000, 6_400_000))));
});

test("the theme goes into the page's style", () => {
  const html = page({ phase: "running" }, [{ type: "Theme", Appearance: "dark", Dark: { Roles: { status: { FG: "#123456" } }, Tags: {} }, Light: { Roles: {}, Tags: {} } }]);
  assert.ok(html.includes("#123456"));
});

test("loading, before any theme, is already in the terminal's colors", () => {
  assert.ok(page({ phase: "loading", progress: { got: 1 } }).includes("--term-bg:"));
});

test("the sidebar's width is the TUI's", () => {
  assert.ok(page({ phase: "running" }).includes(`--side:${sidebarCSS()}`));
});

const kitOpen = (more: Event[] = []): Event[] => [
  { type: "Order", Keys: ["fm/kit"] },
  { type: "Char", Key: "fm/kit", World: "fm", Name: "Kit", State: 2 },
  { type: "Active", Key: "fm/kit" },
  { type: "Lines", Key: "fm/kit", Epoch: 1, Op: "reset", Lines: [
    { Seq: 1, Kind: "server", Day: "", Out: false, Plain: "first line", Spans: [{ Text: "first line" }], Priority: false, Quiet: false },
    { Seq: 2, Kind: "server", Day: "", Out: false, Plain: "second line", Spans: [{ Text: "second line" }], Priority: false, Quiet: false },
  ] },
  ...more,
];

test("the scrollback shows the active character's lines", () => {
  const html = page({ phase: "running" }, kitOpen());
  assert.ok(html.indexOf("first line") > 0 && html.indexOf("second line") > html.indexOf("first line"));
});

test("older history on its way: the loading row", () => {
  const html = page({ phase: "running" }, kitOpen([{ type: "Char", Key: "fm/kit", World: "fm", Name: "Kit", State: 2, More: true, Loading: true }]));
  assert.ok(html.includes(str.scrollbackLoading()));
  assert.ok(!page({ phase: "running" }, kitOpen()).includes(str.scrollbackLoading()));
});

test("the server's prompt sits after the lines", () => {
  const html = page({ phase: "running" }, kitOpen([{ type: "Input", Key: "fm/kit", Text: "", Prompt: [{ Text: "Password: " }] }]));
  assert.ok(html.indexOf("Password: ") > html.indexOf("second line"));
});

const three: Event[] = [
  { type: "Order", Keys: ["fm/kit", "fm/rook", "pf/ash"] },
  { type: "Char", Key: "fm/kit", World: "fm", Name: "Kit", State: 2 },
  { type: "Char", Key: "fm/rook", World: "fm", Name: "Rook", State: 0, Unread: 3, Priority: true },
  { type: "Char", Key: "pf/ash", World: "pf", Name: "Ash", State: 1 },
  { type: "Active", Key: "fm/kit" },
];
// row is the sidebar's <li> holding text.
const row = (html: string, text: string) => [...html.matchAll(/<li[^>]*>.*?<\/li>/g)].map((m) => m[0]).find((r) => r.includes(text)) ?? "";

test("the sidebar: worlds, characters, the active one", () => {
  const html = page({ phase: "running" }, three);
  const nav = html.slice(html.indexOf("<nav"), html.indexOf("</nav>"));
  assert.ok(nav.indexOf(">fm<") < nav.indexOf(">Kit<") && nav.indexOf(">Rook<") < nav.indexOf(">pf<") && nav.indexOf(">pf<") < nav.indexOf(">Ash<"), nav);
  const kit = row(html, ">Kit<");
  assert.ok(kit.includes(roleClass("sidebar.active")), kit);
  assert.match(kit, /<button[^>]*aria-current="true"[^>]*>Kit</);
});

test("the sidebar: unread, priority and closing", () => {
  const rook = row(page({ phase: "running" }, three), ">Rook<");
  assert.ok(rook.includes(`aria-label="${str.webCharPriority("Rook", 3)}"`), rook);
  assert.ok(rook.includes(str.glyph.priority) && rook.includes(roleClass("sidebar.priority")), rook);
  assert.match(rook, new RegExp(`class="${roleClass("sidebar.unread")}"[^>]*>3<`));
  assert.match(rook, new RegExp(`<button[^>]*aria-label="${str.webCloseChar("Rook")}"[^>]*>${str.glyph.remove}</button>`));
  const quiet = page({ phase: "running" }, [...three, { type: "Char", Key: "fm/rook", World: "fm", Name: "Rook", State: 0, Unread: 2 }]);
  assert.ok(row(quiet, ">Rook<").includes(`aria-label="${str.webCharUnread(2, "Rook")}"`));
  assert.ok(!row(quiet, ">Rook<").includes(str.glyph.priority));
});

test("the sidebar: a character connecting", () => {
  const ash = row(page({ phase: "running" }, three), ">Ash<");
  assert.match(ash, new RegExp(`<span class="lead ${roleClass("sidebar.connecting")}"> ${str.glyph.connecting} </span>`), ash);
  assert.ok(!ash.includes(str.webCloseChar("Ash")));
});

test("the sidebar: a world's overview is active on its world", () => {
  const html = page({ phase: "running" }, [...three, { type: "Active", Key: "=fm" }]);
  assert.match(html, new RegExp(`<button[^>]*class="[^"]*${roleClass("sidebar.active")}[^"]*"[^>]*aria-current="true"[^>]*>fm<`));
});

const server = (Seq: number, Plain: string) => ({ Seq, Kind: "server", Day: "", Out: false, Plain, Spans: [{ Text: Plain }], Priority: false, Quiet: false });

test("a world's overview: each character, when it last heard, its last lines", () => {
  const html = page({ phase: "running" }, [
    { type: "Order", Keys: ["fm/kit", "fm/rook"] },
    { type: "Char", Key: "fm/kit", World: "fm", Name: "Kit", State: 2, LastSeen: "21:06" },
    { type: "Char", Key: "fm/rook", World: "fm", Name: "Rook", State: 0, LastSeen: "" },
    { type: "Lines", Key: "fm/kit", Epoch: 1, Op: "reset", Lines: [1, 2, 3, 4, 5, 6, 7].map((n) => server(n, `kit line ${n}`)) },
    { type: "Active", Key: "=fm" },
  ]);
  const main = html.slice(html.indexOf("<main"), html.indexOf("</main>"));
  assert.ok(main.includes(">Kit<") && main.includes(str.separator() + "21:06"), main);
  for (const n of [3, 4, 5, 6, 7]) assert.ok(main.includes(`kit line ${n}<`), `line ${n}`);
  for (const n of [1, 2]) assert.ok(!main.includes(`kit line ${n}<`), `line ${n} is past the last five`);
  assert.ok(main.indexOf("<hr") > main.indexOf("kit line 7") && main.indexOf(">Rook<") > main.indexOf("<hr"));
  assert.ok(main.includes(str.separator() + str.viewOverviewQuiet()));
  assert.ok(main.includes(roleClass("scrollback.overview")) && main.includes(roleClass("scrollback.rule")));
});

test("nothing configured: the empty pane, and nothing breaks", () => {
  const html = page({ phase: "running" }, [{ type: "Worlds", Worlds: [] }]);
  if (str.viewNoCharacters()) assert.ok(html.includes(str.viewNoCharacters()));
  assert.ok(html.includes("<main"));
});


test("the top bar: where you are, and the presence chip", () => {
  const kit = top(page({ phase: "running" }, three));
  assert.ok(kit.includes("fm" + str.separator() + "Kit"), kit);
  assert.match(kit, new RegExp(`<button[^>]*class="[^"]*${roleClass("status.presence.unknown")}[^"]*"[^>]*>${str.viewPresenceUnknown().replace("?", "\\?")}</button>`));
  assert.ok(top(page({ phase: "running" }, [...three, { type: "Presence", State: 1 }])).includes(roleClass("status.presence.here")));
  assert.ok(top(page({ phase: "running" }, [...three, { type: "Presence", State: 2 }])).includes(str.viewPresenceAway()));
  const world = top(page({ phase: "running" }, [...three, { type: "Active", Key: "=fm" }]));
  assert.ok(world.includes(">fm<") && !world.includes("<button"), world);
});

test("the status line: the connection, unless there's news", () => {
  const since = "Connected since 21:14";
  const on = [...three, { type: "Char", Key: "fm/kit", World: "fm", Name: "Kit", State: 2, ConnectedSince: since }];
  assert.ok(status(page({ phase: "running" }, on)).includes(since));
  assert.ok(status(page({ phase: "running" }, [...on, { type: "Char", Key: "fm/kit", World: "fm", Name: "Kit", State: 0 }])).includes(str.stateDisconnected()));
  const news = status(page({ phase: "running" }, [...on, { type: "Status", Text: "saved", Err: false, Gen: 1 }]));
  assert.ok(news.includes("saved") && !news.includes(since), news);
});

test("the background announcer is always there, politely", () => {
  assert.match(page({ phase: "running" }), /<div[^>]*role="status"[^>]*class="sr-only"/);
});

test("a character's name sits a space in under its world, as the TUI's", () => {
  // A flex row drops bare spaces between its children, so the lead is a
  // span of its own.
  for (const name of [">Kit<", ">Rook<", ">Ash<"]) {
    assert.match(row(page({ phase: "running" }, three), name), /^<li[^>]*><span[^>]*> /, name);
  }
});

// logRegion is what's inside the scrollback's live region.
const logRegion = (html: string) => {
  const at = html.indexOf('role="log"');
  return html.slice(at, html.indexOf("</div>", html.indexOf(">", at)));
};

test("lines already there when a character's shown aren't read aloud", () => {
  const html = page({ phase: "running" }, kitOpen());
  assert.ok(html.includes("first line") && !logRegion(html).includes("first line"), logRegion(html));
});

test("the overview, the lock and the empty pane aren't in the live region", () => {
  const ov = page({ phase: "running" }, [...kitOpen(), { type: "Active", Key: "=fm" }]);
  assert.ok(ov.includes(">Kit<") && !logRegion(ov).includes("Kit"), logRegion(ov));
  const lock = page({ phase: "locked" });
  assert.ok(lock.includes(str.webOtherTab()) && !logRegion(lock).includes(str.webOtherTab()));
});

// bottom is the bottom pane.
const bottom = (html: string) => html.slice(html.indexOf('class="bottom'), html.indexOf('class="status '));

test("the input: its name, the gutter, and Go's text", () => {
  const html = bottom(page({ phase: "running" }, [...three, { type: "Input", Key: "fm/kit", Text: "a draft", Ack: 0 }]));
  assert.match(html, new RegExp(`<textarea[^>]*aria-label="${str.webInput("Kit")}"`));
  assert.equal(marks(html), 1);
  assert.match(html, />a draft<\/textarea>|value="a draft"/);
});

// marks counts the input's › marks.
const marks = (html: string) => html.split(`data-mark="${str.glyph.echo}"`).length - 1;

test("each line sent on its own has its ›, as the TUI's input; flattened, only the first", () => {
  const typed: Event = { type: "Input", Key: "fm/kit", Text: "one\ntwo\n", Ack: 0 };
  assert.equal(marks(bottom(page({ phase: "running" }, [...three, typed]))), 3, "batch: the empty last line too");
  const flat: Event = { type: "Char", Key: "fm/kit", World: "fm", Name: "Kit", State: 2, NewlineMode: "flatten" };
  assert.equal(marks(bottom(page({ phase: "running" }, [...three, flat, typed]))), 1);
});

test("the input's hints, as the TUI's prompt", () => {
  const dis = bottom(page({ phase: "running" }, [...three, { type: "Active", Key: "fm/rook" }]));
  assert.ok(dis.includes(`placeholder="${str.viewDisconnected()}"`), dis);
  const con = bottom(page({ phase: "running" }, [...three, { type: "Active", Key: "pf/ash" }]));
  assert.ok(con.includes(`placeholder="${str.viewConnecting()}"`), con);
  const failed = bottom(page({ phase: "running" }, [...three, { type: "Char", Key: "fm/rook", World: "fm", Name: "Rook", State: 3 }, { type: "Active", Key: "fm/rook" }]));
  assert.ok(failed.includes(`placeholder="${str.viewConnectFailed()}"`), failed);
  const cert = bottom(page({ phase: "running" }, [...three, { type: "Char", Key: "fm/rook", World: "fm", Name: "Rook", State: 0, CertChanged: true }, { type: "Active", Key: "fm/rook" }]));
  assert.ok(cert.includes(`placeholder="${str.viewCertChanged()}"`), cert);
  const live = bottom(page({ phase: "running" }, three));
  assert.ok(!live.includes("placeholder="), "connected: no hint");
});

test("nothing open: the command input and its hint", () => {
  const html = bottom(page({ phase: "running" }, [{ type: "Worlds", Worlds: [] }]));
  assert.match(html, new RegExp(`<textarea[^>]*aria-label="${str.webInputIdle()}"`));
  assert.ok(html.includes(`placeholder="${str.viewNothingOpen()}"`), html);
});

test("a world's overview has no input, as the TUI's", () => {
  assert.ok(!bottom(page({ phase: "running" }, [...three, { type: "Active", Key: "=fm" }])).includes("<textarea"));
});

test("Kiln stopped: the input is disabled", () => {
  assert.match(bottom(page({ phase: "stopped" })), /<textarea[^>]*disabled/);
});

test("the password form: a hidden username for password managers, the TUI's label and keys", () => {
  const html = bottom(page({ phase: "running" }, [...three, { type: "Input", Key: "fm/kit", Text: "", Ack: 0, NeedPW: true }]));
  assert.match(html, /<form/);
  assert.match(html, /<input[^>]*type="text"[^>]*name="username"[^>]*autocomplete="username"[^>]*value="Kit@fm"[^>]*hidden/);
  assert.match(html, /<input[^>]*type="password"[^>]*autocomplete="current-password"/);
  assert.match(html, new RegExp(`<label[^>]*>${str.viewPasswordLabel("Kit")}</label>`));
  assert.ok(html.includes(str.viewPasswordKeys()) && html.includes(roleClass("input.hint")));
  const keys = html.match(/<span[^>]*id="([^"]+)"[^>]*>([^<]*)<\/span>/);
  assert.equal(keys?.[2], str.viewPasswordKeys(), "the keys have an id");
  assert.match(html, new RegExp(`<input[^>]*type="password"[^>]*aria-describedby="${keys?.[1]}"`), "the field is described by its keys");
  assert.doesNotMatch(html, /aria-hidden/, "the keys are read");
  assert.ok(!html.includes("<textarea"), "the form takes the input's place");
});

test("while Kiln loads, the input takes typing", () => {
  const html = bottom(page({ phase: "loading", progress: { got: 1 } }));
  assert.match(html, /<textarea/);
  assert.doesNotMatch(html, /<textarea[^>]*disabled/);
  assert.ok(!html.includes(str.viewNothingOpen()), "no hint to choose a connection before there are any");
});

test("a hint has no ›, as the TUI's prompt: it isn't a line bound for the server", () => {
  const dis = bottom(page({ phase: "running" }, [...three, { type: "Active", Key: "fm/rook" }]));
  assert.ok(dis.includes(str.viewDisconnected()) && !dis.includes(str.glyph.echo), dis);
  assert.ok(bottom(page({ phase: "running" }, three)).includes(str.glyph.echo), "connected: the ›");
  const typed = bottom(page({ phase: "running" }, [...three, { type: "Active", Key: "fm/rook" }, { type: "Input", Key: "fm/rook", Text: "hi", Ack: 0 }]));
  assert.ok(typed.includes(str.glyph.echo), "typed: the ›");
});

test("the sidebar ends with + Connection, after a gap, as the TUI's", () => {
  const html = page({ phase: "running" }, three);
  const add = `class="chip ${roleClass("sidebar.add")}">${str.sidebarOpenConnection()}</button>`;
  assert.ok(html.includes(add), html);
  assert.match(html, new RegExp(`<li aria-hidden="true"> </li><li><button class="chip ${roleClass("sidebar.add")}"`), "a gap before it");
});

test("with the picker open, the sidebar is the picker: worlds, characters to open, + Character", () => {
  const s = new Store();
  s.set({ phase: "running" });
  s.apply([...three, { type: "Worlds", Worlds: [{ ID: "fm", Characters: [{ ID: "kit", Name: "Kit" }, { ID: "mira", Name: "Mira" }] }] }]);
  s.openPicker();
  const html = render(<App store={s} />);
  const nav = html.slice(html.indexOf("<nav"), html.indexOf("</nav>"));
  assert.match(nav, /role="listbox"/);
  assert.ok(nav.includes("Mira") && !nav.includes(" Kit<"), "Kit's open: not offered");
  assert.ok(nav.includes(str.pickerAddCharacter()));
  assert.match(nav, /aria-selected="true"[^>]*>[^<]*Mira/);
  assert.ok(nav.includes(roleClass("picker")), "the picker's area role");
});

test("the picker's filter: the TUI's one row, a combobox for the list, in the form's area", () => {
  const s = new Store();
  s.set({ phase: "running" });
  s.apply([...three, { type: "Worlds", Worlds: [{ ID: "fm", Characters: [{ ID: "mira", Name: "Mira" }] }] }]);
  s.openPicker();
  const html = bottom(render(<App store={s} />));
  assert.ok(html.includes(roleClass("form")) && !html.includes("<textarea"), "the filter takes the input's place");
  assert.match(html, new RegExp(`<label[^>]*>${str.pickerFilter()}: </label>`));
  assert.match(html, /role="combobox"/);
  assert.match(html, /aria-activedescendant="pick-fm\/mira"/);
  assert.match(html, /aria-controls="kiln-picker"/);
  assert.ok(html.includes(str.pickerHint()));
});

test("a world's overview has no input, but the picker's filter shows there", () => {
  const s = new Store();
  s.set({ phase: "running" });
  s.apply([...three, { type: "Active", Key: "=fm" }]);
  assert.doesNotMatch(bottom(render(<App store={s} />)), /<input|<textarea/);
  s.openPicker();
  assert.match(bottom(render(<App store={s} />)), /role="combobox"/);
});
