// Package content fetches a saved link's article: Chrome renders it, then
// readitsoon cleans it into a title and sanitized HTML.
package content

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type Preview struct {
	Title   string
	Content string
}

type Readitsoon struct {
	URL    string
	Email  string
	Client *http.Client
}

func NewReaditsoon(email string) *Readitsoon {
	return &Readitsoon{
		URL:    "https://readitsoon.app/preview",
		Email:  email,
		Client: &http.Client{Timeout: 100 * time.Second},
	}
}

// Preview posts the crawled HTML to readitsoon's /preview page and reads the
// cleaned title and HTML out of the form fields it renders.
func (r *Readitsoon) Preview(ctx context.Context, html, pageURL string) (Preview, error) {
	form := url.Values{"inputhtml": {html}, "url": {pageURL}, "email": {r.Email}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.URL, strings.NewReader(form.Encode()))
	if err != nil {
		return Preview{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := r.Client.Do(req)
	if err != nil {
		return Preview{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return Preview{}, err
	}
	if resp.StatusCode/100 != 2 {
		return Preview{}, errors.New(errorMessage(body, resp.StatusCode))
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return Preview{}, err
	}
	sendForm := doc.Find(`form[x-data^="previewSendForm"]`).First()
	if sendForm.Length() == 0 {
		return Preview{}, errors.New("readitsoon preview form not found in response")
	}
	return Preview{
		Title:   sendForm.Find(`input[name="title"]`).First().AttrOr("value", ""),
		Content: doc.Find(`input[name="html_content"]`).First().AttrOr("value", ""),
	}, nil
}

func errorMessage(body []byte, status int) string {
	fallback := fmt.Sprintf("readitsoon preview failed (%d)", status)
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return fallback
	}
	if msg, _ := payload["error"].(string); msg != "" {
		return msg
	}
	if list, ok := payload["errors"].([]any); ok && len(list) > 0 {
		parts := make([]string, len(list))
		for i, v := range list {
			parts[i] = fmt.Sprint(v)
		}
		return strings.Join(parts, ", ")
	}
	return fallback
}
