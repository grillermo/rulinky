package server

import (
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"rulinky/internal/store"
)

type userHandler func(http.ResponseWriter, *http.Request, store.User)

// requireUser passes the signed-in user to next and renews the session
// cookie (sliding 10-day timeout). Without a session it redirects to
// /auth/new, or answers 401 {"redirect": "/auth/new"} to JSON requests.
func (s *Server) requireUser(next userHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := s.Sessions.Read(r)
		if sess.UserID != 0 {
			user, ok, err := s.Store.UserByID(r.Context(), sess.UserID)
			if err != nil {
				s.serverError(w, r, err)
				return
			}
			if ok {
				s.Sessions.Write(w, sess)
				next(w, r, user)
				return
			}
			s.Sessions.Clear(w)
		}
		if wantsJSON(r) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"redirect": "/auth/new"})
			return
		}
		http.Redirect(w, r, "/auth/new", http.StatusFound)
	})
}

// requireToken authenticates API calls with "Authorization: <token>" or
// "Authorization: Bearer <token>".
func (s *Server) requireToken(next userHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if token != "" {
			user, ok, err := s.Store.UserByAPIToken(r.Context(), token)
			if err != nil {
				s.serverError(w, r, err)
				return
			}
			if ok {
				next(w, r, user)
				return
			}
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
	})
}

func (s *Server) serverError(w http.ResponseWriter, r *http.Request, err error) {
	s.Logger.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	if strings.HasPrefix(r.URL.Path, "/api/") || wantsJSON(r) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal Server Error"})
		return
	}
	http.Error(w, "Internal Server Error", http.StatusInternalServerError)
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		if s.Production {
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/up" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.Logger.Info("request", "method", r.Method, "path", r.URL.Path, "status", rec.status, "ms", time.Since(start).Milliseconds())
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				s.Logger.Error("panic", "err", v, "path", r.URL.Path, "stack", string(debug.Stack()))
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
