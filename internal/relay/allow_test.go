package relay

import (
	"net/netip"
	"strings"
	"testing"

	"github.com/latrani/Kiln/internal/str"
)

const sample = `
origins = ["muck.example.org"]
trusted_proxies = ["127.0.0.1", "10.0.0.0/8"]

[[world]]
host = "muck.example.org"
ports = [8888, 8899]
plaintext = true

[[world]]
host = "Other.Example."
ports = [7000]
`

func TestParseAndLookup(t *testing.T) {
	a, err := ParseAllowlist("allow.toml", []byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	w, ok := a.Lookup("MUCK.example.org.", 8899)
	if !ok || !w.Plaintext || w.Host != "muck.example.org" {
		t.Errorf("Lookup = %+v, %v", w, ok)
	}
	if w, ok := a.Lookup("other.example", 7000); !ok || w.Plaintext || w.Host != "other.example" {
		t.Errorf("other = %+v, %v; want TLS-only other.example", w, ok)
	}
	if _, ok := a.Lookup("muck.example.org", 7000); ok {
		t.Error("an unlisted port matched")
	}
	if _, ok := a.Lookup("evil.example", 8888); ok {
		t.Error("an unlisted host matched")
	}
}

func TestTrusted(t *testing.T) {
	a, _ := ParseAllowlist("allow.toml", []byte(sample))
	for ip, want := range map[string]bool{
		"127.0.0.1": true, "::ffff:127.0.0.1": true, "10.2.3.4": true, "192.168.1.1": false,
	} {
		if got := a.Trusted(netip.MustParseAddr(ip)); got != want {
			t.Errorf("Trusted(%s) = %v, want %v", ip, got, want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for _, c := range []struct{ name, toml, want string }{
		{"typo", "[[world]]\nhost = \"a.example\"\nports = [1]\nplaintxt = true\n",
			str.RelayAllowUnknownKey("allow.toml", "world.plaintxt")},
		{"singular port", "[[world]]\nhost = \"a.example\"\nport = 1\n",
			str.RelayAllowUnknownKey("allow.toml", "world.port")},
		{"no host", "[[world]]\nports = [1]\n", str.RelayAllowNoHost("allow.toml", 1)},
		{"no ports", "[[world]]\nhost = \"a.example\"\n", str.RelayAllowNoPorts("allow.toml", "a.example")},
		{"bad port", "[[world]]\nhost = \"a.example\"\nports = [70000]\n", str.RelayAllowBadPort("allow.toml", "a.example", 70000)},
		{"bad proxy", "trusted_proxies = [\"nope\"]\n", str.RelayAllowBadProxy("allow.toml", "nope")},
	} {
		_, err := ParseAllowlist("allow.toml", []byte(c.toml))
		if err == nil || err.Error() != c.want {
			t.Errorf("%s: err = %v, want %q", c.name, err, c.want)
		}
	}
}

func TestParseBadTOML(t *testing.T) {
	_, err := ParseAllowlist("allow.toml", []byte("[[world]\n"))
	if err == nil || !strings.HasPrefix(err.Error(), upTo(str.RelayAllowReading("allow.toml", errMark{}))) {
		t.Errorf("err = %v", err)
	}
}
