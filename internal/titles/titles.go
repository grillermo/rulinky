// Package titles computes a link's display title (docs/PRODUCT.md §4.2).
package titles

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// ContentTitle is raw_title when it is not blank, else the whitespace-
// collapsed <title> found in content, else "".
func ContentTitle(rawTitle, content *string) string {
	if rawTitle != nil && strings.TrimSpace(*rawTitle) != "" {
		return *rawTitle
	}
	if content == nil || strings.TrimSpace(*content) == "" {
		return ""
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(*content))
	if err != nil {
		return ""
	}
	return strings.Join(strings.Fields(doc.Find("title").First().Text()), " ")
}

// Full is the content title, or the scheme-less URL when there is none.
func Full(rawURL string, rawTitle, content *string) string {
	if t := ContentTitle(rawTitle, content); t != "" {
		return t
	}
	return DisplayURL(rawURL)
}

var (
	wwwPrefix     = regexp.MustCompile(`(?i)^www\.`)
	schemePrefix  = regexp.MustCompile(`(?i)^https?://(www\.)?`)
	queryAndAfter = regexp.MustCompile(`\?.*$`)
	defaultPorts  = map[string]string{"http": "80", "https": "443"}
)

// DisplayURL drops the scheme, a leading www., default ports and the query.
func DisplayURL(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return queryAndAfter.ReplaceAllString(schemePrefix.ReplaceAllString(raw, ""), "")
	}
	host := u.Hostname()
	if host == "" {
		return raw
	}
	var b strings.Builder
	b.WriteString(wwwPrefix.ReplaceAllString(host, ""))
	if p := u.Port(); p != "" && p != defaultPorts[strings.ToLower(u.Scheme)] {
		b.WriteString(":" + p)
	}
	b.WriteString(u.EscapedPath())
	if u.Fragment != "" {
		b.WriteString("#" + u.EscapedFragment())
	}
	return b.String()
}

// Truncate cuts s to max characters, ending in "..." (like Rails' truncate).
func Truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-3]) + "..."
}
