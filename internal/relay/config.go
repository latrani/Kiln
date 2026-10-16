// Package relay is kiln-relay's core: it carries a browser's WebSocket to
// a MUCK's TCP port, for the worlds on an allowlist, so Kiln's web build
// can reach MUCKs. TLS runs inside the browser; the relay passes bytes.
package relay

import (
	"errors"
	"net"
	"net/netip"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/latrani/Kiln/internal/str"
)

// World is a MUCK the relay will connect to.
type World struct {
	Host  string `toml:"host"`
	Ports []int  `toml:"ports"`
	// Plaintext allows non-TLS traffic. Off, the relay refuses anything
	// that doesn't start with a TLS handshake, so whoever runs the relay
	// never sees players' passwords. Meant for the operator's own MUCK.
	Plaintext bool `toml:"plaintext"`
}

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

	proxies []netip.Prefix
}

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
	if und := md.Undecoded(); len(und) > 0 {
		return nil, errors.New(str.RelayConfigUnknownKey(path, und[0].String()))
	}
	seen := map[string]bool{}
	for i, w := range a.Worlds {
		h := normHost(w.Host)
		if h == "" {
			return nil, errors.New(str.RelayConfigNoHost(path, i+1))
		}
		if len(w.Ports) == 0 {
			return nil, errors.New(str.RelayConfigNoPorts(path, h))
		}
		for _, p := range w.Ports {
			if p < 1 || p > 65535 {
				return nil, errors.New(str.RelayConfigBadPort(path, h, p))
			}
			// Lookup takes the first match, so a second entry would be
			// silently ignored.
			hp := net.JoinHostPort(h, strconv.Itoa(p))
			if seen[hp] {
				return nil, errors.New(str.RelayConfigDuplicate(path, hp))
			}
			seen[hp] = true
		}
		a.Worlds[i].Host = h
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
	}{{"max_per_ip", a.MaxPerIP}, {"max_conns", a.MaxConns}} { //str:ok
		if md.IsDefined(l.key) && l.v < 1 {
			return nil, errors.New(str.RelayConfigBadLimit(path, l.key))
		}
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

// Lookup finds the world for host:port, comparing host case-insensitively
// and ignoring a trailing dot.
func (a *Config) Lookup(host string, port int) (World, bool) {
	h := normHost(host)
	for _, w := range a.Worlds {
		if w.Host == h && slices.Contains(w.Ports, port) {
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
