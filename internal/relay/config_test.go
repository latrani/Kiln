package relay

import (
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"

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
tls_trust = "ca"
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

// testPin is a well-formed pin that matches no certificate.
var testPin = "sha256:" + strings.Repeat("0", 64)

func TestParseTrust(t *testing.T) {
	a, err := ParseConfig("relay.toml", []byte(sample+"\n[[world]]\nhost = \"pinned.example\"\nports = [1]\npin = \""+testPin+"\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if w, _ := a.Lookup("other.example", 7000); w.TLSTrust != "ca" || w.Pin != "" {
		t.Errorf("ca world = %+v", w)
	}
	if w, _ := a.Lookup("pinned.example", 1); w.Pin != testPin || w.TLSTrust != "" {
		t.Errorf("pinned world = %+v", w)
	}
	if w, _ := a.Lookup("muck.example.org", 8888); !w.Plaintext || w.Pin != "" || w.TLSTrust != "" {
		t.Errorf("plaintext world = %+v", w)
	}
}

func TestParseHoldAndBuffers(t *testing.T) {
	a, err := ParseConfig("relay.toml", []byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if a.HoldFor() != 24*time.Hour || a.BufferCap() != 4<<20 || a.TotalBufferCap() != 256<<20 {
		t.Errorf("defaults = %v, %d, %d; want 24h, 4 MiB, 256 MiB", a.HoldFor(), a.BufferCap(), a.TotalBufferCap())
	}
	a, err = ParseConfig("relay.toml", []byte("hold = \"90m\"\nmax_buffer_mb = 1\nmax_total_buffer_mb = 8\n"+sample))
	if err != nil {
		t.Fatal(err)
	}
	if a.HoldFor() != 90*time.Minute || a.BufferCap() != 1<<20 || a.TotalBufferCap() != 8<<20 {
		t.Errorf("set = %v, %d, %d; want 90m, 1 MiB, 8 MiB", a.HoldFor(), a.BufferCap(), a.TotalBufferCap())
	}
}

func TestParseShortHold(t *testing.T) {
	a, _ := ParseConfig("relay.toml", []byte(sample))
	if a.ShortHoldFor() != 10*time.Minute {
		t.Errorf("default short hold %v, want 10m", a.ShortHoldFor())
	}
	a, err := ParseConfig("relay.toml", []byte("short_hold = \"5m\"\n"+sample))
	if err != nil || a.ShortHoldFor() != 5*time.Minute {
		t.Errorf("short hold %v, %v; want 5m", a.ShortHoldFor(), err)
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
		{"duplicate", "[[world]]\nhost = \"a.example\"\nports = [1, 2]\ntls_trust = \"ca\"\n[[world]]\nhost = \"A.example.\"\nports = [2]\nplaintext = true\n",
			str.RelayConfigDuplicate("relay.toml", "a.example:2")},
		{"no trust", "[[world]]\nhost = \"a.example\"\nports = [1]\n", str.RelayConfigNoTrust("relay.toml", "a.example")},
		{"both trust", "[[world]]\nhost = \"a.example\"\nports = [1]\ntls_trust = \"ca\"\npin = \"" + testPin + "\"\n",
			str.RelayConfigBothTrust("relay.toml", "a.example")},
		{"trust pin", "[[world]]\nhost = \"a.example\"\nports = [1]\ntls_trust = \"pin\"\n",
			str.RelayConfigBadTrust("relay.toml", "a.example", "pin")},
		{"bad pin", "[[world]]\nhost = \"a.example\"\nports = [1]\npin = \"sha256:abc\"\n",
			str.RelayConfigBadPin("relay.toml", "a.example", "sha256:abc")},
		{"plaintext pin", "[[world]]\nhost = \"a.example\"\nports = [1]\nplaintext = true\npin = \"" + testPin + "\"\n",
			str.RelayConfigPlaintextTrust("relay.toml", "a.example")},
		{"plaintext ca", "[[world]]\nhost = \"a.example\"\nports = [1]\nplaintext = true\ntls_trust = \"ca\"\n",
			str.RelayConfigPlaintextTrust("relay.toml", "a.example")},
		{"zero per ip", "max_per_ip = 0\n", str.RelayConfigBadLimit("relay.toml", "max_per_ip")},
		{"negative conns", "max_conns = -1\n", str.RelayConfigBadLimit("relay.toml", "max_conns")},
		{"hold word", "hold = \"soon\"\n", str.RelayConfigBadHold("relay.toml", "soon")},
		{"hold negative", "hold = \"-1h\"\n", str.RelayConfigBadHold("relay.toml", "-1h")},
		{"hold zero", "hold = \"0s\"\n", str.RelayConfigBadHold("relay.toml", "0s")},
		{"short hold word", "short_hold = \"soon\"\n", str.RelayConfigBadShortHold("relay.toml", "soon")},
		{"negative buffer", "max_buffer_mb = -1\n", str.RelayConfigBadLimit("relay.toml", "max_buffer_mb")},
		{"zero total buffer", "max_total_buffer_mb = 0\n", str.RelayConfigBadLimit("relay.toml", "max_total_buffer_mb")},
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
