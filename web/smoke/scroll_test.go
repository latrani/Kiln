package smoke

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/latrani/Kiln/internal/kilntest"
	"github.com/latrani/Kiln/internal/logstore"
)

// The scrollback's moves, in a real browser with real layout: the pager,
// switching back, lines coming while you read history, the pane
// resizing, and older history coming above (scroll.ts decides; this
// checks the page carries it out).
func TestScrollbackMoves(t *testing.T) {
	ws, _ := launch(t)
	ev := evaluator(t, ws)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Order")`, "the first batch")
	seedHistory(t, ev)
	ev(`window.sc = () => document.querySelector("main .scroll");
		window.atEnd = () => { const e = sc(); return e.scrollHeight - e.scrollTop - e.clientHeight < 2; };
		window.pillN = () => { const b = document.querySelector("main .pill"); return b ? Number((b.textContent.match(/\d+/) || [0])[0]) : -1; };
		window.row = (text) => [...sc().querySelectorAll(".sb")].find(r => r.textContent.startsWith(text + ":") || r.textContent === text);
		window.click = (name) => [...document.querySelectorAll("nav button")].find(b => b.textContent === name).click();
		window.firstShown = () => { const e = sc(); return [...e.querySelectorAll(".sb")].find(r => r.offsetTop + r.offsetHeight > e.scrollTop); };`)

	prompt := func(k string, n int) { // open k and connect it, up to its password prompt
		ev(`kilnPage.act({Do: "open", Key: "` + k + `"})`)
		ev(`kilnPage.act({Do: "switch", Key: "` + k + `"})`) // input goes to the character showing
		ev(`kilnPage.act({Do: "submit", Key: "` + k + `", Text: "", N: ` + strconv.Itoa(n) + `})`)
		waitFor(t, ev, `kilnPage.events.some(e => e.type === "Input" && e.Key === "`+k+`" && e.NeedPW)`, "the password prompt for "+k)
	}
	login := func(k string, n int) {
		ev(`kilnPage.act({Do: "switch", Key: "` + k + `"})`)
		ev(`kilnPage.act({Do: "password", Key: "` + k + `", Text: "secret", N: ` + strconv.Itoa(n+1) + `})`)
		waitFor(t, ev, `kilnPage.events.some(e => e.type === "Char" && e.Key === "`+k+`" && e.State === 2)`, k+" connected")
	}
	prompt("fake/tester", 2000)
	prompt("fake/second", 3000)
	// Half a password typed for Second doesn't follow you to Tester's prompt.
	pw := `document.querySelector('.bottom input[type="password"]')`
	waitFor(t, ev, `document.activeElement === `+pw, "Second's password field focused")
	devtools(t, ws, "Input.insertText", map[string]any{"text": "partial"})
	ev(`click("Tester")`)
	waitFor(t, ev, `document.querySelector("header").textContent.includes("Tester") && `+pw+` && `+pw+`.value === ""`, "Tester's own, empty password field")
	login("fake/tester", 2000)
	login("fake/second", 3000)
	ev(`click("Tester")`)

	// Typing is kept across switching: not sent, it's the character's
	// draft; and keys typed just before a switch aren't lost.
	textarea := `document.querySelector(".bottom textarea")`
	waitFor(t, ev, `document.querySelector("header").textContent.includes("Tester") && document.activeElement === `+textarea, "Tester's input focused")
	devtools(t, ws, "Input.insertText", map[string]any{"text": "half a pose"})
	time.Sleep(500 * time.Millisecond)
	ev(`click("Second")`)
	waitFor(t, ev, `document.querySelector("header").textContent.includes("Second") && `+textarea+`.value === ""`, "Second's own input")
	ev(`click("Tester")`)
	waitFor(t, ev, textarea+`.value === "half a pose"`, "Tester's draft back")
	waitFor(t, ev, `document.activeElement === `+textarea, "Tester's input focused again")
	devtools(t, ws, "Input.insertText", map[string]any{"text": " more"})
	ev(`click("Second")`) // before the typing would have been sent
	if got := ev(`(() => { const s = kilnPage.sent; const i = s.findLastIndex(a => a.Do === "input" && a.Text === "half a pose more"); return i >= 0 && i < s.findLastIndex(a => a.Do === "switch"); })()`); got != "true" {
		t.Error("the typing didn't go before the switch")
	}
	waitFor(t, ev, `document.querySelector("header").textContent.includes("Second")`, "on Second")
	ev(`click("Tester")`)
	waitFor(t, ev, textarea+`.value === "half a pose more"`, "keys typed just before a switch kept")
	ev(`(() => { const t = ` + textarea + `; t.select(); })()`)
	devtools(t, ws, "Input.insertText", map[string]any{"text": ""})
	say := func(text string, n int) {
		ev(`kilnPage.act({Do: "submit", Key: "fake/tester", Text: "` + text + `", N: ` + strconv.Itoa(n) + `})`)
	}

	// More than fits comes at once: the view stops at the first of it, and
	// the pill counts the lines wholly below. They're in the live region.
	waitFor(t, ev, `document.querySelector("header").textContent.includes("Tester")`, "Tester showing")
	say("spam 80", 2010)
	waitFor(t, ev, `row("spam 80")`, "the spam drawn")
	waitFor(t, ev, `!atEnd() && pillN() > 0 && pillN() === [...sc().querySelectorAll(".sb")].filter(r => r.offsetTop >= sc().scrollTop + sc().clientHeight).length`, "the pager stopping, the pill counting")
	waitFor(t, ev, `row("spam 80").closest('[role="log"]') !== null`, "new lines in the live region")

	// Esc in the input goes back to the end, as the TUI's does.
	ev(`document.querySelector(".bottom textarea").focus()`)
	pressKey(t, ws, "Escape", 27)
	waitFor(t, ev, `atEnd() && pillN() === -1`, "Esc going to the end")

	// Enter sends, an empty line too, and goes to the end, as the TUI's.
	say("spam 80", 2011)
	waitFor(t, ev, `!atEnd() && pillN() > 0`, "the pager stopping again")
	ev(`document.querySelector(".bottom textarea").focus()`)
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `atEnd() && pillN() === -1`, "Enter going to the end")

	// Live, the pane narrows: the lines rewrap, and the view stays at the
	// end. The MUCK is told the scroller's width in characters.
	// Checked two frames later, before the page's resize could reach Go and
	// come back.
	if got := ev(`new Promise(r => {
		document.querySelector(".kiln").style.gridTemplateColumns = "40ch 1px 1fr";
		const box = (q) => document.querySelector(q).getBoundingClientRect().height;
		const before = { sh: sc().scrollHeight, st: sc().scrollTop, ch: sc().clientHeight, n: sc().querySelectorAll(".sb").length, pill: pillN(), win: innerHeight, top: box(".top"), bottom: box(".bottom"), status: box(".status"), statusText: document.querySelector(".status").textContent };
		requestAnimationFrame(() => requestAnimationFrame(() => r(atEnd() ? "true" : JSON.stringify({ before, sh: sc().scrollHeight, st: sc().scrollTop, ch: sc().clientHeight, n: sc().querySelectorAll(".sb").length, pill: pillN() }))));
	})`); got != `"true"` {
		t.Errorf("a live view after a resize: at the end %s, want true", got)
	}
	// The page's own scroll to the end, its event arriving after the pane
	// changed size: still live, still at the end.
	ev(`sc().scrollTop = 0`)
	waitFor(t, ev, `document.querySelector("main .pill")`, "the pill, scrolled up")
	if got := ev(`new Promise(r => {
		document.querySelector("main .pill").click();
		document.querySelector(".kiln").style.gridTemplateColumns = "60ch 1px 1fr";
		requestAnimationFrame(() => requestAnimationFrame(() => r(atEnd() && pillN() === -1)));
	})`); got != "true" {
		t.Errorf("going to the end as the pane changed size: at the end and live %s, want true", got)
	}
	// A scrollbar that takes room, as on Windows or Linux (headless
	// Chrome's take none): the MUCK's width leaves it out.
	ev(`document.head.append(Object.assign(document.createElement("style"), { textContent: ".scroll::-webkit-scrollbar { width: 30px }" }))`)
	waitFor(t, ev, `(() => {
		const p = Object.assign(document.createElement("span"), { style: "position:absolute;width:1ch" });
		sc().append(p);
		const ch = p.getBoundingClientRect().width;
		p.remove();
		const r = kilnPage.sent.filter(a => a.Do === "resize").at(-1);
		return r && r.Cols === Math.floor(sc().clientWidth / ch);
	})()`, "the width told to the MUCK being the scroller's")

	// Lines come while you're on another character: switching back stops
	// at the first of them.
	say("later 60", 2012)
	ev(`click("Second")`)
	waitFor(t, ev, `kilnPage.events.filter(e => e.type === "Char" && e.Key === "fake/tester").at(-1).Unread >= 60`, "the lines coming in the background")
	ev(`click("Tester")`)
	waitFor(t, ev, `row("late 60") && Math.abs(row("late 1").offsetTop - sc().scrollTop) < 2 && pillN() > 0`, "switching back stopping at the first line not seen")

	// Reading mid-history, you switch away and lines come: switching back,
	// the line you were reading is where it was, and the pill counts them.
	// (Sent first, since sending goes to the end; the lines come a second
	// later.)
	say("later 30", 2013)
	ev(`sc().scrollTop = (sc().scrollHeight - sc().clientHeight) / 2`)
	time.Sleep(300 * time.Millisecond)
	before := ev(`JSON.stringify({ seq: firstShown().dataset.seq, top: firstShown().getBoundingClientRect().top, n: pillN() })`)
	var was struct {
		Seq string
		Top float64
		N   int
	}
	json.Unmarshal([]byte(mustUnquote(t, before)), &was)
	ev(`click("Second")`)
	waitFor(t, ev, `kilnPage.events.filter(e => e.type === "Char" && e.Key === "fake/tester").at(-1).Unread >= 30`, "more lines in the background")
	ev(`click("Tester")`)
	waitFor(t, ev, `(() => { const r = sc().querySelector('.sb[data-seq="`+was.Seq+`"]'); return r && Math.abs(r.getBoundingClientRect().top - `+strconv.FormatFloat(was.Top, 'f', 2, 64)+`) < 2 && pillN() >= `+strconv.Itoa(was.N+30)+`; })()`, "the place kept and the lines counted")

	// Older history comes above the line you're reading, which stays put;
	// it isn't read aloud.
	ev(`kilnPage.act({Do: "open", Key: "fake/historian"})`)
	waitFor(t, ev, `[...document.querySelectorAll("nav button")].some(b => b.textContent === "Historian")`, "Historian in the sidebar")
	ev(`click("Historian")`)
	waitFor(t, ev, `row("history 4-150") && atEnd()`, "Historian's history drawn")
	n := ev(`sc().querySelectorAll(".sb").length`)
	place := mustUnquote(t, ev(`(() => { sc().scrollTop = sc().clientHeight / 2; const r = firstShown(); return JSON.stringify({ seq: r.dataset.seq, top: r.getBoundingClientRect().top }); })()`))
	json.Unmarshal([]byte(place), &was)
	waitFor(t, ev, `sc().querySelectorAll(".sb").length > `+n, "older history coming")
	waitFor(t, ev, `(() => { const r = sc().querySelector('.sb[data-seq="`+was.Seq+`"]'); return Math.abs(r.getBoundingClientRect().top - `+strconv.FormatFloat(was.Top, 'f', 2, 64)+`) < 2; })()`, "the line being read staying put")
	waitFor(t, ev, `row("history 3-1") && row("history 3-1").closest('[role="log"]') === null`, "older history not read aloud")
	// And again: paging goes on as long as there's more.
	n = ev(`sc().querySelectorAll(".sb").length`)
	ev(`sc().scrollTop = 0`)
	waitFor(t, ev, `sc().querySelectorAll(".sb").length > `+n, "a second page of older history")

	// In a world's overview, a link opens; it doesn't switch characters.
	ev(`click("Tester")`)
	say("https://example.test/x", 2014)
	waitFor(t, ev, `sc().querySelector('a[href="https://example.test/x"]')`, "the link drawn")
	ev(`kilnPage.act({Do: "switch", Key: "=fake"})`)
	waitFor(t, ev, `document.querySelector('main a[href="https://example.test/x"]')`, "the link in the overview")
	ev(`document.querySelector('main a[href="https://example.test/x"]').click()`)
	time.Sleep(500 * time.Millisecond)
	if got := ev(`kilnPage.events.filter(e => e.type === "Active").at(-1).Key`); got != `"=fake"` {
		t.Errorf("after a link in the overview, showing %s, want the overview", got)
	}
}

