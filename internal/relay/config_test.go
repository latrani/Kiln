package relay

import (
	"net/netip"
	"slices"
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
	a, err := ParseConfig("relay.toml", []byte(sample))
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
	a, _ := ParseConfig("relay.toml", []byte(sample))
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
			str.RelayConfigUnknownKey("relay.toml", "world.plaintxt")},
		{"singular port", "[[world]]\nhost = \"a.example\"\nport = 1\n",
			str.RelayConfigUnknownKey("relay.toml", "world.port")},
		{"no host", "[[world]]\nports = [1]\n", str.RelayConfigNoHost("relay.toml", 1)},
		{"no ports", "[[world]]\nhost = \"a.example\"\n", str.RelayConfigNoPorts("relay.toml", "a.example")},
		{"bad port", "[[world]]\nhost = \"a.example\"\nports = [70000]\n", str.RelayConfigBadPort("relay.toml", "a.example", 70000)},
		{"bad proxy", "trusted_proxies = [\"nope\"]\n", str.RelayConfigBadProxy("relay.toml", "nope")},
		{"origin URL", "origins = [\"https://a.example\"]\n", str.RelayConfigBadOrigin("relay.toml", "https://a.example")},
		{"origin path", "origins = [\"a.example/kiln\"]\n", str.RelayConfigBadOrigin("relay.toml", "a.example/kiln")},
		{"origin glob", "origins = [\"[a.example\"]\n", str.RelayConfigBadOrigin("relay.toml", "[a.example")},
		{"origin empty", "origins = [\"\"]\n", str.RelayConfigBadOrigin("relay.toml", "")},
		{"duplicate", "[[world]]\nhost = \"a.example\"\nports = [1, 2]\n[[world]]\nhost = \"A.example.\"\nports = [2]\nplaintext = true\n",
			str.RelayConfigDuplicate("relay.toml", "a.example:2")},
		{"zero per ip", "max_per_ip = 0\n", str.RelayConfigBadLimit("relay.toml", "max_per_ip")},
		{"negative conns", "max_conns = -1\n", str.RelayConfigBadLimit("relay.toml", "max_conns")},
	} {
		_, err := ParseConfig("relay.toml", []byte(c.toml))
		if err == nil || err.Error() != c.want {
			t.Errorf("%s: err = %v, want %q", c.name, err, c.want)
		}
	}
}

func TestOriginsNormalized(t *testing.T) {
	a, err := ParseConfig("relay.toml", []byte(`origins = ["Muck.Example.org.", "*.example.net", "localhost:8080"]`))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"muck.example.org", "*.example.net", "localhost:8080"}; !slices.Equal(a.Origins, want) {
		t.Errorf("Origins = %q, want %q", a.Origins, want)
	}
}

func TestParseBadTOML(t *testing.T) {
	_, err := ParseConfig("relay.toml", []byte("[[world]\n"))
	if err == nil || !strings.HasPrefix(err.Error(), upTo(str.RelayConfigReading("relay.toml", errMark{}))) {
		t.Errorf("err = %v", err)
	}
}
