package main

import (
	"strings"
	"testing"

	"github.com/latrani/Kiln/internal/str/catalog"
)

func TestReport(t *testing.T) {
	parse := func(s string) map[string]catalog.Entry {
		c, err := catalog.Parse([]byte(s))
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	old := parse("same = \"x\"\nreworded = \"a {n}\"\ngone = \"bye\"\n# a comment\n")
	cur := parse("same = \"x\"\nreworded = \"b {n}\"\nfresh = { one = \"1\", other = \"{n}|\" }\n")
	got := report(old, cur)
	for _, want := range []string{
		"**New (1)**", "| `fresh` | `` one: 1 · other: {n}\\| `` |",
		"**Changed (1)**", "| `reworded` | `` a {n} `` | `` b {n} `` |",
		"**Removed (1)**", "| `gone` | `` bye `` |",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "same") {
		t.Errorf("unchanged entry reported:\n%s", got)
	}
	if r := report(old, old); r != "" {
		t.Errorf("no changes should print nothing, got %q", r)
	}
}
