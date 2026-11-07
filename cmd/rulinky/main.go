// Command rulinky serves the Rulinky link saver.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"rulinky/internal/auth"
	"rulinky/internal/config"
	"rulinky/internal/content"
	"rulinky/internal/server"
	"rulinky/internal/store"
	"rulinky/web"
)

func main() {
	if err := run(); err != nil {
		slog.Error("rulinky stopped", "err", err)
		os.Exit(1)
	}
}

// newHTTPServer builds the http.Server with every timeout set, so slow or
// idle clients cannot hold connections open forever. The handlers are quick
// (crawling happens in the worker), so WriteTimeout can be modest.
func newHTTPServer(port string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}

// workerFailure turns the result of Worker.Run into an error worth stopping
// for. A clean shutdown returns context.Canceled, which is not a failure.
func workerFailure(err error) error {
	if err == nil || errors.Is(err, context.Canceled) {
		return nil
	}
	return fmt.Errorf("content worker: %w", err)
}

// shutdownGrace bounds the whole shutdown (HTTP drain plus waiting for the
// worker). serve-in-place.sh's free_port SIGKILLs a stopping process after
// 10s, so stay under that.
const shutdownGrace = 8 * time.Second

// serveOpts are the pieces of the process lifecycle, injectable for tests.
type serveOpts struct {
	HTTP   *http.Server
	Listen func(addr string) (net.Listener, error)
	// Worker runs until ctx is cancelled; nil means the worker is disabled.
	Worker func(ctx context.Context) error
	// Stop is the signal.NotifyContext stop func: calling it after the first
	// signal restores default handling so a second signal force-quits.
	Stop   func()
	Grace  time.Duration
	Logger *slog.Logger
	// ListenLog is extra attributes for the "listening" line.
	ListenLog []any
}

// serve binds the port first (failing fast), then starts the worker and the
// HTTP server, and on shutdown drains HTTP and waits for the worker (Chrome
// cleanup) within Grace. The caller must close the store only after it returns.
func serve(ctx context.Context, o serveOpts) error {
	ln, err := o.Listen(o.HTTP.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", o.HTTP.Addr, err)
	}
	o.Logger.Info("rulinky listening", o.ListenLog...)

	wctx, cancelWorker := context.WithCancel(ctx)
	defer cancelWorker()

	var workerRes chan error
	if o.Worker != nil {
		workerRes = make(chan error, 1)
		go func() { workerRes <- workerFailure(o.Worker(wctx)) }()
	}
	var serving sync.WaitGroup
	serveErr := make(chan error, 1)
	serving.Add(1)
	go func() {
		defer serving.Done()
		if err := o.HTTP.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	var result error
	workerExited := false
	select {
	case <-ctx.Done():
		o.Logger.Info("shutdown signal received")
	case result = <-workerRes: // nil channel when disabled: never fires
		workerExited = true
	case result = <-serveErr:
	}
	o.Stop()
	cancelWorker()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), o.Grace)
	defer cancel()
	if err := o.HTTP.Shutdown(shutdownCtx); err != nil {
		o.Logger.Error("http shutdown timed out, forcing close", "err", err)
		_ = o.HTTP.Close()
		if result == nil {
			result = fmt.Errorf("http shutdown: %w", err)
		}
	}
	if o.Worker != nil && !workerExited {
		select {
		case err := <-workerRes:
			if result == nil {
				result = err
			}
		case <-shutdownCtx.Done():
			o.Logger.Error("content worker did not stop in time, giving up on it")
			if result == nil {
				result = errors.New("content worker did not stop within the shutdown grace period")
			}
		}
	}
	return result
}

// configSummary is the one-line startup summary. It never includes the DSN,
// password, session secret or webhook URL.
func configSummary(cfg config.Config) []any {
	host, name := "unknown", "unknown"
	dsn := cfg.DatabaseURL
	if u, err := url.Parse(dsn); err == nil && u.Scheme != "" && u.Host != "" {
		host, name = u.Hostname(), strings.TrimPrefix(u.Path, "/")
	} else {
		for _, f := range splitDSN(dsn) {
			switch f[0] {
			case "host":
				host = f[1]
			case "dbname":
				name = f[1]
			}
		}
	}
	return []any{"port", cfg.Port, "production", cfg.Production,
		"worker", !cfg.DisableWorker, "db_host", host, "db_name", name}
}

