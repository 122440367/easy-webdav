package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/122440367/easy-webdav/internal/auth"
	"github.com/122440367/easy-webdav/internal/store"
)

func testAPI(t *testing.T) *AuthAPI {
	t.Helper()
	dsn := "file:api-" + strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()) + "?mode=memory&cache=shared"
	db, err := store.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return &AuthAPI{Store: db, Sessions: auth.Sessions{Store: db}, Limiter: auth.NewLoginLimiter(), StorageDir: filepath.Join(t.TempDir(), "files")}
}

func loginBody(username, password string) []byte {
	raw, _ := json.Marshal(credentials{Username: username, Password: password})
	return raw
}

func postJSON(handler func(http.ResponseWriter, *http.Request), url string, body []byte, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("X-Requested-With", "fetch")
	req.RemoteAddr = "192.168.1.5:1234"
	if cookie != nil {
		req.AddCookie(cookie)
	}
	req = auth.WithRequestMeta(req, nil)
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	// Parse the Set-Cookie lines as a real response so attributes survive.
	raw := "HTTP/1.1 200 OK\r\n"
	for _, line := range rec.Header().Values("Set-Cookie") {
		raw += "Set-Cookie: " + line + "\r\n"
	}
	raw += "\r\n"
	resp, err := http.ReadResponse(bufio.NewReader(strings.NewReader(raw)), nil)
	if err != nil {
		t.Fatalf("parse set-cookie: %v", err)
	}
	for _, c := range resp.Cookies() {
		if c.Name == auth.SessionCookie {
			return c
		}
	}
	t.Fatalf("no session cookie set: %v", rec.Header().Values("Set-Cookie"))
	return nil
}

func seedAdmin(t *testing.T, a *AuthAPI) {
	t.Helper()
	if err := a.Bootstrap(context.Background(), "admin", "password123"); err != nil {
		t.Fatal(err)
	}
}

func TestSetupLocksAndLogin(t *testing.T) {
	a := testAPI(t)
	body := loginBody("admin", "password123")
	if rec := postJSON(a.Setup, "/api/v1/setup", body, nil); rec.Code != 201 {
		t.Fatalf("setup status=%d body=%s", rec.Code, rec.Body)
	}
	if rec := postJSON(a.Setup, "/api/v1/setup", body, nil); rec.Code != 403 {
		t.Fatalf("expected setup lock, got %d", rec.Code)
	}
	if rec := postJSON(a.Login, "/api/v1/auth/login", body, nil); rec.Code != 200 {
		t.Fatalf("login status=%d body=%s", rec.Code, rec.Body)
	}
}

func TestSetupBootstrap(t *testing.T) {
	a := testAPI(t)
	if err := a.Bootstrap(context.Background(), "admin", "password123"); err != nil {
		t.Fatal(err)
	}
	if count, _ := a.Store.AdminCount(context.Background()); count != 1 {
		t.Fatal("bootstrap did not create admin")
	}
}

func TestLoginFailuresAndDisabledUser(t *testing.T) {
	a := testAPI(t)
	seedAdmin(t, a)
	// Wrong password and unknown user must be indistinguishable.
	rec := postJSON(a.Login, "/api/v1/auth/login", loginBody("admin", "wrongpass1"), nil)
	if rec.Code != 401 {
		t.Fatalf("wrong password status=%d", rec.Code)
	}
	wrongPasswordMessage := rec.Body.String()
	rec = postJSON(a.Login, "/api/v1/auth/login", loginBody("ghost", "password123"), nil)
	if rec.Code != 401 || rec.Body.String() != wrongPasswordMessage {
		t.Fatalf("unknown user differs: %d %s vs %s", rec.Code, rec.Body, wrongPasswordMessage)
	}
	// Disabled users fail even with correct credentials and get no session.
	admin, _ := a.Store.UserByName(context.Background(), "admin")
	if err := a.Store.UpdateUser(context.Background(), admin.ID, "admin", "admin", "readwrite", 0, true); err != nil {
		t.Fatal(err)
	}
	rec = postJSON(a.Login, "/api/v1/auth/login", loginBody("admin", "password123"), nil)
	if rec.Code != 401 {
		t.Fatalf("disabled user login status=%d", rec.Code)
	}
	if len(rec.Header().Values("Set-Cookie")) != 0 {
		t.Fatal("disabled user got a session cookie")
	}
}

