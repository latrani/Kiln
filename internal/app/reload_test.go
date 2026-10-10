package app

import (
	"testing"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/str"
)

func reloadApp(t *testing.T) (*App, string) {
	t.Helper()
	dir := configDir(t, map[string]string{"fm": fmWorld})
	a := New(Deps{ConfigDir: dir, Load: config.Load})
	a.ApplyConfig(load(t, dir))
	return a, dir
}

func TestReloadAppliesTheConfigAgain(t *testing.T) {
	a, dir := reloadApp(t)
	writeWorlds(t, dir, map[string]string{"zz": zzWorld})
	if _, ok := a.Reload(); !ok {
		t.Fatalf("Reload failed: %q", a.Status().Text)
	}
	if _, ok := a.Find("zz/ash"); !ok {
		t.Error("the new world's character isn't configured")
	}
}

func TestReloadKeepsTheConfigOnAnError(t *testing.T) {
	a, dir := reloadApp(t)
	openAll(t, a, "fm/kit")
	cfg := a.Config()
	writeWorlds(t, dir, map[string]string{"fm": "host = "})
	_, err := config.Load(dir)
	if _, ok := a.Reload(); ok {
		t.Fatal("Reload of a broken config succeeded")
	}
	if a.Config() != cfg || a.Char("fm/kit") == nil || a.Active() != "fm/kit" {
		t.Error("a broken config changed what's open")
	}
	if s := a.Status(); !s.Err || s.Text != str.StatusConfigNotReloaded(err) {
		t.Errorf("status = %+v", s)
	}
}
