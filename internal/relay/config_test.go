package relay

import (
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/str"
)

const sample = `
origins = ["muck.example.org"]
trusted_proxies = ["127.0.0.1", "10.0.0.0/8"]

[[world]]
id = "fm"
host = "muck.example.org"
port = 8899
login = "connect {name} {password}"

[[world.characters]]
id = "kit"
name = "Kit"

[[world]]
id = "fm-tls"
host = "muck.example.org"
port = 8888
tls = true
tls_trust = "ca"

[[world]]
id = "other"
host = "Other.Example."
port = 7000
tls = true
tls_trust = "ca"
`

func TestParseAndLookup(t *testing.T) {
	a, err := ParseConfig("relay.toml", []byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	w, ok := a.Lookup("MUCK.example.org.", 8899)
	if !ok || w.TLS || w.Host != "muck.example.org" || w.ID != "fm" {
		t.Errorf("Lookup = %+v, %v", w, ok)
	}
	if w, ok := a.Lookup("other.example", 7000); !ok || !w.TLS || w.Host != "other.example" {
		t.Errorf("other = %+v, %v; want TLS other.example", w, ok)
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
	a, err := ParseConfig("relay.toml", []byte(sample+"\n[[world]]\nid = \"pinned\"\nhost = \"pinned.example\"\nport = 1\ntls = true\npin = \""+testPin+"\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if w, _ := a.Lookup("other.example", 7000); w.TLSTrust != "ca" || w.Pin != "" {
		t.Errorf("ca world = %+v", w)
	}
	if w, _ := a.Lookup("pinned.example", 1); w.Pin != testPin || w.TLSTrust != "" {
		t.Errorf("pinned world = %+v", w)
	}
	if w, _ := a.Lookup("muck.example.org", 8899); w.TLS || w.Pin != "" || w.TLSTrust != "" {
		t.Errorf("plaintext world = %+v", w)
	}
}

// A world's file is its [[world]] table as a Kiln world file: what the
// page is served, without the relay's own keys.
func TestWorldFile(t *testing.T) {
	a, err := ParseConfig("relay.toml", []byte(sample+"\n[[world]]\nid = \"pinned\"\nhost = \"pinned.example\"\nport = 1\ntls = true\npin = \""+testPin+"\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range a.Worlds {
		f := string(w.File())
		if err := config.CheckWorldFile(w.ID, w.ID, w.File()); err != nil {
			t.Errorf("%s's file isn't a world file: %v\n%s", w.ID, err, f)
		}
		if strings.Contains(f, "pin =") || strings.Contains(f, "id = \""+w.ID+"\"") {
			t.Errorf("%s's file has the relay's keys:\n%s", w.ID, f)
		}
	}
	f := string(a.Worlds[0].File())
	for _, k := range []string{"host", "port", "login", "characters", "Kit"} {
		if !strings.Contains(f, k) {
			t.Errorf("fm's file lacks %s:\n%s", k, f)
		}
	}
	if f := string(a.Worlds[1].File()); !strings.Contains(f, "tls = true") || !strings.Contains(f, "tls_trust") {
		t.Errorf("fm-tls's file lacks its TLS:\n%s", f)
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
	// w is a [[world]] fm on a.example:1 with more keys.
	w := func(more string) string { return "[[world]]\nid = \"fm\"\nhost = \"a.example\"\nport = 1\n" + more }
	// world is how a message about world id starts.
	world := func(id string) string { return str.RelayConfigWorld("relay.toml", id) }
	for _, c := range []struct{ name, toml, want string }{
		{"typo", w("tsl = true\n"), str.ConfigUnknownKey(world("fm"), "tsl")},
		{"old ports", "[[world]]\nid = \"fm\"\nhost = \"a.example\"\nports = [1]\n", str.ConfigUnknownKey(world("fm"), "ports")},
		{"no id", "[[world]]\nhost = \"a.example\"\nport = 1\n", str.RelayConfigNoId("relay.toml", 1)},
		{"bad id", "[[world]]\nid = \"f m\"\nhost = \"a.example\"\nport = 1\n", str.ConfigBadWorldId(world("f m"), "f m")},
		{"no host", "[[world]]\nid = \"fm\"\nport = 1\n", str.ConfigHostRequired(world("fm"))},
		{"no port", "[[world]]\nid = \"fm\"\nhost = \"a.example\"\n", str.ConfigBadPort(world("fm"))},
		{"bad port", "[[world]]\nid = \"fm\"\nhost = \"a.example\"\nport = 70000\n", str.ConfigBadPort(world("fm"))},
		{"bad proxy", "trusted_proxies = [\"nope\"]\n", str.RelayConfigBadProxy("relay.toml", "nope")},
		{"origin URL", "origins = [\"https://a.example\"]\n", str.RelayConfigBadOrigin("relay.toml", "https://a.example")},
		{"origin path", "origins = [\"a.example/kiln\"]\n", str.RelayConfigBadOrigin("relay.toml", "a.example/kiln")},
		{"origin glob", "origins = [\"[a.example\"]\n", str.RelayConfigBadOrigin("relay.toml", "[a.example")},
		{"origin empty", "origins = [\"\"]\n", str.RelayConfigBadOrigin("relay.toml", "")},
		{"duplicate", w("tls = true\ntls_trust = \"ca\"\n") + "[[world]]\nid = \"bay\"\nhost = \"A.example.\"\nport = 1\n",
			str.RelayConfigDuplicate("relay.toml", "a.example:1")},
		{"duplicate id", w("") + "[[world]]\nid = \"FM\"\nhost = \"b.example\"\nport = 1\n", str.RelayConfigDuplicateId("relay.toml", "FM")},
		{"no trust", w("tls = true\n"), str.RelayConfigNoTrust("relay.toml", "fm")},
		{"both trust", w("tls = true\ntls_trust = \"ca\"\npin = \""+testPin+"\"\n"), str.RelayConfigBothTrust("relay.toml", "fm")},
		{"trust pin", w("tls = true\ntls_trust = \"pin\"\n"), str.RelayConfigBadTrust("relay.toml", "pin", "fm")},
		{"bad pin", w("tls = true\npin = \"sha256:abc\"\n"), str.RelayConfigBadPin("relay.toml", "sha256:abc", "fm")},
		{"plaintext pin", w("pin = \""+testPin+"\"\n"), str.RelayConfigPlaintextTrust("relay.toml", "fm")},
		{"plaintext ca", w("tls_trust = \"ca\"\n"), str.RelayConfigPlaintextTrust("relay.toml", "fm")},
		{"zero per ip", "max_per_ip = 0\n", str.RelayConfigBadLimit("relay.toml", "max_per_ip")},
		{"negative conns", "max_conns = -1\n", str.RelayConfigBadLimit("relay.toml", "max_conns")},
		{"hold word", "hold = \"soon\"\n", str.RelayConfigBadHold("relay.toml", "soon")},
		{"hold negative", "hold = \"-1h\"\n", str.RelayConfigBadHold("relay.toml", "-1h")},
		{"hold zero", "hold = \"0s\"\n", str.RelayConfigBadHold("relay.toml", "0s")},
		{"short hold word", "short_hold = \"soon\"\n", str.RelayConfigBadShortHold("relay.toml", "soon")},
		{"negative buffer", "max_buffer_mb = -1\n", str.RelayConfigBadLimit("relay.toml", "max_buffer_mb")},
		{"zero total buffer", "max_total_buffer_mb = 0\n", str.RelayConfigBadLimit("relay.toml", "max_total_buffer_mb")},
		{"top-level typo", "orgins = []\n", str.RelayConfigUnknownKey("relay.toml", "orgins")},
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
