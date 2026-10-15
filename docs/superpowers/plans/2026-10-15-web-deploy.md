# Web Deploy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The relay serves the page and a preset without `-dev`; the docs describe a Homebrew deploy; the tap's formula builds `kiln-relay` and the page.

**Architecture:** `cmd/kiln-relay`'s `devRoutes` becomes `webRoutes`, mounted whenever `-web` is given; `-preset` is optional (an empty manifest without it); `-dev` goes. The formula change lives in `latrani/homebrew-tap` and is opened there as its own PR.

**Tech Stack:** Go (`net/http`, `httptest`), Homebrew formula (Ruby), Markdown.

**Spec:** `docs/superpowers/specs/2026-10-15-web-deploy-design.md`

## Global Constraints

- Pre-1.0: `-dev` is removed outright; it becomes an unknown flag.
- User-facing strings (flag help, log lines) live in `internal/str/locales/en.toml`; commits that change them carry a `Strings:` trailer.
- The README is the maintainer's voice: untouched.
- The tap PR is opened, never merged by us; it lands with `brew pr-pull` (Indi), after the in-flight release's bump PR.
- Stage files by name; never `git commit -a` (Indi has uncommitted edits in the tree).

## Review Focus

1. `-web` without `-preset`: the page boots (manifest `[]`), no 404 (Task 1 `TestWebWithoutPreset`).
2. Requests outside `/kiln/` and path tricks (`/kiln/../`, `%2e%2e`): nothing outside the two directories is served (Task 1 `TestWebStaysInItsDirectories`).
3. The WebSocket at `/kiln/relay` still wins over the file server for `/kiln/relay` (Task 1 checks the route).
4. The smoke suite runs the relay without `-dev` (Task 1).
5. The formula installs no test files (`*.test.mjs`) into `share/kiln/web` (Task 3).

---

### Task 1: The relay serves the page and preset without `-dev`

**Files:**
- Rename: `cmd/kiln-relay/dev.go` → `cmd/kiln-relay/web.go` (`devRoutes` → `webRoutes`)
- Modify: `cmd/kiln-relay/main.go`, `cmd/kiln-relay/main_test.go`, `internal/str/locales/en.toml` (relay section), `web/smoke/smoke_test.go:460-475`
- Local only (git-excluded): `run-web.sh`

**Interfaces:**
- Produces: `func webRoutes(mux *http.ServeMux, web, preset string)`; `preset == ""` serves `[]` at `/kiln/preset/manifest.json` and nothing else under `/kiln/preset/`.

- [ ] **Step 1: Write the failing tests** (`cmd/kiln-relay/main_test.go`; replace `TestDevNeedsDirs`)

```go
// webMux is the routes run mounts for -web and -preset, without the relay.
func webMux(t *testing.T, preset bool) *httptest.Server {
	t.Helper()
	web := t.TempDir()
	os.WriteFile(filepath.Join(web, "index.html"), []byte("page"), 0o644)
	dir := ""
	if preset {
		dir = t.TempDir()
		os.MkdirAll(filepath.Join(dir, "config", "worlds"), 0o755)
		os.WriteFile(filepath.Join(dir, "config", "worlds", "fm.toml"), []byte("host"), 0o644)
	}
	mux := http.NewServeMux()
	webRoutes(mux, web, dir)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func TestWebServesThePageAndPreset(t *testing.T) {
	srv := webMux(t, true)
	if code, body := get(t, srv.URL+"/kiln/index.html"); code != 200 || body != "page" {
		t.Errorf("page: %d %q", code, body)
	}
	if code, body := get(t, srv.URL+"/kiln/preset/manifest.json"); code != 200 || strings.TrimSpace(body) != `["config/worlds/fm.toml"]` {
		t.Errorf("manifest: %d %q", code, body)
	}
	if code, body := get(t, srv.URL+"/kiln/preset/config/worlds/fm.toml"); code != 200 || body != "host" {
		t.Errorf("preset file: %d %q", code, body)
	}
}

// Nothing outside the page and preset directories is served, however
// the path is written.
func TestWebStaysInItsDirectories(t *testing.T) {
	web := t.TempDir()
	os.WriteFile(filepath.Join(filepath.Dir(web), "secret"), []byte("secret"), 0o644)
	mux := http.NewServeMux()
	webRoutes(mux, web, "")
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	for _, p := range []string{"/kiln/../secret", "/kiln/%2e%2e/secret", "/kiln/preset/../../secret"} {
		if _, body := get(t, srv.URL+p); strings.Contains(body, "secret") {
			t.Errorf("%s served the file beside the page", p)
		}
	}
}

// Without -preset the page still starts: nothing to preload.
func TestWebWithoutPreset(t *testing.T) {
	srv := webMux(t, false)
	if code, body := get(t, srv.URL+"/kiln/preset/manifest.json"); code != 200 || strings.TrimSpace(body) != "[]" {
		t.Errorf("manifest: %d %q", code, body)
	}
}

// -dev is gone: -web and -preset say what to serve, -listen where.
func TestDevIsGone(t *testing.T) {
	if err := run([]string{"-allow", "x.toml", "-dev"}, io.Discard); err == nil || !strings.Contains(err.Error(), "-dev") {
		t.Errorf("err = %v, want an unknown flag", err)
	}
}
```

Add imports: `net/http`, `net/http/httptest`, `strings`.

- [ ] **Step 2: Run to see them fail**

Run: `go test ./cmd/kiln-relay`
Expected: build failure, `webRoutes undefined`.

