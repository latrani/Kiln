package config

import (
	"embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Dir is Kiln's config directory: $XDG_CONFIG_HOME/kiln, else ~/.config/kiln.
func Dir() (string, error) {
	return xdg("XDG_CONFIG_HOME", ".config")
}

// DataDir is Kiln's data directory: $XDG_DATA_HOME/kiln, else ~/.local/share/kiln.
func DataDir() (string, error) {
	return xdg("XDG_DATA_HOME", filepath.Join(".local", "share"))
}

func xdg(env, fallback string) (string, error) {
	if d := os.Getenv(env); d != "" {
		return filepath.Join(d, "kiln"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, fallback, "kiln"), nil
}

//go:embed defaults
var defaults embed.FS

// EnsureDefaults creates the config directory layout and writes the
// starter config.toml and packs/fuzzball.toml if they don't exist yet.
// Existing files are never overwritten.
func EnsureDefaults(dir string) error {
	for _, sub := range []string{"worlds", "packs"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o700); err != nil {
			return err
		}
	}
	return fs.WalkDir(defaults, "defaults", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel("defaults", path)
		dst := filepath.Join(dir, rel)
		if _, err := os.Stat(dst); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		b, err := defaults.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o600)
	})
}
