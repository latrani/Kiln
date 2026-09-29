// Command diff compares two versions of a string catalog and prints the
// entries added, changed and removed, as Markdown for a PR comment. It
// prints nothing when the entries are the same (a comment-only edit).
//
//	go run ./internal/str/diff old.toml new.toml
//
// A missing old file counts as an empty catalog.
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/latrani/Kiln/internal/str/catalog"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: diff old.toml new.toml")
		os.Exit(2)
	}
	old, err := load(os.Args[1])
	if err == nil {
		var cur map[string]catalog.Entry
		if cur, err = load(os.Args[2]); err == nil {
			fmt.Print(report(old, cur))
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "diff:", err)
		os.Exit(1)
	}
}

func load(path string) (map[string]catalog.Entry, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]catalog.Entry{}, nil
	}
	if err != nil {
		return nil, err
	}
	c, err := catalog.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

func report(old, cur map[string]catalog.Entry) string {
	var added, changed, removed []string
	for _, k := range slices.Sorted(maps.Keys(cur)) {
		o, ok := old[k]
		switch {
		case !ok:
			added = append(added, fmt.Sprintf("| `%s` | %s |", k, cell(cur[k].String())))
		case o.String() != cur[k].String():
			changed = append(changed, fmt.Sprintf("| `%s` | %s | %s |", k, cell(o.String()), cell(cur[k].String())))
		}
	}
	for _, k := range slices.Sorted(maps.Keys(old)) {
		if _, ok := cur[k]; !ok {
			removed = append(removed, fmt.Sprintf("| `%s` | %s |", k, cell(old[k].String())))
		}
	}
	var b strings.Builder
	section := func(title, head string, rows []string) {
		if len(rows) == 0 {
			return
		}
		fmt.Fprintf(&b, "\n**%s (%d)**\n\n%s\n%s\n", title, len(rows), head, strings.Join(rows, "\n"))
	}
	section("New", "| key | text |\n|---|---|", added)
	section("Changed", "| key | was | now |\n|---|---|---|", changed)
	section("Removed", "| key | text |\n|---|---|", removed)
	return b.String()
}

// cell formats text for a Markdown table cell.
func cell(s string) string {
	s = strings.NewReplacer("|", `\|`, "\n", "<br>").Replace(s)
	return "`` " + s + " ``"
}
