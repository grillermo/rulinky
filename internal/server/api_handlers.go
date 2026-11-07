package server

import (
	"net/http"
	"strings"

	"rulinky/internal/store"
	"rulinky/internal/titles"
)

type apiLink struct {
	ID           string  `json:"id"`
	URL          string  `json:"url"`
	Note         *string `json:"note"`
	Read         int     `json:"read"`
	Timestamp    int64   `json:"timestamp"`
	UpdatedAt    int64   `json:"updated_at"`
	Title        string  `json:"title"`
	ContentTitle *string `json:"content_title"`
}

func (s *Server) handleAPIIndex(w http.ResponseWriter, r *http.Request, user store.User) {
	links, err := s.Store.ListLinks(r.Context(), user.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	out := make([]apiLink, 0, len(links))
	for _, l := range links {
		var contentTitle *string
		if t := titles.ContentTitle(l.RawTitle, l.Content); t != "" {
			contentTitle = &t
		}
		out = append(out, apiLink{
			ID: l.ID, URL: l.URL, Note: l.Note, Read: l.Read, Timestamp: l.Timestamp, UpdatedAt: l.UpdatedAt,
			Title: titles.Full(l.URL, l.RawTitle, l.Content), ContentTitle: contentTitle,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func apiError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

type apiCreated struct {
	Message string `json:"message"`
	ID      string `json:"id"`
	Note    string `json:"note"`
	JobID   string `json:"job_id"`
	Title   string `json:"title"`
}

func (s *Server) handleAPICreate(w http.ResponseWriter, r *http.Request, user store.User) {
	params, err := readParams(r)
	if err != nil {
		writeParamsError(w, err)
		return
	}
	link, isString := params["link"].(string)
	if !isString || strings.TrimSpace(link) == "" {
		apiError(w, http.StatusBadRequest, "Invalid link")
		return
	}
	// Rails stores the link exactly as sent (it only strip-checks for blank), so no trimming here.
	note := stringParam(params, "note")
	created, jobID, err := s.Store.CreateLink(r.Context(), user.ID, link, note)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.Jobs.Notify()
	writeJSON(w, http.StatusCreated, apiCreated{
		Message: "Link saved", ID: created.ID, Note: note, JobID: jobID, Title: titles.Full(created.URL, nil, nil),
	})
}

func (s *Server) handleAPIUpdate(w http.ResponseWriter, r *http.Request, user store.User) {
	params, err := readParams(r)
	if err != nil {
		writeParamsError(w, err)
		return
	}
	id, idOK := params["id"].(string)
	read, readOK := castBool(params["read"])
	if !idOK || strings.TrimSpace(id) == "" || !readOK {
		apiError(w, http.StatusBadRequest, "Invalid payload")
		return
	}
	found, err := s.Store.SetLinkRead(r.Context(), user.ID, id, read)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if !found {
		apiError(w, http.StatusNotFound, "Link not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Link updated"})
}

func (s *Server) handleAPIDestroy(w http.ResponseWriter, r *http.Request, user store.User) {
	params, err := readParams(r)
	if err != nil {
		writeParamsError(w, err)
		return
	}
	id, idOK := params["id"].(string)
	if !idOK || strings.TrimSpace(id) == "" {
		apiError(w, http.StatusBadRequest, "Invalid payload")
		return
	}
	found, err := s.Store.DeleteLink(r.Context(), user.ID, id)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if !found {
		apiError(w, http.StatusNotFound, "Link not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Link deleted"})
}
