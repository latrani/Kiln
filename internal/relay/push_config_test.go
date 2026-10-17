package relay

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/latrani/Kiln/internal/relay/push"
	"github.com/latrani/Kiln/internal/str"
)

// vapidDir is a directory holding vapid.pem, a fresh P-256 key, and the
// path relay.toml would have there.
func vapidDir(t *testing.T) (dir, path string, k *ecdsa.PrivateKey) {
	t.Helper()
	dir = t.TempDir()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKCS8PrivateKey(k)
	if err := os.WriteFile(filepath.Join(dir, "vapid.pem"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, filepath.Join(dir, "relay.toml"), k
}

func TestParseVAPID(t *testing.T) {
	_, path, k := vapidDir(t)
	a, err := ParseConfig(path, []byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := a.Push(); got != nil {
		t.Error("no vapid_key, but Push has a key")
	}
	a, err = ParseConfig(path, []byte("vapid_key = \"vapid.pem\"\npush_contact = \"https://fm.example\"\n"+sample))
	if err != nil {
		t.Fatal(err)
	}
	if got, contact := a.Push(); got == nil || !got.Equal(k) || contact != "https://fm.example" {
		t.Errorf("Push = %v, %q", got, contact)
	}
	a, err = ParseConfig(path, []byte("vapid_key = \"vapid.pem\"\npush_contact = \"mailto:ops@fm.example\"\n"))
	if err != nil || a.PushContact != "mailto:ops@fm.example" {
		t.Errorf("mailto contact: %v", err)
	}
}

func TestParseVAPIDErrors(t *testing.T) {
	dir, path, _ := vapidDir(t)
	os.WriteFile(filepath.Join(dir, "junk.pem"), []byte("junk"), 0o600)
	if _, err := ParseConfig(path, []byte("vapid_key = \"missing.pem\"\npush_contact = \"https://fm.example\"\n")); err == nil || !errors.Is(err, fs.ErrNotExist) || err.Error() != str.RelayConfigVapidReading(path, errors.Unwrap(err)) {
		t.Errorf("missing key: %v", err)
	}
	for _, c := range []struct{ toml, want string }{
		{"vapid_key = \"junk.pem\"\npush_contact = \"https://fm.example\"\n", str.RelayConfigBadVapid(path, "junk.pem")},
		{"vapid_key = \"vapid.pem\"\n", str.RelayConfigNoContact(path)},
		{"vapid_key = \"vapid.pem\"\npush_contact = \"ops@fm.example\"\n", str.RelayConfigBadContact(path, "ops@fm.example")},
		{"vapid_key = \"vapid.pem\"\npush_contact = \"http://fm.example\"\n", str.RelayConfigBadContact(path, "http://fm.example")},
		{"push_contact = \"https://fm.example\"\n", str.RelayConfigContactNoKey(path)},
	} {
		if _, err := ParseConfig(path, []byte(c.toml)); err == nil || err.Error() != c.want {
			t.Errorf("%q: %v, want %q", c.toml, err, c.want)
		}
	}
}

func TestPushKey(t *testing.T) {
	_, path, k := vapidDir(t)
	get := func(toml, method string) *httptest.ResponseRecorder {
		a, err := ParseConfig(path, []byte(toml))
		if err != nil {
			t.Fatal(err)
		}
		s := &Server{Log: func(string) {}}
		s.SetConfig(a)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest(method, "/relay?push_key=1", nil))
		return w
	}
	keyed := "vapid_key = \"vapid.pem\"\npush_contact = \"https://fm.example\"\n"
	if w := get(keyed, "GET"); w.Code != http.StatusOK || w.Body.String() != push.PublicKey(k) || w.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Errorf("with a key: %d %q %q", w.Code, w.Body, w.Header().Get("Content-Type"))
	}
	if w := get("", "GET"); w.Code != http.StatusNotFound {
		t.Errorf("without a key: %d", w.Code)
	}
	if w := get(keyed, "POST"); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d", w.Code)
	}
}
