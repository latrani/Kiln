package rules

import (
	"reflect"
	"testing"

	"github.com/latrani/Kiln/internal/classify"
	"github.com/latrani/Kiln/internal/style"
	"github.com/latrani/Kiln/internal/theme"
)

func mustTheme(t *testing.T, tags string) *theme.Theme {
	t.Helper()
	th, err := theme.FromTOML("[tags]\n" + tags)
	if err != nil {
		t.Fatal(err)
	}
	return th
}

// tag is a tag with spans.
func tag(name string, spans ...classify.Span) classify.Tag {
	return classify.Tag{Name: name, Spans: spans}
}

func sgr(t *testing.T, th *theme.Theme, names ...string) string {
	t.Helper()
	var s theme.Style
	for _, n := range names {
		_, ts, ok := th.Tag(n)
		if !ok {
			t.Fatalf("no style for %s", n)
		}
		s = s.Over(ts.Style)
	}
	return s.SGR()
}

func TestWholeLineTags(t *testing.T) {
	th := mustTheme(t, `"page/in" = { fg = "#ff9f43", bold = true }
self = { italic = true }`)
	h := New(th)
	plain := "Mira pages: hi Kit"
	if got := h.Runs("Rook waves.", nil); got != nil {
		t.Errorf("untagged line styled: %+v", got)
	}
	got := h.Runs(plain, []classify.Tag{tag("page"), tag("page/in"), tag("self", classify.Span{Start: 15, End: 18})})
	want := []style.Run{{Start: 0, End: len(plain), SGR: sgr(t, th, "page/in", "self")}}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("Runs = %+v, want %+v", got, want)
	}
}

func TestMatchScopeOnTopOfLine(t *testing.T) {
	th := mustTheme(t, `highlight = { fg = "#ffd166", scope = "match" }
page = { italic = true }`)
	h := New(th)
	// The match-scope tag comes first on the line but still draws on top.
	got := h.Runs("Mira pages: the lighthouse", []classify.Tag{tag("highlight", classify.Span{Start: 16, End: 26}), tag("page", classify.Span{Start: 0, End: 11})})
	want := []style.Run{
		{Start: 0, End: 16, SGR: sgr(t, th, "page")},
		{Start: 16, End: 26, SGR: sgr(t, th, "page", "highlight")},
	}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("Runs = %+v, want %+v", got, want)
	}
}

func TestMatchScopeWithoutSpans(t *testing.T) {
	th := mustTheme(t, `highlight = { fg = "#ffd166", scope = "match" }`)
	if got := New(th).Runs("x", []classify.Tag{tag("highlight")}); got != nil {
		t.Errorf("a match-scope tag with no spans styled the line: %+v", got)
	}
}

func TestListsMatchTagAndChildren(t *testing.T) {
	j := Judge{Priority: []string{"page"}}
	for _, c := range []struct {
		tag  string
		want bool
	}{{"page", true}, {"page/in", true}, {"page/in/x", true}, {"pages", false}, {"whisper", false}} {
		if got := j.Of([]classify.Tag{tag(c.tag)}).Priority; got != c.want {
			t.Errorf("priority for %q = %v, want %v", c.tag, got, c.want)
		}
	}
}

func TestQuietWinsOverPriority(t *testing.T) {
	j := Judge{Priority: []string{"self", "spam"}, Quiet: []string{"spam"}}
	for _, tags := range [][]classify.Tag{{tag("spam")}, {tag("self"), tag("spam")}} {
		v := j.Of(tags)
		if !v.Quiet || v.Priority {
			t.Errorf("%v: quiet %v priority %v, want quiet only", tags, v.Quiet, v.Priority)
		}
	}
}

// The verdict doesn't depend on the theme: a tag with no style still
// is a priority.
func TestJudgeIgnoresStyles(t *testing.T) {
	if !(Judge{Priority: []string{"page"}}).Of([]classify.Tag{tag("page")}).Priority {
		t.Error("an unstyled tag in the priority list wasn't a priority")
	}
}

func TestTagRunsNameStylesInFoldOrder(t *testing.T) {
	th := mustTheme(t, `"page/in" = { fg = "#ff9f43" }
self = { scope = "match", bold = true }
url = { scope = "match", underline = true }
quiet = { faint = true }`)
	h := New(th)
	plain := "Mira pages: Kit, see https://x.test"
	got := h.TagRuns(plain, []classify.Tag{
		tag("self", classify.Span{Start: 12, End: 15}),
		tag("page/in/mira"), // falls back to page/in
		tag("url", classify.Span{Start: 21, End: 35}),
		tag("quiet"),
	})
	want := []TagRun{
		{0, 12, []string{"page/in", "quiet"}},
		{12, 15, []string{"page/in", "quiet", "self"}},
		{15, 21, []string{"page/in", "quiet"}},
		{21, 35, []string{"page/in", "quiet", "url"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("TagRuns = %+v\nwant %+v", got, want)
	}
	if h.TagRuns(plain, []classify.Tag{tag("unstyled")}) != nil {
		t.Error("a line with no styled tag should have no runs")
	}
}
