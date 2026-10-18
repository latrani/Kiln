// Package relay is kiln-relay's core: it carries a browser's WebSocket to
// a MUCK's TCP port, for the worlds on an allowlist, so Kiln's web build
// can reach MUCKs. The relay does TLS with each TLS world itself.
package relay

import (
	"bytes"
	"crypto/ecdsa"
	"errors"
	"net"
	"net/netip"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/conn"
	"github.com/latrani/Kiln/internal/relay/push"
	"github.com/latrani/Kiln/internal/str"
)

// World is a MUCK the relay will connect to. Its [[world]] table is a
// Kiln world file (see internal/config) with an id, its file's name,
// and how the relay trusts it; the page is served the rest as the
// world's file.
type World struct {
	ID   string `toml:"id"`
	Host string `toml:"host"`
	Port int    `toml:"port"`
	TLS  bool   `toml:"tls"`
	// How the relay trusts a TLS world: TLSTrust "ca" (a real CA), or a
	// pinned certificate, "sha256:…" as conn.Fingerprint makes it. A TLS
	// world has exactly one; nothing is trusted on first use.
	TLSTrust string `toml:"tls_trust"`
	Pin      string `toml:"pin"`

	file []byte
}

// File is w as a Kiln world file, worlds/<id>.toml: its table without
// id and pin, which are the relay's.
func (w World) File() []byte { return w.file }

// Config is the relay's config file, relay.toml. Its [[world]] list is the
// allowlist.
type Config struct {
	Worlds []World `toml:"world"`
	// Origins are extra host patterns allowed to open the relay from a
	// page (the relay's own host always is).
	Origins []string `toml:"origins"`
	// TrustedProxies (addresses or CIDRs) may say who the client is in
	// X-Forwarded-For.
	TrustedProxies []string `toml:"trusted_proxies"`
	// MaxPerIP caps connections from one client: an IPv4 address, or an
	// IPv6 /64. 0: 8.
	MaxPerIP int `toml:"max_per_ip"`
	// MaxConns caps connections in all. 0: 256.
	MaxConns int `toml:"max_conns"`
	// Hold is how long a world connection is kept after its WebSocket
	// dies without the page closing it, as time.ParseDuration reads it.
	// "": 24h.
	Hold string `toml:"hold"`
	// ShortHold is the hold for a connection whose page said it closed
	// (a tab closed, not one switched away from). "": 10m.
	ShortHold string `toml:"short_hold"`
	// MaxBufferMB caps what's buffered for one connection, and
	// MaxTotalBufferMB for the whole relay; past them the oldest lines
	// are dropped. 0: 4 and 256.
	MaxBufferMB      int `toml:"max_buffer_mb"`
	MaxTotalBufferMB int `toml:"max_total_buffer_mb"`
	// VAPIDKey is a PEM PKCS#8 P-256 private key file, relative to this
	// file's directory, that signs Web Push messages; PushContact (an
	// https: or mailto: URL) is who push services may ask about them.
	// Without a key the relay sends no pushes.
	VAPIDKey    string `toml:"vapid_key"`
	PushContact string `toml:"push_contact"`

	proxies   []netip.Prefix
	vapid     *ecdsa.PrivateKey
	hold      time.Duration
	shortHold time.Duration
}

// ShortHoldFor is how long a connection whose page closed is held.
func (a *Config) ShortHoldFor() time.Duration { return or(a.shortHold, 10*time.Minute) }

// Push is the key that signs pushes and the contact they name, or nil
// when the relay sends none.
func (a *Config) Push() (*ecdsa.PrivateKey, string) { return a.vapid, a.PushContact }

// HoldFor is how long a detached connection is held.
func (a *Config) HoldFor() time.Duration { return or(a.hold, 24*time.Hour) }

// BufferCap is the most bytes buffered for one connection.
func (a *Config) BufferCap() int { return or(a.MaxBufferMB, 4) << 20 }

// TotalBufferCap is the most bytes buffered across the relay.
func (a *Config) TotalBufferCap() int { return or(a.MaxTotalBufferMB, 256) << 20 }

// LoadConfig reads and checks the allowlist at path.
func LoadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, str.Wrap(str.RelayConfigReading(path, err), err)
	}
	return ParseConfig(path, b)
}

