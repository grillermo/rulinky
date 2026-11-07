package titles

import (
	"strings"
	"testing"
)

func ptr(s string) *string { return &s }

func TestContentTitle(t *testing.T) {
	cases := []struct {
		name         string
		raw, content *string
		want         string
	}{
		{"raw title wins", ptr("My Article"), ptr("<title>Ignored</title>"), "My Article"},
		{"content title when raw blank", ptr("  "), ptr("<html><head><title>  From \n Content </title></head></html>"), "From Content"},
		{"nil raw uses content", nil, ptr("<title>C</title>"), "C"},
		{"both nil", nil, nil, ""},
		{"content without title", nil, ptr("<p>x</p>"), ""},
	}
	for _, c := range cases {
		if got := ContentTitle(c.raw, c.content); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDisplayURL(t *testing.T) {
	cases := map[string]string{
		"https://www.example.com/path/to?q=1#frag": "example.com/path/to#frag",
		"https://WWW.Example.com":                  "Example.com",
		"http://example.com:8080/a":                "example.com:8080/a",
		"https://example.com:443/a":                "example.com/a",
		"http://example.com:80":                    "example.com",
		"example.com/no-scheme":                    "example.com/no-scheme",
		"":                                         "",
		"https://exa mple.com/x?y":                 "exa mple.com/x",
	}
	for in, want := range cases {
		if got := DisplayURL(in); got != want {
			t.Errorf("DisplayURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFullFallsBackToURL(t *testing.T) {
	if got := Full("https://www.a.com/x", nil, nil); got != "a.com/x" {
		t.Errorf("Full without title = %q", got)
	}
	if got := Full("https://a.com", ptr("T"), nil); got != "T" {
		t.Errorf("Full with title = %q", got)
	}
}

func TestTruncate(t *testing.T) {
	if got := Truncate("short", 50); got != "short" {
		t.Errorf("got %q", got)
	}
	long := strings.Repeat("é", 60)
	got := Truncate(long, 50)
	if len([]rune(got)) != 50 || !strings.HasSuffix(got, "...") {
		t.Errorf("got %q (%d runes)", got, len([]rune(got)))
	}
}
