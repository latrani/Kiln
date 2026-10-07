package catalog

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWebJSON(t *testing.T) {
	got, err := WebJSON([]byte(`
[web]
a = "hello {name}"
[other]
b = "x"
`))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]string
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatal(err)
	}
	if len(m) != 1 || m["a"] != "hello {name}" {
		t.Errorf("WebJSON = %v", m)
	}
	// Each must parse, so the error is WebJSON's own rejection.
	for bad, want := range map[string]string{
		"[web]\na = { one = \"1\", other = \"{n}\" }": "plural",
		"[web]\na = \"{x:%q}\"":                       "fmt verbs",
		"[web]\na = \"{{literal}}\"":                  "braces",
	} {
		if _, err := Parse([]byte(bad)); err != nil {
			t.Errorf("Parse(%q): %v", bad, err)
		}
		if _, err := WebJSON([]byte(bad)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("WebJSON(%q) = %v, want the %s error", bad, err, want)
		}
	}
}
