package server

import "net/http"

func (s *Server) handleAuthNew(w http.ResponseWriter, r *http.Request)        { http.NotFound(w, r) }
func (s *Server) handleAuthCreate(w http.ResponseWriter, r *http.Request)     { http.NotFound(w, r) }
func (s *Server) handleAuthVerifyForm(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }
func (s *Server) handleAuthVerify(w http.ResponseWriter, r *http.Request)     { http.NotFound(w, r) }
func (s *Server) handleAuthDestroy(w http.ResponseWriter, r *http.Request)    { http.NotFound(w, r) }
