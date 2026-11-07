package content

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/chromedp/chromedp"
)

// ChromeCrawler renders a page in the user's real (logged-in) Chrome profile.
// Chrome refuses DevTools on its default user-data-dir, so the session files
// are copied to a separate automation dir first. Every running Chrome is
// killed first, so its profile files are complete on disk.
type ChromeCrawler struct {
	RealUserDataDir string
	UserDataDir     string
	Profile         string
	PageLoadTimeout time.Duration
	Logger          *slog.Logger
}

var profileEntries = []string{"Cookies", "Local Storage", "Session Storage", "IndexedDB"}

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

func (c *ChromeCrawler) Crawl(ctx context.Context, pageURL string) (string, error) {
	logger := c.Logger
	if logger == nil {
		logger = slog.Default()
	}
	timeout := c.PageLoadTimeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	killChrome(ctx, logger)
	if err := c.syncProfile(); err != nil {
		return "", err
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx,
		chromedp.UserDataDir(c.UserDataDir),
		chromedp.Flag("profile-directory", c.Profile),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Flag("no-restore-last-session", true),
		chromedp.Flag("disable-session-crashed-bubble", true),
		chromedp.Flag("disable-sync", true),
	)
	defer cancelAlloc() // kills the Chrome we started
	tabCtx, cancelTab := chromedp.NewContext(allocCtx)
	defer cancelTab()

	// Start the browser on a context without a deadline; a timeout on the
	// first Run would tie Chrome's lifetime to it.
	if err := chromedp.Do(tabCtx); err != nil {
		return "", err
	}
	navCtx, cancelNav := context.WithTimeout(tabCtx, timeout)
	defer cancelNav()

	logger.Info("crawler navigating", "url", pageURL)
	if err := chromedp.Do(navCtx, chromedp.Navigate(pageURL)); err != nil {
		return "", err
	}
	html, err := chromedp.Run(navCtx, chromedp.Evaluate[string](cleanJS))
	if err != nil {
		return "", err
	}
	logger.Info("crawler done", "html_bytes", len(html))
	return html, nil
}

// syncProfile replaces the automation profile's session files with fresh
// copies from the real profile. Missing sources are skipped.
func (c *ChromeCrawler) syncProfile() error {
	src := filepath.Join(c.RealUserDataDir, c.Profile)
	dst := filepath.Join(c.UserDataDir, c.Profile)
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, name := range profileEntries {
		from, to := filepath.Join(src, name), filepath.Join(dst, name)
		info, err := os.Stat(from)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if err := os.RemoveAll(to); err != nil {
			return err
		}
		if info.IsDir() {
			err = os.CopyFS(to, os.DirFS(from))
		} else {
			err = copyFile(from, to)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func copyFile(from, to string) error {
	data, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(to, data, 0o600)
}

// killChrome quits every running Google Chrome and waits up to 15s for it to exit.
func killChrome(ctx context.Context, logger *slog.Logger) {
	logger.Info("crawler killing Chrome")
	_ = exec.CommandContext(ctx, "pkill", "-i", "Google Chrome").Run()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if exec.CommandContext(ctx, "pgrep", "-i", "Google Chrome").Run() != nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)
}

var _ Crawler = (*ChromeCrawler)(nil)
