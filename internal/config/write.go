package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// HighlightStyle is the style /highlight gives new rules.
var HighlightStyle = Style{FG: "#ffd166", Bold: true}

// AppendHighlight adds a literal-text, case-insensitive highlight rule to
// the end of worlds/<world>.toml. Appending is safe because TOML table
// headers are absolute: "[[highlight]]" always means the world's top-level
// list, even after a [characters.x] table. The rule uses inline tables so
// nothing after the header can be mistaken for a sub-table. Existing
// content and comments are kept.
func AppendHighlight(dir, world, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("nothing to highlight")
	}
	if !idRE.MatchString(world) {
		return fmt.Errorf("bad world id %q", world)
	}
	pattern, err := tomlString("(?i)" + regexp.QuoteMeta(text))
	if err != nil {
		return err
	}
	rule := fmt.Sprintf("\n# added by /highlight\n[[highlight]]\nmatch = { pattern = %s }\nstyle = { fg = %q, bold = %t }\n",
		pattern, HighlightStyle.FG, HighlightStyle.Bold)
	path := filepath.Join(dir, "worlds", world+".toml")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(rule)
	return err
}

// tomlString quotes s as a TOML basic string. JSON string escapes
// (\" \\ \n \uXXXX) are all valid TOML escapes. JSON leaves DEL (0x7f)
// raw, but TOML forbids it in a basic string, so it is escaped here.
func tomlString(s string) (string, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return "", err
	}
	out := strings.TrimSuffix(b.String(), "\n")
	return strings.ReplaceAll(out, "\x7f", `\u007f`), nil
}
