package server

import (
	"net/http"
	"strings"
	"time"

	"rulinky/internal/store"
	"rulinky/internal/titles"
)

type pageLink struct {
	ID          string  `json:"id"`
	URL         string  `json:"url"`
	Title       string  `json:"title"`
	FullTitle   string  `json:"fullTitle"`
	Note        *string `json:"note"`
	Read        bool    `json:"read"`
	UpdatedAt   string  `json:"updatedAt"`
	UpdatedAtMs int64   `json:"updatedAtMs"`
}

type linksProps struct {
	Links       []pageLink `json:"links"`
	ReadCount   int        `json:"readCount"`
	UnreadCount int        `json:"unreadCount"`
	APIToken    string     `json:"apiToken"`
}

func buildLinksProps(links []store.Link, apiToken string) linksProps {
	// make() keeps an empty list serializing as [], never null.
	props := linksProps{Links: make([]pageLink, 0, len(links)), APIToken: apiToken}
	for _, l := range links {
		read := l.Read == 1
		if read {
			props.ReadCount++
		} else {
			props.UnreadCount++
		}
		full := titles.Full(l.URL, l.RawTitle, l.Content)
		props.Links = append(props.Links, pageLink{
			ID: l.ID, URL: l.URL, Title: titles.Truncate(full, 50), FullTitle: full,
			Note: l.Note, Read: read, UpdatedAt: localTime(l.UpdatedAt), UpdatedAtMs: l.UpdatedAt,
		})
	}
	return props
}

// localTime formats epoch milliseconds like the Rails helper: "3/9/2026, 4:05:06 PM".
func localTime(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).Local().Format("1/2/2006, 3:04:05 PM")
}

// handleLinksPage renders the app, or returns just its props to
// Accept: application/json (the UI reloads the list that way).
func (s *Server) handleLinksPage(w http.ResponseWriter, r *http.Request, user store.User) {
	links, err := s.Store.ListLinks(r.Context(), user.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	props := buildLinksProps(links, user.APIToken)
	if wantsJSON(r) {
		writeJSON(w, http.StatusOK, props)
		return
	}
	s.render(w, r, "Links/Index", props)
}

func (s *Server) handleCreateLink(w http.ResponseWriter, r *http.Request, user store.User) {
	params, err := readParams(r)
	if err != nil {
		writeParamsError(w, err)
		return
	}
	url := strings.TrimSpace(stringParam(params, "url"))
	if url == "" {
		writeJSON(w, http.StatusUnprocessableEntity, errorsBody("url", "Enter a link"))
		return
	}
	link, _, err := s.Store.CreateLink(r.Context(), user.ID, url, stringParam(params, "note"))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.Jobs.Notify()
	writeJSON(w, http.StatusCreated, map[string]string{"id": link.ID})
}
