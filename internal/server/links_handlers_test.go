package server

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"rulinky/internal/auth"
)

type pageLinkJSON struct {
	ID          string  `json:"id"`
	URL         string  `json:"url"`
	Title       string  `json:"title"`
	FullTitle   string  `json:"fullTitle"`
	Note        *string `json:"note"`
	Read        bool    `json:"read"`
	UpdatedAt   string  `json:"updatedAt"`
	UpdatedAtMs int64   `json:"updatedAtMs"`
}

type linksPropsJSON struct {
	Links       []pageLinkJSON `json:"links"`
	ReadCount   int            `json:"readCount"`
	UnreadCount int            `json:"unreadCount"`
	APIToken    string         `json:"apiToken"`
}

func TestLinksPageProps(t *testing.T) {
	srv, _, _ := newTestServer(t)
	ctx := context.Background()
	u := createUser(t, srv, "me@example.com")
	plain, _, _ := srv.Store.CreateLink(ctx, u.ID, "https://www.plain.com/x?q=1", "a note")
	titled, _, _ := srv.Store.CreateLink(ctx, u.ID, "https://titled.com", "")
	long := strings.Repeat("L", 60)
	_, _ = srv.Store.Pool().Exec(ctx, `UPDATE links SET raw_title = $2 WHERE id = $1`, titled.ID, long)
	time.Sleep(3 * time.Millisecond) // so marking read gives plain a strictly newer updated_at
	_, _ = srv.Store.SetLinkRead(ctx, u.ID, plain.ID, true)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept", "application/json")
	req.AddCookie(sessionCookie(t, srv, auth.Session{UserID: u.ID}))
	rec := serve(srv, req)
	if rec.Code != 200 {
		t.Fatalf("code %d", rec.Code)
	}
	props := decode[linksPropsJSON](t, rec)
	if props.APIToken != u.APIToken || props.ReadCount != 1 || props.UnreadCount != 1 || len(props.Links) != 2 {
		t.Fatalf("props %+v", props)
	}
	first, second := props.Links[0], props.Links[1] // plain was updated last (marked read)
	if first.ID != plain.ID || first.Title != "plain.com/x" || first.FullTitle != "plain.com/x" || !first.Read ||
		first.Note == nil || *first.Note != "a note" || first.UpdatedAtMs < plain.UpdatedAt || first.UpdatedAt == "" {
		t.Errorf("plain link %+v", first)
	}
	if second.ID != titled.ID || second.FullTitle != long || len(second.Title) != 50 || !strings.HasSuffix(second.Title, "...") {
		t.Errorf("titled link %+v", second)
	}
}

func TestLinksPageHTMLEscapesData(t *testing.T) {
	srv, _, _ := newTestServer(t)
	u := createUser(t, srv, "me@example.com")
	_, _, _ = srv.Store.CreateLink(context.Background(), u.ID, "https://a.com", "</script><b>x")

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(sessionCookie(t, srv, auth.Session{UserID: u.ID}))
	body := serve(srv, req).Body.String()
	if !strings.Contains(body, "window.__PAGE__ = ") || strings.Contains(body, "</script><b>") {
		t.Fatalf("unsafe or missing page data: %s", body)
	}
}

func TestCreateLinkFromWeb(t *testing.T) {
	srv, _, jobs := newTestServer(t)
	u := createUser(t, srv, "me@example.com")
	cookie := sessionCookie(t, srv, auth.Session{UserID: u.ID})

	req := jsonRequest("POST", "/links", `{"url":"   ","note":""}`)
	req.AddCookie(cookie)
	rec := serve(srv, req)
	if rec.Code != 422 || decode[map[string]map[string]string](t, rec)["errors"]["url"] != "Enter a link" {
		t.Fatalf("blank url: %d %s", rec.Code, rec.Body.String())
	}

	req = jsonRequest("POST", "/links", `{"url":" https://x.com ","note":"hi"}`)
	req.AddCookie(cookie)
	if rec := serve(srv, req); rec.Code != 201 || jobs.n != 1 {
		t.Fatalf("create: %d jobs=%d", rec.Code, jobs.n)
	}
	links, _ := srv.Store.ListLinks(context.Background(), u.ID)
	if len(links) != 1 || links[0].URL != "https://x.com" || *links[0].Note != "hi" {
		t.Fatalf("links %+v", links)
	}

	if rec := serve(srv, jsonRequest("POST", "/links", `{"url":"https://y.com"}`)); rec.Code != 401 {
		t.Fatalf("signed-out create: %d", rec.Code)
	}
}

func TestLinksPageEmptyListIsArrayNotNull(t *testing.T) {
	srv, _, _ := newTestServer(t)
	u := createUser(t, srv, "me@example.com")
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept", "application/json")
	req.AddCookie(sessionCookie(t, srv, auth.Session{UserID: u.ID}))
	body := serve(srv, req).Body.String()
	if !strings.Contains(body, `"links":[]`) {
		t.Fatalf("want empty array, got %s", body)
	}
	req = httptest.NewRequest("GET", "/", nil)
	req.AddCookie(sessionCookie(t, srv, auth.Session{UserID: u.ID}))
	if html := serve(srv, req).Body.String(); !strings.Contains(html, `"links":[]`) {
		t.Fatalf("html page props: %s", html)
	}
}

func TestLinksAreScopedToSignedInUser(t *testing.T) {
	srv, _, _ := newTestServer(t)
	ctx := context.Background()
	a := createUser(t, srv, "a@example.com")
	b := createUser(t, srv, "b@example.com")
	bLink, _, _ := srv.Store.CreateLink(ctx, b.ID, "https://b.com", "secret")

	// A's page never shows B's link.
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept", "application/json")
	req.AddCookie(sessionCookie(t, srv, auth.Session{UserID: a.ID}))
	rec := serve(srv, req)
	if props := decode[linksPropsJSON](t, rec); len(props.Links) != 0 || props.APIToken != a.APIToken {
		t.Fatalf("A sees %+v", props)
	}
	// A cannot mark or delete B's link (the store operations the handlers use).
	if found, _ := srv.Store.SetLinkRead(ctx, a.ID, bLink.ID, true); found {
		t.Error("A marked B's link read")
	}
	if found, _ := srv.Store.DeleteLink(ctx, a.ID, bLink.ID); found {
		t.Error("A deleted B's link")
	}
	if links, _ := srv.Store.ListLinks(ctx, b.ID); len(links) != 1 || links[0].Read != 0 {
		t.Errorf("B's link changed: %+v", links)
	}
	// A's created link lands under A, not B.
	r := jsonRequest("POST", "/links", `{"url":"https://a.com"}`)
	r.AddCookie(sessionCookie(t, srv, auth.Session{UserID: a.ID}))
	if rec := serve(srv, r); rec.Code != 201 {
		t.Fatalf("create %d", rec.Code)
	}
	if links, _ := srv.Store.ListLinks(ctx, b.ID); len(links) != 1 {
		t.Errorf("B has %d links", len(links))
	}
}
