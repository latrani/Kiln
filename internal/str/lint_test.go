package str

import (
	"fmt"
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

// isOK reports whether c carries //str:ok, alone or with a reason.
func isOK(c *ast.Comment) bool { return strings.Contains(c.Text, "str:ok") }

// TestTestsReadTheCatalog keeps tests from copying catalog text: a test
// that expects "copied to clipboard" breaks the day that's reworded. It
// flags string literals in tests that contain a phrase from en.toml (the
// literal text between placeholders, when it's two or more words). Build
// the expected text with the str function instead, or mark the line
// //str:ok (a fixture that has to be literal). Failure messages
// (t.Errorf("…")) don't count.
func TestTestsReadTheCatalog(t *testing.T) {
	var phrases []string
	for _, e := range fallback {
		for _, tmpl := range e.Forms {
			for _, s := range tmpl {
				if p := strings.TrimSpace(s.Text); s.Name == "" && len(p) >= 8 && strings.Contains(p, " ") {
					phrases = append(phrases, p)
				}
			}
		}
	}
	root := filepath.Join("..", "..")
	var copied []string
	for _, top := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && path == filepath.Join(root, "internal", "str") {
				return filepath.SkipDir
			}
			if strings.HasSuffix(path, "_test.go") {
				copied = append(copied, copiesIn(t, path, phrases)...)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(copied) > 0 {
		t.Errorf("%d test string(s) copy catalog text, so rewording it breaks them. Build the "+
			"expected text with the str function (str.StatusCopied(), str.Separator(), …) instead.\n%s",
			len(copied), strings.Join(copied, "\n"))
	}
}

// failureFuncs are testing methods whose string arguments are messages
// about the test, not expectations.
var failureFuncs = map[string]bool{"Error": true, "Errorf": true, "Fatal": true, "Fatalf": true,
	"Log": true, "Logf": true, "Skip": true, "Skipf": true}

func copiesIn(t *testing.T, path string, phrases []string) []string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
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
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && failureFuncs[sel.Sel.Name] {
				for _, a := range call.Args {
					skip[a] = true
				}
			}
		}
		bl, ok := n.(*ast.BasicLit)
		if !ok || bl.Kind != token.STRING || skip[bl] {
			return true
		}
		start, end := fset.Position(bl.Pos()), fset.Position(bl.End())
		if okLines[start.Line] || okLines[end.Line] {
			return true
		}
		s, err := strconv.Unquote(bl.Value)
		if err != nil {
			return true
		}
		for _, p := range phrases {
			if strings.Contains(s, p) {
				rel, _ := filepath.Rel(filepath.Join("..", ".."), start.Filename)
				lit, _, more := strings.Cut(bl.Value, "\n")
				if more {
					lit += "…"
				}
				out = append(out, fmt.Sprintf("%s:%d: %s (%q)", rel, start.Line, lit, p))
				break
			}
		}
		return true
	})
	return out
}
