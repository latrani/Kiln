package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var sgrRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// TestNoHardCodedStyles keeps escape codes out of the UI's drawing: every
// color and attribute comes from a theme role (internal/theme). It flags
// string literals holding an SGR sequence in this package and cmd/kiln.
func TestNoHardCodedStyles(t *testing.T) {
	var files []string
	for _, glob := range []string{"*.go", filepath.Join("..", "..", "cmd", "kiln", "*.go")} {
		m, _ := filepath.Glob(glob)
		files = append(files, m...)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if bl, ok := n.(*ast.BasicLit); ok && bl.Kind == token.STRING {
				if s, err := strconv.Unquote(bl.Value); err == nil && sgrRE.MatchString(s) {
					t.Errorf("%s: %s: draw it with theme.Paint and a role instead", path, bl.Value)
				}
			}
			return true
		})
	}
}
