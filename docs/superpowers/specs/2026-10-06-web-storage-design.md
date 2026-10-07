# Web Kiln, part 3: storage across visits

Epic #74 (web app), sub-project 3 of 5. Part 1 (shim, bridge, relay) is
in `2026-10-01-web-shim-relay-design.md`; its memfs held up, so this part
is "save memfs to IndexedDB" plus what the web needs around it, not a
storage interface in Go.

## Why

Today everything in the web build lasts one visit: config edits, added
worlds, TLS pins, logs. A returning newcomer starts from the preset again,
and `pin` worlds are trust-on-first-use on every visit (#114).

## What we're keeping, and how hard

- **Everything in memfs except `/tmp`** is saved to IndexedDB, best effort.
  That covers config, world defs, `known_hosts` and logs.
- IndexedDB is not a vault. The user can clear it with site data, private
  windows drop it, and browsers can evict it: under disk pressure, and in
  Safari after 7 days of Safari use without visiting the site (ITP's cap
  on script-writable storage). `navigator.storage.persist()` is asked for,
  but Safari often says no, so nothing depends on it.
- **Logs:** best effort is fine, and the docs say so. Saving one log the
  user cares about is browse mode's existing save, which on the web
  becomes a browser download.
- **World defs:** best effort is not fine on its own, so they get a
  user-held backup: **Back up** downloads a zip, **Restore** reads one back.

## Done means

- Reloading the tab, or closing it and coming back, keeps config edits,
  added worlds, pins and logs.
- A second tab shows "Kiln's open in another tab" with **Use here**, which
  moves Kiln to that tab without losing saved state.
- Browse-mode save downloads the file.
- `↓ Back up` and `↑ Restore` at the bottom of the sidebar download and
  restore a zip of the config dir and `known_hosts`.
- Desktop Kiln looks and behaves exactly as before; its golden screens
  don't change.

## 1. Saving memfs: `web/static/persist.mjs`

- **Database:** one IndexedDB database, `kiln`, with one object store,
  `files`, keyed by absolute path. A record is
  `{kind: "file", bytes, mode, mtimeMs}` or `{kind: "dir", mode}`.
- **memfs change hook:** `createFS({ onChange })`. memfs calls
  `onChange(path)` after any call that changes a path: `write`,
  `ftruncate`, `rename` (old and new path, and every path under a renamed
  directory), `unlink`, `rmdir`, `mkdir`, `chmod`, `fchmod`, `utimes`.
  memfs knows nothing about IndexedDB. For a call made on an fd, memfs
  reports the fd's file's current path: an fd follows renames of its file
  (or a directory above it), and once its file is unlinked or another file
  is renamed over it, the fd reports nothing.
- **Flushing:** changed paths collect in a set. A flush runs about 500 ms
  after the last change, at once on `fsync`, and at once when the page
  goes hidden (`visibilitychange`). memfs also gets an `onSync(path)` hook
  for that `fsync` case. A flush is one readwrite transaction. For each
  path: if it exists in memfs, put its current record; if not, delete its
  record. Paths under `/tmp` are skipped.
- **Loading:** `persist.load(fs)` reads every record and recreates it in
  memfs, before the preset is seeded and before Go starts. Order doesn't
  matter: a file's parents are made as needed, and a directory's record
  then sets its mode.
- **Testability:** `persist.mjs` takes a store adapter (`idbStore`, which
  wraps the IndexedDB factory) with `open`, `all` and `apply`. Tests use
  a small in-memory fake store (and a fake factory for the adapter)
  written in the test files, not an npm package. `idbStore.open` gives
  up after 5 s, so a hung open falls back to plain memfs.

### Boot order (`kiln.js`)

1. Take the tab lock (§2). Without it, stop and show the lock screen.
2. Open IndexedDB and load saved files into memfs.
3. Seed the preset, **only paths that don't exist yet**. Seeded files
   aren't saved, so an untouched preset world follows the server, a saved
   edit wins, and a preset world the user deletes comes back next visit.
4. Ask for `navigator.storage.persist()`, best effort. The answer isn't
   logged or used.
5. Load `wasm_exec.js` and start Go.

### When saving can't work

- IndexedDB won't open or the load fails (some private modes): Kiln runs
  on plain memfs. Once Go starts, Kiln shows a statusline message
  (`ui.StatusMsg`, not a sys line, since nothing may be open at boot):
  nothing will be saved in this browser.
- A flush fails (quota, for example): the same statusline message, once per visit.
  The paths stay marked as changed and go out with the next flush.
- JS tells Go through the bridge: `kiln.start` gets a boolean `saving`
  argument, and later failures call `kiln.storageFailed()`. `kiln-web`
  turns either into the statusline message.

## 2. One tab owns Kiln: `web/static/tablock.mjs`

Two tabs each saving their own memfs would overwrite each other's files,
and two live Kilns would double-connect characters.

- **The lock:** `navigator.locks.request("kiln", {ifAvailable: true}, …)`
  at boot, before IndexedDB is touched. Holding it means owning storage
  and connections; it's held for the page's lifetime.
- **Without it:** no IndexedDB, no Go. The page shows a plain HTML screen,
  "Kiln's open in another tab", with a **Use here** button.
- **Use here:**
  1. The new tab posts `release` on `BroadcastChannel("kiln")` and waits
     on a blocking `locks.request("kiln", …)`.
  2. The old tab saves (`persist.handOff()`: it flushes until nothing is
     changed, so writes that land mid-flush go out too, then stops
     saving), lets go of the lock, and reloads itself. If the save throws,
     it lets go anyway. Reloading closes its connections. Coming back up,
     it finds the lock taken and shows the same screen, so the user can
     switch back.
  3. If the new tab hasn't got the lock after 3 s (old tab frozen or
     throttled in the background), it requests again with `{steal: true}`.
     The worst case is losing the old tab's last unflushed writes.
