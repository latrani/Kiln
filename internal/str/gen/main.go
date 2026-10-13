// Command gen writes keys_gen.go from locales/en.toml: one function per
// catalog entry; and web/ui/src/str.gen.ts, the same for the page, with
// the glyphs. Run it with go generate ./internal/str.
package main

import (
	"fmt"
	"os"

	"github.com/latrani/Kiln/internal/glyph"
	"github.com/latrani/Kiln/internal/str/catalog"
)

func main() {
	data, err := os.ReadFile("locales/en.toml")
	if err == nil {
		prev, _ := os.ReadFile("keys_gen.go") // keeps signatures stable; absent the first time
		var src []byte
		if src, err = catalog.Generate(data, prev); err == nil {
			err = os.WriteFile("keys_gen.go", src, 0o644)
		}
		if err == nil {
			var ts []byte
			if ts, err = catalog.TS(data, src, glyph.All); err == nil {
				err = os.WriteFile("../../web/ui/src/str.gen.ts", ts, 0o644)
			}
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}
