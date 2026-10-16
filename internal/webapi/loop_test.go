package webapi

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/feed"
	"github.com/latrani/Kiln/internal/kilntest"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/str"
)

const fmWorld = `host = "muck.test"
port = 8888
tls = true
use = ["fuzzball"]

[[characters]]
id = "kit"
name = "Kit"

[[characters]]
id = "rook"
name = "Rook"
`

var day = time.Date(2026, 9, 24, 21, 0, 0, 0, time.Local)

// newApp is an App over fm (Kit, Rook) with fake connections, and its
// config dir.
func newApp(t *testing.T) (*app.App, string) {
	t.Helper()
	dir := t.TempDir()
	if err := config.EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "worlds", "fm.toml"), []byte(fmWorld), 0o600); err != nil {
		t.Fatal(err)
	}
	a := app.New(app.Deps{
		ConfigDir: dir,
		Load:      config.Load,
		LogRoot:   filepath.Join(dir, "logs"),
		Dial: func(context.Context, config.Character) (session.LineConn, error) {
			return kilntest.NewConn(), nil
		},
		NewLog:   func(logstore.Layout) session.Appender { return kilntest.NopLog{} },
		Password: func(string, string, string) (string, error) { return "", errors.New("none") },
		Now:      func() time.Time { return day },
	})
	if _, ok := a.Reload(); !ok {
		t.Fatal(a.Status().Text)
	}
	return a, dir
}

// harness runs a Loop and collects its batches.
type harness struct {
	t       *testing.T
	a       *app.App
	dir     string
	l       *Loop
	batches chan []map[string]any
}

func start(t *testing.T) *harness {
	t.Helper()
	a, dir := newApp(t)
	return startOn(t, a, dir)
}

func startOn(t *testing.T, a *app.App, dir string) *harness {
	t.Helper()
	return startWith(t, a, dir, Options{})
}

// startWith is startOn with Run's options.
func startWith(t *testing.T, a *app.App, dir string, opts Options) *harness {
	t.Helper()
	h := &harness{t: t, a: a, dir: dir, batches: make(chan []map[string]any, 100)}
	h.l = New(a, func(b []byte) { h.batches <- decode(t, b) })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go h.l.Run(ctx, opts)
	h.next() // the first batch: everything
	return h
}

// next is the next batch, or fails after a second.
func (h *harness) next() []map[string]any {
	h.t.Helper()
	select {
	case b := <-h.batches:
		return b
	case <-time.After(time.Second):
		h.t.Fatal("no batch")
		return nil
	}
}

// act sends an action and returns its turn's batch.
func (h *harness) act(js string) []map[string]any {
	h.t.Helper()
	h.l.Act([]byte(js))
	return h.next()
}

func types(b []map[string]any) []string {
	var out []string
	for _, e := range b {
		out = append(out, e["type"].(string))
	}
	return out
}

// event is b's first event of type typ, nil if none.
func event(b []map[string]any, typ string) map[string]any {
	for _, e := range b {
		if e["type"] == typ {
			return e
		}
	}
	return nil
}

func TestFirstBatchIsEverything(t *testing.T) {
	a, _ := newApp(t)
	got := make(chan []map[string]any, 1)
	l := New(a, func(b []byte) { got <- decode(t, b) })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go l.Run(ctx, Options{})
	if b := <-got; types(b)[0] != "Start" {
		t.Errorf("first batch %v", types(b))
	}
}

func TestOpenSubmitAndInput(t *testing.T) {
	h := start(t)
	b := h.act(`{"Do":"open","Key":"fm/kit"}`)
	if !slices.Contains(types(b), "Char") || !slices.Contains(types(b), "Active") {
		t.Fatalf("open: %v", types(b))
	}
	b = h.act(`{"Do":"input","Key":"fm/kit","Text":"hello","N":1}`)
	if slices.Contains(types(b), "Input") {
		t.Errorf("the page's own text came back: %v", b)
	}
	b = h.act(`{"Do":"submit","Key":"fm/kit","Text":"/notify off","N":2}`) // a command: no connection needed
	var cleared bool
	for _, e := range b {
		if e["type"] == "Input" && e["Key"] == "fm/kit" && e["Text"] == "" {
			cleared = true
		}
	}
	if !cleared {
		t.Errorf("submit should clear the input: %v", b)
	}
	b = h.act(`{"Do":"older","Key":"fm/kit","Text":"notify","N":3}`) // ↑ on the first row: the panel, the text its query
	hist := event(b, "History")
	if hist == nil || hist["Open"] != true || hist["Query"] != "notify" || hist["Sel"] != 0.0 {
		t.Fatalf("history: %v", b)
	}
	if rows := hist["Rows"].([]any); len(rows) != 1 || rows[0].(map[string]any)["Text"] != "/notify off" || rows[0].(map[string]any)["Clock"] == "" {
		t.Errorf("rows: %v", rows)
	}
	b = h.act(`{"Do":"history_pick","N":-1}`)
	if in := event(b, "Input"); in == nil || in["Text"] != "/notify off" {
		t.Errorf("picked: %v", b)
	}
	if hist := event(b, "History"); hist == nil || hist["Open"] != false {
		t.Errorf("the panel stayed up: %v", hist)
	}
	h.act(`{"Do":"input","Key":"fm/kit","Text":"draft","N":4}`)
	h.act(`{"Do":"history_toggle"}`)
	h.act(`{"Do":"input","Key":"fm/kit","Text":"no","N":5}`)
	b = h.act(`{"Do":"history_close"}`)
	if in := event(b, "Input"); in == nil || in["Text"] != "draft" {
		t.Errorf("closed: %v; want the draft back", b)
	}
}

func TestBadActionsSaySo(t *testing.T) {
	h := start(t)
	want := upTo(str.StatusBadAction(errors.New(mark)))
	for _, js := range []string{`not json`, `{"Do":"frobnicate"}`} {
		b := h.act(js)
		if st := event(b, "Status"); st == nil || st["Err"] != true || !strings.HasPrefix(st["Text"].(string), want) {
			t.Errorf("%s: %v", js, b)
		}
	}
	h.act(`{"Do":"close","Key":"nobody/here"}`) // ignored, as the TUI would
	h.act(`{"Do":"mark","Key":"nobody/here","Seq":3}`)
}

func TestRunWorkComesBackOnTheLoop(t *testing.T) {
	h := start(t)
	h.act(`{"Do":"open","Key":"fm/kit"}`)
	h.act(`{"Do":"submit","Key":"fm/kit","Text":"","N":1}`) // connect: a Run waiting on the session
	deadline := time.After(2 * time.Second)
	for {
		select {
		case b := <-h.batches:
			if c := event(b, "Char"); c != nil && c["State"] == float64(session.Connected) {
				return
			}
		case <-deadline:
			t.Fatal("never connected")
		}
	}
}

