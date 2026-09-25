package conn

import (
	"bufio"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Fingerprint is the pin for a certificate: "sha256:<hex>".
func Fingerprint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// ValidateFingerprint checks that fp has the form Fingerprint produces,
// "sha256:" and 64 lowercase hex digits, so a typo is never pinned.
func ValidateFingerprint(fp string) error {
	hexPart, ok := strings.CutPrefix(fp, "sha256:")
	if ok && len(hexPart) == 2*sha256.Size && strings.ToLower(hexPart) == hexPart {
		if _, err := hex.DecodeString(hexPart); err == nil {
			return nil
		}
	}
	return fmt.Errorf("malformed fingerprint %q: want sha256: followed by 64 lowercase hex digits", fp)
}

// PinMismatchError means a server presented a different certificate than
// the one pinned on first use.
type PinMismatchError struct {
	HostPort string
	Pinned   string
	Got      string
}

func (e *PinMismatchError) Error() string {
	return fmt.Sprintf("certificate for %s changed: pinned %s, got %s", e.HostPort, e.Pinned, e.Got)
}

// KnownHosts is a file of "host:port fingerprint" lines.
type KnownHosts struct {
	Path string
}

var knownHostsMu sync.Mutex

// Lookup returns the pinned fingerprint for hostport.
func (k KnownHosts) Lookup(hostport string) (string, bool, error) {
	knownHostsMu.Lock()
	defer knownHostsMu.Unlock()
	m, err := k.read()
	if err != nil {
		return "", false, err
	}
	fp, ok := m[hostport]
	return fp, ok, nil
}

// Trust pins fp for hostport, replacing any existing pin.
func (k KnownHosts) Trust(hostport, fp string) error {
	knownHostsMu.Lock()
	defer knownHostsMu.Unlock()
	m, err := k.read()
	if err != nil {
		return err
	}
	m[hostport] = fp
	keys := make([]string, 0, len(m))
	for h := range m {
		keys = append(keys, h)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, h := range keys {
		fmt.Fprintf(&b, "%s %s\n", h, m[h])
	}
	if err := os.MkdirAll(filepath.Dir(k.Path), 0o700); err != nil {
		return err
	}
	tmp := k.Path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, k.Path)
}

func (k KnownHosts) read() (map[string]string, error) {
	m := map[string]string{}
	f, err := os.Open(k.Path)
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		host, fp, ok := strings.Cut(strings.TrimSpace(sc.Text()), " ")
		if ok {
			m[host] = fp
		}
	}
	return m, sc.Err()
}
