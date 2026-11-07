// Command rulinky serves the Rulinky link saver.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
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

func run() error {
	crawlURL := flag.String("crawl", "", "crawl one URL with Chrome + readitsoon, print the result and exit (kills running Chrome)")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := config.LoadDotenv("."); err != nil {
		return err
	}
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	crawler := &content.ChromeCrawler{
		RealUserDataDir: cfg.ChromeRealUserDataDir,
		UserDataDir:     cfg.ChromeUserDataDir,
		Profile:         cfg.ChromeProfileDirectory,
		Logger:          logger,
	}
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
	defer st.Close()

	dist, err := web.Dist()
	if err != nil {
		return err
	}
	index, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		return fmt.Errorf("web/dist/index.html missing, run pnpm run build before go build: %w", err)
	}
	pages, err := server.NewPages(index)
	if err != nil {
		return err
	}

	// A worker failure (its startup requeue hitting a database error) is
	// fatal: the process exits non-zero and the operator restarts it, rather
	// than serving a site whose saved links never get content.
	workerErr := make(chan error, 1)
	worker := content.NewWorker(st, crawler, previewer, logger)
	if cfg.DisableWorker {
		logger.Warn("content worker disabled (RULINKY_DISABLE_WORKER=1)")
	} else {
		go func() { workerErr <- workerFailure(worker.Run(ctx)) }()
	}

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
	httpServer := newHTTPServer(cfg.Port, srv.Handler())

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("rulinky listening", "port", cfg.Port, "production", cfg.Production)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	var result error
	select {
	case <-ctx.Done():
	case result = <-workerErr:
		stop()
	case result = <-serveErr:
		stop()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil && result == nil {
		result = err
	}
	return result
}
