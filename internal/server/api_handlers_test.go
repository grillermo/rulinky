package server

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"rulinky/internal/auth"
)

func api(t *testing.T, srv *Server, method, body, authHeader string) *httptest.ResponseRecorder {
	t.Helper()
	req := jsonRequest(method, "/api/links", body)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	return serve(srv, req)
}

func TestAPIRequiresToken(t *testing.T) {
	srv, _, _ := newTestServer(t)
	for _, header := range []string{"", "nope", "Bearer nope"} {
		rec := api(t, srv, "GET", "", header)
		if rec.Code != 401 || decode[map[string]string](t, rec)["error"] != "Unauthorized" {
			t.Errorf("header %q: %d %s", header, rec.Code, rec.Body.String())
		}
	}
}

func TestAPIIndex(t *testing.T) {
	srv, _, _ := newTestServer(t)
	ctx := context.Background()
	u := createUser(t, srv, "me@example.com")
	l, _, _ := srv.Store.CreateLink(ctx, u.ID, "https://www.a.com/p", "n")
	otherUser := createUser(t, srv, "other@example.com")
	_, _, _ = srv.Store.CreateLink(ctx, otherUser.ID, "https://other.com", "")

	for _, header := range []string{u.APIToken, "Bearer " + u.APIToken} {
		rec := api(t, srv, "GET", "", header)
		if rec.Code != 200 {
			t.Fatalf("%q: %d", header, rec.Code)
		}
		got := decode[[]map[string]any](t, rec)
		if len(got) != 1 {
			t.Fatalf("want 1 link, got %v", got)
		}
		row := got[0]
		if row["id"] != l.ID || row["url"] != "https://www.a.com/p" || row["note"] != "n" || row["read"] != float64(0) ||
			row["timestamp"] != float64(l.Timestamp) || row["updated_at"] != float64(l.UpdatedAt) ||
			row["title"] != "a.com/p" || row["content_title"] != nil {
			t.Errorf("row %v", row)
		}
	}

	empty := createUser(t, srv, "empty@example.com")
	if rec := api(t, srv, "GET", "", empty.APIToken); strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("empty list must be [], got %s", rec.Body.String())
	}
}

func TestAPICreate(t *testing.T) {
	srv, _, jobs := newTestServer(t)
	u := createUser(t, srv, "me@example.com")

	rec := api(t, srv, "POST", `{"link":"https://a.com/x","note":"n"}`, u.APIToken)
	if rec.Code != 201 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	body := decode[map[string]string](t, rec)
	if body["message"] != "Link saved" || body["id"] == "" || body["note"] != "n" || body["job_id"] == "" || body["title"] != "a.com/x" {
		t.Fatalf("body %v", body)
	}
	if jobs.n != 1 {
		t.Fatalf("worker notified %d times", jobs.n)
	}
	if status, _, _, _ := srv.Store.JobStatus(context.Background(), body["job_id"]); status != "queued" {
		t.Fatalf("job status %q", status)
	}

	for _, bad := range []string{`{"link":""}`, `{"link":"   "}`, `{"link":5}`, `{}`} {
		rec := api(t, srv, "POST", bad, u.APIToken)
		if rec.Code != 400 || decode[map[string]string](t, rec)["error"] != "Invalid link" {
			t.Errorf("%s: %d %s", bad, rec.Code, rec.Body.String())
		}
	}
}