// seedHistory writes Historian's log for four days before today into the
// page's files, 150 lines a day: more than the 200 preloaded, so older
// days come by paging.
func seedHistory(t *testing.T, ev func(string) string) {
	t.Helper()
	dir := t.TempDir()
	layout := logstore.Layout{Root: dir, World: "fake", Char: "historian", CharName: "Historian"}
	today := time.Now().Truncate(24 * time.Hour)
	for d := 1; d <= 4; d++ {
		var ls []string
		for i := 1; i <= 150; i++ {
			ls = append(ls, "history "+strconv.Itoa(d)+"-"+strconv.Itoa(i))
		}
		kilntest.WriteLog(t, layout, today.AddDate(0, 0, d-5).Add(9*time.Hour), ls...)
	}
	filepath.WalkDir(dir, func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		b, _ := os.ReadFile(p)
		rel, _ := filepath.Rel(dir, p)
		ev(`fs.writeFileAll("/home/kiln/.local/share/kiln/logs/` + filepath.ToSlash(rel) + `", Uint8Array.from(atob("` + base64.StdEncoding.EncodeToString(b) + `"), c => c.charCodeAt(0)))`)
		return nil
	})
}

// mustUnquote is a JSON string ev returned, unquoted.
func mustUnquote(t *testing.T, s string) string {
	t.Helper()
	u, err := strconv.Unquote(s)
	if err != nil {
		t.Fatalf("%q: %v", s, err)
	}
	return u
}

// devtools calls a DevTools method in the tab and returns its result.
func devtools(t *testing.T, ws *websocket.Conn, method string, params map[string]any) json.RawMessage {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	const id = 1 << 30
	msg, _ := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	if err := ws.Write(ctx, websocket.MessageText, msg); err != nil {
		t.Fatal(err)
	}
	for {
		_, b, err := ws.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var r struct {
			ID     int
			Result json.RawMessage
		}
		if json.Unmarshal(b, &r) == nil && r.ID == id {
			return r.Result
		}
	}
}
