// Command gen writes keys_gen.go from locales/en.toml: one function per
// catalog entry, and web/static/strings.json, the [web] table for the
// page. Run it with go generate ./internal/str.
package main

import (
	"fmt"
	"os"

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
			var js []byte
			if js, err = catalog.WebJSON(data); err == nil {
				err = os.WriteFile("../../web/static/strings.json", js, 0o644)
			}
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}
