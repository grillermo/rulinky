package content

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func previewHTML(title, content string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html><body>
  <form action="/send" method="POST" x-data="previewSendForm()" x-init="init()">
    <input type="hidden" name="url" value="https://example.com">
    <input type="hidden" name="markdown_content" value="# md">
    <input type="hidden" name="title" value="%s">
    <input type="hidden" name="author" value="Jane">
  </form>
  <form action="/subscriptions/new" method="POST">
    <input type="hidden" name="html_content" value="%s">
  </form>
</body></html>`, title, content)
}

func newPreviewServer(t *testing.T, status int, body string, check func(*http.Request)) *Readitsoon {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if check != nil {
			check(r)
		}
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	r := NewReaditsoon("me@kindle.com")
	r.URL = srv.URL
	r.Client = srv.Client()
	return r
}

func TestPreviewExtractsTitleAndContent(t *testing.T) {
	r := newPreviewServer(t, 200, previewHTML("Clean Title", "&lt;p&gt;Body&lt;/p&gt;"), nil)
	got, err := r.Preview(context.Background(), "<html>raw</html>", "https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Clean Title" || got.Content != "<p>Body</p>" {
		t.Fatalf("got %+v", got)
	}
}

func TestPreviewSendsFormFields(t *testing.T) {
	r := newPreviewServer(t, 200, previewHTML("T", "C"), func(req *http.Request) {
		_ = req.ParseForm()
		if req.PostForm.Get("inputhtml") != "<html>raw</html>" || req.PostForm.Get("url") != "https://example.com" ||
			req.PostForm.Get("email") != "me@kindle.com" {
			t.Errorf("form = %v", req.PostForm)
		}
	})
	if _, err := r.Preview(context.Background(), "<html>raw</html>", "https://example.com"); err != nil {
		t.Fatal(err)
	}
}

func TestPreviewErrorMessages(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   string
	}{
		{400, `{"error":"missing inputhtml"}`, "missing inputhtml"},
		{422, `{"errors":["a","b"]}`, "a, b"},
		{502, `<html>bad gateway</html>`, "readitsoon preview failed (502)"},
	}
	for _, c := range cases {
		r := newPreviewServer(t, c.status, c.body, nil)
		_, err := r.Preview(context.Background(), "x", "https://example.com")
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("status %d: err = %v, want %q", c.status, err, c.want)
		}
	}
}

func TestPreviewWithoutForm(t *testing.T) {
	r := newPreviewServer(t, 200, "<html><body><p>no form</p></body></html>", nil)
	_, err := r.Preview(context.Background(), "x", "https://example.com")
	if err == nil || err.Error() != "readitsoon preview form not found in response" {
		t.Fatalf("err = %v", err)
	}
}
