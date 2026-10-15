// Command gen writes web/ui/src/terminal.gen.ts from catppuccin.toml's
// [terminal]. Run it with go generate ./internal/theme.
package main

import (
	"fmt"
	"os"

	"github.com/latrani/Kiln/internal/theme"
)

func main() {
	if err := os.WriteFile("../../web/ui/src/terminal.gen.ts", theme.TerminalTS(), 0o644); err != nil { //str:ok: a path
		fmt.Fprintln(os.Stderr, "gen:", err) //str:ok: a developer tool
		os.Exit(1)
	}
}
