package server

import (
	"net/http/httptest"
	"strings"
	"testing"

	"rulinky/internal/auth"
)

func TestUp(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rec := serve(srv, httptest.NewRequest("GET", "/up", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "background-color: green") {
		t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestStaticFilesAndCaching(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rec := serve(srv, httptest.NewRequest("GET", "/favicon.svg", nil))
	if rec.Code != 200 || rec.Body.String() != "<svg/>" {
		t.Fatalf("favicon: %d %q", rec.Code, rec.Body.String())
	}
	rec = serve(srv, httptest.NewRequest("GET", "/assets/app-123.js", nil))
	if rec.Code != 200 || !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("asset: %d cache=%q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	if rec := serve(srv, httptest.NewRequest("GET", "/nope.txt", nil)); rec.Code != 404 {
		t.Fatalf("missing file: %d", rec.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rec := serve(srv, httptest.NewRequest("GET", "/up", nil))
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("X-Frame-Options") != "SAMEORIGIN" {
		t.Fatalf("headers: %v", rec.Header())
	}
	if rec.Header().Get("Strict-Transport-Security") != "" {
		t.Fatal("HSTS must be off outside production")
	}
	srv.Production = true
	rec = serve(srv, httptest.NewRequest("GET", "/up", nil))
	if !strings.HasPrefix(rec.Header().Get("Strict-Transport-Security"), "max-age=") {
		t.Fatal("HSTS missing in production")
	}
}

func TestPagesRequireMarker(t *testing.T) {
	if _, err := NewPages([]byte("<html></html>")); err == nil {
		t.Fatal("expected an error without the PAGE_DATA marker")
	}
}

func TestCastBool(t *testing.T) {
	cases := []struct {
		in       any
		want, ok bool
	}{
		{true, true, true}, {false, false, true},
		{float64(1), true, true}, {float64(0), false, true},
		{"true", true, true}, {"1", true, true}, {"yes", true, true}, {"False", true, true},
		{"false", false, true}, {"FALSE", false, true}, {"0", false, true}, {"f", false, true},
		{"F", false, true}, {"off", false, true}, {"OFF", false, true},
		{"", false, false}, {nil, false, false},
	}
	for _, c := range cases {
		got, ok := castBool(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("castBool(%#v) = %v,%v want %v,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

// Added beyond the plan: middleware behavior.
func TestRequireUser(t *testing.T) {
	srv, _, _ := newTestServer(t)
	u := createUser(t, srv, "a@example.com")

	// anonymous: redirect for HTML, 401 JSON otherwise
	rec := serve(srv, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 302 || rec.Header().Get("Location") != "/auth/new" {
		t.Fatalf("anon html: %d %v", rec.Code, rec.Header())
	}
	rec = serve(srv, jsonRequest("GET", "/", ""))
	if rec.Code != 401 || !strings.Contains(rec.Body.String(), "/auth/new") {
		t.Fatalf("anon json: %d %q", rec.Code, rec.Body.String())
	}

	// signed in: passes through and renews the cookie (10-day Max-Age)
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(sessionCookie(t, srv, auth.Session{UserID: u.ID}))
	rec = serve(srv, req)
	if rec.Code == 302 || rec.Code == 401 {
		t.Fatalf("signed-in request was rejected: %d", rec.Code)
	}
	if c := cookieFrom(t, rec); c.MaxAge != int(auth.Timeout.Seconds()) || c.Value == "" {
		t.Fatalf("session not renewed: %+v", c)
	}

	// unknown user id: cookie cleared and redirected
	req = httptest.NewRequest("GET", "/", nil)
	req.AddCookie(sessionCookie(t, srv, auth.Session{UserID: u.ID + 9999}))
	rec = serve(srv, req)
	if rec.Code != 302 || cookieFrom(t, rec).MaxAge >= 0 {
		t.Fatalf("stale user: %d", rec.Code)
	}
}

func TestRequireToken(t *testing.T) {
	srv, _, _ := newTestServer(t)
	u := createUser(t, srv, "a@example.com")
	rec := serve(srv, jsonRequest("GET", "/api/links", ""))
	if rec.Code != 401 || !strings.Contains(rec.Body.String(), "Unauthorized") {
		t.Fatalf("no token: %d %q", rec.Code, rec.Body.String())
	}
	for _, h := range []string{u.APIToken, "Bearer " + u.APIToken} {
		req := jsonRequest("GET", "/api/links", "")
		req.Header.Set("Authorization", h)
		if rec := serve(srv, req); rec.Code == 401 {
			t.Fatalf("valid token rejected (%q form)", strings.Fields(h)[0])
		}
	}
}
