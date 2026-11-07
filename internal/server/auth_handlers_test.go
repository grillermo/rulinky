package server

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
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

func TestAuthCreateLogsOTP(t *testing.T) {
	srv, otp, _ := newTestServer(t)
	var buf bytes.Buffer
	srv.Logger = slog.New(slog.NewTextHandler(&buf, nil))
	rec := serve(srv, jsonRequest("POST", "/auth", `{"email":"log@example.com"}`))
	if rec.Code != 200 || len(otp.codes) != 1 {
		t.Fatalf("%d %s codes=%v", rec.Code, rec.Body.String(), otp.codes)
	}
	out := buf.String()
	if !strings.Contains(out, "[OTP]") || !strings.Contains(out, "log@example.com") || !strings.Contains(out, otp.codes[0]) {
		t.Fatalf("log missing OTP line, email or code: %s", out)
	}
}

func decodeCookie(t *testing.T, srv *Server, c *http.Cookie) auth.Session {
	t.Helper()
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(c)
	return srv.Sessions.Read(req)
}

func TestAuthCreateSucceedsWhenOTPDeliveryFails(t *testing.T) {
	srv, otp, _ := newTestServer(t)
	otp.err = errors.New("slack down")
	rec := serve(srv, jsonRequest("POST", "/auth", `{"email":"me@example.com"}`))
	if rec.Code != 200 || decode[map[string]string](t, rec)["redirect"] != "/auth/verify" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if got := decodeCookie(t, srv, cookieFrom(t, rec)); got.AuthEmail != "me@example.com" {
		t.Fatalf("pending session: %+v", got)
	}
}

func TestAuthVerifyClearsPendingEmailAndSetsUser(t *testing.T) {
	srv, otp, _ := newTestServer(t)
	rec := serve(srv, jsonRequest("POST", "/auth", `{"email":"me@example.com"}`))
	req := jsonRequest("POST", "/auth/verify", `{"code":"`+otp.codes[0]+`"}`)
	req.AddCookie(cookieFrom(t, rec))
	rec = serve(srv, req)
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	got := decodeCookie(t, srv, cookieFrom(t, rec))
	if got.AuthEmail != "" || got.UserID == 0 {
		t.Fatalf("session after verify: %+v", got)
	}
}

func TestAuthVerifyWithoutPendingSessionIs422(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rec := serve(srv, jsonRequest("POST", "/auth/verify", `{"code":"123456"}`))
	if rec.Code != 422 || decode[map[string]map[string]string](t, rec)["errors"]["code"] != "Invalid or expired code" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestAuthVerifyRejectsReusedCode(t *testing.T) {
	srv, otp, _ := newTestServer(t)
	rec := serve(srv, jsonRequest("POST", "/auth", `{"email":"me@example.com"}`))
	pending := cookieFrom(t, rec)
	body := `{"code":"` + otp.codes[0] + `"}`
	req := jsonRequest("POST", "/auth/verify", body)
	req.AddCookie(pending)
	if rec := serve(srv, req); rec.Code != 200 {
		t.Fatalf("first use: %d %s", rec.Code, rec.Body.String())
	}
	req = jsonRequest("POST", "/auth/verify", body)
	req.AddCookie(pending)
	if rec := serve(srv, req); rec.Code != 422 {
		t.Fatalf("reuse: %d %s", rec.Code, rec.Body.String())
	}
}

func TestLoginCookieFlags(t *testing.T) {
	srv, _, _ := newTestServer(t)
	c := cookieFrom(t, serve(srv, jsonRequest("POST", "/auth", `{"email":"me@example.com"}`)))
	if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" {
		t.Fatalf("cookie flags: %+v", c)
	}
}
