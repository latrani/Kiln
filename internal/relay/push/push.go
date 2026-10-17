// Package push sends contentless Web Push messages: an empty POST to a
// browser's push service, signed with the relay's VAPID key (RFC 8292),
// which the push service turns into a "something happened" for the
// page's service worker. With no payload there's nothing to encrypt, so
// only the subscription's endpoint is needed.
package push

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/latrani/Kiln/internal/str"
)

// Hosts are the push services the relay posts to by default: exact hosts, or,
// with a leading dot, any host ending in it. A page names the endpoint,
// so anything else would let it point the relay at any URL.
var Hosts = []string{
	"fcm.googleapis.com",                // Chrome, Edge on Android //str:ok host name
	"updates.push.services.mozilla.com", // Firefox //str:ok host name
	"web.push.apple.com",                // Safari //str:ok host name
	".notify.windows.com",               // Edge on Windows //str:ok host name
}

// MaxEndpoint is the longest endpoint taken.
const MaxEndpoint = 2048

var errKey = errors.New("not a P-256 PKCS#8 key") //str:ok never shown: config says which file

// ParseKey reads a PEM PKCS#8 P-256 private key.
func ParseKey(b []byte) (*ecdsa.PrivateKey, error) {
	blk, _ := pem.Decode(b)
	if blk == nil {
		return nil, errKey
	}
	k, err := x509.ParsePKCS8PrivateKey(blk.Bytes)
	if err != nil {
		return nil, err
	}
	ek, ok := k.(*ecdsa.PrivateKey)
	if !ok || ek.Curve != elliptic.P256() {
		return nil, errKey
	}
	return ek, nil
}

// PublicKey is k's public half as a page passes it to
// pushManager.subscribe: the uncompressed point, base64url.
func PublicKey(k *ecdsa.PrivateKey) string {
	b, _ := k.PublicKey.Bytes()
	return base64.RawURLEncoding.EncodeToString(b)
}

// Allowed reports whether endpoint is an https URL on one of hosts (as
// Hosts says them), and not too long.
func Allowed(endpoint string, hosts []string) bool {
	if len(endpoint) > MaxEndpoint {
		return false
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.User != nil { //str:ok URL scheme
		return false
	}
	h := strings.ToLower(u.Hostname())
	for _, ok := range hosts {
		if h == ok || strings.HasPrefix(ok, ".") && strings.HasSuffix(h, ok) && len(h) > len(ok) {
			return true
		}
	}
	return false
}

// Send posts an empty push to endpoint, signed by k, naming contact (an
// https: or mailto: URL) as who to ask about it. gone says the push
// service no longer knows the subscription (404 or 410): stop using it.
func Send(ctx context.Context, c *http.Client, k *ecdsa.PrivateKey, contact, endpoint string, now time.Time) (gone bool, err error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return false, err
	}
	jwt, err := token(k, u.Scheme+"://"+u.Host, contact, now)
	if err != nil {
		return false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, http.NoBody)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "vapid t="+jwt+", k="+PublicKey(k)) //str:ok protocol
	req.Header.Set("TTL", "86400")                                      //str:ok protocol
	req.Header.Set("Urgency", "high")                                   //str:ok protocol
	resp, err := c.Do(req)
	if err != nil {
		return false, err
	}
	resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return true, nil
	case resp.StatusCode >= 300:
		return false, errors.New(str.RelayPushStatus(resp.Status))
	}
	return false, nil
}

// token is the VAPID JWT (ES256) for aud, good for 12 hours.
func token(k *ecdsa.PrivateKey, aud, contact string, now time.Time) (string, error) {
	enc := base64.RawURLEncoding
	hdr, _ := json.Marshal(map[string]string{"typ": "JWT", "alg": "ES256"}) //str:ok protocol
	claims, _ := json.Marshal(map[string]any{"aud": aud, "exp": now.Add(12 * time.Hour).Unix(), "sub": contact})
	signed := enc.EncodeToString(hdr) + "." + enc.EncodeToString(claims)
	h := sha256.Sum256([]byte(signed))
	r, s, err := ecdsa.Sign(rand.Reader, k, h[:])
	if err != nil {
		return "", err
	}
	// JWS wants r and s as two fixed 32-byte halves, not ASN.1.
	var sig [64]byte
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	var b bytes.Buffer
	b.WriteString(signed)
	b.WriteByte('.')
	b.WriteString(enc.EncodeToString(sig[:]))
	return b.String(), nil
}
