package server

import (
	"net/http"

	"rulinky/internal/store"
)

func (s *Server) handleAPIIndex(w http.ResponseWriter, r *http.Request, _ store.User) {
	http.NotFound(w, r)
}
func (s *Server) handleAPICreate(w http.ResponseWriter, r *http.Request, _ store.User) {
	http.NotFound(w, r)
}
func (s *Server) handleAPIUpdate(w http.ResponseWriter, r *http.Request, _ store.User) {
	http.NotFound(w, r)
}
func (s *Server) handleAPIDestroy(w http.ResponseWriter, r *http.Request, _ store.User) {
	http.NotFound(w, r)
}
