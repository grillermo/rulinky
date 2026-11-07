package server

import (
	"net/http"
	"strings"

	"rulinky/internal/auth"
)

func (s *Server) handleAuthNew(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "Auth/New", map[string]any{})
}

func (s *Server) handleAuthCreate(w http.ResponseWriter, r *http.Request) {
	params, err := readParams(r)
	if err != nil {
		writeParamsError(w, err)
		return
	}
	email := strings.TrimSpace(strings.ToLower(stringParam(params, "email")))
	if !auth.ValidEmail(email) {
		writeJSON(w, http.StatusUnprocessableEntity, errorsBody("email", "Enter a valid email"))
		return
	}
	ctx := r.Context()
	user, err := s.Store.FindOrCreateUserByEmail(ctx, email)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	code, err := auth.NewOTP()
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if err := s.Store.SetOTP(ctx, user.ID, code); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.Logger.Info("[OTP]", "email", email, "code", code)
	if err := s.OTP.SendOTP(ctx, code); err != nil {
		s.Logger.Error("OTP delivery failed", "err", err)
	}
	sess := s.Sessions.Read(r)
	sess.AuthEmail = email
	s.Sessions.Write(w, sess)
	writeJSON(w, http.StatusOK, map[string]string{"redirect": "/auth/verify"})
}

func (s *Server) handleAuthVerifyForm(w http.ResponseWriter, r *http.Request) {
	sess := s.Sessions.Read(r)
	if sess.AuthEmail == "" {
		http.Redirect(w, r, "/auth/new", http.StatusFound)
		return
	}
	s.render(w, r, "Auth/Verify", map[string]string{"email": sess.AuthEmail})
}

func (s *Server) handleAuthVerify(w http.ResponseWriter, r *http.Request) {
	params, err := readParams(r)
	if err != nil {
		writeParamsError(w, err)
		return
	}
	sess := s.Sessions.Read(r)
	invalid := func() {
		writeJSON(w, http.StatusUnprocessableEntity, errorsBody("code", "Invalid or expired code"))
	}
	if sess.AuthEmail == "" {
		invalid()
		return
	}
	user, ok, err := s.Store.VerifyOTP(r.Context(), sess.AuthEmail, strings.TrimSpace(stringParam(params, "code")), clientIP(r))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if !ok {
		invalid()
		return
	}
	s.Sessions.Write(w, auth.Session{UserID: user.ID})
	writeJSON(w, http.StatusOK, map[string]string{"redirect": "/"})
}

func (s *Server) handleAuthDestroy(w http.ResponseWriter, r *http.Request) {
	s.Sessions.Clear(w)
	writeJSON(w, http.StatusOK, map[string]string{"redirect": "/auth/new"})
}