func TestLoginRateLimited(t *testing.T) {
	a := testAPI(t)
	seedAdmin(t, a)
	for i := 0; i < 10; i++ {
		if rec := postJSON(a.Login, "/api/v1/auth/login", loginBody("admin", "wrongpass1"), nil); rec.Code != 401 {
			t.Fatalf("attempt %d status=%d", i, rec.Code)
		}
	}
	rec := postJSON(a.Login, "/api/v1/auth/login", loginBody("admin", "password123"), nil)
	if rec.Code != 429 {
		t.Fatalf("expected 429 after 10 failures, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("Retry-After header missing")
	}
}

func TestLogoutInvalidatesSession(t *testing.T) {
	a := testAPI(t)
	seedAdmin(t, a)
	rec := postJSON(a.Login, "/api/v1/auth/login", loginBody("admin", "password123"), nil)
	cookie := sessionCookie(t, rec)
	if rec := postJSON(a.Logout, "/api/v1/auth/logout", nil, cookie); rec.Code != 200 {
		t.Fatalf("logout status=%d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(cookie)
	req = auth.WithRequestMeta(req, nil)
	rec = httptest.NewRecorder()
	a.Me(rec, req)
	if rec.Code != 401 {
		t.Fatal("old cookie still valid after logout")
	}
}

func TestPasswordChangeKeepsCurrentSessionRevokesOthers(t *testing.T) {
	a := testAPI(t)
	seedAdmin(t, a)
	first := sessionCookie(t, postJSON(a.Login, "/api/v1/auth/login", loginBody("admin", "password123"), nil))
	second := sessionCookie(t, postJSON(a.Login, "/api/v1/auth/login", loginBody("admin", "password123"), nil))
	body, _ := json.Marshal(map[string]string{"CurrentPassword": "password123", "Password": "newpassword456"})
	if rec := postJSON(a.Password, "/api/v1/auth/password", body, first); rec.Code != 200 {
		t.Fatalf("password change status=%d body=%s", rec.Code, rec.Body)
	}
	meRequest := func(cookie *http.Cookie) int {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		req.AddCookie(cookie)
		req = auth.WithRequestMeta(req, nil)
		rec := httptest.NewRecorder()
		a.Me(rec, req)
		return rec.Code
	}
	if code := meRequest(first); code != 200 {
		t.Fatalf("current session should stay valid, got %d", code)
	}
	if code := meRequest(second); code != 401 {
		t.Fatalf("other session should be revoked, got %d", code)
	}
	// Old password no longer works; the new one does.
	if rec := postJSON(a.Login, "/api/v1/auth/login", loginBody("admin", "password123"), nil); rec.Code != 401 {
		t.Fatal("old password accepted after change")
	}
	if rec := postJSON(a.Login, "/api/v1/auth/login", loginBody("admin", "newpassword456"), nil); rec.Code != 200 {
		t.Fatalf("new password rejected: %d", rec.Code)
	}
}

func TestLoginCSRFRejectedWithoutHeader(t *testing.T) {
	a := testAPI(t)
	seedAdmin(t, a)
	chained := auth.CSRF(http.HandlerFunc(a.Login))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginBody("admin", "password123")))
	req.RemoteAddr = "192.168.1.5:1234"
	req = auth.WithRequestMeta(req, nil)
	rec := httptest.NewRecorder()
	chained.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("cross-site POST should be rejected, got %d", rec.Code)
	}
}

func TestSpoofedForwardedHeadersIgnoredByLimiter(t *testing.T) {
	a := testAPI(t)
	seedAdmin(t, a)
	attempt := func(remoteAddr string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginBody("admin", "wrongpass1")))
		req.Header.Set("X-Requested-With", "fetch")
		// Untrusted client claims to be someone else; no proxies are trusted.
		req.Header.Set("X-Forwarded-For", "1.2.3.4")
		req.RemoteAddr = remoteAddr
		req = auth.WithRequestMeta(req, nil)
		rec := httptest.NewRecorder()
		a.Login(rec, req)
		return rec.Code
	}
	for i := 0; i < 10; i++ {
		if code := attempt("203.0.113.9:1111"); code != 401 {
			t.Fatalf("attempt %d status %d", i, code)
		}
	}
	if code := attempt("203.0.113.9:1111"); code != 429 {
		t.Fatalf("real IP should be limited, got %d", code)
	}
	if code := attempt("198.51.100.7:2222"); code != 401 {
		t.Fatalf("different real IP must not inherit the limit, got %d", code)
	}
}

func TestInsecureFlagScenarios(t *testing.T) {
	a := testAPI(t)
	status := func(remoteAddr string, header func(*http.Request), trusted []*net.IPNet) bool {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/status", nil)
		req.RemoteAddr = remoteAddr
		if header != nil {
			header(req)
		}
		req = auth.WithRequestMeta(req, trusted)
		rec := httptest.NewRecorder()
		a.SetupStatus(rec, req)
		var out struct {
			Insecure bool `json:"insecure"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("bad status body %q: %v", rec.Body, err)
		}
		return out.Insecure
	}
	_, lan, _ := net.ParseCIDR("10.0.0.0/8")
	cases := []struct {
		name       string
		remoteAddr string
		header     func(*http.Request)
		trusted    []*net.IPNet
		insecure   bool
	}{
		{"plain http from lan", "192.168.1.5:1234", nil, nil, true},
		{"https behind trusted proxy", "10.0.0.2:5", func(r *http.Request) { r.Header.Set("X-Forwarded-Proto", "https") }, []*net.IPNet{lan}, false},
		{"http on loopback", "127.0.0.1:8080", nil, nil, false},
	}
	for _, tc := range cases {
		if got := status(tc.remoteAddr, tc.header, tc.trusted); got != tc.insecure {
			t.Errorf("%s: insecure=%v want %v", tc.name, got, tc.insecure)
		}
	}
}

func TestSessionCookieAttributes(t *testing.T) {
	a := testAPI(t)
	seedAdmin(t, a)
	rec := postJSON(a.Login, "/api/v1/auth/login", loginBody("admin", "password123"), nil)
	cookie := sessionCookie(t, rec)
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie attributes wrong: %+v", cookie)
	}
	if cookie.Secure {
		t.Fatal("plain http login must not set Secure")
	}
	// A login observed as https (trusted proxy) sets Secure.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginBody("admin", "password123")))
	req.Header.Set("X-Requested-With", "fetch")
	req.RemoteAddr = "10.0.0.2:5"
	req.Header.Set("X-Forwarded-Proto", "https")
	_, lan, _ := net.ParseCIDR("10.0.0.0/8")
	req = auth.WithRequestMeta(req, []*net.IPNet{lan})
	rec = httptest.NewRecorder()
	a.Login(rec, req)
	if rec.Code != 200 {
		t.Fatalf("login status=%d body=%s", rec.Code, rec.Body)
	}
	if c := sessionCookie(t, rec); !c.Secure {
		t.Fatal("https login must set Secure cookie")
	}
}
