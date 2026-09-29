package str

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestNoStrayStrings keeps words people read out of the Go source: they
// belong in locales/en.toml. It flags every string literal with a run of two
// or more letters, outside tests and this package, unless it looks like
// vocabulary rather than prose (see exempt) or its line, or the var/const
// block around it, carries a //str:ok comment.
//
// What it can't see: a user-facing string that's a single lowercase word
// ("on", "none") looks just like a config value, so those pass. Put them in
// the catalog anyway.
func TestNoStrayStrings(t *testing.T) {
	root := filepath.Join("..", "..")
	var stray []string
	for _, top := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if path == filepath.Join(root, "internal", "str") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			stray = append(stray, strayIn(t, path)...)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(stray) > 0 {
		t.Errorf("%d user-facing string(s) in Go source. Move each to internal/str/locales/en.toml, "+
			"run go generate ./internal/str, and call the generated str function. If one really isn't "+
			"for people to read (a protocol token, a file format), end its line with //str:ok.\n%s",
			len(stray), strings.Join(stray, "\n"))
	}
}

var (
	wordy = regexp.MustCompile(`[A-Za-z]{2,}`)
	// Vocabulary, not prose: identifiers and config values, env vars and
	// protocol words, key names, slash commands, file names, colors.
	exempt = []*regexp.Regexp{
		regexp.MustCompile(`^[a-z][a-z0-9_]*$`),
		regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`),
		regexp.MustCompile(`^((ctrl|alt|shift|super)\+)*(up|down|left|right|enter|esc|tab|space|backspace|delete|home|end|pgup|pgdown|[a-z0-9])$`),
		regexp.MustCompile(`^/[a-z]+$`),
		regexp.MustCompile(`^[\w*-]*\.[a-z*-]+$`),
		regexp.MustCompile(`^#[0-9a-fA-F]{6}$`),
		regexp.MustCompile(`^\{[a-z_]+\}$`),
	}
)

func strayIn(t *testing.T, path string) []string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	if ast.IsGenerated(f) {
		return nil
	}
	okLines := map[int]bool{}
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if isOK(c) {
				okLines[fset.Position(c.Pos()).Line] = true
			}
		}
	}
	skip := map[ast.Node]bool{}
	for _, im := range f.Imports {
		skip[im.Path] = true
	}
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.GenDecl:
			if n.Doc != nil && slices.ContainsFunc(n.Doc.List, isOK) {
				return false
			}
		case *ast.Field:
			if n.Tag != nil {
				skip[n.Tag] = true
			}
		case *ast.BasicLit:
			if n.Kind != token.STRING || skip[n] {
				return true
			}
			start, end := fset.Position(n.Pos()), fset.Position(n.End())
			if okLines[start.Line] || okLines[end.Line] {
				return true
			}
			s, err := strconv.Unquote(n.Value)
			if err != nil || !wordy.MatchString(s) {
				return true
			}
			for _, re := range exempt {
				if re.MatchString(s) {
					return true
				}
			}
			rel, _ := filepath.Rel(filepath.Join("..", ".."), start.Filename)
			lit, _, more := strings.Cut(n.Value, "\n")
			if more {
				lit += "…"
			}
			out = append(out, rel+":"+strconv.Itoa(start.Line)+": "+lit)
		}
		return true
	})
	return out
}

// isOK reports whether c is a //str:ok directive, alone or after a comment.
func isOK(c *ast.Comment) bool { return strings.HasSuffix(c.Text, "str:ok") }