// connectKit opens Kit and waits until it's connected.
func connectKit(t *testing.T, h *harness) {
	t.Helper()
	h.act(`{"Do":"open","Key":"fm/kit"}`)
	h.act(`{"Do":"submit","Key":"fm/kit","Text":"","N":1}`)
	deadline := time.After(2 * time.Second)
	for {
		select {
		case b := <-h.batches:
			if c := event(b, "Char"); c != nil && c["State"] == float64(session.Connected) {
				return
			}
		case <-deadline:
			t.Fatal("never connected")
		}
	}
}

// sendingKit is a harness with Kit connected and no login, so what's
// typed goes to the server, not a password prompt; extra is more world
// settings.
func sendingKit(t *testing.T, extra string) *harness {
	t.Helper()
	a, dir := newApp(t)
	writeFile(t, filepath.Join(dir, "worlds", "fm.toml"), strings.Replace(fmWorld, "use = [\"fuzzball\"]\n", "use = [\"fuzzball\"]\nlogin = \"\"\n"+extra, 1))
	if _, ok := a.Reload(); !ok {
		t.Fatal(a.Status().Text)
	}
	h := startWith(t, a, dir, Options{})
	connectKit(t, h)
	return h
}

// drain takes the batches the connection sends by itself.
func drain(h *harness) {
	for {
		select {
		case <-h.batches:
		case <-time.After(100 * time.Millisecond):
			return
		}
	}
}

// lastSeq is the Seq of Kit's line n from the end (1 is the newest).
func lastSeq(h *harness, n int) float64 {
	ls := h.a.Char("fm/kit").Lines
	return float64(ls[len(ls)-n].Seq)
}

// A line that goes out tells the page, which then shows the end, as the
// TUI does: an empty one too (it's sent); a command doesn't. Seen is the
// newest line: nothing was echoed.
func TestSentTellsThePage(t *testing.T) {
	h := sendingKit(t, "")
	for i, c := range []struct {
		text string
		sent bool
	}{{"hello", true}, {"", true}, {"/notify off", false}} {
		drain(h)
		b := h.act(fmt.Sprintf(`{"Do":"submit","Key":"fm/kit","Text":%q,"N":%d}`, c.text, i+2))
		e := event(b, "Sent")
		if (e != nil) != c.sent || e != nil && (e["Key"] != "fm/kit" || e["Seen"] != lastSeq(h, 1)) {
			t.Errorf("%q: Sent %v, want sent %v, seen %v", c.text, e, c.sent, lastSeq(h, 1))
		}
	}
}

// With local echo, the echo is the newest line and stays unseen: Seen
// is the line before it, so the pager can stop with the echo at the top.
func TestSentLeavesTheEchoUnseen(t *testing.T) {
	h := sendingKit(t, "local_echo = true\n")
	drain(h)
	e := event(h.act(`{"Do":"submit","Key":"fm/kit","Text":"hello","N":2}`), "Sent")
	if e == nil || e["Seen"] != lastSeq(h, 2) {
		t.Errorf("Sent %v, want seen %v (the echo is %v)", e, lastSeq(h, 2), lastSeq(h, 1))
	}
}

// The password form's answer isn't a line sent: it doesn't go to the end.
func TestPasswordIsNotSent(t *testing.T) {
	h := start(t)
	connectKit(t, h)
	deadline := time.After(2 * time.Second)
	for !h.a.Char("fm/kit").NeedPW {
		select {
		case <-h.batches:
		case <-deadline:
			t.Fatal("no password prompt")
		}
	}
	drain(h)
	if b := h.act(`{"Do":"password","Key":"fm/kit","Text":"hunter2","N":2}`); event(b, "Sent") != nil {
		t.Error("the password was taken as a line sent")
	}
}

func TestQuitEndsTheLoop(t *testing.T) {
	a, _ := newApp(t)
	got := make(chan []map[string]any, 10)
	l := New(a, func(b []byte) { got <- decode(t, b) })
	done := make(chan struct{})
	go func() { l.Run(context.Background(), Options{}); close(done) }()
	<-got
	l.Act([]byte(`{"Do":"quit"}`))
	b := <-got
	if last := b[len(b)-1]; last["type"] != "Effect" || last["effect"] != "Quit" {
		t.Errorf("last batch %v", types(b))
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("the loop kept running")
	}
}

func TestSinceOrEverything(t *testing.T) {
	h := start(t)
	h.act(`{"Do":"open","Key":"fm/kit"}`)
	h.a.Char("fm/kit") // opened on the loop; read-only here
	b := h.act(`{"Do":"since","Key":"fm/kit","Epoch":999,"Seq":0}`)
	if types(b)[0] != "Start" {
		t.Errorf("a stale epoch should get everything: %v", types(b))
	}
}

func TestOverLimit(t *testing.T) {
	if OverLimit("12345", 5, "batch") || !OverLimit("123456", 5, "batch") || !OverLimit("123\n45", 5, "flatten") {
		t.Error("OverLimit")
	}
}

// webapi is plain Go: no js, no terminal, no UI package.
func TestWebapiDependsOnNoFrontEnd(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, dep := range strings.Fields(string(out)) {
		if dep == "syscall/js" || strings.HasSuffix(dep, "/internal/ui") || strings.Contains(dep, "bubbletea") || strings.Contains(dep, "lipgloss") {
			t.Errorf("webapi depends on %s", dep)
		}
	}
}

func TestIdleInputIsNotEchoed(t *testing.T) {
	h := start(t)
	if b := h.act(`{"Do":"input","Key":"","Text":"/open fm/kit"}`); slices.Contains(types(b), "Input") {
		t.Errorf("the idle input came back: %v", b)
	}
	if h.a.IdleText() != "/open fm/kit" {
		t.Errorf("idle text %q", h.a.IdleText())
	}
}

