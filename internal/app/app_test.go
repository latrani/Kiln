package app

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/latrani/Kiln/internal/config"
)

const fmWorld = `host = "muck.test"
port = 8888
tls = true
use = ["fuzzball"]
login = "connect {name} {password}"

[[characters]]
id = "kit"
name = "Kit"

[[characters]]
id = "rook"
name = "Rook"
`

const zzWorld = `host = "zz.test"
port = 7777
tls = true

[[characters]]
id = "ash"
name = "Ash"
`

// configDir writes worlds into a fresh config dir and returns it.
func configDir(t *testing.T, worlds map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := config.EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	writeWorlds(t, dir, worlds)
	return dir
}

func writeWorlds(t *testing.T, dir string, worlds map[string]string) {
	t.Helper()
	for name, body := range worlds {
		if err := os.WriteFile(filepath.Join(dir, "worlds", name+".toml"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func load(t *testing.T, dir string) *config.Config {
	t.Helper()
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// testApp is an App over fm (Kit, Rook) and zz (Ash), nothing open.
func testApp(t *testing.T) (*App, string) {
	t.Helper()
	dir := configDir(t, map[string]string{"fm": fmWorld, "zz": zzWorld})
	a := New(Deps{LogRoot: filepath.Join(dir, "logs")})
	a.ApplyConfig(load(t, dir))
	return a, dir
}

func openAll(t *testing.T, a *App, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if c, err := a.Open(k); c == nil || err != nil {
			t.Fatalf("Open(%s) = %v, %v", k, c, err)
		}
	}
}

func TestOpenSortsAndActivatesTheFirst(t *testing.T) {
	a, _ := testApp(t)
	openAll(t, a, "zz/ash", "fm/rook", "fm/kit")
	if got, want := a.Order(), []string{"fm/kit", "fm/rook", "zz/ash"}; !slices.Equal(got, want) {
		t.Errorf("Order = %v, want %v", got, want)
	}
	if a.Active() != "zz/ash" {
		t.Errorf("Active = %q, want the first one opened", a.Active())
	}
	again, _ := a.Open("fm/kit")
	if again != a.Char("fm/kit") {
		t.Error("opening an open character made a new one")
	}
	if c, _ := a.Open("fm/nobody"); c != nil {
		t.Error("opened a character that isn't configured")
	}
}

func TestSwitchClearsUnreadAndRemembersWhereYouWere(t *testing.T) {
	a, _ := testApp(t)
	openAll(t, a, "fm/kit", "fm/rook")
	rook := a.Char("fm/rook")
	rook.Unread, rook.Attention = 3, true
	if !a.Switch("fm/rook") {
		t.Fatal("Switch to an open character failed")
	}
	if rook.Unread != 0 || rook.Attention {
		t.Errorf("unread %d attention %v after switching to it", rook.Unread, rook.Attention)
	}
	if got := a.UnreadTarget(1); got != "fm/kit" {
		t.Errorf("UnreadTarget with nothing unread = %q, want the one before (fm/kit)", got)
	}
	if a.Switch("fm/nobody") || a.Active() != "fm/rook" {
		t.Error("Switch to a closed character changed something")
	}
	if !a.Switch(WorldSel("fm")) {
		t.Fatal("Switch to an open world's overview failed")
	}
	if w, ok := a.ActiveWorld(); !ok || w != "fm" {
		t.Errorf("ActiveWorld = %q, %v", w, ok)
	}
	if a.CanSwitch(WorldSel("zz")) || a.Switch(WorldSel("zz")) {
		t.Error("switched to the overview of a world with nothing open")
	}
}

func TestStepTargetWalksWorldsAndCharacters(t *testing.T) {
	a, _ := testApp(t)
	openAll(t, a, "fm/kit", "fm/rook", "zz/ash")
	if got, want := a.Stops(), []string{WorldSel("fm"), "fm/kit", "fm/rook", WorldSel("zz"), "zz/ash"}; !slices.Equal(got, want) {
		t.Errorf("Stops = %v, want %v", got, want)
	}
	if got := a.StepTarget(1); got != "fm/rook" {
		t.Errorf("StepTarget(1) from Kit = %q", got)
	}
	if got := a.StepTarget(-1); got != WorldSel("fm") {
		t.Errorf("StepTarget(-1) from Kit = %q", got)
	}
	a.Switch("zz/ash")
	if got := a.StepTarget(1); got != WorldSel("fm") {
		t.Errorf("StepTarget(1) from the last stop = %q, want it to wrap", got)
	}
}

func TestUnreadTargetFindsTheNextUnread(t *testing.T) {
	a, _ := testApp(t)
	openAll(t, a, "fm/kit", "fm/rook", "zz/ash")
	a.Char("zz/ash").Unread = 1
	for _, dir := range []int{1, -1} {
		if got := a.UnreadTarget(dir); got != "zz/ash" {
			t.Errorf("UnreadTarget(%d) = %q, want zz/ash", dir, got)
		}
	}
	empty, _ := testApp(t)
	if got := empty.UnreadTarget(1); got != "" {
		t.Errorf("UnreadTarget with nothing open = %q", got)
	}
}

func TestCloseActivatesTheNextDown(t *testing.T) {
	a, _ := testApp(t)
	openAll(t, a, "fm/kit", "fm/rook", "zz/ash")
	a.Switch("fm/rook")
	a.Close("fm/rook")
	if a.Active() != "zz/ash" || a.Char("fm/rook") != nil {
		t.Errorf("after closing Rook: active %q, rook %v", a.Active(), a.Char("fm/rook"))
	}
	a.Close("zz/ash")
	if a.Active() != "fm/kit" {
		t.Errorf("closing the last one should activate the one above: %q", a.Active())
	}
	a.Close("fm/kit")
	if a.Active() != "" || len(a.Order()) != 0 {
		t.Errorf("all closed: active %q order %v", a.Active(), a.Order())
	}
}

// Closing a world's last character while its overview shows moves on, as
// closing the active character does.
func TestClosingTheOverviewsLastCharacter(t *testing.T) {
	a, _ := testApp(t)
	openAll(t, a, "fm/kit", "zz/ash")
	a.Switch(WorldSel("zz"))
	a.Close("zz/ash")
	if a.Active() != "fm/kit" {
		t.Errorf("Active = %q, want fm/kit", a.Active())
	}
}

func TestApplyConfigClosesRemovedCharacters(t *testing.T) {
	a, dir := testApp(t)
	openAll(t, a, "fm/kit", "zz/ash")
	if err := os.Remove(filepath.Join(dir, "worlds", "zz.toml")); err != nil {
		t.Fatal(err)
	}
	res := a.ApplyConfig(load(t, dir))
	if !slices.Equal(res.Closed, []string{"zz/ash"}) || a.Char("zz/ash") != nil {
		t.Errorf("Closed = %v, ash still open: %v", res.Closed, a.Char("zz/ash") != nil)
	}
	if len(res.Errs) != 0 {
		t.Errorf("Errs = %v", res.Errs)
	}
}

func TestStatusGenerations(t *testing.T) {
	a, _ := testApp(t)
	a.SetStatus(true, "first")
	g := a.Status().Gen
	a.SetStatus(false, "second")
	if s := a.Status(); s.Text != "second" || s.Err || s.Gen != g+1 {
		t.Errorf("Status = %+v after a second message", s)
	}
	a.ClearStatus()
	if s := a.Status(); s.Text != "" || s.Gen != g+1 {
		t.Errorf("Status = %+v after clearing; Gen shouldn't move", s)
	}
}

func TestPasswordStoreFollowsTheConfig(t *testing.T) {
	if got := New(Deps{}).PasswordStore(); got != config.DefaultPasswordStore {
		t.Errorf("before a config: %q", got)
	}
	a, _ := testApp(t)
	if got := a.PasswordStore(); got != a.Config().PasswordStore {
		t.Errorf("PasswordStore = %q, config says %q", got, a.Config().PasswordStore)
	}
}