- **Testability:** `tablock.mjs` takes `locks`, a channel factory and a
  clock as parameters; node tests drive both sides with fakes.

### Strings for the page

The lock screen is shown with no Go running, but its text still lives in
the catalog. Add a `[web]` table to `en.toml` and have
`go generate ./internal/str` also write `web/static/strings.json`, holding
the `[web]` entries only. `kiln.js` fetches it only when it's about to show
the lock screen, so a failed fetch can't stop the owning tab from booting.
`TestGeneratedIsFresh` checks the file against `catalog.WebJSON`. Placeholders are filled in JS with the same `{name}` syntax; plural
tables are left out of `[web]` until something needs one. The
`TestTestsReadTheCatalog`/lint rules don't cover JS; review does.

## 3. Web-only behavior in Kiln: hooks on `ui.Deps`

Following `OpenURL`'s pattern: each thing that works differently on the
web is a hook on `ui.Deps`, nil on desktop. The UI changes shape only when
a hook is set. There is no "web mode" flag.

### Browse save → download

- `SaveFile func(name string, data []byte) error`.
- When set, browse save:
  - asks for a file name, defaulting to the same `export_name` name it
    uses today; a name containing `/` is cut to its base name;
  - skips `export_dir` (ignored on the web) and the never-overwrite check
    (the browser handles name clashes);
  - calls `SaveFile` with the rendered export, and on success says
    `downloaded {name}` (new catalog entry).
- `kiln-web` sets it to call `kilnDownload(name, bytes)` in `kiln.js`,
  which makes a Blob, clicks an `<a download>`, and revokes the URL.

### Back up and Restore

- `Backup func() (name string, err error)` and
  `Restore func() (n int, err error)`; `Restore` returns `0, nil` when the
  user cancels. When both are set, the
  sidebar pins two rows to its bottom: `↓ Back up` and `↑ Restore`
  (new catalog entries). They're clickable and keyboard-reachable the same
  way as `+ Connection`. They get a new role, `sidebar.action`, styled
  like `sidebar.add` in `default.toml` and listed in `docs/themes.md`.
  If the sidebar is too short for its rows plus these two, the worlds
  list scrolls; the two rows stay.