func TestLogModeByNumber(t *testing.T) {
	a, dir := newApp(t)
	ch, _ := a.Find("fm/kit")
	l, _ := a.LogLayout(ch)
	kilntest.WriteLog(t, l, day.AddDate(0, 0, -1), "one", "two", "three")
	h := startOn(t, a, dir)
	h.act(`{"Do":"open","Key":"fm/kit"}`)
	b := h.act(`{"Do":"log","Key":"fm/kit"}`)
	ll := event(b, "LogLines")
	if event(b, "Log") == nil || ll == nil {
		t.Fatalf("log: %v", types(b))
	}
	var seqs []int
	for _, x := range ll["Lines"].([]any) {
		seqs = append(seqs, int(x.(map[string]any)["Seq"].(float64)))
	}
	h.act(fmt.Sprintf(`{"Do":"mark","Key":"fm/kit","Seq":%d}`, seqs[1]))
	b = h.act(fmt.Sprintf(`{"Do":"mark_to","Key":"fm/kit","Seq":%d}`, seqs[2]))
	if lg := event(b, "Log"); lg == nil || lg["Marking"] != true || lg["Start"] != float64(seqs[1]) || lg["End"] != float64(seqs[2]) {
		t.Errorf("mark_to: %v", lg)
	}
	b = h.act(fmt.Sprintf(`{"Do":"grow","Key":"fm/kit","Seq":%d}`, seqs[0]))
	if lg := event(b, "Log"); lg == nil || lg["Start"] != float64(seqs[0]) || lg["End"] != float64(seqs[2]) {
		t.Errorf("grow: %v", lg)
	}
	b = h.act(`{"Do":"end_mark","Key":"fm/kit"}`)
	if lg := event(b, "Log"); lg == nil || lg["Marking"] != false || lg["Start"] != nil {
		t.Errorf("end_mark: %v", lg)
	}
	h.act(fmt.Sprintf(`{"Do":"mark","Key":"fm/kit","Seq":%d}`, seqs[0]))
	b = h.act(fmt.Sprintf(`{"Do":"mark","Key":"fm/kit","Seq":%d}`, seqs[2]))
	lg := event(b, "Log")
	if lg == nil || lg["Start"] != float64(seqs[0]) || lg["End"] != float64(seqs[2]) || lg["Selected"] != 3.0 {
		t.Errorf("range: %v", lg)
	}
	b = h.act(`{"Do":"add_text","Key":"fm/kit","Term":"two"}`)
	if lg := event(b, "Log"); lg == nil || len(lg["Hidden"].([]any)) != 2 {
		t.Errorf("filter: %v", lg)
	}
	if event(b, "Filter") == nil {
		t.Errorf("no Filter: %v", types(b))
	}
	b = h.act(`{"Do":"log_close","Key":"fm/kit"}`)
	if lg := event(b, "Log"); lg == nil || lg["Open"] != false {
		t.Errorf("close: %v", lg)
	}
	h.act(`{"Do":"mark","Key":"fm/kit","Seq":0}`) // log mode closed: nothing
}

func TestSettingsReportEveryProblemOrSaved(t *testing.T) {
	h := start(t)
	b := h.act(`{"Do":"add_world","ID":"fm","WorldSettings":{"Host":"","Port":0}}`)
	p := event(b, "Problems")
	var fields []string
	if p != nil {
		for _, x := range p["Problems"].([]any) {
			fields = append(fields, x.(map[string]any)["Field"].(string))
		}
	}
	if p == nil || p["Op"] != "add_world" || !slices.Equal(fields, []string{"port", "host", "id"}) {
		t.Errorf("problems %v", p)
	}
	b = h.act(`{"Do":"add_world","ID":"zz","WorldSettings":{"Host":"zz.test","Port":7777}}`)
	if s := event(b, "Saved"); s == nil || s["Op"] != "add_world" {
		t.Errorf("saved: %v", types(b))
	}
	w := event(b, "Worlds")
	if w == nil || len(w["Worlds"].([]any)) != 2 {
		t.Errorf("worlds after adding: %v", w)
	}
}

func TestMarkUpwardOrdersTheRange(t *testing.T) {
	a, dir := newApp(t)
	ch, _ := a.Find("fm/kit")
	l, _ := a.LogLayout(ch)
	kilntest.WriteLog(t, l, day.AddDate(0, 0, -1), "one", "two", "three")
	h := startOn(t, a, dir)
	h.act(`{"Do":"open","Key":"fm/kit"}`)
	h.act(`{"Do":"log","Key":"fm/kit"}`)
	h.act(`{"Do":"mark","Key":"fm/kit","Seq":-1}`)
	b := h.act(`{"Do":"mark","Key":"fm/kit","Seq":-3}`) // selected upward
	if lg := event(b, "Log"); lg == nil || lg["Start"] != -3.0 || lg["End"] != -1.0 {
		t.Errorf("range: %v", lg)
	}
}

// The page numbers its input actions (N); Input says which it has seen
// (Ack), so the page only takes Text when nothing newer is in flight.
func TestInputAcksThePage(t *testing.T) {
	h := start(t)
	h.act(`{"Do":"open","Key":"fm/kit"}`)
	h.l.Act([]byte(`{"Do":"submit","Key":"fm/kit","Text":"/notify off","N":1}`))
	h.l.Act([]byte(`{"Do":"input","Key":"fm/kit","Text":"x","N":2}`))
	first, second := h.next(), h.next()
	if in := event(first, "Input"); in == nil || in["Text"] != "" || in["Ack"] != 1.0 {
		t.Errorf("submit: %v", in)
	}
	if in := event(second, "Input"); in != nil {
		t.Errorf("input came back: %v", in)
	}
	b := h.act(`{"Do":"older","Key":"fm/kit","Text":"x","N":3}`) // the text doesn't change: still answered
	if in := event(b, "Input"); in == nil || in["Ack"] != 3.0 {
		t.Errorf("older: %v", in)
	}
}

func TestActNeverBlocksAfterTheLoopEnds(t *testing.T) {
	a, _ := newApp(t)
	got := make(chan []byte, 10)
	l := New(a, func(b []byte) { got <- b })
	done := make(chan struct{})
	go func() { l.Run(context.Background(), Options{}); close(done) }()
	<-got
	l.Act([]byte(`{"Do":"quit"}`))
	<-done
	finished := make(chan struct{})
	go func() {
		for range 1000 {
			l.Act([]byte(`{"Do":"here"}`))
		}
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Error("Act blocked after the loop ended")
	}
}

func TestSubmitForAnInputThatIsntActiveIsntSent(t *testing.T) {
	h := start(t)
	act := event(h.act(`{"Do":"open","Key":"fm/kit"}`), "Active")["Key"].(string)
	if a := event(h.act(`{"Do":"open","Key":"fm/rook"}`), "Active"); a != nil {
		act = a["Key"].(string)
	}
	other := "fm/kit"
	if act == "fm/kit" {
		other = "fm/rook"
	}
	b := h.act(`{"Do":"submit","Key":"` + other + `","Text":"/notify off","N":1}`)
	if in := inputOf(b, act); in != nil {
		t.Errorf("a submit for %s went to %s: %v", other, act, in)
	}
	if st := event(b, "Status"); st != nil && st["Text"] != "" {
		t.Errorf("the command ran: %v", st)
	}
}

