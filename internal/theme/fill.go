package theme

import (
	"strings"

	xansi "github.com/charmbracelet/x/ansi"

	sgr "github.com/latrani/Kiln/internal/style"
)

// Fill draws row as exactly w cells of area r: cut or padded, with r's
// colors under the padding and re-applied after every reset inside row,
// so the area has no holes. With r unstyled it's plain cut-and-pad.
func (t *Theme) Fill(r Role, row string, w int) string {
	if w <= 0 {
		return ""
	}
	row = xansi.Truncate(row, w, "")
	pad := strings.Repeat(" ", max(0, w-xansi.StringWidth(row)))
	base := t.SGR(r)
	if base == "" {
		return row + pad
	}
	return base + sgr.Reassert(row, base) + pad + Reset
}

// Fill is Active().Fill.
func Fill(r Role, row string, w int) string { return Active().Fill(r, row, w) }
