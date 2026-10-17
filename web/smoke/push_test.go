package smoke

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/latrani/Kiln/internal/str"
)

// TestServiceWorkerShowsPush gives the relay a VAPID key and the page
// notifications: the page fetches the relay's key and registers its
// service worker, and a push delivered to that worker shows the away
// notification. Headless Chrome has no push service to subscribe with,
// so the relay's side of it is covered by internal/relay's tests.
func TestServiceWorkerShowsPush(t *testing.T) {
	key := filepath.Join(t.TempDir(), "vapid.pem")
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalPKCS8PrivateKey(k)
	if err := os.WriteFile(key, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	relaySettings = fmt.Sprintf("vapid_key = %q\npush_contact = \"https://fm.example\"\n", key)
	t.Cleanup(func() { relaySettings = "" })

	ws, _ := launch(t)
	ev := evaluator(t, ws)
	page := mustUnquote(t, ev(`location.href`))
	u, _ := url.Parse(page)
	origin := u.Scheme + "://" + u.Host
	u.RawQuery = ""
	scope := u.String()
	devtools(t, ws, "Browser.grantPermissions", map[string]any{"permissions": []string{"notifications"}, "origin": origin})
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Order")`, "the first batch")
	devtools(t, ws, "Page.reload", map[string]any{})
	time.Sleep(200 * time.Millisecond) // the old page is gone
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Order")`, "the reloaded page's first batch")
	waitFor(t, ev, `performance.getEntriesByType("resource").some(e => e.name.includes("push_key=1") && e.responseStatus === 200)`, "the page getting the relay's key")
	waitFor(t, ev, `navigator.serviceWorker.getRegistration().then(r => !!r?.active)`, "the service worker running")

	devtools(t, ws, "ServiceWorker.deliverPushMessage", map[string]any{"origin": origin, "registrationId": registration(t, ws, scope), "data": ""})
	want, _ := json.Marshal([]string{str.WebPushTitle(), str.WebPushBody()})
	waitFor(t, ev, `navigator.serviceWorker.ready.then(r => r.getNotifications()).then(ns => ns.length === 1 && JSON.stringify([ns[0].title, ns[0].body]) === `+jsString(string(want))+`)`, "the away notification")
}

// registration is the DevTools id of the service worker registered for
// scope.
func registration(t *testing.T, ws *websocket.Conn, scope string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	msg, _ := json.Marshal(map[string]any{"id": 1<<30 + 1, "method": "ServiceWorker.enable"})
	if err := ws.Write(ctx, websocket.MessageText, msg); err != nil {
		t.Fatal(err)
	}
	for {
		_, b, err := ws.Read(ctx)
		if err != nil {
			t.Fatalf("no registration for %s: %v", scope, err)
		}
		var e struct {
			Method string
			Params struct {
				Registrations []struct {
					RegistrationID string `json:"registrationId"`
					ScopeURL       string `json:"scopeURL"`
					IsDeleted      bool   `json:"isDeleted"`
				}
			}
		}
		if json.Unmarshal(b, &e) != nil || e.Method != "ServiceWorker.workerRegistrationUpdated" {
			continue
		}
		for _, r := range e.Params.Registrations {
			if r.ScopeURL == scope && !r.IsDeleted {
				return r.RegistrationID
			}
		}
	}
}