func TestAddCharacterSaysItsKey(t *testing.T) {
	h := start(t)
	b := h.act(`{"Do":"add_character","World":"fm","Name":"Ash"}`)
	if s := event(b, "Saved"); s == nil || s["Key"] != "fm/Ash" {
		t.Errorf("saved %v", s)
	}
}

func TestStatusExpiresAndTypingDismissesIt(t *testing.T) {
	a, dir := newApp(t)
	h := &harness{t: t, a: a, dir: dir, batches: make(chan []map[string]any, 100)}
	h.l = New(a, func(b []byte) { h.batches <- decode(t, b) })
	h.l.statusTimeout = 50 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go h.l.Run(ctx, Options{})
	h.next()
	h.act(`{"Do":"frobnicate"}`)
	if st := event(h.next(), "Status"); st == nil || st["Text"] != "" {
		t.Errorf("the status should time out: %v", st)
	}
	h.act(`{"Do":"frobnicate"}`)
	if st := event(h.act(`{"Do":"input","Key":"","Text":"x","N":1}`), "Status"); st == nil || st["Text"] != "" {
		t.Errorf("typing should dismiss the status: %v", st)
	}
}

func TestAllKeepsTheTurnsEffects(t *testing.T) {
	a, _ := newApp(t)
	l := New(a, func([]byte) {})
	evs := l.events([]app.Effect{app.Copy{Text: "x"}, app.Run{Func: func() any { return nil }}}, true)
	var effects int
	for _, e := range evs {
		if _, ok := e.(feed.Effect); ok {
			effects++
		}
	}
	if effects != 1 {
		t.Errorf("All with a Copy and a Run: %d effects", effects)
	}
}

func TestUnknownActionFieldsAreRefused(t *testing.T) {
	h := start(t)
	b := h.act(`{"Do":"mark","Key":"fm/kit","Seqs":3}`)
	if st := event(b, "Status"); st == nil || st["Err"] != true || !strings.HasPrefix(st["Text"].(string), upTo(str.StatusBadAction(errors.New(mark)))) {
		t.Errorf("unknown field: %v", b)
	}
}

func TestDraftsSurviveARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drafts.json")
	a, _ := newApp(t)
	got := make(chan []map[string]any, 100)
	l := New(a, func(b []byte) { got <- decode(t, b) })
	done := make(chan struct{})
	go func() { l.Run(context.Background(), Options{Drafts: path}); close(done) }()
	<-got
	l.Act([]byte(`{"Do":"open","Key":"fm/kit"}`))
	<-got
	l.Act([]byte(`{"Do":"input","Key":"fm/kit","Text":"half a thought","N":1}`))
	<-got
	l.Act([]byte(`{"Do":"save_drafts"}`))
	<-got
	l.Act([]byte(`{"Do":"quit"}`))
	<-done

	b, _ := newApp(t)
	h := &harness{t: t, a: b, batches: make(chan []map[string]any, 100)}
	h.l = New(b, func(x []byte) { h.batches <- decode(t, x) })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go h.l.Run(ctx, Options{Drafts: path})
	h.next()
	var restored bool
	for _, e := range h.act(`{"Do":"open","Key":"fm/kit"}`) {
		if e["type"] == "Input" && e["Key"] == "fm/kit" && e["Text"] == "half a thought" {
			restored = true
		}
	}
	if !restored {
		t.Error("the draft didn't come back")
	}
}

func TestBadDraftsFileIsIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drafts.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	a, _ := newApp(t)
	h := &harness{t: t, a: a, batches: make(chan []map[string]any, 100)}
	h.l = New(a, func(x []byte) { h.batches <- decode(t, x) })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go h.l.Run(ctx, Options{Drafts: path})
	h.next()
	if b := h.act(`{"Do":"open","Key":"fm/kit"}`); event(b, "Char") == nil {
		t.Errorf("open after a bad drafts file: %v", types(b))
	}
}

func TestSayPutsAStatusOnTheLoop(t *testing.T) {
	h := start(t)
	h.l.Say(true, "x") //str:ok: a test status
	if st := event(h.next(), "Status"); st == nil || st["Text"] != "x" || st["Err"] != true {
		t.Errorf("status %v", st)
	}
}

// drafted runs a loop over a with drafts at path, does acts, saves the
// drafts and quits.
func drafted(t *testing.T, a *app.App, path string, acts ...string) {
	t.Helper()
	got := make(chan []map[string]any, 100)
	l := New(a, func(b []byte) { got <- decode(t, b) })
	done := make(chan struct{})
	go func() { l.Run(context.Background(), Options{Drafts: path}); close(done) }()
	<-got
	for _, js := range append(acts, `{"Do":"save_drafts"}`, `{"Do":"quit"}`) {
		l.Act([]byte(js))
		<-got
	}
	<-done
}

func TestDraftsNeverHoldThePassword(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drafts.json")
	a2, _ := newApp(t)
	a2.Open("fm/kit")
	a2.SetInput("half a thought")
	sess := attachTo(a2, "fm/kit")
	a2.Handle(app.SessionMsg{Key: "fm/kit", Sess: sess, OK: true, Ev: session.Event{Kind: session.EventNeedPassword}})
	a2.SetInput("hunter2")
	drafted(t, a2, path)
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), "hunter2") || !strings.Contains(string(b), "half a thought") {
		t.Errorf("drafts file %s", b)
	}
}

func TestSavingKeepsDraftsNotYetRestored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drafts.json")
	if err := os.WriteFile(path, []byte(`{"fm/rook":"for rook"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	a, _ := newApp(t)
	drafted(t, a, path, `{"Do":"open","Key":"fm/kit"}`) // Rook never opened
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "for rook") {
		t.Errorf("Rook's draft was dropped: %s", b)
	}
}

func TestSendingADraftTakesItOffDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drafts.json")
	if err := os.WriteFile(path, []byte(`{"fm/kit":"/notify off"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	a, _ := newApp(t)
	got := make(chan []map[string]any, 100)
	l := New(a, func(b []byte) { got <- decode(t, b) })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go l.Run(ctx, Options{Drafts: path})
	<-got
	l.Act([]byte(`{"Do":"open","Key":"fm/kit"}`))
	<-got
	l.Act([]byte(`{"Do":"submit","Key":"fm/kit","Text":"/notify off","N":1}`)) // the restored draft, sent
	<-got
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), "notify") {
		t.Errorf("a sent draft is still on disk: %s", b)
	}
}

