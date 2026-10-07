package catalog

import (
	"encoding/json"
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
	for _, bad := range []string{
		"[web]\na = { one = \"1\", other = \"{n}\" }",
		"[web]\na = \"{x:%q}\"",
		"[web]\na = \"{{literal}}\"",
	} {
		if _, err := WebJSON([]byte(bad)); err == nil {
			t.Errorf("WebJSON(%q): no error", bad)
		}
	}
}
