package theme

import (
	"bytes"
	"os"
	"testing"
)

// The web's terminal colors are today's (Catppuccin's terminal ports):
// accents resolved, the brights repeating the normals.
func TestTerminal(t *testing.T) {
	for _, c := range []struct {
		ap   Appearance
		want TerminalColors
	}{
		{Dark, TerminalColors{FG: "#cad3f5", BG: "#24273a", ANSI: [16]string{"#494d64", "#ed8796", "#a6da95", "#eed49f", "#8aadf4", "#f5bde6", "#8bd5ca", "#b8c0e0",
			"#5b6078", "#ed8796", "#a6da95", "#eed49f", "#8aadf4", "#f5bde6", "#8bd5ca", "#a5adcb"}}},
		{Light, TerminalColors{FG: "#4c4f69", BG: "#eff1f5", ANSI: [16]string{"#bcc0cc", "#d20f39", "#40a02b", "#df8e1d", "#1e66f5", "#ea76cb", "#179299", "#5c5f77",
			"#acb0be", "#d20f39", "#40a02b", "#df8e1d", "#1e66f5", "#ea76cb", "#179299", "#6c6f85"}}},
	} {
		if got := Terminal(c.ap); got != c.want {
			t.Errorf("%v: %+v\nwant %+v", c.ap, got, c.want)
		}
	}
}

func TestTerminalGeneratedIsFresh(t *testing.T) {
	got, err := os.ReadFile("../../web/ui/src/terminal.gen.ts")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, TerminalTS()) {
		t.Error("web/ui/src/terminal.gen.ts is stale: run go generate ./internal/theme")
	}
}