// attachTo gives k a session, so session messages for it are taken.
func attachTo(a *app.App, k string) *session.Session {
	c := a.Char(k)
	c.Sess = session.New(session.Options{Char: c.Ch, Log: kilntest.NopLog{}})
	return c.Sess
}

func TestTrailingDataAfterAnActionIsRefused(t *testing.T) {
	h := start(t)
	b := h.act(`{"Do":"here"} {"Do":"quit"}`)
	if st := event(b, "Status"); st == nil || st["Err"] != true {
		t.Errorf("trailing data: %v", b)
	}
}

// Act never blocks, even with the loop busy (here: not running at all),
// and what it queued is taken in order.
func TestActNeverBlocksAndKeepsOrder(t *testing.T) {
	a, _ := newApp(t)
	got := make(chan []map[string]any, 1000)
	l := New(a, func(b []byte) { got <- decode(t, b) })
	done := make(chan struct{})
	go func() {
		for range 1000 {
			l.Act([]byte(`{"Do":"here"}`))
		}
		l.Act([]byte(`{"Do":"open","Key":"fm/kit"}`))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Act blocked while the loop wasn't draining")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go l.Run(ctx, Options{})
	<-got // everything
	for range 1000 {
		if b := <-got; event(b, "Active") != nil {
			t.Fatal("the open came before the heres queued ahead of it")
		}
	}
	if b := <-got; event(b, "Active") == nil {
		t.Errorf("the open, last: %v", types(b))
	}
}

func TestIdleDraftIsRestored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drafts.json")
	if err := os.WriteFile(path, []byte(`{"":"idle words"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	a, _ := newApp(t)
	got := make(chan []map[string]any, 10)
	l := New(a, func(b []byte) { got <- decode(t, b) })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go l.Run(ctx, Options{Drafts: path})
	if in := event(<-got, "Input"); in == nil || in["Key"] != "" || in["Text"] != "idle words" {
		t.Errorf("idle input %v", in)
	}
}

func TestCheckField(t *testing.T) {
	if got, want := CheckField("port", "55a"), app.AcceptPort("55a").Error(); got != want {
		t.Errorf("CheckField(port, 55a) = %q, want %q", got, want)
	}
	if got := CheckField("port", "5555"); got != "" {
		t.Errorf("CheckField(port, 5555) = %q", got)
	}
}

func TestAwaySaysSo(t *testing.T) {
	h := start(t)
	h.act(`{"Do":"focus","In":true}`) // unfocused is away already
	b := h.act(`{"Do":"away"}`)
	if st := event(b, "Status"); st == nil || st["Text"] != str.StatusAway() {
		t.Errorf("status %v", st)
	}
	if p := event(b, "Presence"); p == nil || p["State"] != float64(app.PresenceAway) {
		t.Errorf("presence %v", p)
	}
}

func TestStepAndNextUnreadSwitch(t *testing.T) {
	h := start(t)
	h.act(`{"Do":"open","Key":"fm/kit"}`)
	h.act(`{"Do":"open","Key":"fm/rook"}`)
	// active sends js and is the Active event's key that follows; opening
	// sends more batches as history loads, so it reads on until one comes.
	active := func(js string) any {
		h.l.Act([]byte(js))
		for {
			if e := event(h.next(), "Active"); e != nil {
				return e["Key"]
			}
		}
	}
	active(`{"Do":"switch","Key":"=fm"}`) // whichever opened active, this changes it
	active(`{"Do":"switch","Key":"fm/kit"}`)
	if got := active(`{"Do":"step","N":1}`); got != "fm/rook" {
		t.Errorf("step 1 from Kit: %v", got)
	}
	if got := active(`{"Do":"step","N":-1}`); got != "fm/kit" {
		t.Errorf("step -1 from Rook: %v", got)
	}
	if got := active(`{"Do":"next_unread","N":1}`); got != "fm/rook" {
		t.Errorf("next unread, none unread: %v, want the last one active", got)
	}
}

func TestInputForACharacterSwitchedAwayKeepsItsDraft(t *testing.T) {
	h := start(t)
	h.act(`{"Do":"open","Key":"fm/kit"}`)
	h.act(`{"Do":"open","Key":"fm/rook"}`)
	h.act(`{"Do":"switch","Key":"fm/rook"}`)
	h.act(`{"Do":"input","Key":"fm/kit","Text":"half a pose","N":7}`) // typed before the page heard of the switch
	if got := h.a.Draft("fm/kit"); got != "half a pose" {
		t.Errorf("Kit's draft %q", got)
	}
	if got := h.a.Text(); got != "" {
		t.Errorf("Rook's input %q: Kit's typing went to Rook", got)
	}
}

// inputOf is b's Input event for key, nil if none.
func inputOf(b []map[string]any, key string) map[string]any {
	for _, e := range b {
		if e["type"] == "Input" && e["Key"] == key {
			return e
		}
	}
	return nil
}

// promptUp opens and connects fm/kit and waits for its password prompt
// (fm has no saved passwords).
func (h *harness) promptUp() {
	h.t.Helper()
	h.act(`{"Do":"open","Key":"fm/kit"}`)
	for b := h.act(`{"Do":"submit","Key":"fm/kit","Text":"","N":1}`); ; b = h.next() {
		if in := inputOf(b, "fm/kit"); in != nil && in["NeedPW"] == true {
			return
		}
	}
}

func TestSubmitForACharacterSwitchedAwayKeepsItAsItsDraft(t *testing.T) {
	h := start(t)
	h.act(`{"Do":"open","Key":"fm/kit"}`)
	h.act(`{"Do":"open","Key":"fm/rook"}`)
	h.act(`{"Do":"switch","Key":"fm/rook"}`)
	b := h.act(`{"Do":"submit","Key":"fm/kit","Text":"a pose","N":7}`) // Enter before the page heard of the switch
	if got := h.a.Draft("fm/kit"); got != "a pose" {
		t.Errorf("Kit's draft %q", got)
	}
	if in := inputOf(b, "fm/kit"); in == nil || in["Text"] != "a pose" || in["Ack"] != float64(7) {
		t.Errorf("Kit's input %v: the page cleared it and should hear it back", in)
	}
}

func TestTypingDuringThePasswordPromptIsTheDraft(t *testing.T) {
	h := start(t)
	h.promptUp()
	h.act(`{"Do":"input","Key":"fm/kit","Text":"half a pose","N":3}`) // typed before the page heard of the prompt
	if got := h.a.Draft("fm/kit"); got != "half a pose" {
		t.Errorf("draft %q", got)
	}
	h.act(`{"Do":"cancel"}`)
	if got := h.a.Text(); got != "half a pose" {
		t.Errorf("after skipping the login, the input holds %q", got)
	}
}

func TestSubmitDuringThePasswordPromptIsntThePassword(t *testing.T) {
	h := start(t)
	h.promptUp()
	b := h.act(`{"Do":"submit","Key":"fm/kit","Text":"a pose","N":3}`) // Enter before the page heard of the prompt
	if !h.a.Char("fm/kit").NeedPW {
		t.Fatal("the pose was taken as the password")
	}
	if in := inputOf(b, "fm/kit"); in == nil || in["Text"] != "a pose" || in["Ack"] != float64(3) {
		t.Errorf("Kit's input %v: the page cleared it and should hear it back", in)
	}
	h.act(`{"Do":"password","Key":"fm/kit","Text":"hunter2","N":4}`)
	if h.a.Char("fm/kit").NeedPW {
		t.Error("the password wasn't taken")
	}
	if got := h.a.Text(); got != "a pose" {
		t.Errorf("after logging in, the input holds %q", got)
	}
}

func TestOverRanges(t *testing.T) {
	for _, c := range []struct {
		text string
		max  int
		mode string
		want [][2]int
	}{
		{"12345", 5, "batch", nil},
		{"123456", 5, "batch", [][2]int{{5, 6}}},
		{"123\n456789", 5, "batch", [][2]int{{9, 10}}},
		{"1234567\n123456", 5, "batch", [][2]int{{5, 7}, {13, 14}}},
		{"123\n456", 5, "flatten", [][2]int{{5, 7}}}, // "123 456": the 6th byte is "5"
		{"1234567\n12", 5, "flatten", [][2]int{{5, 10}}},
		{"éééé", 5, "batch", [][2]int{{2, 4}}},      // é is 2 bytes: the 3rd é ends at byte 6
		{"ab😀", 3, "batch", [][2]int{{2, 4}}},       // the emoji is 4 bytes, 2 UTF-16 units
		{"ae\u0301", 2, "batch", [][2]int{{1, 3}}},  // the combining accent takes "é" over: all of it is marked
		{"a👩\u200d💻", 6, "batch", [][2]int{{1, 6}}}, // a ZWJ sequence is one character
		{"1234567\n\n12", 5, "flatten", [][2]int{{5, 11}}},
		{"anything", 0, "batch", nil},
	} {
		if got := OverRanges(c.text, c.max, c.mode); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q max %d %s: %v, want %v", c.text, c.max, c.mode, got, c.want)
		}
	}
}

func TestBackupIsADownload(t *testing.T) {
	a, dir := newApp(t)
	h := startWith(t, a, dir, Options{Backup: func() ([]byte, error) { return []byte("zip!"), nil }})
	b := h.act(`{"Do":"submit","Key":"","Text":"/backup","N":1}`)
	e := event(b, "Effect")
	if e == nil || e["effect"] != "SaveFile" || e["Name"] != str.BackupFileName(a.Now().Format("2006-01-02")) || e["Data"] != base64.StdEncoding.EncodeToString([]byte("zip!")) {
		t.Errorf("effect %v", e)
	}
}

func TestBackupFailingSaysSo(t *testing.T) {
	a, dir := newApp(t)
	boom := errors.New("boom")
	h := startWith(t, a, dir, Options{Backup: func() ([]byte, error) { return nil, boom }})
	b := h.act(`{"Do":"submit","Key":"","Text":"/backup","N":1}`)
	if st := event(b, "Status"); st == nil || st["Text"] != str.StatusBackupFailed(boom) || st["Err"] != true {
		t.Errorf("status %v", st)
	}
	if event(b, "Effect") != nil {
		t.Error("an effect after a failed backup")
	}
}

// /reset asks first: the first Enter keeps /reset in the input and warns,
// the second goes to the page. Esc in between asks again.
func TestResetAsksFirst(t *testing.T) {
	a, dir := newApp(t)
	h := startWith(t, a, dir, Options{})
	b := h.act(`{"Do":"submit","Key":"","Text":"/reset","N":1}`)
	if event(b, "Effect") != nil {
		t.Fatal("the first Enter reset")
	}
	if st := event(b, "Status"); st == nil || st["Text"] != str.StatusResetConfirm() || st["Err"] != true {
		t.Errorf("status %v", st)
	}
	if in := event(b, "Input"); in == nil || in["Text"] != "/reset" {
		t.Errorf("input %v", in)
	}
	b = h.act(`{"Do":"submit","Key":"","Text":"/reset","N":2}`)
	if e := event(b, "Effect"); e == nil || e["effect"] != "Do" || e["Cmd"] != "/reset" {
		t.Errorf("the second Enter: effect %v", e)
	}
	h.act(`{"Do":"submit","Key":"","Text":"/reset","N":3}`)
	h.act(`{"Do":"cancel"}`)
	if b := h.act(`{"Do":"submit","Key":"","Text":"/reset","N":4}`); event(b, "Effect") != nil {
		t.Error("Esc should make /reset ask again")
	}
}

// The /reset arm lasts one step: editing the text, or another command,
// in between asks again. What the page sends on its own (presence,
// focus, size) doesn't.
func TestResetArmIsOneShot(t *testing.T) {
	for _, c := range []struct {
		name    string
		between []string
		resets  bool
	}{
		{"an edit taken back", []string{`{"Do":"input","Key":"","Text":"/rese","N":2}`, `{"Do":"input","Key":"","Text":"/reset","N":3}`}, false},
		{"another command", []string{`{"Do":"submit","Key":"","Text":"/away","N":2}`, `{"Do":"input","Key":"","Text":"/reset","N":3}`}, false},
		{"the page's own", []string{`{"Do":"here"}`, `{"Do":"focus","In":true}`, `{"Do":"resize","Cols":80,"Rows":24}`, `{"Do":"input","Key":"","Text":"/reset","N":3}`}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			a, dir := newApp(t)
			h := startWith(t, a, dir, Options{})
			h.act(`{"Do":"submit","Key":"","Text":"/reset","N":1}`)
			for _, b := range c.between {
				h.act(b)
			}
			b := h.act(`{"Do":"submit","Key":"","Text":"/reset","N":4}`)
			if got := event(b, "Effect") != nil; got != c.resets {
				t.Errorf("reset %v, want %v", got, c.resets)
			}
		})
	}
}

func TestRestoreTakesTheFile(t *testing.T) {
	a, dir := newApp(t)
	var got []byte
	h := startWith(t, a, dir, Options{Restore: func(b []byte) (int, error) { got = b; return 3, nil }})
	b := h.act(`{"Do":"restore","Data":"` + base64.StdEncoding.EncodeToString([]byte("zip!")) + `"}`)
	if string(got) != "zip!" {
		t.Errorf("restored %q", got)
	}
	if st := event(b, "Status"); st == nil || st["Text"] != str.StatusRestored(3) {
		t.Errorf("status %v", st)
	}
}

func TestRestoreFailingSaysSo(t *testing.T) {
	a, dir := newApp(t)
	boom := errors.New("boom")
	h := startWith(t, a, dir, Options{Restore: func([]byte) (int, error) { return 0, boom }})
	b := h.act(`{"Do":"restore","Data":"eA=="}`)
	if st := event(b, "Status"); st == nil || st["Text"] != str.StatusRestoreFailed(boom) || st["Err"] != true {
		t.Errorf("status %v", st)
	}
}

func TestPickOpensSwitchesAndConnects(t *testing.T) {
	h := start(t)
	b := h.act(`{"Do":"pick","Key":"fm/rook"}`)
	if a := event(b, "Active"); a == nil || a["Key"] != "fm/rook" {
		t.Errorf("active %v", a)
	}
	deadline := time.After(2 * time.Second)
	for {
		if c := event(b, "Char"); c != nil && c["Key"] == "fm/rook" && c["State"] != float64(session.Disconnected) {
			return
		}
		select {
		case b = <-h.batches:
		case <-deadline:
			t.Fatal("never connecting")
		}
	}
}

func TestPickAnOpenCharacterOnlySwitches(t *testing.T) {
	h := start(t)
	h.act(`{"Do":"open","Key":"fm/kit"}`)
	h.act(`{"Do":"open","Key":"fm/rook"}`)
	h.act(`{"Do":"switch","Key":"fm/kit"}`)
	h.act(`{"Do":"pick","Key":"fm/rook"}`)
	if got := h.a.Active(); got != "fm/rook" {
		t.Errorf("active %q", got)
	}
	if h.a.Char("fm/rook").Sess != nil {
		t.Error("picking an open character connected it")
	}
}

func TestPickSomeoneUnknownDoesNothing(t *testing.T) {
	h := start(t)
	h.act(`{"Do":"pick","Key":"fm/nobody"}`)
	if h.a.Char("fm/nobody") != nil || h.a.Active() != "" {
		t.Error("picked a character that isn't configured")
	}
}

func TestSettingsAreWhatTheEditorStartsFrom(t *testing.T) {
	h := start(t)
	w := event(h.act(`{"Do":"settings","World":"fm"}`), "Settings")
	if w == nil || w["World"] != "fm" || w["CharSettings"] != nil {
		t.Fatalf("fm: %v", w)
	}
	ws := w["WorldSettings"].(map[string]any)
	if ws["Host"] != "muck.test" || ws["Port"] != 8888.0 || ws["Login"] != nil {
		t.Errorf("fm's settings: %v (an unset setting is null)", ws)
	}
	if w["Inherited"].(map[string]any)["NewlineMode"] == "" {
		t.Errorf("fm inherits: %v", w["Inherited"])
	}
	c := event(h.act(`{"Do":"settings","World":"fm","Char":"kit"}`), "Settings")
	if c == nil || c["Char"] != "kit" || c["Name"] != "Kit" || c["CharSettings"] == nil || c["WorldSettings"] != nil {
		t.Errorf("fm/kit: %v", c)
	}
	n := event(h.act(`{"Do":"settings"}`), "Settings")
	if n == nil || n["World"] != "" || fmt.Sprint(n["WorldSettings"].(map[string]any)["Use"]) != "[fuzzball]" {
		t.Errorf("a new world: %v", n)
	}
}

func TestSettingsThatCantBeReadSayWhy(t *testing.T) {
	h := start(t)
	b := h.act(`{"Do":"settings","World":"nope"}`)
	if s := event(b, "Settings"); s != nil {
		t.Errorf("settings for a world that isn't there: %v", s)
	}
	if st := event(b, "Status"); st == nil || st["Err"] != true {
		t.Errorf("status: %v", st)
	}
}

// logWith opens fm/kit's log mode over a day of lines and returns the
// harness and the lines' seqs, oldest first.
func logWith(t *testing.T, texts ...string) (*harness, []int) {
	t.Helper()
	a, dir := newApp(t)
	ch, _ := a.Find("fm/kit")
	l, _ := a.LogLayout(ch)
	kilntest.WriteLog(t, l, day, texts...)
	h := startOn(t, a, dir)
	h.act(`{"Do":"open","Key":"fm/kit"}`)
	b := h.act(`{"Do":"log","Key":"fm/kit"}`)
	var seqs []int
	for _, e := range b {
		if e["type"] == "LogLines" {
			for _, x := range e["Lines"].([]any) {
				seqs = append(seqs, int(x.(map[string]any)["Seq"].(float64)))
			}
		}
	}
	return h, seqs
}

func TestFindAndStepsSayWhereTheyLand(t *testing.T) {
	h, seqs := logWith(t, "apple one", "pear", "apple two")
	b := h.act(`{"Do":"find","Key":"fm/kit","Term":"apple","Seq":-1}`)
	if at := event(b, "LogAt"); at == nil || at["Seq"] != float64(seqs[2]) {
		t.Fatalf("find: %v", at)
	}
	b = h.act(fmt.Sprintf(`{"Do":"find_older","Key":"fm/kit","Seq":%d}`, seqs[2]))
	if at := event(b, "LogAt"); at == nil || at["Seq"] != float64(seqs[0]) {
		t.Errorf("n: %v", at)
	}
	b = h.act(fmt.Sprintf(`{"Do":"find_newer","Key":"fm/kit","Seq":%d}`, seqs[0]))
	if at := event(b, "LogAt"); at == nil || at["Seq"] != float64(seqs[2]) {
		t.Errorf("N: %v", at)
	}
	b = h.act(fmt.Sprintf(`{"Do":"find_newer","Key":"fm/kit","Seq":%d}`, seqs[2]))
	if at := event(b, "LogAt"); at != nil {
		t.Errorf("N past the newest: %v", at)
	}
	if st := event(b, "Status"); st == nil || st["Text"] != str.BrowseNoNewerMatches("apple") {
		t.Errorf("N past the newest says so: %v", st)
	}
}

func TestFindOlderLandsOnALaterTurn(t *testing.T) {
	a, dir := newApp(t)
	ch, _ := a.Find("fm/kit")
	l, _ := a.LogLayout(ch)
	kilntest.WriteLog(t, l, day.AddDate(0, 0, -1), "needle")
	kilntest.WriteLog(t, l, day, "hay")
	h := startOn(t, a, dir)
	h.act(`{"Do":"open","Key":"fm/kit"}`)
	h.act(`{"Do":"log","Key":"fm/kit"}`)
	h.l.Act([]byte(`{"Do":"find","Key":"fm/kit","Term":"needle","Seq":-1}`))
	for range 10 { // the older day is read, then the find goes on in it
		if at := event(h.next(), "LogAt"); at != nil {
			return
		}
	}
	t.Error("no LogAt after reading the older day")
}

func TestGotoLands(t *testing.T) {
	h, seqs := logWith(t, "one", "two")
	b := h.act(`{"Do":"goto","Key":"fm/kit","When":"` + day.Format("2006-01-02") + `"}`)
	if at := event(b, "LogAt"); at == nil || at["Seq"] != float64(seqs[0]) {
		t.Errorf("goto a day: %v", at)
	}
	b = h.act(`{"Do":"goto","Key":"fm/kit","When":"` + day.Add(time.Hour).Local().Format("2006-01-02 15:04") + `"}`)
	if at := event(b, "LogAt"); at == nil || at["Seq"] != float64(seqs[1]) {
		t.Errorf("goto a time after both: %v", at)
	}
}

func TestTagsExportNameAndSay(t *testing.T) {
	h, seqs := logWith(t, "one", "two")
	h.act(`{"Do":"say","Text":"news","Err":false}`)
	b := h.act(fmt.Sprintf(`{"Do":"tags","Key":"fm/kit","Seq":%d}`, seqs[0]))
	if lt := event(b, "LineTags"); lt == nil || lt["Seq"] != float64(seqs[0]) || lt["Text"] != "" {
		t.Errorf("tags: %v", lt)
	}
	if st := event(b, "Status"); st != nil {
		t.Errorf("asking a line's tags isn't acting: the status went to %v", st)
	}
	b = h.act(`{"Do":"export_name","Key":"fm/kit","Format":"plain"}`)
	if st := event(b, "Status"); st == nil || st["Text"] != str.BrowseMarkRange() || event(b, "ExportName") != nil {
		t.Errorf("nothing marked: %v", types(b))
	}
	h.act(fmt.Sprintf(`{"Do":"mark","Key":"fm/kit","Seq":%d}`, seqs[0]))
	h.act(fmt.Sprintf(`{"Do":"mark","Key":"fm/kit","Seq":%d}`, seqs[1]))
	b = h.act(`{"Do":"export_name","Key":"fm/kit","Format":"plain"}`)
	if n := event(b, "ExportName"); n == nil || n["Format"] != "plain" || n["Name"] == "" || strings.Contains(n["Name"].(string), "/") {
		t.Errorf("export name: %v (no directory: a download)", n)
	}
	b = h.act(`{"Do":"say","Text":"hello","Err":true}`)
	if st := event(b, "Status"); st == nil || st["Text"] != "hello" || st["Err"] != true {
		t.Errorf("say: %v", st)
	}
}

// Log mode's message goes to the status line, timed as any status, and
// acting in log mode again drops it, as the TUI's takeLogStatus and
// clearLogStatus: it never lingers over how many lines are selected.
func TestLogStatusIsTheStatusLinesUntilYouActAgain(t *testing.T) {
	h, seqs := logWith(t, "one", "two")
	b := h.act(`{"Do":"copy","Key":"fm/kit"}`) // nothing marked: says to mark a range
	if st := event(b, "Status"); st == nil || st["Text"] != str.BrowseMarkRange() {
		t.Fatalf("copy's message on the status line: %v", types(b))
	}
	if lg := event(b, "Log"); lg != nil && lg["Status"] != "" {
		t.Errorf("still log mode's own: %v", lg["Status"])
	}
	b = h.act(fmt.Sprintf(`{"Do":"mark_to","Key":"fm/kit","Seq":%d}`, seqs[0])) // not selecting: does nothing, says nothing
	if st := event(b, "Status"); st == nil || st["Text"] != "" {
		t.Errorf("acting again leaves it: %v", st)
	}
}

// Opening log mode, or showing it again, takes down what the status line
// said, as the TUI's openBrowse: it's about what you were doing before.
func TestOpeningLogModeClearsTheStatus(t *testing.T) {
	h, _ := logWith(t, "one")
	h.act(`{"Do":"say","Text":"old news","Err":false}`)
	b := h.act(`{"Do":"log","Key":"fm/kit"}`) // open already: shown again
	if st := event(b, "Status"); st == nil || st["Text"] != "" {
		t.Errorf("the old status stays: %v", st)
	}
}

func TestCallRunsOnTheLoop(t *testing.T) {
	h := start(t)
	h.l.Call(func(a *app.App) { a.SetStatus(true, "x") }) //str:ok: a test status
	if st := event(h.next(), "Status"); st == nil || st["Text"] != "x" {
		t.Errorf("status %v", st)
	}
}

func TestAnswersSayTheirTarget(t *testing.T) {
	h := start(t)
	b := h.act(`{"Do":"save_world","World":"fm","WorldSettings":{"Host":"","Port":0}}`)
	if p := event(b, "Problems"); p == nil || p["World"] != "fm" || p["Char"] != "" {
		t.Errorf("Problems = %v", p)
	}
	b = h.act(`{"Do":"save_character","World":"fm","Char":"kit","CharSettings":{}}`)
	if s := event(b, "Saved"); s == nil || s["World"] != "fm" || s["Char"] != "kit" {
		t.Errorf("Saved = %v (%v)", s, types(b))
	}
}

func TestSettingsThatCantBeReadSayProblems(t *testing.T) {
	h := start(t)
	b := h.act(`{"Do":"settings","World":"nowhere","Char":""}`)
	if p := event(b, "Problems"); p == nil || p["Op"] != "settings" || p["World"] != "nowhere" {
		t.Errorf("batch = %v", types(b))
	}
}

// While selecting, a find's landing moves the range's end, as the TUI's
// found does: the cursor goes there.
func TestALandingMovesALiveRange(t *testing.T) {
	h, seqs := logWith(t, "one", "two", "three")
	h.act(fmt.Sprintf(`{"Do":"mark","Key":"fm/kit","Seq":%d}`, seqs[2]))
	b := h.act(fmt.Sprintf(`{"Do":"find","Key":"fm/kit","Term":"one","Seq":%d}`, seqs[2]))
	if lg := event(b, "Log"); lg == nil || lg["Start"] != float64(seqs[0]) || lg["End"] != float64(seqs[2]) || lg["Marking"] != true {
		t.Errorf("after find landed on one: %v", lg)
	}
}
