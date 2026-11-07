package content

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

// EgoCrawler renders a page in ego-browser, the user's own Chromium-based
// browser, so pages open with their real logged-in session. It runs a short
// `ego-browser nodejs` script per crawl; no browser is killed or restarted.
type EgoCrawler struct {
	Bin             string // ego-browser executable; default "ego-browser"
	PageLoadTimeout time.Duration
	Logger          *slog.Logger
}

const (
	egoHTMLPrefix = "RULINKY_HTML:"
	egoErrPrefix  = "RULINKY_ERR:"
)

// cleanJS is the readitsoon bookmarklet's cleaning step.
const cleanJS = `(function () {
  var isTwitter = /^https?:\/\/(www\.)?(x\.com|twitter\.com)\//.test(window.location.href);
  var articleEl = isTwitter ? document.querySelector('[data-testid="twitterArticleReadView"]') : null;
  var clone;
  if (articleEl) {
    clone = document.createElement("html");
    clone.appendChild(document.createElement("body"));
    clone.querySelector("body").appendChild(articleEl.cloneNode(true));
  } else {
    clone = document.documentElement.cloneNode(true);
  }
  var junk = clone.querySelectorAll("script,style,canvas,select,textarea");
  for (var i = junk.length - 1; i >= 0; i--) {
    if (junk[i].parentNode) junk[i].parentNode.removeChild(junk[i]);
  }
  var titleH1 = document.createElement("h1");
  titleH1.textContent = document.title;
  var cloneBody = clone.querySelector("body") || clone;
  cloneBody.insertBefore(titleH1, cloneBody.firstChild);
  return "<!DOCTYPE html>\n" + clone.outerHTML;
})()`

func egoScript(pageURL string, timeout time.Duration) string {
	u, _ := json.Marshal(pageURL) // a JSON string is a valid JS string literal
	clean, _ := json.Marshal(cleanJS)
	return fmt.Sprintf(`const task = await taskSpace("rulinky crawl");
const page = task.page("p1");
try {
  await page.goto(%s, { timeout: %d });
  const html = await page.evaluate(%s);
  console.log(%q + JSON.stringify(html));
  await task.finish({ keep: [] });
} catch (e) {
  console.log(%q + JSON.stringify(String((e && e.message) || e)));
  process.exitCode = 1;
}
`, u, timeout.Milliseconds(), clean, egoHTMLPrefix, egoErrPrefix)
}

func (c *EgoCrawler) Crawl(ctx context.Context, pageURL string) (string, error) {
	logger := c.Logger
	if logger == nil {
		logger = slog.Default()
	}
	timeout := c.PageLoadTimeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	bin := c.Bin
	if bin == "" {
		bin = "ego-browser"
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout+30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, bin, "nodejs")
	cmd.Stdin = strings.NewReader(egoScript(pageURL, timeout))
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr

	logger.Info("crawler navigating", "url", pageURL)
	runErr := cmd.Run()
	html, err := parseEgoOutput(out.String() + "\n" + stderr.String()) // ego-browser may print to either stream
	if err != nil {
		if runErr != nil {
			return "", fmt.Errorf("ego-browser: %w: %s", runErr, strings.TrimSpace(stderr.String()+" "+out.String()))
		}
		return "", fmt.Errorf("%w (output: %.300q)", err, strings.TrimSpace(stderr.String()+" "+out.String()))
	}
	logger.Info("crawler done", "html_bytes", len(html))
	return html, nil
}

// parseEgoOutput finds the result line the script printed.
func parseEgoOutput(out string) (string, error) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, egoHTMLPrefix):
			var html string
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, egoHTMLPrefix)), &html); err != nil {
				return "", err
			}
			return html, nil
		case strings.HasPrefix(line, egoErrPrefix):
			var msg string
			_ = json.Unmarshal([]byte(strings.TrimPrefix(line, egoErrPrefix)), &msg)
			return "", errors.New("ego-browser: " + msg)
		}
	}
	return "", errors.New("ego-browser: no result from script")
}

var _ Crawler = (*EgoCrawler)(nil)
