package webapi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	h := &harness{t: t, a: a, dir: dir, batches: make(chan []map[string]any, 100)}
	h.l = New(a, func(b []byte) { h.batches <- decode(t, b) })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go h.l.Run(ctx, Options{})
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
	b = h.act(`{"Do":"older","Key":"fm/kit","Text":"draft","N":3}`)
	if in := event(b, "Input"); in == nil || in["Text"] != "/notify off" {
		t.Errorf("history: %v", b)
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

func TestRangeUpwardDoesNotPanic(t *testing.T) {
	a, dir := newApp(t)
	ch, _ := a.Find("fm/kit")
	l, _ := a.LogLayout(ch)
	kilntest.WriteLog(t, l, day.AddDate(0, 0, -1), "one", "two", "three")
	h := startOn(t, a, dir)
	h.act(`{"Do":"open","Key":"fm/kit"}`)
	h.act(`{"Do":"log","Key":"fm/kit"}`)
	b := h.act(`{"Do":"range","Key":"fm/kit","Start":-1,"End":-3}`) // dragged upward
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
	b := h.act(`{"Do":"newer","Key":"fm/kit","Text":"x","N":3}`) // nothing to recall: still answered
	if in := event(b, "Input"); in == nil || in["Ack"] != 3.0 {
		t.Errorf("newer with nothing newer: %v", in)
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

func TestSubmitForAnInputThatIsntActiveIsDropped(t *testing.T) {
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
	if in := event(b, "Input"); in != nil {
		t.Errorf("a submit for %s went to %s: %v", other, act, in)
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
