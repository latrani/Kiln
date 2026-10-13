package glyph

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"testing"
	"unicode/utf8"

	xansi "github.com/charmbracelet/x/ansi"
)

// TestEachIsOneCell: a glyph is one character, one cell wide.
func TestEachIsOneCell(t *testing.T) {
	for _, g := range All {
		if utf8.RuneCountInString(g[1]) != 1 || xansi.StringWidth(g[1]) != 1 {
			t.Errorf("%s = %q isn't one character in one cell", g[0], g[1])
		}
	}
}

// TestAllNamesEveryGlyph: the page gets its glyphs from All, so a new
// constant left out of it would be missing there.
func TestAllNamesEveryGlyph(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "glyph.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var consts []string
	for _, d := range f.Decls {
		if gd, ok := d.(*ast.GenDecl); ok && gd.Tok == token.CONST {
			for _, sp := range gd.Specs {
				for _, n := range sp.(*ast.ValueSpec).Names {
					consts = append(consts, n.Name)
				}
			}
		}
	}
	var named []string
	for _, g := range All {
		named = append(named, g[0])
	}
	if !slices.Equal(consts, named) {
		t.Errorf("constants %v\nAll       %v", consts, named)
	}
}
