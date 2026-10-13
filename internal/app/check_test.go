package app

import "testing"

func TestCheckField(t *testing.T) {
	for _, c := range []struct {
		field, value string
		want         error // nil: fine
	}{
		{"id", "pf-muck_2", nil},
		{"id", "pf muck", AcceptWorldID("pf muck")},
		{"port", "5555", nil},
		{"port", "55a", AcceptPort("55a")},
		{"host", "muck.test", nil},
		{"host", "muck test", AcceptHost("muck test")},
		{"use", "fuzzball, pfmuck", nil},
		{"use", "fuzz;ball", AcceptPacks("fuzz;ball")},
		{"max_line_bytes", "12x", AcceptByteCount("12x")},
		{"aliases", "Kit Kitsune", nil},
		{"aliases", "Kit !Kit", AcceptAliases("Kit !Kit")},
		{"tls", "anything", nil},
	} {
		got := CheckField(c.field, c.value)
		if (got == nil) != (c.want == nil) || got != nil && got.Error() != c.want.Error() {
			t.Errorf("CheckField(%q, %q) = %v, want %v", c.field, c.value, got, c.want)
		}
	}
}
