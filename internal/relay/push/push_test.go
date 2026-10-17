package push

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func pemOf(t *testing.T, k any) []byte {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

func newKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestParseKey(t *testing.T) {
	k := newKey(t)
	got, err := ParseKey(pemOf(t, k))
	if err != nil || !got.Equal(k) {
		t.Fatalf("ParseKey = %v, %v", got, err)
	}
	k384, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	rk, _ := rsa.GenerateKey(rand.Reader, 1024)
	for name, b := range map[string][]byte{"P-384": pemOf(t, k384), "RSA": pemOf(t, rk), "garbage": []byte("not a key")} {
		if _, err := ParseKey(b); err == nil {
			t.Errorf("%s parsed", name)
		}
	}
}

func TestPublicKey(t *testing.T) {
	b, err := base64.RawURLEncoding.DecodeString(PublicKey(newKey(t)))
	if err != nil || len(b) != 65 || b[0] != 4 {
		t.Fatalf("public key %x, %v", b, err)
	}
}

func TestAllowed(t *testing.T) {
	for _, e := range []string{
		"https://fcm.googleapis.com/fcm/send/x",
		"https://updates.push.services.mozilla.com/wpush/v2/x",
		"https://web.push.apple.com/x",
		"https://db5p.notify.windows.com/w/?token=x",
	} {
		if !Allowed(e) {
			t.Errorf("Allowed(%q) = false", e)
		}
	}
	for _, e := range []string{
		"http://fcm.googleapis.com/x",
		"https://example.com/x",
		"https://fcm.googleapis.com.evil.net/x",
		"https://notify.windows.com.evil.net/x",
		"https://notify.windows.com/x",
		"https://fcm.googleapis.com/" + strings.Repeat("x", 2049-len("https://fcm.googleapis.com/")),
		"",
	} {
		if Allowed(e) {
			t.Errorf("Allowed(%.60q) = true", e)
		}
	}
}

// pushServer is a push service under test: Hosts names it, and its
// client trusts it. It answers status and hands each request to seen.
func pushServer(t *testing.T, status int, seen func(*http.Request, []byte)) (*httptest.Server, *http.Client) {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		seen(r, b)
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	old := Hosts
	Hosts = []string{"127.0.0.1"}
	t.Cleanup(func() { Hosts = old })
	return srv, srv.Client()
}

func TestSendSignsAndIsEmpty(t *testing.T) {
	k := newKey(t)
	now := time.Unix(1_800_000_000, 0)
	var req *http.Request
	var body []byte
	srv, c := pushServer(t, http.StatusCreated, func(r *http.Request, b []byte) { req, body = r, b })
	gone, err := Send(context.Background(), c, k, "https://fm.example", srv.URL+"/push/abc", now)
	if gone || err != nil {
		t.Fatalf("Send = %v, %v", gone, err)
	}
	if req.Method != http.MethodPost || len(body) != 0 || req.Header.Get("TTL") != "86400" || req.Header.Get("Urgency") != "high" {
		t.Fatalf("request %s, body %q, headers %v", req.Method, body, req.Header)
	}
	auth, ok := strings.CutPrefix(req.Header.Get("Authorization"), "vapid ")
	if !ok {
		t.Fatalf("Authorization = %q", req.Header.Get("Authorization"))
	}
	parts := map[string]string{}
	for _, p := range strings.Split(auth, ",") {
		kv := strings.SplitN(strings.TrimSpace(p), "=", 2)
		parts[kv[0]] = kv[1]
	}
	if parts["k"] != PublicKey(k) {
		t.Errorf("k = %q", parts["k"])
	}
	seg := strings.Split(parts["t"], ".")
	if len(seg) != 3 {
		t.Fatalf("jwt %q", parts["t"])
	}
	var hdr, claims map[string]any
	for i, v := range []*map[string]any{&hdr, &claims} {
		b, err := base64.RawURLEncoding.DecodeString(seg[i])
		if err != nil || json.Unmarshal(b, v) != nil {
			t.Fatalf("segment %d: %q", i, b)
		}
	}
	u, _ := url.Parse(srv.URL)
	if hdr["alg"] != "ES256" || hdr["typ"] != "JWT" {
		t.Errorf("header %v", hdr)
	}
	if claims["aud"] != "https://"+u.Host || claims["sub"] != "https://fm.example" || claims["exp"] != float64(now.Add(12*time.Hour).Unix()) {
		t.Errorf("claims %v", claims)
	}
	sig, err := base64.RawURLEncoding.DecodeString(seg[2])
	if err != nil || len(sig) != 64 {
		t.Fatalf("signature %x, %v", sig, err)
	}
	h := sha256.Sum256([]byte(seg[0] + "." + seg[1]))
	if !ecdsa.Verify(&k.PublicKey, h[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Error("signature doesn't verify")
	}
}

func TestSendGone(t *testing.T) {
	k := newKey(t)
	for _, c := range []struct {
		status int
		gone   bool
		err    bool
	}{{404, true, false}, {410, true, false}, {500, false, true}, {201, false, false}} {
		srv, cl := pushServer(t, c.status, func(*http.Request, []byte) {})
		gone, err := Send(context.Background(), cl, k, "https://fm.example", srv.URL+"/x", time.Now())
		if gone != c.gone || (err != nil) != c.err {
			t.Errorf("status %d: Send = %v, %v", c.status, gone, err)
		}
	}
}