- **Backup** (`kiln-web`): zip the config dir and `known_hosts` with
  `archive/zip`. Paths in the zip are `config/…` and `known_hosts`. The
  file is named `kiln-backup-YYYY-MM-DD.zip` and goes out through the
  same download as browse save. The status line says `downloaded {name}`.
  A Back up (click or `/backup`) while one is running is ignored, so a
  double-click downloads one zip. Restore isn't gated: a second pick
  replaces the first in the page, and a browser that never fires the file
  input's cancel event would otherwise leave Restore stuck.
- **Restore** (`kiln-web`):
  1. Ask JS for a file: `kilnPickFile()` clicks a hidden
     `<input type="file" accept=".zip">` and resolves with the bytes, or
     with nothing if the user cancels.
  2. Check the zip, reading every entry before writing any: only `config/…` and `known_hosts` entries, no `..`,
     no absolute paths, at most 10 MB unpacked. Anything else rejects the
     whole zip, with a catalog error on the status line.
  3. Write each file over the existing one. Nothing is deleted: a world
     the user added since the backup survives a restore. A name that
     appears twice in the zip is one file (the later copy) and counts once.
  4. Reload config with the existing `reloadNow()` path, and say
     `restored {n} files` (plural table, new entry).
- The writes go through memfs, so they're saved like any other change.

### Settings that mean nothing on the web

- `export_dir`: ignored when `SaveFile` is set (above).
- `log_dir`: no change. It's a folder template inside memfs, and an
  absolute `log_dir` is saved too, since everything outside `/tmp` is.
- `password_store`: sub-project 4 (login form).
- `notify_method`: sub-project 5 (browser notifications).

## Testing

- **`memfs.test.mjs`:** `onChange` fires for each changing call with the
  right paths (including fds and directory renames); `onSync` on `fsync`.
- **`persist.test.mjs`:** load restores files and directories with modes
  and mtimes; flush writes changed files, deletes removed ones, skips
  `/tmp`, batches into one transaction; a failing flush keeps the paths
  and reports once.
- **`tablock.test.mjs`:** first tab gets the lock; second sees it taken;
  `release` makes the holder flush then reload; steal after the timeout.
- **`internal/ui`:** with fake hooks: the sidebar rows show only when
  `Backup`/`Restore` are set; clicking and keyboard both call them; browse
  save calls `SaveFile` with the name and rendered bytes and skips
  `export_dir`. A new golden screen shows the sidebar with the two rows;
  existing goldens don't change.
- **`web/backup` (native `go test`; the zip code is plain Go):** backup
  zips the right paths; restore writes them back, rejects `..`, absolute
  paths, unknown top-level names and oversized zips before writing
  anything, and never deletes.
- **By hand, headless Chrome smoke script:** edit a world, reload, it's
  still there; second tab shows the lock screen and Use here moves Kiln;
  Back up downloads a zip and Restore brings it back.

## Not in this spec

Pruning or capping logs, syncing to a server, picking a folder on disk
(`showDirectoryPicker` is Chromium-only), telling a returning user that
the preset changed, the login form and password handling (4), browser
notifications, the PFMuck preset, the landing page and deploy (5).

## Risks

- **File picker needs a user gesture.** A click goes xterm.js → Go → JS
  asynchronously, so `input.click()` runs a moment after the event.
  Browsers allow this within transient activation (a few seconds), but
  Safari is strict. The plan's first task checks this in Chrome and
  Safari. Fallback: restore by dropping the zip onto the page.
- **Rewriting a whole log on every flush.** Fine at MUCK-log sizes (a few
  MB). If it shows up, store files in chunks; not now.
- **Flush on hide doesn't finish.** IndexedDB is async and the page can
  die mid-transaction. The 500 ms debounce keeps the window small; losing
  the last fraction of a second on a hard close is accepted.
- **#114:** pins now survive visits, so a pin is trust-on-first-use once
  per browser, not per visit. That fixes #114's per-visit problem;
  seeded pins or `ca` trust for the PFMuck preset is still sub-project 5.
