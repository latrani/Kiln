package webapi

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/theme"
)

var updateFold = flag.Bool("update", false, "rewrite web/ui/src/testdata/fold.json")

// TestFoldFixture writes what the page's fold must give for each span,
// so the TypeScript fold is checked against the answers the Go fold (and
// so the TUI) gives.
func TestFoldFixture(t *testing.T) {
	hl, looks := foldTheme(t)
	type fcase struct {
		Name  string
		Spans []Span
		Tags  map[string]theme.TagLook
		Want  []ansi.SpanStyle
	}
	var cases []fcase
	for _, c := range spanCases() {
		sp := spansOf(c.text, hl, c.tags)
		var want []ansi.SpanStyle
		for _, s := range sp {
			want = append(want, fold(s, looks))
		}
		cases = append(cases, fcase{c.name, sp, looks, want})
	}
	got, err := json.MarshalIndent(cases, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := "../../web/ui/src/testdata/fold.json"
	if *updateFold {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if was, err := os.ReadFile(path); err != nil || !bytes.Equal(was, got) {
		t.Errorf("%s is stale: go test ./internal/webapi -run TestFoldFixture -update", path)
	}
}
