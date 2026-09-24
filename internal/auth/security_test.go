package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLimiterAndCSRF(t *testing.T) {
	l := NewLoginLimiter()
	for i := 0; i < 9; i++ {
		l.Fail("127.0.0.1")
		if ok, _ := l.Allow("127.0.0.1"); !ok {
			t.Fatalf("failure %d unexpectedly limited", i)
		}
	}
	l.Fail("127.0.0.1") // tenth consecutive failure trips the limit
	if ok, _ := l.Allow("127.0.0.1"); ok {
		t.Fatal("limit not enforced")
	}
	// Successful requests never count toward the limit and clear the streak.
	l.Reset("127.0.0.1")
	if ok, _ := l.Allow("127.0.0.1"); !ok {
		t.Fatal("reset did not clear the limit")
	}
	handler := CSRF(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	r := httptest.NewRequest("POST", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)
	if rec.Code != 403 {
		t.Fatalf("csrf status %d", rec.Code)
	}
	r.Header.Set("X-Requested-With", "fetch")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, r)
	if rec.Code != 204 {
		t.Fatalf("csrf header status %d", rec.Code)
	}
}
