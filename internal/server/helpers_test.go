package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"rulinky/internal/auth"
	"rulinky/internal/store"
)

type fakeOTP struct{ codes []string }

func (f *fakeOTP) SendOTP(_ context.Context, code string) error {
	f.codes = append(f.codes, code)
	return nil
}

type fakeJobs struct{ n int }

func (f *fakeJobs) Notify() { f.n++ }

const testIndex = `<!DOCTYPE html><html><head><!-- PAGE_DATA --></head><body><div id="app"></div></body></html>`

func newTestServer(t *testing.T) (*Server, *fakeOTP, *fakeJobs) {
	t.Helper()
	pages, err := NewPages([]byte(testIndex))
	if err != nil {
		t.Fatal(err)
	}
	otp, jobs := &fakeOTP{}, &fakeJobs{}
	srv := &Server{
		Store:    store.OpenTest(t),
		Sessions: auth.NewSessions([]byte(strings.Repeat("k", 32)), false),
		OTP:      otp,
		Jobs:     jobs,
		Pages:    pages,
		Static: fstest.MapFS{
			"favicon.svg":       {Data: []byte("<svg/>")},
			"assets/app-123.js": {Data: []byte("console.log(1)")},
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	return srv, otp, jobs
}

func serve(srv *Server, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func jsonRequest(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return req
}

func createUser(t *testing.T, srv *Server, email string) store.User {
	t.Helper()
	u, err := srv.Store.FindOrCreateUserByEmail(context.Background(), email)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func sessionCookie(t *testing.T, srv *Server, sess auth.Session) *http.Cookie {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Sessions.Write(rec, sess)
	return rec.Result().Cookies()[0]
}

func cookieFrom(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	var found *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName {
			found = c
		}
	}
	if found == nil {
		t.Fatal("response set no session cookie")
	}
	return found
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return v
}
