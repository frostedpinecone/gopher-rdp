package web

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	rdp "github.com/bouncyball-git/gopher-rdp"
)

func TestSessionToken(t *testing.T) {
	for _, path := range []string{"/", "/ws", "/ws/control", "/disconnect", "/monitor/0"} {
		for _, tc := range []struct {
			token, origin string
			allowed       bool
		}{
			{"", "", false},
			{"wrong", "", false},
			{"secret", "https://attacker.invalid", false},
			{"secret", "http://localhost:8081", false},
			{"secret", "null", false},
			{"secret", "http://localhost:8080", true},
			{"secret", "", true},
		} {
			t.Run(path+"/"+tc.token+"/"+tc.origin, func(t *testing.T) {
				called := false
				h := WithSessionToken(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }), "secret")
				r := httptest.NewRequest("GET", "http://localhost:8080"+path+"?token="+tc.token, nil)
				r.Header.Set("Origin", tc.origin)
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				if called != tc.allowed {
					t.Fatalf("handler called=%v, want %v", called, tc.allowed)
				}
				if !tc.allowed && w.Code != http.StatusForbidden {
					t.Fatalf("status=%d", w.Code)
				}
				if w.Header().Get("Referrer-Policy") != "no-referrer" {
					t.Fatal("missing referrer protection")
				}
			})
		}
	}
}

func TestViewerTokenRedirects(t *testing.T) {
	opts := rdp.DefaultOptions()
	auto, _ := NewAutoWebHandler(opts)
	d := NewDispatcher(opts, []MonitorRect{{Index: 0, Width: 640, Height: 480, Primary: true}}, slog.New(slog.DiscardHandler), rdp.KeyboardScancode)
	defer d.Close()
	multi := NewMultiMonitorHandler(d)
	for _, tc := range []struct {
		handler http.Handler
		path    string
	}{
		{NewWebHandler(nil, nil, 640, 480, rdp.KeyboardScancode), "/"},
		{auto, "/"},
		{multi, "/monitor/0"},
	} {
		w := httptest.NewRecorder()
		WithSessionToken(tc.handler, "secret").ServeHTTP(w, httptest.NewRequest("GET", tc.path+"?token=secret", nil))
		if w.Code != http.StatusFound {
			t.Fatalf("status=%d", w.Code)
		}
		u, err := url.Parse(w.Header().Get("Location"))
		if err != nil || u.Query().Get("token") != "secret" {
			t.Fatalf("token lost in redirect: %s", w.Header().Get("Location"))
		}
	}
	w := httptest.NewRecorder()
	WithSessionToken(multi, "secret").ServeHTTP(w, httptest.NewRequest("GET", "/?token=secret", nil))
	for _, link := range []string{"/monitor/0?token=secret", "/disconnect?token=secret", "/ws/control?token="} {
		if !strings.Contains(w.Body.String(), link) {
			t.Fatalf("missing token in %s", link)
		}
	}
}

func TestWebSocketRejectsForeignOrigin(t *testing.T) {
	r := httptest.NewRequest("GET", "http://localhost:8080/ws", nil)
	r.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	r.Header.Set("Origin", "https://attacker.invalid")
	w := httptest.NewRecorder()
	if _, err := upgradeWebSocket(w, r); err == nil || w.Code != http.StatusForbidden {
		t.Fatalf("foreign origin accepted: status=%d, err=%v", w.Code, err)
	}
}

func TestTokenWebSocketUpgrade(t *testing.T) {
	srv := httptest.NewServer(WithSessionToken(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgradeWebSocket(w, r)
		if err != nil {
			t.Error(err)
			return
		}
		conn.Close()
	}), "secret"))
	defer srv.Close()
	conn := dialWebSocket(t, srv.URL, "/ws?token=secret")
	conn.Close()
}

func TestListenAddrLoopback(t *testing.T) {
	if got := ListenAddr("8080"); got != "127.0.0.1:8080" {
		t.Fatalf("listen address=%q", got)
	}
}
