package app

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/str"
)

// Each runs on every keystroke, so every prefix of a good value passes.
func TestAcceptFilters(t *testing.T) {
	for _, c := range []struct {
		name   string
		accept func(string) error
		good   []string
		bad    []string
	}{
		{"world id", AcceptWorldID, []string{"", "fm", "fm-2_b"}, []string{"f m", "fm!"}},
		{"host", AcceptHost, []string{"", "muck.test"}, []string{"a b", "é.test"}},
		{"port", AcceptPort, []string{"", "8", "65535", "99999"}, []string{"123456", "8a"}},
		{"packs", AcceptPacks, []string{"", "fuzzball", "fuzzball, x y"}, []string{"a;b"}},
		{"bytes", AcceptByteCount, []string{"", "4096"}, []string{"x", "1234567890"}},
		{"aliases", AcceptAliases, []string{"", "Kitty", "Kitty, K"}, []string{"#Kit", "K=t"}},
	} {
		for _, s := range c.good {
			if err := c.accept(s); err != nil {
				t.Errorf("%s: %q rejected: %v", c.name, s, err)
			}
		}
		for _, s := range c.bad {
			if c.accept(s) == nil {
				t.Errorf("%s: %q accepted", c.name, s)
			}
		}
	}
}

func TestDeleteWarning(t *testing.T) {
	if w := DeleteWarning("fm", ""); w != str.EditorDeleteWorldWarning("fm") {
		t.Errorf("world: %q", w)
	}
	if w := DeleteWarning("fm", "kit"); w != str.EditorDeleteCharacterWarning("fm/kit") {
		t.Errorf("character: %q", w)
	}
}

func TestAddWorldWritesIt(t *testing.T) {
	a, _ := reloadApp(t)
	if err := a.AddWorld("zz", config.WorldSettings{Host: "zz.test", Port: 7777, TLS: true}); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.Reload(); !ok {
		t.Fatal(a.Status().Text)
	}
	if !slices.ContainsFunc(a.Config().Worlds, func(w config.World) bool { return w.ID == "zz" }) {
		t.Error("zz isn't in the config")
	}
	if err := a.AddWorld("z z", config.WorldSettings{Host: "zz.test", Port: 7777}); err == nil {
		t.Error("a bad world id was written")
	}
}

func TestDeleteCharacterClosesAndForgetsThePassword(t *testing.T) {
	a, _ := reloadApp(t)
	var forgot []string
	pwErr := error(nil)
	a.d.DeletePassword = func(store, world, char string) error {
		forgot = append(forgot, world+"/"+char)
		return pwErr
	}
	openAll(t, a, "fm/kit", "fm/rook")
	if perr, err := a.DeleteCharacter("fm", "kit"); perr != nil || err != nil {
		t.Fatalf("DeleteCharacter: %v, %v", perr, err)
	}
	if a.Char("fm/kit") != nil || !slices.Equal(forgot, []string{"fm/kit"}) {
		t.Errorf("open %v, forgot %q", a.Char("fm/kit") != nil, forgot)
	}
	pwErr = errors.New("locked")
	if perr, err := a.DeleteCharacter("fm", "rook"); perr != pwErr || err != nil {
		t.Errorf("a kept password: %v, %v", perr, err)
	}
	if _, err := a.DeleteCharacter("fm", "nobody"); err == nil {
		t.Error("deleting a missing character succeeded")
	}
}

func TestForgetPassword(t *testing.T) {
	a, _ := reloadApp(t)
	if a.CanForgetPasswords() {
		t.Error("can forget with no DeletePassword")
	}
	a.d.DeletePassword = func(string, string, string) error { return nil }
	if !a.CanForgetPasswords() || a.ForgetPassword("fm", "kit") != nil {
		t.Error("ForgetPassword failed")
	}
}

// A world whose settings can't be written is taken back, so a failed add
// leaves no half-made world behind.
func TestAddWorldTakesBackAHalfMadeWorld(t *testing.T) {
	a, dir := reloadApp(t)
	if err := a.AddWorld("zz", config.WorldSettings{Host: "zz.test", Port: 7777, TLS: true, TLSTrust: "bogus"}); err == nil {
		t.Fatal("bad settings were written")
	}
	if _, err := os.Stat(filepath.Join(dir, "worlds", "zz.toml")); !os.IsNotExist(err) {
		t.Errorf("the half-made world is still there: %v", err)
	}
}

func TestAddWorldChecksEverythingFirst(t *testing.T) {
	a, dir := reloadApp(t)
	err := a.AddWorld("fm", config.WorldSettings{Host: "", Port: 0})
	var got []string
	for _, e := range Problems(err) {
		var fe *FieldError
		if errors.As(e, &fe) {
			got = append(got, fe.Field)
		}
	}
	if !slices.Equal(got, []string{"port", "host", "id"}) {
		t.Errorf("fields %q, from %v", got, err)
	}
	if first := Problems(a.AddWorld("", config.WorldSettings{Port: 1})); len(first) == 0 || first[0].Error() != str.ConfigWorldIdChars() {
		t.Errorf("an empty id and host: first problem %v; want the id's, as the TUI always showed", first)
	}
	if _, err := os.Stat(filepath.Join(dir, "worlds", "fm.toml")); err != nil {
		t.Fatal(err) // reloadApp's fm is still there
	}
}

func TestProblemsFlattens(t *testing.T) {
	a, b, c := errors.New("a"), errors.New("b"), errors.New("c")
	if got := Problems(errors.Join(errors.Join(a, b), nil, c)); !slices.Equal(got, []error{a, b, c}) {
		t.Errorf("got %v", got)
	}
	if got := Problems(a); !slices.Equal(got, []error{a}) {
		t.Errorf("one: %v", got)
	}
	if Problems(nil) != nil {
		t.Error("nil should have no problems")
	}
}

func TestCheckFieldName(t *testing.T) {
	if CheckField("name", "Kit") != nil {
		t.Error("Kit refused")
	}
	if err := CheckField("name", "Kit Two"); err == nil || err.Error() != str.ConfigNameHasSpaces() {
		t.Errorf("a space: %v", err)
	}
}