// ParseConfig checks an allowlist file's contents; path is for messages.
// Unknown settings are errors: a typo in a security setting must not be
// silently ignored.
func ParseConfig(path string, b []byte) (*Config, error) {
	var a Config
	md, err := toml.Decode(string(b), &a)
	if err != nil {
		return nil, str.Wrap(str.RelayConfigReading(path, err), err)
	}
	// A world's other keys are its file's, which Kiln's check reads.
	for _, k := range md.Undecoded() {
		if k[0] != "world" { //str:ok config key
			return nil, errors.New(str.RelayConfigUnknownKey(path, k.String()))
		}
	}
	var raw struct {
		World []map[string]any `toml:"world"`
	}
	if _, err := toml.Decode(string(b), &raw); err != nil {
		return nil, str.Wrap(str.RelayConfigReading(path, err), err)
	}
	ids, seen := map[string]bool{}, map[string]bool{}
	for i, w := range a.Worlds {
		if w.ID == "" {
			return nil, errors.New(str.RelayConfigNoId(path, i+1))
		}
		t := raw.World[i]
		delete(t, "id")  //str:ok config key
		delete(t, "pin") //str:ok config key
		var f bytes.Buffer
		if err := toml.NewEncoder(&f).Encode(t); err != nil {
			return nil, str.Wrap(str.RelayConfigReading(path, err), err)
		}
		if err := config.CheckWorldFile(str.RelayConfigWorld(path, w.ID), w.ID, f.Bytes()); err != nil {
			return nil, err
		}
		// Case-insensitively, as world ids name log folders.
		if ids[strings.ToLower(w.ID)] {
			return nil, errors.New(str.RelayConfigDuplicateId(path, w.ID))
		}
		ids[strings.ToLower(w.ID)] = true
		h := normHost(w.Host)
		// Lookup takes the first match, so a second entry would be
		// silently ignored.
		hp := net.JoinHostPort(h, strconv.Itoa(w.Port))
		if seen[hp] {
			return nil, errors.New(str.RelayConfigDuplicate(path, hp))
		}
		seen[hp] = true
		if err := checkTrust(path, w); err != nil {
			return nil, err
		}
		a.Worlds[i].Host = h
		a.Worlds[i].file = f.Bytes()
	}
	for i, o := range a.Origins {
		n := normHost(o)
		if !validOrigin(n) {
			return nil, errors.New(str.RelayConfigBadOrigin(path, o))
		}
		a.Origins[i] = n
	}
	for _, l := range []struct {
		key string
		v   int
	}{{"max_per_ip", a.MaxPerIP}, {"max_conns", a.MaxConns}, {"max_buffer_mb", a.MaxBufferMB}, {"max_total_buffer_mb", a.MaxTotalBufferMB}} { //str:ok
		if md.IsDefined(l.key) && l.v < 1 {
			return nil, errors.New(str.RelayConfigBadLimit(path, l.key))
		}
	}
	if a.Hold != "" {
		d, err := time.ParseDuration(a.Hold)
		if err != nil || d <= 0 {
			return nil, errors.New(str.RelayConfigBadHold(path, a.Hold))
		}
		a.hold = d
	}
	if a.ShortHold != "" {
		d, err := time.ParseDuration(a.ShortHold)
		if err != nil || d <= 0 {
			return nil, errors.New(str.RelayConfigBadShortHold(path, a.ShortHold))
		}
		a.shortHold = d
	}
	if err := a.loadVAPID(path); err != nil {
		return nil, err
	}
	for _, s := range a.TrustedProxies {
		p, err := parsePrefix(s)
		if err != nil {
			return nil, errors.New(str.RelayConfigBadProxy(path, s))
		}
		a.proxies = append(a.proxies, p)
	}
	return &a, nil
}

// loadVAPID reads vapid_key, beside path, and checks push_contact.
func (a *Config) loadVAPID(path string) error {
	switch {
	case a.VAPIDKey == "" && a.PushContact != "":
		return errors.New(str.RelayConfigContactNoKey(path))
	case a.VAPIDKey == "":
		return nil
	case a.PushContact == "":
		return errors.New(str.RelayConfigNoContact(path))
	case !strings.HasPrefix(a.PushContact, "https:") && !strings.HasPrefix(a.PushContact, "mailto:"): //str:ok URL schemes
		return errors.New(str.RelayConfigBadContact(path, a.PushContact))
	}
	kp := a.VAPIDKey
	if !filepath.IsAbs(kp) {
		kp = filepath.Join(filepath.Dir(path), kp)
	}
	b, err := os.ReadFile(kp)
	if err != nil {
		return str.Wrap(str.RelayConfigVapidReading(path, err), err)
	}
	if a.vapid, err = push.ParseKey(b); err != nil {
		return errors.New(str.RelayConfigBadVapid(path, a.VAPIDKey))
	}
	return nil
}

// checkTrust checks that w says how it's trusted, if it's TLS, and not
// if it isn't.
func checkTrust(path string, w World) error {
	if !w.TLS {
		if w.TLSTrust != "" || w.Pin != "" {
			return errors.New(str.RelayConfigPlaintextTrust(path, w.ID))
		}
		return nil
	}
	switch {
	case w.TLSTrust != "" && w.Pin != "":
		return errors.New(str.RelayConfigBothTrust(path, w.ID))
	case w.TLSTrust != "":
		if w.TLSTrust != "ca" { //str:ok config vocabulary
			return errors.New(str.RelayConfigBadTrust(path, w.TLSTrust, w.ID))
		}
	case w.Pin != "":
		if conn.ValidateFingerprint(w.Pin) != nil {
			return errors.New(str.RelayConfigBadPin(path, w.Pin, w.ID))
		}
	default:
		return errors.New(str.RelayConfigNoTrust(path, w.ID))
	}
	return nil
}

// Lookup finds the world for host:port, comparing host case-insensitively
// and ignoring a trailing dot.
func (a *Config) Lookup(host string, port int) (World, bool) {
	h := normHost(host)
	for _, w := range a.Worlds {
		if w.Host == h && w.Port == port {
			return w, true
		}
	}
	return World{}, false
}

// Trusted reports whether ip is a trusted proxy.
func (a *Config) Trusted(ip netip.Addr) bool {
	ip = ip.Unmap()
	for _, p := range a.proxies {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// validOrigin reports whether o is a host pattern, with an optional port,
// as websocket.AcceptOptions.OriginPatterns matches it. A URL (scheme or
// path) means the operator expected something else, so it's refused.
func validOrigin(o string) bool {
	if o == "" || strings.ContainsAny(o, "/\\") {
		return false
	}
	_, err := path.Match(o, "")
	return err == nil
}

func normHost(h string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(h)), ".")
}

func parsePrefix(s string) (netip.Prefix, error) {
	if strings.Contains(s, "/") {
		return netip.ParsePrefix(s)
	}
	ip, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, err
	}
	ip = ip.Unmap()
	return netip.PrefixFrom(ip, ip.BitLen()), nil
}
