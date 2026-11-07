package content

import (
	"strings"
	"testing"
	"time"
)

func TestParseEgoOutput(t *testing.T) {
	html, err := parseEgoOutput("noise\nRULINKY_HTML:\"<h1>hi\\n</h1>\"\n")
	if err != nil || html != "<h1>hi\n</h1>" {
		t.Fatalf("html=%q err=%v", html, err)
	}
	if _, err := parseEgoOutput("RULINKY_ERR:\"boom\""); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err=%v", err)
	}
	if _, err := parseEgoOutput("nothing"); err == nil {
		t.Fatal("want error")
	}
}

func TestEgoScriptEscapesURL(t *testing.T) {
	s := egoScript(`https://x.test/?q="a";alert(1)`, 5*time.Second)
	if !strings.Contains(s, `"https://x.test/?q=\"a\";alert(1)"`) || !strings.Contains(s, "timeout: 5000") {
		t.Fatalf("script:\n%s", s)
	}
}
