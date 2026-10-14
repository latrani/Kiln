package kilntest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/latrani/Kiln/internal/config"
)

func TestGoldenWorldLoads(t *testing.T) {
	dir := t.TempDir()
	if err := config.EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "worlds", "fm.toml"), []byte(GoldenWorld), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, w := range cfg.Worlds {
		for _, c := range w.Characters {
			got = append(got, c.ID)
		}
	}
	if len(got) != 2 || got[0] != "kit" || got[1] != "rook" {
		t.Errorf("characters = %v", got)
	}
}