func TestAPICreateFormEncoded(t *testing.T) {
	srv, _, _ := newTestServer(t)
	u := createUser(t, srv, "me@example.com")
	req := httptest.NewRequest("POST", "/api/links", strings.NewReader("link=https%3A%2F%2Fb.com"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", u.APIToken)
	rec := serve(srv, req)
	if rec.Code != 201 || decode[map[string]string](t, rec)["note"] != "" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestRejectsNULBytes(t *testing.T) {
	srv, _, jobs := newTestServer(t)
	u := createUser(t, srv, "me@example.com")
	cookie := sessionCookie(t, srv, auth.Session{UserID: u.ID})
	form := "application/x-www-form-urlencoded"

	cases := []struct {
		name, method, path, ctype, body string
		api                             bool
	}{
		{"api json", "POST", "/api/links", "application/json", `{"link":"https://a.com/\u0000"}`, true},
		{"api json note", "POST", "/api/links", "application/json", `{"link":"https://a.com","note":"a\u0000b"}`, true},
		{"api json nested", "POST", "/api/links", "application/json", `{"link":"https://a.com","x":{"y":["\u0000"]}}`, true},
		{"api form", "POST", "/api/links", form, "link=https%3A%2F%2Fa.com%2F%00", true},
		{"api query", "POST", "/api/links?note=%00", "application/json", `{"link":"https://a.com"}`, true},
		{"web json", "POST", "/links", "application/json", `{"url":"https://a.com/\u0000","note":""}`, false},
		{"web form", "POST", "/links", form, "url=https%3A%2F%2Fa.com%2F%00&note=", false},
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, c.path, strings.NewReader(c.body))
		req.Header.Set("Content-Type", c.ctype)
		if c.api {
			req.Header.Set("Authorization", u.APIToken)
		} else {
			req.AddCookie(cookie)
		}
		if rec := serve(srv, req); rec.Code != 400 {
			t.Errorf("%s: %d %s", c.name, rec.Code, rec.Body.String())
		}
	}
	if links, _ := srv.Store.ListLinks(context.Background(), u.ID); len(links) != 0 || jobs.n != 0 {
		t.Errorf("nothing may be stored: %d links, %d notifies", len(links), jobs.n)
	}
}

func TestAPIUpdate(t *testing.T) {
	srv, _, _ := newTestServer(t)
	ctx := context.Background()
	u := createUser(t, srv, "me@example.com")
	l, _, _ := srv.Store.CreateLink(ctx, u.ID, "https://a.com", "")
	readValue := func() int {
		links, _ := srv.Store.ListLinks(ctx, u.ID)
		return links[0].Read
	}

	rec := api(t, srv, "PATCH", `{"id":"`+l.ID+`","read":true}`, u.APIToken)
	if rec.Code != 200 || decode[map[string]string](t, rec)["message"] != "Link updated" || readValue() != 1 {
		t.Fatalf("%d %s read=%d", rec.Code, rec.Body.String(), readValue())
	}
	for raw, want := range map[string]int{`"0"`: 0, `"yes"`: 1, `"f"`: 0, `1`: 1, `false`: 0} {
		api(t, srv, "PATCH", `{"id":"`+l.ID+`","read":`+raw+`}`, u.APIToken)
		if got := readValue(); got != want {
			t.Errorf("read=%s stored %d, want %d", raw, got, want)
		}
	}
	for _, bad := range []string{`{"id":"` + l.ID + `"}`, `{"id":"","read":true}`, `{"read":true}`, `{"id":"` + l.ID + `","read":""}`} {
		if rec := api(t, srv, "PATCH", bad, u.APIToken); rec.Code != 400 || decode[map[string]string](t, rec)["error"] != "Invalid payload" {
			t.Errorf("%s: %d", bad, rec.Code)
		}
	}
	if rec := api(t, srv, "PATCH", `{"id":"nope","read":true}`, u.APIToken); rec.Code != 404 || decode[map[string]string](t, rec)["error"] != "Link not found" {
		t.Errorf("missing link: %d", rec.Code)
	}
	// Make the owner's state deterministic: unread.
	api(t, srv, "PATCH", `{"id":"`+l.ID+`","read":false}`, u.APIToken)
	before, _ := srv.Store.ListLinks(ctx, u.ID)
	if len(before) != 1 || before[0].Read != 0 {
		t.Fatalf("setup: %+v", before)
	}
	other := createUser(t, srv, "other@example.com")
	if rec := api(t, srv, "PATCH", `{"id":"`+l.ID+`","read":true}`, other.APIToken); rec.Code != 404 {
		t.Errorf("other user's link: %d", rec.Code)
	}
	after, _ := srv.Store.ListLinks(ctx, u.ID)
	if len(after) != 1 {
		t.Fatalf("owner's link vanished: %+v", after)
	}
	if after[0].Read != before[0].Read || after[0].UpdatedAt != before[0].UpdatedAt {
		t.Errorf("owner's link changed by another user: before %+v after %+v", before[0], after[0])
	}
}

func TestAPIDestroy(t *testing.T) {
	srv, _, _ := newTestServer(t)
	u := createUser(t, srv, "me@example.com")
	l, _, _ := srv.Store.CreateLink(context.Background(), u.ID, "https://a.com", "")

	rec := api(t, srv, "DELETE", `{"id":"`+l.ID+`"}`, u.APIToken)
	if rec.Code != 200 || decode[map[string]string](t, rec)["message"] != "Link deleted" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if rec := api(t, srv, "DELETE", `{"id":"`+l.ID+`"}`, u.APIToken); rec.Code != 404 {
		t.Errorf("second delete: %d", rec.Code)
	}
	if rec := api(t, srv, "DELETE", `{}`, u.APIToken); rec.Code != 400 {
		t.Errorf("missing id: %d", rec.Code)
	}
	if rec := api(t, srv, "DELETE", `{not json`, u.APIToken); rec.Code != 400 {
		t.Errorf("bad json: %d", rec.Code)
	}
}

func TestAPIOtherUserCannotDestroyOrOversize(t *testing.T) {
	srv, _, _ := newTestServer(t)
	ctx := context.Background()
	owner := createUser(t, srv, "owner@example.com")
	other := createUser(t, srv, "other@example.com")
	l, jobID, _ := srv.Store.CreateLink(ctx, owner.ID, "https://a.com", "")

	if rec := api(t, srv, "DELETE", `{"id":"`+l.ID+`"}`, other.APIToken); rec.Code != 404 {
		t.Errorf("other user's delete: %d", rec.Code)
	}
	if links, _ := srv.Store.ListLinks(ctx, owner.ID); len(links) != 1 {
		t.Errorf("link must survive, have %d", len(links))
	}
	links, _ := srv.Store.ListLinks(ctx, owner.ID)
	if len(links) != 1 {
		t.Fatalf("link must survive, have %d", len(links))
	}
	if links[0].Read != 0 {
		t.Errorf("link must stay unread")
	}
	if _, _, found, _ := srv.Store.JobStatus(ctx, jobID); !found {
		t.Errorf("owner's job row must survive another user's delete")
	}
	big := `{"link":"` + strings.Repeat("a", 2<<20) + `"}`
	if rec := api(t, srv, "POST", big, owner.APIToken); rec.Code != 413 {
		t.Errorf("oversized body: %d", rec.Code)
	}
}
