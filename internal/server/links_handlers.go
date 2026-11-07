package server

import (
	"net/http"

	"rulinky/internal/store"
)

func (s *Server) handleLinksPage(w http.ResponseWriter, r *http.Request, _ store.User) {
	http.NotFound(w, r)
}
func (s *Server) handleCreateLink(w http.ResponseWriter, r *http.Request, _ store.User) {
	http.NotFound(w, r)
}
