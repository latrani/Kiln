// Package relay is kiln-relay's core: it carries a browser's WebSocket to
// a MUCK's TCP port, for the worlds on an allowlist, so Kiln's web build
// can reach MUCKs. TLS runs inside the browser; the relay passes bytes.
package relay

import (
	"errors"
	"net/netip"
	"os"
	"slices"
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

// Allowlist is the relay's config file.
type Allowlist struct {
	Worlds []World `toml:"world"`
	// Origins are extra host patterns allowed to open the relay from a
	// page (the relay's own host always is).
	Origins []string `toml:"origins"`
	// TrustedProxies (addresses or CIDRs) may say who the client is in
	// X-Forwarded-For.
	TrustedProxies []string `toml:"trusted_proxies"`

	proxies []netip.Prefix
}

// LoadAllowlist reads and checks the allowlist at path.
func LoadAllowlist(path string) (*Allowlist, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, str.Wrap(str.RelayAllowReading(path, err), err)
	}
	return ParseAllowlist(path, b)
}

// ParseAllowlist checks an allowlist file's contents; path is for messages.
// Unknown settings are errors: a typo in a security setting must not be
// silently ignored.
func ParseAllowlist(path string, b []byte) (*Allowlist, error) {
	var a Allowlist
	md, err := toml.Decode(string(b), &a)
	if err != nil {
		return nil, str.Wrap(str.RelayAllowReading(path, err), err)
	}
	if und := md.Undecoded(); len(und) > 0 {
		return nil, errors.New(str.RelayAllowUnknownKey(path, und[0].String()))
	}
	for i, w := range a.Worlds {
		h := normHost(w.Host)
		if h == "" {
			return nil, errors.New(str.RelayAllowNoHost(path, i+1))
		}
		if len(w.Ports) == 0 {
			return nil, errors.New(str.RelayAllowNoPorts(path, h))
		}
		for _, p := range w.Ports {
			if p < 1 || p > 65535 {
				return nil, errors.New(str.RelayAllowBadPort(path, h, p))
			}
		}
		a.Worlds[i].Host = h
	}
	for _, s := range a.TrustedProxies {
		p, err := parsePrefix(s)
		if err != nil {
			return nil, errors.New(str.RelayAllowBadProxy(path, s))
		}
		a.proxies = append(a.proxies, p)
	}
	return &a, nil
}

// Lookup finds the world for host:port, comparing host case-insensitively
// and ignoring a trailing dot.
func (a *Allowlist) Lookup(host string, port int) (World, bool) {
	h := normHost(host)
	for _, w := range a.Worlds {
		if w.Host == h && slices.Contains(w.Ports, port) {
			return w, true
		}
	}
	return World{}, false
}

// Trusted reports whether ip is a trusted proxy.
func (a *Allowlist) Trusted(ip netip.Addr) bool {
	ip = ip.Unmap()
	for _, p := range a.proxies {
		if p.Contains(ip) {
			return true
		}
	}
	return false
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
