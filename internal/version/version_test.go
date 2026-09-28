package version

import "testing"

func TestDescribe(t *testing.T) {
	const rev = "4eec20402f054b68809133cde51387913cbd85e2"
	for _, c := range []struct {
		v, rev   string
		modified bool
		want     string
	}{
		{"v0.2.1", rev, false, "v0.2.1"},                                       // a release
		{"v0.2.1+dirty", rev, true, "v0.2.1*"},                                 // a release tag with local changes
		{"v0.2.2-0.20260928054407-4eec20402f05", rev, false, "v0.2.1+4eec204"}, // after v0.2.1
		{"v0.2.2-0.20260928054407-4eec20402f05+dirty", rev, true, "v0.2.1+4eec204*"},
		{"v0.0.0-20260928054407-4eec20402f05", rev, false, "dev+4eec204"}, // before any release
		{"(devel)", rev, false, "dev+4eec204"},                            // no module version
		{"(devel)", "", false, "dev"},                                     // no VCS info at all
		{"", "", false, "dev"},
	} {
		if got := describe(c.v, c.rev, c.modified); got != c.want {
			t.Errorf("describe(%q, %q, %v) = %q, want %q", c.v, c.rev, c.modified, got, c.want)
		}
	}
}

func TestStringIsSomething(t *testing.T) {
	if String() == "" {
		t.Error("String() is empty")
	}
}
