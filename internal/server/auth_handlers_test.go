package server

import (
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"rulinky/internal/auth"
)

func TestSignInFlow(t *testing.T) {
	srv, otp, _ := newTestServer(t)

	rec := serve(srv, jsonRequest("POST", "/auth", `{"email":"  Me@Example.com "}`))
	if rec.Code != 200 || decode[map[string]string](t, rec)["redirect"] != "/auth/verify" {
		t.Fatalf("POST /auth: %d %s", rec.Code, rec.Body.String())
	}
	if len(otp.codes) != 1 || !regexp.MustCompile(`^\d{6}$`).MatchString(otp.codes[0]) {
		t.Fatalf("codes sent: %v", otp.codes)
	}
	pending := cookieFrom(t, rec)

	req := httptest.NewRequest("GET", "/auth/verify", nil)
	req.AddCookie(pending)
	rec = serve(srv, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"component":"Auth/Verify"`) ||
		!strings.Contains(rec.Body.String(), `"email":"me@example.com"`) {
		t.Fatalf("verify form: %d %s", rec.Code, rec.Body.String())
	}

	req = jsonRequest("POST", "/auth/verify", `{"code":"000000x"}`)
	req.AddCookie(pending)
	rec = serve(srv, req)
	if rec.Code != 422 || decode[map[string]map[string]string](t, rec)["errors"]["code"] != "Invalid or expired code" {
		t.Fatalf("wrong code: %d %s", rec.Code, rec.Body.String())
	}

	req = jsonRequest("POST", "/auth/verify", `{"code":" `+otp.codes[0]+` "}`)
	req.AddCookie(pending)
	rec = serve(srv, req)
	if rec.Code != 200 || decode[map[string]string](t, rec)["redirect"] != "/" {
		t.Fatalf("right code: %d %s", rec.Code, rec.Body.String())
	}
	signedIn := cookieFrom(t, rec)

	req = httptest.NewRequest("GET", "/", nil)
	req.AddCookie(signedIn)
	if rec := serve(srv, req); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"component":"Links/Index"`) {
		t.Fatalf("GET / after sign-in: %d", rec.Code)
	}

	req = jsonRequest("DELETE", "/auth", "")
	req.AddCookie(signedIn)
	rec = serve(srv, req)
	if rec.Code != 200 || decode[map[string]string](t, rec)["redirect"] != "/auth/new" || cookieFrom(t, rec).MaxAge >= 0 {
		t.Fatalf("sign out: %d %s", rec.Code, rec.Body.String())
	}
}

func TestAuthCreateRejectsInvalidEmail(t *testing.T) {
	srv, otp, _ := newTestServer(t)
	rec := serve(srv, jsonRequest("POST", "/auth", `{"email":"nope"}`))
	if rec.Code != 422 || decode[map[string]map[string]string](t, rec)["errors"]["email"] != "Enter a valid email" || len(otp.codes) != 0 {
		t.Fatalf("%d %s codes=%v", rec.Code, rec.Body.String(), otp.codes)
	}
}

func TestPagesRedirectWhenSignedOut(t *testing.T) {
	srv, _, _ := newTestServer(t)

	rec := serve(srv, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 302 || rec.Header().Get("Location") != "/auth/new" {
		t.Fatalf("GET /: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept", "application/json")
	if rec := serve(srv, req); rec.Code != 401 {
		t.Fatalf("GET / json: %d", rec.Code)
	}
	rec = serve(srv, httptest.NewRequest("GET", "/auth/verify", nil))
	if rec.Code != 302 || rec.Header().Get("Location") != "/auth/new" {
		t.Fatalf("GET /auth/verify without pending email: %d", rec.Code)
	}
	rec = serve(srv, httptest.NewRequest("GET", "/auth/new", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"component":"Auth/New"`) {
		t.Fatalf("GET /auth/new: %d", rec.Code)
	}
}

func TestSessionForDeletedUserRedirects(t *testing.T) {
	srv, _, _ := newTestServer(t)
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(sessionCookie(t, srv, auth.Session{UserID: 424242}))
	if rec := serve(srv, req); rec.Code != 302 {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestCrossSiteFormPostIsRejected(t *testing.T) {
	srv, otp, _ := newTestServer(t)
	req := jsonRequest("POST", "/auth", `{"email":"me@example.com"}`)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	if rec := serve(srv, req); rec.Code != 403 || len(otp.codes) != 0 {
		t.Fatalf("got %d, codes %v", rec.Code, otp.codes)
	}
}