- [ ] **Step 3: Implement.**
  - `git mv cmd/kiln-relay/dev.go cmd/kiln-relay/web.go`; rename `devRoutes` → `webRoutes`; its comment: "webRoutes serves the built page at /kiln/ and a preset at /kiln/preset/, with a manifest of the preset's files made on each request; with no preset, the manifest is empty. nginx proxies all of /kiln/ here." In the manifest handler, `if preset == "" { m = []string{} } else { m, err = manifest(preset) }`; mount the preset file server only when `preset != ""`.
  - `main.go`: drop the `dev` flag and its check; default listen is always `127.0.0.1:7801`; `if *web != "" { webRoutes(mux, *web, *preset); logf(str.RelayServingPage("http://" + addr + "/kiln/")) }`. `mux.Handle("/kiln/relay", s)` stays: the more specific pattern wins over `/kiln/`.
  - `en.toml` `[relay]`: remove `dev_serving`, `dev_needs_dirs`, `flag_dev`; add `serving_page = "serving the page at {url}"`; `flag_listen = "address to listen on (default 127.0.0.1:7801)"`; `flag_web = "serve the built page at /kiln/ (web/static, or Homebrew's share/kiln/web)"`; `flag_preset = "serve these files at /kiln/preset/ for the page to preload (config/… and known_hosts)"`. Run `go generate ./internal/str`.
  - `web/smoke/smoke_test.go`: `startRelay`'s comment "runs kiln-relay on a free port, serving the page and preset"; drop `"-dev",` from its `exec.Command`.
  - `run-web.sh` (local): `exec go run ./cmd/kiln-relay -listen localhost:8080 -allow …`.

- [ ] **Step 4: Run**

Run: `go test ./cmd/kiln-relay ./internal/str && (cd web && KILN_SMOKE=1 go test -count=1 ./smoke)`
Expected: PASS.

- [ ] **Step 5: Commit** (by name)

```bash
git add cmd/kiln-relay internal/str web/ui/src/str.gen.ts web/smoke/smoke_test.go
git commit  # message: "kiln-relay: -web and -preset serve the page without -dev" + Strings: relay.serving_page (new), relay.flag_listen, relay.flag_web, relay.flag_preset, relay.dev_serving, relay.dev_needs_dirs, relay.flag_dev (removed)
```

---

### Task 2: Docs

**Files:**
- Modify: `docs/web-relay.md` (rewrite the deploy parts), `docs/web-dev.md` (run command)

- [ ] **Step 1: `docs/web-dev.md`.** The run block's last command becomes `go run ./cmd/kiln-relay -listen localhost:8080 -allow web/dev-allow.toml -web web/static -preset web/preset.local`.

- [ ] **Step 2: `docs/web-relay.md`.** Keep the intro, Allowlist and TLS trust sections. Add, after the intro, "## Installing": `brew install latrani/tap/kiln` installs `kiln`, `kiln-relay` and the page (`$(brew --prefix)/share/kiln/web`), from a release whose formula builds them; `brew upgrade kiln` then `systemctl restart kiln-relay` deploys a new one. Add "## Preset": `/etc/kiln-relay/preset/` holds `config/worlds/*.toml` (and any `config/` file) and, for self-signed worlds, `known_hosts`; the relay lists them for the page itself; without `-preset` the page starts with nothing preset; what a preset does in the browser is in `docs/web-dev.md`. Replace the nginx block with the spec's single `location /kiln/` (and the `map $http_upgrade $connection_upgrade { default upgrade; '' close; }` it needs, in `http {}`), and say: https, since the tab lock and the clipboard need a secure context; `$http_host` as before. Replace the systemd `ExecStart` with `/home/linuxbrew/.linuxbrew/bin/kiln-relay -allow /etc/kiln-relay/allow.toml -web /home/linuxbrew/.linuxbrew/share/kiln/web -preset /etc/kiln-relay/preset`, noting to use your `brew --prefix` (systemd doesn't expand it) and that `DynamicUser` needs `/etc/kiln-relay` world-readable or a `ReadOnlyPaths=`; read the existing unit and keep its other lines.

- [ ] **Step 3: Check and commit**

Run: `grep -n -- '-dev' docs/*.md` (excluding `docs/superpowers`)
Expected: no relay `-dev` left.

```bash
git add docs/web-relay.md docs/web-dev.md
git commit -m "docs: deploying the web build with Homebrew: one nginx block, the relay serves the page"
```

---

### Task 3: The tap formula (separate repo, its own PR)

**Files:** `latrani/homebrew-tap` `Formula/kiln.rb`

- [ ] **Step 1:** Clone the tap to the scratchpad on a branch `kiln-relay-and-web`. Wait for no open `bump-kiln-*` PR (the in-flight release); rebase on its main.

- [ ] **Step 2: Edit `Formula/kiln.rb`:**

```ruby
  depends_on "go" => :build
  depends_on "node" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w"), "./cmd/kiln"
    system "go", "build", *std_go_args(ldflags: "-s -w", output: bin/"kiln-relay"), "./cmd/kiln-relay"
    # The web page: the wasm and the page itself, built into web/static.
    system "web/build.sh"
    (share/"kiln/web").install Dir["web/static/*"].reject { |f| f.end_with?(".test.mjs") }
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/kiln version")
    assert_match "-allow", shell_output("#{bin}/kiln-relay 2>&1", 1)
    assert_path_exists share/"kiln/web/index.html"
    assert_path_exists share/"kiln/web/kiln.wasm"
  end
```

(Check `kiln-relay` with no flags exits 1 and its message mentions `-allow`: `str.RelayNeedAllow`.)

- [ ] **Step 3:** `brew style Formula/kiln.rb` (fix what it says). Push the branch and open a PR in the tap describing the change and that it lands with `brew pr-pull`. Don't merge it.
