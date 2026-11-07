// Package server is Rulinky's HTTP layer.
package server

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"rulinky/internal/auth"
	"rulinky/internal/store"
)

type OTPSender interface {
	SendOTP(ctx context.Context, code string) error
}

type JobNotifier interface {
	Notify()
}

type Server struct {
	Store      *store.Store
	Sessions   *auth.Sessions
	OTP        OTPSender
	Jobs       JobNotifier
	Pages      *Pages
	Static     fs.FS
	Logger     *slog.Logger
	Production bool
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	csrf := http.NewCrossOriginProtection()

	mux.HandleFunc("GET /up", s.handleUp)

	mux.Handle("GET /{$}", s.requireUser(s.handleLinksPage))
	mux.Handle("POST /links", csrf.Handler(s.requireUser(s.handleCreateLink)))

	mux.HandleFunc("GET /auth/new", s.handleAuthNew)
	mux.Handle("POST /auth", csrf.Handler(http.HandlerFunc(s.handleAuthCreate)))
	mux.HandleFunc("GET /auth/verify", s.handleAuthVerifyForm)
	mux.Handle("POST /auth/verify", csrf.Handler(http.HandlerFunc(s.handleAuthVerify)))
	mux.Handle("DELETE /auth", csrf.Handler(http.HandlerFunc(s.handleAuthDestroy)))

	mux.Handle("GET /api/links", s.requireToken(s.handleAPIIndex))
	mux.Handle("POST /api/links", s.requireToken(s.handleAPICreate))
	mux.Handle("PATCH /api/links", s.requireToken(s.handleAPIUpdate))
	mux.Handle("DELETE /api/links", s.requireToken(s.handleAPIDestroy))

	mux.Handle("GET /", s.staticHandler())

	return s.recoverer(s.logRequests(s.securityHeaders(s.limitBody(mux))))
}

func (s *Server) handleUp(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!DOCTYPE html><html><body style="background-color: green"></body></html>`))
}

// staticHandler serves real asset files only: no directory listings, no
// dotfiles, and no raw index.html (its PAGE_DATA marker is only filled in by
// Pages, which serves every HTML page).
func (s *Server) staticHandler() http.Handler {
	files := http.FileServerFS(s.Static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" || name == "index.html" || strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		for _, seg := range strings.Split(name, "/") {
			if strings.HasPrefix(seg, ".") {
				http.NotFound(w, r)
				return
			}
		}
		if info, err := fs.Stat(s.Static, name); err != nil || info.IsDir() {
			http.NotFound(w, r)
			return
		}
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		files.ServeHTTP(w, r)
	})
}