// splitDSN extracts key/value pairs from a libpq keyword DSN with
// single-quoted values (as config builds it).
func splitDSN(dsn string) [][2]string {
	var out [][2]string
	for i := 0; i < len(dsn); {
		eq := strings.IndexByte(dsn[i:], '=')
		if eq < 0 {
			break
		}
		key := strings.TrimSpace(dsn[i : i+eq])
		i += eq + 1
		var val strings.Builder
		if i < len(dsn) && dsn[i] == '\'' {
			i++
			for i < len(dsn) && dsn[i] != '\'' {
				if dsn[i] == '\\' && i+1 < len(dsn) {
					i++
				}
				val.WriteByte(dsn[i])
				i++
			}
			i++
		} else {
			for i < len(dsn) && dsn[i] != ' ' {
				val.WriteByte(dsn[i])
				i++
			}
		}
		out = append(out, [2]string{key, val.String()})
		for i < len(dsn) && dsn[i] == ' ' {
			i++
		}
	}
	return out
}

func loadPages() (fs.FS, *server.Pages, error) {
	dist, err := web.Dist()
	if err != nil {
		return nil, nil, err
	}
	index, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		return nil, nil, fmt.Errorf("web/dist/index.html missing, run pnpm run build before go build: %w", err)
	}
	pages, err := server.NewPages(index)
	if err != nil {
		return nil, nil, fmt.Errorf("web/dist/index.html: %w", err)
	}
	return dist, pages, nil
}

func run() error {
	crawlURL := flag.String("crawl", "", "crawl one URL with ego-browser + readitsoon, print the result and exit")
	check := flag.Bool("check", false, "verify config, database and web build, print 'config ok' and exit (no worker, port or browser)")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := config.LoadDotenv("."); err != nil {
		return err
	}
	var cfg config.Config
	var err error
	if *crawlURL != "" {
		cfg, err = config.FromEnvNoSecret() // -crawl never serves, so no session secret needed
	} else {
		cfg, err = config.FromEnv()
	}
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	crawler := &content.EgoCrawler{Bin: cfg.EgoBrowserBin, Logger: logger}
	previewer := content.NewReaditsoon(cfg.ReaditsoonEmail)

	if *crawlURL != "" {
		html, err := crawler.Crawl(ctx, *crawlURL)
		if err != nil {
			return err
		}
		preview, err := previewer.Preview(ctx, html, *crawlURL)
		if err != nil {
			return err
		}
		fmt.Printf("title: %s\nhtml_bytes: %d\ncontent_bytes: %d\n", preview.Title, len(html), len(preview.Content))
		return nil
	}

	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer st.Close() // runs after serve returns, i.e. after the worker stopped
	if err := st.Ping(ctx); err != nil {
		return fmt.Errorf("database ping: %w", err)
	}
	dist, pages, err := loadPages()
	if err != nil {
		return err
	}
	if *check {
		fmt.Println("config ok")
		return nil
	}

	worker := content.NewWorker(st, crawler, previewer, logger)
	var workerRun func(context.Context) error
	if cfg.DisableWorker {
		logger.Warn("content worker disabled (RULINKY_DISABLE_WORKER=1)")
	} else {
		workerRun = worker.Run
	}
	logger.Info("config", configSummary(cfg)...)

	srv := &server.Server{
		Store:      st,
		Sessions:   auth.NewSessions(cfg.SessionSecret, cfg.Production),
		OTP:        auth.SlackNotifier{WebhookURL: cfg.SlackOTPWebhookURL, Client: &http.Client{Timeout: 10 * time.Second}},
		Jobs:       worker,
		Pages:      pages,
		Static:     dist,
		Logger:     logger,
		Production: cfg.Production,
	}
	return serve(ctx, serveOpts{
		HTTP:      newHTTPServer(cfg.Port, srv.Handler()),
		Listen:    func(addr string) (net.Listener, error) { return net.Listen("tcp", addr) },
		Worker:    workerRun,
		Stop:      stop,
		Grace:     shutdownGrace,
		Logger:    logger,
		ListenLog: []any{"port", cfg.Port, "production", cfg.Production},
	})
}
