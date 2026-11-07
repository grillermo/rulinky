package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const pageMarker = "<!-- PAGE_DATA -->"

// Pages serves the Vite-built index.html with the page component and its
// props injected as window.__PAGE__ (this replaces Inertia).
type Pages struct {
	before, after string
}

func NewPages(index []byte) (*Pages, error) {
	before, after, ok := strings.Cut(string(index), pageMarker)
	if !ok {
		return nil, fmt.Errorf("index.html has no %q marker; rebuild the frontend with: pnpm run build", pageMarker)
	}
	return &Pages{before: before, after: after}, nil
}

func (p *Pages) Render(w http.ResponseWriter, component string, props any) error {
	// json.Marshal escapes <, > and &, so the data cannot close the script tag.
	data, err := json.Marshal(map[string]any{"component": component, "props": props})
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, err = io.WriteString(w, p.before+"<script>window.__PAGE__ = "+string(data)+"</script>"+p.after)
	return err
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, component string, props any) {
	if err := s.Pages.Render(w, component, props); err != nil {
		s.serverError(w, r, err)
	}
}
