package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"rulinky/internal/store"
)

func TestRenderEscapesPageData(t *testing.T) {
	pages, err := NewPages([]byte(testIndex))
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	props := map[string]string{"x": "</script><b>&  "}
	if err := pages.Render(rec, "Links", props); err != nil {
		t.Fatal(err)
	}
	out := rec.Body.String()
	if strings.Count(out, "</script>") != 1 {
		t.Fatalf("raw </script> leaked: %q", out)
	}
	for _, bad := range []string{"<b>", "&", " ", " "} {
		if strings.Contains(out, bad) {
			t.Fatalf("%q appears raw in %q", bad, out)
		}
	}
}

func TestRequireTokenHandlerRuns(t *testing.T) {
	srv, _, _ := newTestServer(t)
	u := createUser(t, srv, "a@example.com")
	var ran string
	h := srv.requireToken(func(w http.ResponseWriter, r *http.Request, got store.User) {
		ran = got.Email
		w.WriteHeader(204)
	})
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 401 || ran != "" {
		t.Fatalf("wrong token: %d ran=%q", rec.Code, ran)
	}
	req = httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer "+u.APIToken)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 204 || ran != u.Email {
		t.Fatalf("valid token: %d ran=%q", rec.Code, ran)
	}
}

func TestCrossOriginPostForbidden(t *testing.T) {
	srv, _, _ := newTestServer(t)
	req := httptest.NewRequest("POST", "/auth", strings.NewReader("email=a@example.com"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	if rec := serve(srv, req); rec.Code != 403 {
		t.Fatalf("cross-site POST: %d", rec.Code)
	}
}

func TestRecovererReturnsGeneric500(t *testing.T) {
	srv, _, _ := newTestServer(t)
	h := srv.recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("secret detail") }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/boom", nil))
	if rec.Code != 500 || strings.Contains(rec.Body.String(), "secret") || !strings.Contains(rec.Body.String(), "Internal Server Error") {
		t.Fatalf("recoverer: %d %q", rec.Code, rec.Body.String())
	}
}

func TestStaticHardening(t *testing.T) {
	srv, _, _ := newTestServer(t)
	srv.Static = fstest.MapFS{
		"index.html":        {Data: []byte(testIndex)},
		"favicon.svg":       {Data: []byte("<svg/>")},
		"assets/app-123.js": {Data: []byte("console.log(1)")},
		".env":              {Data: []byte("SECRET=1")},
		"assets/.hidden":    {Data: []byte("x")},
		".git/config":       {Data: []byte("x")},
	}
	for _, p := range []string{
		"/assets/", "/assets", "/index.html", "/index.html/", "/.env", "/assets/.hidden",
		"/.git/config", "/../etc/passwd", "/assets/../.env", "/%2e%2e/etc/passwd", "/%2eenv", "/assets/%2ehidden",
	} {
		rec := serve(srv, httptest.NewRequest("GET", p, nil))
		// ServeMux itself redirects unclean paths (307) to the cleaned one,
		// which is then covered by its own 404 case.
		if (rec.Code != 404 && rec.Code != 307) || strings.Contains(rec.Body.String(), "PAGE_DATA") || strings.Contains(rec.Body.String(), "SECRET") {
			t.Errorf("GET %s: %d %q", p, rec.Code, rec.Body.String())
		}
	}
	rec := serve(srv, httptest.NewRequest("GET", "/assets/app-123.js", nil))
	if rec.Code != 200 || !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("asset: %d", rec.Code)
	}
}

func TestSecurityHeadersCSP(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rec := serve(srv, httptest.NewRequest("GET", "/up", nil))
	csp := rec.Header().Get("Content-Security-Policy")
	for _, want := range []string{"default-src 'self'", "script-src 'self' 'unsafe-inline'", "frame-ancestors 'none'", "object-src 'none'", "form-action 'self'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP %q lacks %q", csp, want)
		}
	}
}

func TestWriteJSONNoStore(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, 200, map[string]string{"a": "b"})
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache-control: %q", rec.Header().Get("Cache-Control"))
	}
}

func TestBodyLimit(t *testing.T) {
	srv, _, _ := newTestServer(t)
	h := srv.limitBody(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := readParams(r); err != nil {
			writeParamsError(w, err)
			return
		}
		w.WriteHeader(200)
	}))
	big := strings.Repeat("a", 2<<20)
	cases := map[string]struct{ ctype, body string }{
		"json": {"application/json", `{"k":"` + big + `"}`},
		"form": {"application/x-www-form-urlencoded", "k=" + big},
	}
	for name, c := range cases {
		req := httptest.NewRequest("POST", "/x", strings.NewReader(c.body))
		req.Header.Set("Content-Type", c.ctype)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 413 {
			t.Errorf("%s: got %d %q", name, rec.Code, rec.Body.String())
		}
	}
	req := httptest.NewRequest("POST", "/x", strings.NewReader(`{"k":"v"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("small body: %d", rec.Code)
	}
}
