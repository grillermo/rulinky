# Rulinky Go Rewrite Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use subagent-driven-development (recommended) or executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Rails 8.1 Rulinky app with one Go binary. It uses the same Postgres database and gives the same behavior (`docs/PRODUCT.md`), with a fraction of the RAM.

**Architecture:** A `net/http` server (Go 1.22+ routing patterns) on top of a small `store` package (pgx), with one background worker goroutine. The worker runs the Chrome crawl and the readitsoon pipeline one job at a time, and the jobs live in the existing `link_content_jobs` table. The React UI is kept. Vite builds it into `web/dist`, the binary embeds that folder, and the server injects each page's props into `index.html` (this replaces Inertia). Form submits use `fetch` with JSON and get back `{redirect}` or `{errors}`.

**Tech Stack:** Go 1.26, `github.com/jackc/pgx/v5`, `github.com/PuerkitoBio/goquery`, `github.com/chromedp/chromedp`, React 19 + Vite 6 (pnpm), Postgres.

**Spec:** `docs/PRODUCT.md` (approved). Read it before starting. Every "Rewrite decision" in it is binding.

---

## Ground rules for whoever runs this plan

- **Work on branch `go-rewrite` in a worktree, never on `main`.** The repo uses git-sync. Any commit that lands on `main` reaches the macmini and runs `./activate` → `./serve`, which restarts production. The branch is merged only in Task 18 (cutover).
- Tests use a real Postgres: `TEST_DATABASE_URL`, default `postgres://grillermo@192.168.1.1/rulinky_test` (the same database the Rails tests use). Each test creates its own throwaway schema, so tests can run in parallel and never touch `public`.
- Every timestamp column of type `timestamp` is written in SQL as `(now() AT TIME ZONE 'UTC')`, never from Go. That matches how Rails stored them (UTC, without time zone). `links.timestamp` and `links.updated_at` are **epoch milliseconds** (`bigint`) and are written from Go with `time.Now().UnixMilli()`.
- Commit after every task. Commit messages end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Small deliberate differences from Rails (not worth a spec round-trip)

| Rails | Go | Why |
|---|---|---|
| `allow_browser versions: :modern` | dropped | single user, modern browsers |
| `config.hosts` / `HOSTS` env | dropped | the app is reachable only through the Cloudflare tunnel |
| session cookie lasts until the browser closes, with a 10-day inactivity timeout | cookie `Max-Age` 10 days, renewed on every request | same timeout, and it survives browser restarts |
| CSRF token (`X-XSRF-TOKEN`) | `http.CrossOriginProtection` (Sec-Fetch-Site / Origin) | stdlib, nothing to wire into the UI |
| logs in `log/production.log` + STDOUT | STDOUT only (the tmux pane) | |
| the crawler copies the profile, *then* kills Chrome | kills Chrome, *then* copies the profile | Chrome writes cookies and LevelDB to disk on exit, so the copy is fresh and consistent |
| `GET /` needs Inertia JSON for reloads | `GET /` with `Accept: application/json` returns the page props | replaces Inertia's partial reloads |
| existing Rails sessions | invalid after cutover; you sign in once with a new OTP | a different cookie format |

## File structure

```
go.mod / go.sum                      module "rulinky"
cmd/rulinky/main.go                  wiring, flags, graceful shutdown, -crawl debug flag
internal/config/dotenv.go            .env / .env.local loader (real env wins)
internal/config/config.go            Config + FromEnv (DSN, secrets, defaults)
internal/store/store.go              Store type, Open, Pool, Ping
internal/store/schema.sql            app tables (copy of db/schema.rb without informant_*); used by tests only
internal/store/testdb.go             OpenTest: per-test schema in the *_test database
internal/store/ids.go                UUID v4 + base58 API tokens
internal/store/users.go              users, OTP, trackable
internal/store/links.go              links CRUD scoped by user
internal/store/jobs.go               link_content_jobs queue
internal/titles/titles.go            display-title rules (§4.2)
internal/auth/session.go             signed cookie session
internal/auth/otp.go                 OTP code + email validation
internal/auth/slack.go               Slack OTP notifier
internal/content/readitsoon.go       readitsoon /preview client
internal/content/crawler.go          Chrome crawler (chromedp)
internal/content/worker.go           serial job worker
internal/server/server.go            Server type, routes
internal/server/params.go            request parsing, JSON helpers, castBool, clientIP
internal/server/pages.go             index.html + page-data injection
internal/server/middleware.go        logging, recover, security headers, requireUser/requireToken
internal/server/auth_handlers.go     /auth/*
internal/server/links_handlers.go    GET / and POST /links
internal/server/api_handlers.go      /api/links
web/                                 frontend: index.html, vite.config.js, src/, public/, test/, embed.go
serve                                rewritten for Go
```

---

### Task 0: Prerequisites and worktree

**Files:** none

- [ ] **Step 1: Find out which machine you are on**

Run: `ipconfig getifaddr en0 || ipconfig getifaddr en1`
Expected: an IP from the table in the my-machines rule. If it is `192.168.1.1`, you are on the macmini (production). Work only in the worktree from Step 2.

- [ ] **Step 2: Create the worktree on branch `go-rewrite`**

Use the `using-git-worktrees` skill. Branch name: `go-rewrite`. All later paths are relative to the worktree root.

- [ ] **Step 3: Check that the test database is reachable**

Run: `psql postgres://grillermo@192.168.1.1/rulinky_test -c 'select 1'`
Expected: one row, `1`. If the database does not exist, stop and ask the user. Do not create databases on the macmini without asking.

- [ ] **Step 4: Check the toolchain here and on the macmini**

Run: `go version && pnpm --version && command ssh -o BatchMode=yes -o ConnectTimeout=10 macmini 'export PATH=/opt/homebrew/bin:$HOME/Library/pnpm:$PATH; go version; pnpm --version'`
Expected: Go ≥ 1.25 on both (Go 1.26.1 here), and a pnpm version on both. If Go is missing on the macmini, stop and tell the user (`brew install go` there needs their approval).

- [ ] **Step 5: Record Rails' memory as the baseline**

Run: `command ssh macmini 'for p in $(lsof -ti tcp:3002 -sTCP:LISTEN); do ps -o pid=,rss=,command= -p $p; done'`
Expected: the Rails/Puma process with its RSS in KB. Write the number down. Task 18 compares against it.

---

### Task 1: Go module and dotenv loader

**Files:**
- Create: `go.mod`
- Create: `internal/config/dotenv.go`
- Test: `internal/config/dotenv_test.go`

- [ ] **Step 1: Initialize the module and fetch the dependencies**

```bash
go mod init rulinky
go get github.com/jackc/pgx/v5@latest github.com/PuerkitoBio/goquery@latest github.com/chromedp/chromedp@latest
```

Then edit the `go` line in `go.mod` to `go 1.26`.

- [ ] **Step 2: Write the failing test**

`internal/config/dotenv_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDotenvPrecedence(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".env", "# comment\n\nRULINKY_T_A=from_env\nRULINKY_T_B=\"quoted\"\nRULINKY_T_C=from_env\nRULINKY_T_REAL=from_file\nnot a pair\n")
	writeFile(t, dir, ".env.local", "RULINKY_T_C = from_local \n")
	t.Setenv("RULINKY_T_REAL", "from_process")
	for _, k := range []string{"RULINKY_T_A", "RULINKY_T_B", "RULINKY_T_C"} {
		t.Cleanup(func() { os.Unsetenv(k) })
	}

	if err := LoadDotenv(dir); err != nil {
		t.Fatal(err)
	}

	want := map[string]string{
		"RULINKY_T_A":    "from_env",
		"RULINKY_T_B":    "quoted",
		"RULINKY_T_C":    "from_local",
		"RULINKY_T_REAL": "from_process",
	}
	for k, v := range want {
		if got := os.Getenv(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

func TestLoadDotenvWithoutFiles(t *testing.T) {
	if err := LoadDotenv(t.TempDir()); err != nil {
		t.Fatalf("missing files must be ignored, got %v", err)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/config/ -run TestLoadDotenv -v`
Expected: FAIL, `undefined: LoadDotenv`.

- [ ] **Step 4: Implement**

`internal/config/dotenv.go`:

```go
package config

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// LoadDotenv reads dir/.env and then dir/.env.local into the process
// environment. .env.local overrides .env; variables that were already set in
// the process environment before the call always win. Format: KEY=VALUE per
// line, blank lines and # comments ignored, optional surrounding double quotes.
func LoadDotenv(dir string) error {
	preserved := map[string]bool{}
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		preserved[k] = true
	}
	for _, name := range []string{".env", ".env.local"} {
		if err := loadFile(filepath.Join(dir, name), preserved); err != nil {
			return err
		}
	}
	return nil
}

func loadFile(path string, preserved map[string]bool) error {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if len(v) >= 2 && strings.HasPrefix(v, `"`) && strings.HasSuffix(v, `"`) {
			v = v[1 : len(v)-1]
		}
		if preserved[k] {
			continue
		}
		if err := os.Setenv(k, v); err != nil {
			return err
		}
	}
	return sc.Err()
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/config/ -run TestLoadDotenv -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/config/
git commit -m "feat(go): module and .env loader"
```

---

### Task 2: Config from the environment

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

`internal/config/config_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func clearEnv(t *testing.T) {
	for _, k := range []string{"DATABASE_URL", "DATABASE_HOST", "DATABASE_PORT", "DATABASE_USER",
		"DATABASE_PASSWORD", "DATABASE_NAME", "SESSION_SECRET", "PORT", "RULINKY_ENV",
		"READITSOON_EMAIL", "SLACK_OTP_WEBHOOK_URL", "RULINKY_DISABLE_WORKER",
		"CHROME_REAL_USER_DATA_DIR", "CHROME_USER_DATA_DIR", "CHROME_PROFILE_DIRECTORY"} {
		t.Setenv(k, "")
	}
}

func TestFromEnvDefaults(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_HOST", "192.168.1.1")
	t.Setenv("DATABASE_USER", "grillermo")
	t.Setenv("DATABASE_PASSWORD", `it's\x`)
	t.Setenv("SESSION_SECRET", strings.Repeat("a", 64))

	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}

	wantDSN := `host='192.168.1.1' port='5432' user='grillermo' password='it\'s\\x' dbname='rulinky'`
	if cfg.DatabaseURL != wantDSN {
		t.Errorf("DatabaseURL = %s, want %s", cfg.DatabaseURL, wantDSN)
	}
	if cfg.Port != "3002" {
		t.Errorf("Port = %q", cfg.Port)
	}
	if cfg.Production || cfg.DisableWorker {
		t.Errorf("Production/DisableWorker should default to false")
	}
	if cfg.ReaditsoonEmail != "guillermo.siliceo@kindle.com" {
		t.Errorf("ReaditsoonEmail = %q", cfg.ReaditsoonEmail)
	}
	home, _ := os.UserHomeDir()
	if cfg.ChromeRealUserDataDir != filepath.Join(home, "Library/Application Support/Google/Chrome") ||
		cfg.ChromeUserDataDir != filepath.Join(home, "Library/Application Support/Google/ChromeRulinky") ||
		cfg.ChromeProfileDirectory != "Default" {
		t.Errorf("chrome defaults wrong: %+v", cfg)
	}
}

func TestFromEnvOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://x@y/z")
	t.Setenv("SESSION_SECRET", strings.Repeat("b", 32))
	t.Setenv("PORT", "3012")
	t.Setenv("RULINKY_ENV", "production")
	t.Setenv("RULINKY_DISABLE_WORKER", "1")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "postgres://x@y/z" || cfg.Port != "3012" || !cfg.Production || !cfg.DisableWorker {
		t.Errorf("overrides not applied: %+v", cfg)
	}
}

func TestFromEnvRequiresLongSessionSecret(t *testing.T) {
	clearEnv(t)
	t.Setenv("SESSION_SECRET", "short")
	if _, err := FromEnv(); err == nil {
		t.Fatal("expected an error for a short SESSION_SECRET")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/config/ -run TestFromEnv -v`
Expected: FAIL, `undefined: FromEnv`.

- [ ] **Step 3: Implement**

`internal/config/config.go`:

```go
package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Port                   string
	DatabaseURL            string
	SessionSecret          []byte
	Production             bool
	DisableWorker          bool
	SlackOTPWebhookURL     string
	ReaditsoonEmail        string
	ChromeRealUserDataDir  string
	ChromeUserDataDir      string
	ChromeProfileDirectory string
}

// FromEnv builds the config from environment variables (call LoadDotenv
// first). An empty variable counts as unset.
func FromEnv() (Config, error) {
	secret := os.Getenv("SESSION_SECRET")
	if len(secret) < 32 {
		return Config{}, errors.New("SESSION_SECRET must be set to at least 32 characters (generate one with: openssl rand -hex 32)")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, err
	}
	return Config{
		Port:                   envOr("PORT", "3002"),
		DatabaseURL:            databaseURL(),
		SessionSecret:          []byte(secret),
		Production:             os.Getenv("RULINKY_ENV") == "production",
		DisableWorker:          os.Getenv("RULINKY_DISABLE_WORKER") == "1",
		SlackOTPWebhookURL:     os.Getenv("SLACK_OTP_WEBHOOK_URL"),
		ReaditsoonEmail:        envOr("READITSOON_EMAIL", "guillermo.siliceo@kindle.com"),
		ChromeRealUserDataDir:  envOr("CHROME_REAL_USER_DATA_DIR", filepath.Join(home, "Library/Application Support/Google/Chrome")),
		ChromeUserDataDir:      envOr("CHROME_USER_DATA_DIR", filepath.Join(home, "Library/Application Support/Google/ChromeRulinky")),
		ChromeProfileDirectory: envOr("CHROME_PROFILE_DIRECTORY", "Default"),
	}, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// databaseURL returns DATABASE_URL, or a libpq keyword DSN built from the
// DATABASE_* variables the Rails app used (database name defaults to rulinky).
func databaseURL() string {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v
	}
	parts := []string{}
	add := func(key, value string) {
		if value != "" {
			parts = append(parts, key+"="+quoteDSN(value))
		}
	}
	add("host", os.Getenv("DATABASE_HOST"))
	add("port", envOr("DATABASE_PORT", "5432"))
	add("user", os.Getenv("DATABASE_USER"))
	add("password", os.Getenv("DATABASE_PASSWORD"))
	add("dbname", envOr("DATABASE_NAME", "rulinky"))
	return strings.Join(parts, " ")
}

func quoteDSN(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `'`, `\'`)
	return "'" + v + "'"
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: PASS (all 5 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(go): config from environment"
```

---

### Task 3: Store, test database, IDs, users

**Files:**
- Create: `internal/store/schema.sql`
- Create: `internal/store/store.go`
- Create: `internal/store/testdb.go`
- Create: `internal/store/ids.go`
- Create: `internal/store/users.go`
- Test: `internal/store/users_test.go`

- [ ] **Step 1: Write the schema (app tables from `db/schema.rb`, without `informant_*`)**

`internal/store/schema.sql`:

```sql
-- Mirror of the production schema (db/schema.rb) for the app's own tables.
-- Only tests load this file; production keeps its existing tables.
CREATE TABLE users (
  id bigserial PRIMARY KEY,
  api_token varchar,
  confirmed_at timestamp(6),
  created_at timestamp(6) NOT NULL,
  current_sign_in_at timestamp(6),
  current_sign_in_ip varchar,
  email varchar NOT NULL DEFAULT '',
  last_sign_in_at timestamp(6),
  last_sign_in_ip varchar,
  otp_code varchar,
  otp_expires_at timestamp(6),
  remember_created_at timestamp(6),
  sign_in_count integer NOT NULL DEFAULT 0,
  updated_at timestamp(6) NOT NULL
);
CREATE UNIQUE INDEX index_users_on_api_token ON users (api_token);
CREATE UNIQUE INDEX index_users_on_email ON users (email);

CREATE TABLE links (
  id varchar PRIMARY KEY,
  content text,
  note text,
  raw_title text,
  read integer NOT NULL DEFAULT 0,
  "timestamp" bigint NOT NULL,
  updated_at bigint NOT NULL,
  url text NOT NULL,
  user_id bigint NOT NULL REFERENCES users (id)
);
CREATE UNIQUE INDEX index_links_on_id ON links (id);
CREATE INDEX index_links_on_read ON links (read);
CREATE INDEX index_links_on_updated_at ON links (updated_at);
CREATE INDEX index_links_on_user_id ON links (user_id);

CREATE TABLE link_content_jobs (
  id varchar PRIMARY KEY,
  created_at timestamp(6) NOT NULL,
  error_message text,
  link_id varchar NOT NULL,
  status varchar NOT NULL DEFAULT 'queued',
  updated_at timestamp(6) NOT NULL
);
CREATE INDEX index_link_content_jobs_on_link_id ON link_content_jobs (link_id);
CREATE INDEX index_link_content_jobs_on_status ON link_content_jobs (status);
```

- [ ] **Step 2: Write the Store type and the test helper**

`internal/store/store.go`:

```go
// Package store is Rulinky's Postgres access layer. It reads and writes the
// tables the Rails app created; it never migrates them.
package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// nowUTC is how every `timestamp` column is written: Rails stored them as UTC
// wall-clock time without a time zone.
const nowUTC = `(now() AT TIME ZONE 'UTC')`

type Store struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Pool exposes the connection pool, for tests that need to arrange rows directly.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

func (s *Store) Close() { s.pool.Close() }
```

`internal/store/testdb.go`:

```go
package store

import (
	"context"
	_ "embed"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaSQL string

const defaultTestDatabaseURL = "postgres://grillermo@192.168.1.1/rulinky_test"

// OpenTest returns a Store on a fresh, empty schema inside the test database
// (TEST_DATABASE_URL, whose name must end in _test). The schema is dropped
// when the test ends, so tests and packages can run in parallel.
func OpenTest(t testing.TB) *Store {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = defaultTestDatabaseURL
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatalf("refusing to use database %q: its name must end in _test", cfg.ConnConfig.Database)
	}
	ctx := context.Background()
	schema := "t_" + strings.ReplaceAll(newUUID(), "-", "")

	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		_ = admin.Close(ctx)
	})
	if _, err := pool.Exec(ctx, schemaSQL); err != nil {
		t.Fatal(err)
	}
	return New(pool)
}
```

- [ ] **Step 3: Write the ID helpers**

`internal/store/ids.go`:

```go
package store

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// newUUID returns a random (version 4) UUID string, the format the Rails app
// used for links.id and link_content_jobs.id. crypto/rand.Read never fails
// (Go 1.24+).
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// newAPIToken returns 24 random base58 characters, like Rails' has_secure_token.
func newAPIToken() (string, error) {
	out := make([]byte, 24)
	max := big.NewInt(int64(len(base58Alphabet)))
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = base58Alphabet[n.Int64()]
	}
	return string(out), nil
}
```

- [ ] **Step 4: Write the failing user tests**

`internal/store/users_test.go`:

```go
package store

import (
	"context"
	"testing"
)

func TestFindOrCreateUserByEmail(t *testing.T) {
	st := OpenTest(t)
	ctx := context.Background()

	u1, err := st.FindOrCreateUserByEmail(ctx, "a@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if u1.ID == 0 || u1.Email != "a@example.com" || len(u1.APIToken) != 24 {
		t.Fatalf("unexpected user %+v", u1)
	}
	u2, err := st.FindOrCreateUserByEmail(ctx, "a@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if u2.ID != u1.ID || u2.APIToken != u1.APIToken {
		t.Fatalf("second call must return the same user: %+v vs %+v", u2, u1)
	}
}

func TestFindOrCreateFillsMissingAPIToken(t *testing.T) {
	st := OpenTest(t)
	ctx := context.Background()
	if _, err := st.Pool().Exec(ctx, `INSERT INTO users (email, created_at, updated_at) VALUES ('old@example.com', now(), now())`); err != nil {
		t.Fatal(err)
	}
	u, err := st.FindOrCreateUserByEmail(ctx, "old@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(u.APIToken) != 24 {
		t.Fatalf("token not filled: %q", u.APIToken)
	}
}

func TestVerifyOTP(t *testing.T) {
	st := OpenTest(t)
	ctx := context.Background()
	u, _ := st.FindOrCreateUserByEmail(ctx, "me@example.com")
	if err := st.SetOTP(ctx, u.ID, "012345"); err != nil {
		t.Fatal(err)
	}

	if _, ok, err := st.VerifyOTP(ctx, "me@example.com", "999999", "1.2.3.4"); err != nil || ok {
		t.Fatalf("wrong code accepted: ok=%v err=%v", ok, err)
	}
	got, ok, err := st.VerifyOTP(ctx, "me@example.com", "012345", "1.2.3.4")
	if err != nil || !ok || got.ID != u.ID {
		t.Fatalf("right code rejected: ok=%v err=%v user=%+v", ok, err, got)
	}
	if _, ok, _ := st.VerifyOTP(ctx, "me@example.com", "012345", "1.2.3.4"); ok {
		t.Fatal("a code must work only once")
	}

	var count int
	var ip string
	var confirmed, otpNull bool
	err = st.Pool().QueryRow(ctx, `SELECT sign_in_count, current_sign_in_ip, confirmed_at IS NOT NULL, otp_code IS NULL FROM users WHERE id = $1`, u.ID).
		Scan(&count, &ip, &confirmed, &otpNull)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || ip != "1.2.3.4" || !confirmed || !otpNull {
		t.Fatalf("trackable/confirm not updated: count=%d ip=%q confirmed=%v otpNull=%v", count, ip, confirmed, otpNull)
	}
}

func TestVerifyOTPExpired(t *testing.T) {
	st := OpenTest(t)
	ctx := context.Background()
	u, _ := st.FindOrCreateUserByEmail(ctx, "me@example.com")
	_ = st.SetOTP(ctx, u.ID, "123456")
	if _, err := st.Pool().Exec(ctx, `UPDATE users SET otp_expires_at = `+nowUTC+` - interval '1 minute' WHERE id = $1`, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := st.VerifyOTP(ctx, "me@example.com", "123456", ""); ok {
		t.Fatal("expired code accepted")
	}
}

func TestVerifyOTPUnknownEmail(t *testing.T) {
	st := OpenTest(t)
	if _, ok, err := st.VerifyOTP(context.Background(), "nobody@example.com", "123456", ""); ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestUserLookups(t *testing.T) {
	st := OpenTest(t)
	ctx := context.Background()
	u, _ := st.FindOrCreateUserByEmail(ctx, "me@example.com")

	byID, ok, err := st.UserByID(ctx, u.ID)
	if err != nil || !ok || byID != u {
		t.Fatalf("UserByID: %+v ok=%v err=%v", byID, ok, err)
	}
	byToken, ok, err := st.UserByAPIToken(ctx, u.APIToken)
	if err != nil || !ok || byToken != u {
		t.Fatalf("UserByAPIToken: %+v ok=%v err=%v", byToken, ok, err)
	}
	if _, ok, _ := st.UserByAPIToken(ctx, "nope"); ok {
		t.Fatal("unknown token found a user")
	}
	if _, ok, _ := st.UserByID(ctx, 999999); ok {
		t.Fatal("unknown id found a user")
	}
}
```

- [ ] **Step 5: Run the tests to verify they fail**

Run: `go test ./internal/store/ -v`
Expected: FAIL, `undefined: ... FindOrCreateUserByEmail` (and the other user methods).

- [ ] **Step 6: Implement the users**

`internal/store/users.go`:

```go
package store

import (
	"context"
	"crypto/subtle"
	"errors"

	"github.com/jackc/pgx/v5"
)

type User struct {
	ID       int64
	Email    string
	APIToken string
}

// FindOrCreateUserByEmail returns the user with this (already normalized)
// email, creating it if needed, and makes sure it has an API token.
func (s *Store) FindOrCreateUserByEmail(ctx context.Context, email string) (User, error) {
	token, err := newAPIToken()
	if err != nil {
		return User{}, err
	}
	var u User
	err = s.pool.QueryRow(ctx, `
		INSERT INTO users (email, api_token, created_at, updated_at)
		VALUES ($1, $2, `+nowUTC+`, `+nowUTC+`)
		ON CONFLICT (email) DO UPDATE SET api_token = COALESCE(users.api_token, EXCLUDED.api_token)
		RETURNING id, email, api_token`, email, token).Scan(&u.ID, &u.Email, &u.APIToken)
	return u, err
}

// SetOTP stores a fresh one-time code that expires in 10 minutes.
func (s *Store) SetOTP(ctx context.Context, userID int64, code string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE users SET otp_code = $2, otp_expires_at = `+nowUTC+` + interval '10 minutes', updated_at = `+nowUTC+`
		WHERE id = $1`, userID, code)
	return err
}

// VerifyOTP checks the code for email. On success it consumes the code, sets
// confirmed_at the first time, and records the sign-in (Devise trackable).
// A wrong, expired or already-used code returns ok=false and no error.
func (s *Store) VerifyOTP(ctx context.Context, email, code, ip string) (User, bool, error) {
	var u User
	var stored string
	var fresh bool
	err := s.pool.QueryRow(ctx, `
		SELECT id, email, COALESCE(api_token, ''), COALESCE(otp_code, ''),
		       COALESCE(otp_expires_at > `+nowUTC+`, false)
		FROM users WHERE email = $1`, email).Scan(&u.ID, &u.Email, &u.APIToken, &stored, &fresh)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, err
	}
	if stored == "" || !fresh || subtle.ConstantTimeCompare([]byte(stored), []byte(code)) != 1 {
		return User{}, false, nil
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE users SET
		  otp_code = NULL, otp_expires_at = NULL,
		  confirmed_at = COALESCE(confirmed_at, `+nowUTC+`),
		  last_sign_in_at = COALESCE(current_sign_in_at, `+nowUTC+`),
		  last_sign_in_ip = COALESCE(current_sign_in_ip, $2),
		  current_sign_in_at = `+nowUTC+`,
		  current_sign_in_ip = $2,
		  sign_in_count = sign_in_count + 1,
		  updated_at = `+nowUTC+`
		WHERE id = $1 AND otp_code = $3`, u.ID, ip, stored)
	if err != nil {
		return User{}, false, err
	}
	return u, tag.RowsAffected() == 1, nil
}

func (s *Store) UserByID(ctx context.Context, id int64) (User, bool, error) {
	return s.userWhere(ctx, "id = $1", id)
}

func (s *Store) UserByAPIToken(ctx context.Context, token string) (User, bool, error) {
	return s.userWhere(ctx, "api_token = $1", token)
}

func (s *Store) userWhere(ctx context.Context, where string, arg any) (User, bool, error) {
	var u User
	err := s.pool.QueryRow(ctx, `SELECT id, email, COALESCE(api_token, '') FROM users WHERE `+where, arg).
		Scan(&u.ID, &u.Email, &u.APIToken)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, err
	}
	return u, true, nil
}
```

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go test ./internal/store/ -v`
Expected: PASS (6 tests).

- [ ] **Step 8: Commit**

```bash
git add internal/store/
git commit -m "feat(go): store with users, OTP and per-test schemas"
```

---

### Task 4: Store, links

**Files:**
- Create: `internal/store/links.go`
- Create: `internal/store/jobs.go` (only `JobStatus` here; the rest comes in Task 5)
- Test: `internal/store/links_test.go`

- [ ] **Step 1: Write the failing tests**

`internal/store/links_test.go`:

```go
package store

import (
	"context"
	"testing"
	"time"
)

func newUser(t *testing.T, st *Store, email string) User {
	t.Helper()
	u, err := st.FindOrCreateUserByEmail(context.Background(), email)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestCreateAndListLinks(t *testing.T) {
	st := OpenTest(t)
	ctx := context.Background()
	u := newUser(t, st, "me@example.com")

	l1, job1, err := st.CreateLink(ctx, u.ID, "https://a.com", "first")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * time.Millisecond)
	l2, _, err := st.CreateLink(ctx, u.ID, "https://b.com", "")
	if err != nil {
		t.Fatal(err)
	}

	links, err := st.ListLinks(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 || links[0].ID != l2.ID || links[1].ID != l1.ID {
		t.Fatalf("want newest first, got %+v", links)
	}
	got := links[1]
	if got.URL != "https://a.com" || got.Note == nil || *got.Note != "first" || got.Read != 0 ||
		got.Timestamp != got.UpdatedAt || got.Timestamp < time.Now().Add(-time.Minute).UnixMilli() {
		t.Fatalf("unexpected link %+v", got)
	}
	status, _, found, err := st.JobStatus(ctx, job1)
	if err != nil || !found || status != "queued" {
		t.Fatalf("job: status=%q found=%v err=%v", status, found, err)
	}

	other := newUser(t, st, "other@example.com")
	if links, _ := st.ListLinks(ctx, other.ID); len(links) != 0 {
		t.Fatalf("other user sees %d links", len(links))
	}
}

func TestListLinksLoadsContentOnlyWithoutRawTitle(t *testing.T) {
	st := OpenTest(t)
	ctx := context.Background()
	u := newUser(t, st, "me@example.com")
	withTitle, _, _ := st.CreateLink(ctx, u.ID, "https://a.com", "")
	withoutTitle, _, _ := st.CreateLink(ctx, u.ID, "https://b.com", "")
	_, _ = st.Pool().Exec(ctx, `UPDATE links SET raw_title = 'T', content = '<p>big</p>' WHERE id = $1`, withTitle.ID)
	_, _ = st.Pool().Exec(ctx, `UPDATE links SET raw_title = '', content = '<title>C</title>' WHERE id = $1`, withoutTitle.ID)

	links, _ := st.ListLinks(ctx, u.ID)
	for _, l := range links {
		switch l.ID {
		case withTitle.ID:
			if l.Content != nil || l.RawTitle == nil || *l.RawTitle != "T" {
				t.Errorf("with raw title: %+v", l)
			}
		case withoutTitle.ID:
			if l.Content == nil || *l.Content != "<title>C</title>" {
				t.Errorf("without raw title: %+v", l)
			}
		}
	}
}

func TestSetLinkRead(t *testing.T) {
	st := OpenTest(t)
	ctx := context.Background()
	u := newUser(t, st, "me@example.com")
	l, _, _ := st.CreateLink(ctx, u.ID, "https://a.com", "")
	time.Sleep(3 * time.Millisecond)

	other := newUser(t, st, "other@example.com")
	if ok, err := st.SetLinkRead(ctx, other.ID, l.ID, true); ok || err != nil {
		t.Fatalf("other user updated link: ok=%v err=%v", ok, err)
	}
	if ok, err := st.SetLinkRead(ctx, u.ID, l.ID, true); !ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	links, _ := st.ListLinks(ctx, u.ID)
	if links[0].Read != 1 || links[0].UpdatedAt <= l.UpdatedAt {
		t.Fatalf("read not set or updated_at not bumped: %+v", links[0])
	}
	_, _ = st.SetLinkRead(ctx, u.ID, l.ID, false)
	links, _ = st.ListLinks(ctx, u.ID)
	if links[0].Read != 0 {
		t.Fatalf("unread not set: %+v", links[0])
	}
}

func TestDeleteLink(t *testing.T) {
	st := OpenTest(t)
	ctx := context.Background()
	u := newUser(t, st, "me@example.com")
	l, jobID, _ := st.CreateLink(ctx, u.ID, "https://a.com", "")

	other := newUser(t, st, "other@example.com")
	if ok, err := st.DeleteLink(ctx, other.ID, l.ID); ok || err != nil {
		t.Fatalf("other user deleted link: ok=%v err=%v", ok, err)
	}
	if ok, err := st.DeleteLink(ctx, u.ID, l.ID); !ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if links, _ := st.ListLinks(ctx, u.ID); len(links) != 0 {
		t.Fatal("link still listed")
	}
	if _, _, found, _ := st.JobStatus(ctx, jobID); found {
		t.Fatal("job not deleted with its link")
	}
	if ok, _ := st.DeleteLink(ctx, u.ID, l.ID); ok {
		t.Fatal("second delete reported success")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/store/ -run 'Link' -v`
Expected: FAIL, `undefined: ... CreateLink` (and the other link methods and `JobStatus`).

- [ ] **Step 3: Implement the links**

`internal/store/links.go`:

```go
package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// Link mirrors a links row. Content is loaded only when RawTitle is blank,
// because the only thing that reads it is the title fallback (and it can be
// a whole article).
type Link struct {
	ID        string
	URL       string
	Note      *string
	Read      int
	Timestamp int64
	UpdatedAt int64
	Content   *string
	RawTitle  *string
}

const linkColumns = `id, url, note, read, "timestamp", updated_at,
	CASE WHEN btrim(COALESCE(raw_title, '')) = '' THEN content END, raw_title`

func scanLink(row pgx.Row) (Link, error) {
	var l Link
	err := row.Scan(&l.ID, &l.URL, &l.Note, &l.Read, &l.Timestamp, &l.UpdatedAt, &l.Content, &l.RawTitle)
	return l, err
}

// ListLinks returns all of the user's links, most recently updated first.
func (s *Store) ListLinks(ctx context.Context, userID int64) ([]Link, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+linkColumns+` FROM links WHERE user_id = $1 ORDER BY updated_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Link, error) { return scanLink(row) })
}

// CreateLink inserts an unread link and its queued content job in one
// transaction, and returns the link and the job id.
func (s *Store) CreateLink(ctx context.Context, userID int64, url, note string) (Link, string, error) {
	nowMs := time.Now().UnixMilli()
	link := Link{ID: newUUID(), URL: url, Note: &note, Timestamp: nowMs, UpdatedAt: nowMs}
	jobID := newUUID()
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO links (id, user_id, url, note, read, "timestamp", updated_at)
			VALUES ($1, $2, $3, $4, 0, $5, $5)`, link.ID, userID, url, note, nowMs); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO link_content_jobs (id, link_id, status, created_at, updated_at)
			VALUES ($1, $2, 'queued', `+nowUTC+`, `+nowUTC+`)`, jobID, link.ID)
		return err
	})
	if err != nil {
		return Link{}, "", err
	}
	return link, jobID, nil
}

// SetLinkRead sets read to 1 or 0 and bumps updated_at. found is false when
// the link does not exist or belongs to another user.
func (s *Store) SetLinkRead(ctx context.Context, userID int64, id string, read bool) (bool, error) {
	value := 0
	if read {
		value = 1
	}
	tag, err := s.pool.Exec(ctx, `UPDATE links SET read = $3, updated_at = $4 WHERE id = $2 AND user_id = $1`,
		userID, id, value, time.Now().UnixMilli())
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// DeleteLink deletes the user's link and its content jobs.
func (s *Store) DeleteLink(ctx context.Context, userID int64, id string) (bool, error) {
	found := false
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM links WHERE id = $2 AND user_id = $1`, userID, id)
		if err != nil {
			return err
		}
		found = tag.RowsAffected() == 1
		if !found {
			return nil
		}
		_, err = tx.Exec(ctx, `DELETE FROM link_content_jobs WHERE link_id = $1`, id)
		return err
	})
	return found, err
}
```

`internal/store/jobs.go` (it grows in Task 5):

```go
package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// JobStatus returns a content job's status and error message.
func (s *Store) JobStatus(ctx context.Context, id string) (status, errorMessage string, found bool, err error) {
	err = s.pool.QueryRow(ctx, `SELECT status, COALESCE(error_message, '') FROM link_content_jobs WHERE id = $1`, id).
		Scan(&status, &errorMessage)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return status, errorMessage, true, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/store/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/
git commit -m "feat(go): store links scoped by user"
```

---

### Task 5: Store, content job queue

**Files:**
- Modify: `internal/store/jobs.go`
- Test: `internal/store/jobs_test.go`

- [ ] **Step 1: Write the failing tests**

`internal/store/jobs_test.go`:

```go
package store

import (
	"context"
	"testing"
	"time"
)

func TestClaimFinishJobs(t *testing.T) {
	st := OpenTest(t)
	ctx := context.Background()
	u := newUser(t, st, "me@example.com")
	l1, job1, _ := st.CreateLink(ctx, u.ID, "https://a.com", "")
	time.Sleep(3 * time.Millisecond)
	_, job2, _ := st.CreateLink(ctx, u.ID, "https://b.com", "")

	got, ok, err := st.ClaimNextJob(ctx)
	if err != nil || !ok || got.ID != job1 || got.LinkID != l1.ID || got.URL != "https://a.com" {
		t.Fatalf("first claim: %+v ok=%v err=%v", got, ok, err)
	}
	if status, _, _, _ := st.JobStatus(ctx, job1); status != "running" {
		t.Fatalf("claimed job status = %q", status)
	}
	second, ok, _ := st.ClaimNextJob(ctx)
	if !ok || second.ID != job2 {
		t.Fatalf("second claim: %+v ok=%v", second, ok)
	}
	if _, ok, _ := st.ClaimNextJob(ctx); ok {
		t.Fatal("claimed a job from an empty queue")
	}

	if err := st.FinishJob(ctx, job1, l1.ID, "Title", "<p>Body</p>"); err != nil {
		t.Fatal(err)
	}
	if status, _, _, _ := st.JobStatus(ctx, job1); status != "finished" {
		t.Fatalf("status = %q", status)
	}
	var title, content string
	var updatedAt int64
	_ = st.Pool().QueryRow(ctx, `SELECT raw_title, content, updated_at FROM links WHERE id = $1`, l1.ID).Scan(&title, &content, &updatedAt)
	if title != "Title" || content != "<p>Body</p>" || updatedAt != l1.UpdatedAt {
		t.Fatalf("link after finish: title=%q content=%q updatedAt=%d (want %d)", title, content, updatedAt, l1.UpdatedAt)
	}
}

func TestFailAndRequeueJobs(t *testing.T) {
	st := OpenTest(t)
	ctx := context.Background()
	u := newUser(t, st, "me@example.com")
	_, job1, _ := st.CreateLink(ctx, u.ID, "https://a.com", "")
	_, _ = st.ClaimNextJob(ctx)

	if err := st.FailJob(ctx, job1, "boom"); err != nil {
		t.Fatal(err)
	}
	if status, msg, _, _ := st.JobStatus(ctx, job1); status != "failed" || msg != "boom" {
		t.Fatalf("status=%q msg=%q", status, msg)
	}

	_, job2, _ := st.CreateLink(ctx, u.ID, "https://b.com", "")
	_, _ = st.ClaimNextJob(ctx) // job2 is now "running", as if the process died mid-job
	n, err := st.RequeueRunningJobs(ctx)
	if err != nil || n != 1 {
		t.Fatalf("requeued %d, err=%v", n, err)
	}
	if status, _, _, _ := st.JobStatus(ctx, job2); status != "queued" {
		t.Fatalf("status = %q", status)
	}
}

func TestClaimJobWhoseLinkIsGone(t *testing.T) {
	st := OpenTest(t)
	ctx := context.Background()
	if _, err := st.Pool().Exec(ctx, `INSERT INTO link_content_jobs (id, link_id, status, created_at, updated_at) VALUES ('j1', 'missing', 'queued', now(), now())`); err != nil {
		t.Fatal(err)
	}
	job, ok, err := st.ClaimNextJob(ctx)
	if err != nil || !ok || job.URL != "" {
		t.Fatalf("job=%+v ok=%v err=%v", job, ok, err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/store/ -run 'Job' -v`
Expected: FAIL, `undefined: ... ClaimNextJob`.

- [ ] **Step 3: Implement (append to `internal/store/jobs.go`)**

```go
// Job is a claimed content job. URL is "" when its link no longer exists.
type Job struct {
	ID     string
	LinkID string
	URL    string
}

// ClaimNextJob marks the oldest queued job as running and returns it.
func (s *Store) ClaimNextJob(ctx context.Context) (Job, bool, error) {
	var j Job
	err := s.pool.QueryRow(ctx, `
		WITH next AS (
		  SELECT id FROM link_content_jobs WHERE status = 'queued'
		  ORDER BY created_at, id LIMIT 1 FOR UPDATE SKIP LOCKED
		)
		UPDATE link_content_jobs j
		SET status = 'running', error_message = NULL, updated_at = `+nowUTC+`
		FROM next WHERE j.id = next.id
		RETURNING j.id, j.link_id, COALESCE((SELECT url FROM links WHERE links.id = j.link_id), '')`).
		Scan(&j.ID, &j.LinkID, &j.URL)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, err
	}
	return j, true, nil
}

// FinishJob stores the fetched title and content on the link (without
// touching its updated_at, so it keeps its place in the list) and marks the
// job finished.
func (s *Store) FinishJob(ctx context.Context, jobID, linkID, title, content string) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE links SET content = $2, raw_title = $3 WHERE id = $1`, linkID, content, title); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE link_content_jobs SET status = 'finished', updated_at = `+nowUTC+` WHERE id = $1`, jobID)
		return err
	})
}

func (s *Store) FailJob(ctx context.Context, jobID, message string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE link_content_jobs SET status = 'failed', error_message = $2, updated_at = `+nowUTC+`
		WHERE id = $1`, jobID, message)
	return err
}

// RequeueRunningJobs puts back jobs left running by a process that stopped
// mid-job. Call it once at startup, before the worker claims anything.
func (s *Store) RequeueRunningJobs(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE link_content_jobs SET status = 'queued', updated_at = `+nowUTC+` WHERE status = 'running'`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/store/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/
git commit -m "feat(go): persistent content job queue"
```

---

### Task 6: Display titles

**Files:**
- Create: `internal/titles/titles.go`
- Test: `internal/titles/titles_test.go`

- [ ] **Step 1: Write the failing tests**

`internal/titles/titles_test.go`:

```go
package titles

import (
	"strings"
	"testing"
)

func ptr(s string) *string { return &s }

func TestContentTitle(t *testing.T) {
	cases := []struct {
		name          string
		raw, content  *string
		want          string
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/titles/ -v`
Expected: FAIL, `undefined: ContentTitle`.

- [ ] **Step 3: Implement**

`internal/titles/titles.go`:

```go
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
	wwwPrefix      = regexp.MustCompile(`(?i)^www\.`)
	schemePrefix   = regexp.MustCompile(`(?i)^https?://(www\.)?`)
	queryAndAfter  = regexp.MustCompile(`\?.*$`)
	defaultPorts   = map[string]string{"http": "80", "https": "443"}
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
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/titles/ -v`
Expected: PASS. The `"https://exa mple.com/x?y"` case exercises the regex fallback: `url.Parse` rejects a space in the host.

- [ ] **Step 5: Commit**

```bash
git add internal/titles/
git commit -m "feat(go): display title rules with URL fallback fix"
```

---

### Task 7: Auth primitives (session cookie, OTP, email, Slack)

**Files:**
- Create: `internal/auth/session.go`, `internal/auth/otp.go`, `internal/auth/slack.go`
- Test: `internal/auth/auth_test.go`

- [ ] **Step 1: Write the failing tests**

`internal/auth/auth_test.go`:

```go
package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

func roundTrip(s *Sessions, sess Session) *http.Request {
	rec := httptest.NewRecorder()
	s.Write(rec, sess)
	req := httptest.NewRequest("GET", "/", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	return req
}

func TestSessionRoundTrip(t *testing.T) {
	s := NewSessions([]byte(strings.Repeat("k", 32)), true)
	got := s.Read(roundTrip(s, Session{UserID: 7, AuthEmail: "a@b.c"}))
	if got.UserID != 7 || got.AuthEmail != "a@b.c" {
		t.Fatalf("got %+v", got)
	}

	rec := httptest.NewRecorder()
	s.Write(rec, Session{UserID: 7})
	c := rec.Result().Cookies()[0]
	if c.Name != CookieName || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.MaxAge != int(Timeout/time.Second) {
		t.Fatalf("cookie flags wrong: %+v", c)
	}
}

func TestSessionRejectsTamperingAndOtherSecrets(t *testing.T) {
	s := NewSessions([]byte(strings.Repeat("k", 32)), false)
	req := roundTrip(s, Session{UserID: 7})
	c, _ := req.Cookie(CookieName)

	forged := httptest.NewRequest("GET", "/", nil)
	forged.AddCookie(&http.Cookie{Name: CookieName, Value: "x" + c.Value})
	if got := s.Read(forged); got.UserID != 0 {
		t.Fatal("tampered cookie accepted")
	}
	other := NewSessions([]byte(strings.Repeat("z", 32)), false)
	if got := other.Read(req); got.UserID != 0 {
		t.Fatal("cookie signed with another secret accepted")
	}
}

func TestSessionExpiresAfterInactivity(t *testing.T) {
	s := NewSessions([]byte(strings.Repeat("k", 32)), false)
	start := time.Now()
	s.now = func() time.Time { return start }
	req := roundTrip(s, Session{UserID: 7})

	s.now = func() time.Time { return start.Add(Timeout - time.Minute) }
	if s.Read(req).UserID != 7 {
		t.Fatal("expired too early")
	}
	s.now = func() time.Time { return start.Add(Timeout + time.Minute) }
	if s.Read(req).UserID != 0 {
		t.Fatal("not expired after the timeout")
	}
}

func TestSessionClear(t *testing.T) {
	s := NewSessions([]byte(strings.Repeat("k", 32)), false)
	rec := httptest.NewRecorder()
	s.Clear(rec)
	if c := rec.Result().Cookies()[0]; c.Name != CookieName || c.MaxAge >= 0 {
		t.Fatalf("clear cookie wrong: %+v", c)
	}
}

func TestNewOTP(t *testing.T) {
	re := regexp.MustCompile(`^\d{6}$`)
	for range 50 {
		code, err := NewOTP()
		if err != nil || !re.MatchString(code) {
			t.Fatalf("code %q err %v", code, err)
		}
	}
}

func TestValidEmail(t *testing.T) {
	for _, e := range []string{"me@example.com", "a.b+c@sub.example.co"} {
		if !ValidEmail(e) {
			t.Errorf("%q should be valid", e)
		}
	}
	for _, e := range []string{"", "me", "me@", "@example.com", "me@exa mple.com", "me@-x.com"} {
		if ValidEmail(e) {
			t.Errorf("%q should be invalid", e)
		}
	}
}

func TestSlackSendOTP(t *testing.T) {
	var got map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content type %q", r.Header.Get("Content-Type"))
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
	}))
	defer srv.Close()

	n := SlackNotifier{WebhookURL: srv.URL, Client: srv.Client()}
	if err := n.SendOTP(context.Background(), "123456"); err != nil {
		t.Fatal(err)
	}
	if got["text"] != "Your OTP for rulinky 123456" {
		t.Fatalf("payload %v", got)
	}
}

func TestSlackSkipsWithoutWebhookAndReportsErrors(t *testing.T) {
	if err := (SlackNotifier{}).SendOTP(context.Background(), "1"); err != nil {
		t.Fatalf("empty webhook must be a no-op, got %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	if err := (SlackNotifier{WebhookURL: srv.URL, Client: srv.Client()}).SendOTP(context.Background(), "1"); err == nil {
		t.Fatal("expected an error on HTTP 500")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/auth/ -v`
Expected: FAIL, `undefined: NewSessions` and the rest.

- [ ] **Step 3: Implement**

`internal/auth/session.go`:

```go
// Package auth holds the session cookie, OTP generation and OTP delivery.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const (
	CookieName = "_rulinky_session"
	// Timeout is the inactivity limit (Devise timeoutable was 10 days).
	Timeout = 10 * 24 * time.Hour
)

// Session is what the signed cookie carries. UserID is set once signed in;
// AuthEmail is the address waiting for its OTP.
type Session struct {
	UserID    int64  `json:"u,omitempty"`
	AuthEmail string `json:"e,omitempty"`
	LastSeen  int64  `json:"t"`
}

type Sessions struct {
	secret []byte
	secure bool
	now    func() time.Time
}

func NewSessions(secret []byte, secure bool) *Sessions {
	return &Sessions{secret: secret, secure: secure, now: time.Now}
}

// Read returns the request's session, or a zero Session when the cookie is
// missing, forged, or older than Timeout since its last write.
func (s *Sessions) Read(r *http.Request) Session {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return Session{}
	}
	payload, sig, ok := strings.Cut(c.Value, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(s.sign(payload))) {
		return Session{}
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return Session{}
	}
	var sess Session
	if json.Unmarshal(raw, &sess) != nil {
		return Session{}
	}
	if s.now().Sub(time.Unix(sess.LastSeen, 0)) > Timeout {
		return Session{}
	}
	return sess
}

// Write stores sess (stamped with the current time) in the cookie. Call it
// before writing the response body.
func (s *Sessions) Write(w http.ResponseWriter, sess Session) {
	sess.LastSeen = s.now().Unix()
	raw, _ := json.Marshal(sess)
	payload := base64.RawURLEncoding.EncodeToString(raw)
	http.SetCookie(w, s.cookie(payload+"."+s.sign(payload), int(Timeout/time.Second)))
}

func (s *Sessions) Clear(w http.ResponseWriter) {
	http.SetCookie(w, s.cookie("", -1))
}

func (s *Sessions) cookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name: CookieName, Value: value, Path: "/", MaxAge: maxAge,
		HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteLaxMode,
	}
}

func (s *Sessions) sign(payload string) string {
	m := hmac.New(sha256.New, s.secret)
	m.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}
```

`internal/auth/otp.go`:

```go
package auth

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"regexp"
)

// NewOTP returns a random 6-digit code, zero-padded.
func NewOTP() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// emailPattern is Ruby's URI::MailTo::EMAIL_REGEXP.
var emailPattern = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")

func ValidEmail(email string) bool { return emailPattern.MatchString(email) }
```

`internal/auth/slack.go`:

```go
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// SlackNotifier posts OTP codes to a Slack incoming webhook. With no
// WebhookURL it does nothing (the code is still logged by the caller).
type SlackNotifier struct {
	WebhookURL string
	Client     *http.Client
}

func (n SlackNotifier) SendOTP(ctx context.Context, code string) error {
	if n.WebhookURL == "" {
		return nil
	}
	body, _ := json.Marshal(map[string]string{"text": "Your OTP for rulinky " + code})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := n.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("slack webhook answered %d", resp.StatusCode)
	}
	return nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/auth/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/
git commit -m "feat(go): signed session cookie, OTP and Slack delivery"
```

---

### Task 8: readitsoon preview client

**Files:**
- Create: `internal/content/readitsoon.go`
- Test: `internal/content/readitsoon_test.go`

- [ ] **Step 1: Write the failing tests (ported from `test/services/readitsoon_preview_test.rb`)**

`internal/content/readitsoon_test.go`:

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/content/ -v`
Expected: FAIL, `undefined: NewReaditsoon`.

- [ ] **Step 3: Implement**

`internal/content/readitsoon.go`:

```go
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
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/content/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/content/
git commit -m "feat(go): readitsoon preview client"
```

---

### Task 9: Serial content worker

**Files:**
- Create: `internal/content/worker.go`
- Test: `internal/content/worker_test.go`

- [ ] **Step 1: Write the failing tests**

`internal/content/worker_test.go`:

```go
package content

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"rulinky/internal/store"
)

type fakeCrawler struct {
	html string
	err  error
	urls []string
}

func (f *fakeCrawler) Crawl(_ context.Context, u string) (string, error) {
	f.urls = append(f.urls, u)
	return f.html, f.err
}

type fakePreviewer struct {
	gotHTML string
	preview Preview
	err     error
}

func (f *fakePreviewer) Preview(_ context.Context, html, _ string) (Preview, error) {
	f.gotHTML = html
	return f.preview, f.err
}

var discard = slog.New(slog.NewTextHandler(io.Discard, nil))

func setupJob(t *testing.T) (*store.Store, store.Link, string) {
	t.Helper()
	st := store.OpenTest(t)
	ctx := context.Background()
	u, err := st.FindOrCreateUserByEmail(ctx, "me@example.com")
	if err != nil {
		t.Fatal(err)
	}
	link, jobID, err := st.CreateLink(ctx, u.ID, "https://a.com", "")
	if err != nil {
		t.Fatal(err)
	}
	return st, link, jobID
}

func TestWorkerRunOnceFinishesJob(t *testing.T) {
	st, link, jobID := setupJob(t)
	crawler := &fakeCrawler{html: "<html>rendered</html>"}
	prev := &fakePreviewer{preview: Preview{Title: "Title", Content: "<p>x</p>"}}
	w := NewWorker(st, crawler, prev, discard)

	processed, err := w.RunOnce(context.Background())
	if err != nil || !processed {
		t.Fatalf("processed=%v err=%v", processed, err)
	}
	if len(crawler.urls) != 1 || crawler.urls[0] != link.URL || prev.gotHTML != "<html>rendered</html>" {
		t.Fatalf("crawler urls=%v preview html=%q", crawler.urls, prev.gotHTML)
	}
	if status, _, _, _ := st.JobStatus(context.Background(), jobID); status != "finished" {
		t.Fatalf("status = %q", status)
	}
	if processed, _ := w.RunOnce(context.Background()); processed {
		t.Fatal("processed a job from an empty queue")
	}
}

func TestWorkerRunOnceRecordsFailure(t *testing.T) {
	st, _, jobID := setupJob(t)
	w := NewWorker(st, &fakeCrawler{err: errors.New("boom")}, &fakePreviewer{}, discard)

	if processed, err := w.RunOnce(context.Background()); !processed || err != nil {
		t.Fatalf("processed=%v err=%v", processed, err)
	}
	if status, msg, _, _ := st.JobStatus(context.Background(), jobID); status != "failed" || msg != "boom" {
		t.Fatalf("status=%q msg=%q", status, msg)
	}
}

func TestWorkerRunRequeuesThenProcesses(t *testing.T) {
	st, _, jobID := setupJob(t)
	if _, _, err := st.ClaimNextJob(context.Background()); err != nil { // left "running" by a dead process
		t.Fatal(err)
	}
	w := NewWorker(st, &fakeCrawler{html: "h"}, &fakePreviewer{preview: Preview{Title: "T"}}, discard)
	w.PollInterval = 10 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	deadline := time.Now().Add(3 * time.Second)
	for {
		status, _, _, _ := st.JobStatus(context.Background(), jobID)
		if status == "finished" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job never finished, status %q", status)
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run returned %v", err)
	}
}

func TestWorkerNotifyNeverBlocks(t *testing.T) {
	w := NewWorker(nil, nil, nil, discard)
	for range 5 {
		w.Notify()
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/content/ -run Worker -v`
Expected: FAIL, `undefined: NewWorker`.

- [ ] **Step 3: Implement**

`internal/content/worker.go`:

```go
package content

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"rulinky/internal/store"
)

type Crawler interface {
	Crawl(ctx context.Context, pageURL string) (string, error)
}

type Previewer interface {
	Preview(ctx context.Context, html, pageURL string) (Preview, error)
}

// Worker processes content jobs one at a time, so only one Chrome runs at once.
type Worker struct {
	Store        *store.Store
	Crawler      Crawler
	Previewer    Previewer
	Logger       *slog.Logger
	PollInterval time.Duration
	wake         chan struct{}
}

func NewWorker(st *store.Store, c Crawler, p Previewer, logger *slog.Logger) *Worker {
	return &Worker{Store: st, Crawler: c, Previewer: p, Logger: logger, PollInterval: time.Minute, wake: make(chan struct{}, 1)}
}

// Notify wakes the worker after a job was queued. It never blocks.
func (w *Worker) Notify() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// Run requeues jobs left running by a previous process, then processes the
// queue until ctx is cancelled. It waits for Notify or PollInterval between
// passes.
func (w *Worker) Run(ctx context.Context) error {
	n, err := w.Store.RequeueRunningJobs(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		w.Logger.Info("requeued interrupted content jobs", "count", n)
	}
	ticker := time.NewTicker(w.PollInterval)
	defer ticker.Stop()
	for {
		for {
			processed, err := w.RunOnce(ctx)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err != nil {
				w.Logger.Error("content worker", "err", err)
				break
			}
			if !processed {
				break
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-w.wake:
		case <-ticker.C:
		}
	}
}

// RunOnce claims and processes one job. processed is false when the queue is
// empty. A failing job is recorded on the job row, not returned. A job
// interrupted by shutdown stays "running" and is requeued at the next start.
func (w *Worker) RunOnce(ctx context.Context) (bool, error) {
	job, ok, err := w.Store.ClaimNextJob(ctx)
	if err != nil || !ok {
		return false, err
	}
	w.Logger.Info("content job start", "job_id", job.ID, "url", job.URL)
	title, contentHTML, err := w.fetch(ctx, job)
	if ctx.Err() != nil {
		return true, ctx.Err()
	}
	if err != nil {
		w.Logger.Error("content job failed", "job_id", job.ID, "err", err)
		return true, w.Store.FailJob(ctx, job.ID, err.Error())
	}
	if err := w.Store.FinishJob(ctx, job.ID, job.LinkID, title, contentHTML); err != nil {
		return true, err
	}
	w.Logger.Info("content job finished", "job_id", job.ID, "title", title, "content_bytes", len(contentHTML))
	return true, nil
}

func (w *Worker) fetch(ctx context.Context, job store.Job) (string, string, error) {
	if job.URL == "" {
		return "", "", errors.New("link not found")
	}
	html, err := w.Crawler.Crawl(ctx, job.URL)
	if err != nil {
		return "", "", err
	}
	preview, err := w.Previewer.Preview(ctx, html, job.URL)
	if err != nil {
		return "", "", err
	}
	return preview.Title, preview.Content, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/content/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/content/
git commit -m "feat(go): serial persistent content worker"
```

---

### Task 10: Chrome crawler

**Files:**
- Create: `internal/content/crawler.go`
- Test: `internal/content/crawler_test.go`

The crawl itself kills the user's Chrome, so no automated test runs it. Task 17 checks it by hand with the `-crawl` flag. Only the profile copy is unit-tested.

- [ ] **Step 1: Write the failing test**

`internal/content/crawler_test.go`:

```go
package content

import (
	"os"
	"path/filepath"
	"testing"
)

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSyncProfileCopiesSessionFiles(t *testing.T) {
	real, auto := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(real, "Default", "Cookies"), "cookies")
	mustWrite(t, filepath.Join(real, "Default", "Local Storage", "leveldb", "000003.log"), "ls")
	mustWrite(t, filepath.Join(real, "Default", "IndexedDB", "site", "data"), "idb")
	mustWrite(t, filepath.Join(auto, "Default", "Local Storage", "stale"), "old")

	c := &ChromeCrawler{RealUserDataDir: real, UserDataDir: auto, Profile: "Default"}
	if err := c.syncProfile(); err != nil {
		t.Fatal(err)
	}

	for path, want := range map[string]string{
		"Cookies":                          "cookies",
		"Local Storage/leveldb/000003.log": "ls",
		"IndexedDB/site/data":              "idb",
	} {
		got, err := os.ReadFile(filepath.Join(auto, "Default", path))
		if err != nil || string(got) != want {
			t.Errorf("%s: got %q err %v", path, got, err)
		}
	}
	if _, err := os.Stat(filepath.Join(auto, "Default", "Local Storage", "stale")); !os.IsNotExist(err) {
		t.Error("stale file from the previous copy was kept")
	}
	if _, err := os.Stat(filepath.Join(auto, "Default", "Session Storage")); !os.IsNotExist(err) {
		t.Error("missing source must be skipped, not created")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/content/ -run SyncProfile -v`
Expected: FAIL, `undefined: ChromeCrawler`.

- [ ] **Step 3: Implement**

`internal/content/crawler.go`:

```go
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
	if err := chromedp.Run(tabCtx); err != nil {
		return "", err
	}
	navCtx, cancelNav := context.WithTimeout(tabCtx, timeout)
	defer cancelNav()

	logger.Info("crawler navigating", "url", pageURL)
	var html string
	if err := chromedp.Run(navCtx, chromedp.Navigate(pageURL), chromedp.Evaluate(cleanJS, &html)); err != nil {
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
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/content/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/content/
git commit -m "feat(go): chromedp crawler with full profile sync"
```

---

### Task 11: Server foundation (params, pages, middleware, /up, static)

**Files:**
- Create: `internal/server/server.go`, `internal/server/params.go`, `internal/server/pages.go`, `internal/server/middleware.go`
- Create (stub handlers, filled in by Tasks 12–14): `internal/server/auth_handlers.go`, `internal/server/links_handlers.go`, `internal/server/api_handlers.go`
- Test: `internal/server/helpers_test.go`, `internal/server/foundation_test.go`

- [ ] **Step 1: Write the test helpers and the failing foundation tests**

`internal/server/helpers_test.go`:

```go
package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"rulinky/internal/auth"
	"rulinky/internal/store"
)

type fakeOTP struct{ codes []string }

func (f *fakeOTP) SendOTP(_ context.Context, code string) error {
	f.codes = append(f.codes, code)
	return nil
}

type fakeJobs struct{ n int }

func (f *fakeJobs) Notify() { f.n++ }

const testIndex = `<!DOCTYPE html><html><head><!-- PAGE_DATA --></head><body><div id="app"></div></body></html>`

func newTestServer(t *testing.T) (*Server, *fakeOTP, *fakeJobs) {
	t.Helper()
	pages, err := NewPages([]byte(testIndex))
	if err != nil {
		t.Fatal(err)
	}
	otp, jobs := &fakeOTP{}, &fakeJobs{}
	srv := &Server{
		Store:    store.OpenTest(t),
		Sessions: auth.NewSessions([]byte(strings.Repeat("k", 32)), false),
		OTP:      otp,
		Jobs:     jobs,
		Pages:    pages,
		Static: fstest.MapFS{
			"favicon.svg":       {Data: []byte("<svg/>")},
			"assets/app-123.js": {Data: []byte("console.log(1)")},
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	return srv, otp, jobs
}

func serve(srv *Server, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func jsonRequest(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return req
}

func createUser(t *testing.T, srv *Server, email string) store.User {
	t.Helper()
	u, err := srv.Store.FindOrCreateUserByEmail(context.Background(), email)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func sessionCookie(t *testing.T, srv *Server, sess auth.Session) *http.Cookie {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Sessions.Write(rec, sess)
	return rec.Result().Cookies()[0]
}

func cookieFrom(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	var found *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName {
			found = c
		}
	}
	if found == nil {
		t.Fatal("response set no session cookie")
	}
	return found
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return v
}
```

`internal/server/foundation_test.go`:

```go
package server

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUp(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rec := serve(srv, httptest.NewRequest("GET", "/up", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "background-color: green") {
		t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestStaticFilesAndCaching(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rec := serve(srv, httptest.NewRequest("GET", "/favicon.svg", nil))
	if rec.Code != 200 || rec.Body.String() != "<svg/>" {
		t.Fatalf("favicon: %d %q", rec.Code, rec.Body.String())
	}
	rec = serve(srv, httptest.NewRequest("GET", "/assets/app-123.js", nil))
	if rec.Code != 200 || !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("asset: %d cache=%q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	if rec := serve(srv, httptest.NewRequest("GET", "/nope.txt", nil)); rec.Code != 404 {
		t.Fatalf("missing file: %d", rec.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rec := serve(srv, httptest.NewRequest("GET", "/up", nil))
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("X-Frame-Options") != "SAMEORIGIN" {
		t.Fatalf("headers: %v", rec.Header())
	}
	if rec.Header().Get("Strict-Transport-Security") != "" {
		t.Fatal("HSTS must be off outside production")
	}
	srv.Production = true
	rec = serve(srv, httptest.NewRequest("GET", "/up", nil))
	if !strings.HasPrefix(rec.Header().Get("Strict-Transport-Security"), "max-age=") {
		t.Fatal("HSTS missing in production")
	}
}

func TestPagesRequireMarker(t *testing.T) {
	if _, err := NewPages([]byte("<html></html>")); err == nil {
		t.Fatal("expected an error without the PAGE_DATA marker")
	}
}

func TestCastBool(t *testing.T) {
	cases := []struct {
		in       any
		want, ok bool
	}{
		{true, true, true}, {false, false, true},
		{float64(1), true, true}, {float64(0), false, true},
		{"true", true, true}, {"1", true, true}, {"yes", true, true}, {"False", true, true},
		{"false", false, true}, {"FALSE", false, true}, {"0", false, true}, {"f", false, true},
		{"F", false, true}, {"off", false, true}, {"OFF", false, true},
		{"", false, false}, {nil, false, false},
	}
	for _, c := range cases {
		got, ok := castBool(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("castBool(%#v) = %v,%v want %v,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/server/ -v`
Expected: FAIL, `undefined: Server` / `NewPages` / `castBool`.

- [ ] **Step 3: Implement the foundation**

`internal/server/server.go`:

```go
// Package server is Rulinky's HTTP layer.
package server

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"rulinky/internal/auth"
	"rulinky/internal/store"
)

type OTPSender interface {
	SendOTP(ctx context.Context, code string) error
}

type JobNotifier interface {
	Notify()
}

type Server struct {
	Store      *store.Store
	Sessions   *auth.Sessions
	OTP        OTPSender
	Jobs       JobNotifier
	Pages      *Pages
	Static     fs.FS
	Logger     *slog.Logger
	Production bool
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	csrf := http.NewCrossOriginProtection()

	mux.HandleFunc("GET /up", s.handleUp)

	mux.Handle("GET /{$}", s.requireUser(s.handleLinksPage))
	mux.Handle("POST /links", csrf.Handler(s.requireUser(s.handleCreateLink)))

	mux.HandleFunc("GET /auth/new", s.handleAuthNew)
	mux.Handle("POST /auth", csrf.Handler(http.HandlerFunc(s.handleAuthCreate)))
	mux.HandleFunc("GET /auth/verify", s.handleAuthVerifyForm)
	mux.Handle("POST /auth/verify", csrf.Handler(http.HandlerFunc(s.handleAuthVerify)))
	mux.Handle("DELETE /auth", csrf.Handler(http.HandlerFunc(s.handleAuthDestroy)))

	mux.Handle("GET /api/links", s.requireToken(s.handleAPIIndex))
	mux.Handle("POST /api/links", s.requireToken(s.handleAPICreate))
	mux.Handle("PATCH /api/links", s.requireToken(s.handleAPIUpdate))
	mux.Handle("DELETE /api/links", s.requireToken(s.handleAPIDestroy))

	mux.Handle("GET /", s.staticHandler())

	return s.recoverer(s.logRequests(s.securityHeaders(mux)))
}

func (s *Server) handleUp(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!DOCTYPE html><html><body style="background-color: green"></body></html>`))
}

func (s *Server) staticHandler() http.Handler {
	files := http.FileServerFS(s.Static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		files.ServeHTTP(w, r)
	})
}
```

`internal/server/params.go`:

```go
package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"strings"
)

// readParams merges query parameters with a JSON or form-encoded body (the
// body wins), like Rails' params.
func readParams(r *http.Request) (map[string]any, error) {
	params := map[string]any{}
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			params[k] = v[0]
		}
	}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			return nil, err
		}
		if len(bytes.TrimSpace(body)) == 0 {
			return params, nil
		}
		var data map[string]any
		if err := json.Unmarshal(body, &data); err != nil {
			return nil, err
		}
		maps.Copy(params, data)
		return params, nil
	}
	if err := r.ParseForm(); err != nil {
		return nil, err
	}
	for k, v := range r.PostForm {
		if len(v) > 0 {
			params[k] = v[0]
		}
	}
	return params, nil
}

// stringParam is Ruby's params[key].to_s: "" for missing/null, the string
// itself, or the value printed.
func stringParam(params map[string]any, key string) string {
	switch v := params[key].(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

// castBool mirrors ActiveModel::Type::Boolean: ok is false for nil and "",
// a fixed set of strings and 0 mean false, everything else means true.
func castBool(v any) (value, ok bool) {
	switch b := v.(type) {
	case nil:
		return false, false
	case bool:
		return b, true
	case float64:
		return b != 0, true
	case string:
		switch b {
		case "":
			return false, false
		case "0", "f", "F", "false", "FALSE", "off", "OFF":
			return false, true
		}
		return true, true
	}
	return true, true
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func errorsBody(field, message string) map[string]map[string]string {
	return map[string]map[string]string{"errors": {field: message}}
}

func wantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json") ||
		strings.HasPrefix(r.Header.Get("Content-Type"), "application/json")
}

// clientIP prefers Cloudflare's header (the app sits behind a tunnel).
func clientIP(r *http.Request) string {
	if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
		return ip
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		first, _, _ := strings.Cut(fwd, ",")
		return strings.TrimSpace(first)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
```

`internal/server/pages.go`:

```go
package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const pageMarker = "<!-- PAGE_DATA -->"

// Pages serves the Vite-built index.html with the page component and its
// props injected as window.__PAGE__ (this replaces Inertia).
type Pages struct {
	before, after string
}

func NewPages(index []byte) (*Pages, error) {
	before, after, ok := strings.Cut(string(index), pageMarker)
	if !ok {
		return nil, fmt.Errorf("index.html has no %q marker; rebuild the frontend with: pnpm run build", pageMarker)
	}
	return &Pages{before: before, after: after}, nil
}

func (p *Pages) Render(w http.ResponseWriter, component string, props any) error {
	// json.Marshal escapes <, > and &, so the data cannot close the script tag.
	data, err := json.Marshal(map[string]any{"component": component, "props": props})
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, err = io.WriteString(w, p.before+"<script>window.__PAGE__ = "+string(data)+"</script>"+p.after)
	return err
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, component string, props any) {
	if err := s.Pages.Render(w, component, props); err != nil {
		s.serverError(w, r, err)
	}
}
```

`internal/server/middleware.go`:

```go
package server

import (
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"rulinky/internal/store"
)

type userHandler func(http.ResponseWriter, *http.Request, store.User)

// requireUser passes the signed-in user to next and renews the session
// cookie (sliding 10-day timeout). Without a session it redirects to
// /auth/new, or answers 401 {"redirect": "/auth/new"} to JSON requests.
func (s *Server) requireUser(next userHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := s.Sessions.Read(r)
		if sess.UserID != 0 {
			user, ok, err := s.Store.UserByID(r.Context(), sess.UserID)
			if err != nil {
				s.serverError(w, r, err)
				return
			}
			if ok {
				s.Sessions.Write(w, sess)
				next(w, r, user)
				return
			}
			s.Sessions.Clear(w)
		}
		if wantsJSON(r) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"redirect": "/auth/new"})
			return
		}
		http.Redirect(w, r, "/auth/new", http.StatusFound)
	})
}

// requireToken authenticates API calls with "Authorization: <token>" or
// "Authorization: Bearer <token>".
func (s *Server) requireToken(next userHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if token != "" {
			user, ok, err := s.Store.UserByAPIToken(r.Context(), token)
			if err != nil {
				s.serverError(w, r, err)
				return
			}
			if ok {
				next(w, r, user)
				return
			}
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
	})
}

func (s *Server) serverError(w http.ResponseWriter, r *http.Request, err error) {
	s.Logger.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	if strings.HasPrefix(r.URL.Path, "/api/") || wantsJSON(r) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal Server Error"})
		return
	}
	http.Error(w, "Internal Server Error", http.StatusInternalServerError)
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		if s.Production {
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/up" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.Logger.Info("request", "method", r.Method, "path", r.URL.Path, "status", rec.status, "ms", time.Since(start).Milliseconds())
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				s.Logger.Error("panic", "err", v, "path", r.URL.Path, "stack", string(debug.Stack()))
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
```

Stub handlers so the package compiles. Tasks 12–14 replace each file whole.

`internal/server/auth_handlers.go`:

```go
package server

import "net/http"

func (s *Server) handleAuthNew(w http.ResponseWriter, r *http.Request)        { http.NotFound(w, r) }
func (s *Server) handleAuthCreate(w http.ResponseWriter, r *http.Request)     { http.NotFound(w, r) }
func (s *Server) handleAuthVerifyForm(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }
func (s *Server) handleAuthVerify(w http.ResponseWriter, r *http.Request)     { http.NotFound(w, r) }
func (s *Server) handleAuthDestroy(w http.ResponseWriter, r *http.Request)    { http.NotFound(w, r) }
```

`internal/server/links_handlers.go`:

```go
package server

import (
	"net/http"

	"rulinky/internal/store"
)

func (s *Server) handleLinksPage(w http.ResponseWriter, r *http.Request, _ store.User)  { http.NotFound(w, r) }
func (s *Server) handleCreateLink(w http.ResponseWriter, r *http.Request, _ store.User) { http.NotFound(w, r) }
```

`internal/server/api_handlers.go`:

```go
package server

import (
	"net/http"

	"rulinky/internal/store"
)

func (s *Server) handleAPIIndex(w http.ResponseWriter, r *http.Request, _ store.User)   { http.NotFound(w, r) }
func (s *Server) handleAPICreate(w http.ResponseWriter, r *http.Request, _ store.User)  { http.NotFound(w, r) }
func (s *Server) handleAPIUpdate(w http.ResponseWriter, r *http.Request, _ store.User)  { http.NotFound(w, r) }
func (s *Server) handleAPIDestroy(w http.ResponseWriter, r *http.Request, _ store.User) { http.NotFound(w, r) }
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/server/ -v`
Expected: PASS (5 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/server/
git commit -m "feat(go): server foundation, pages, middleware, /up, static files"
```

---

### Task 12: Auth handlers

**Files:**
- Replace: `internal/server/auth_handlers.go`
- Test: `internal/server/auth_handlers_test.go`

- [ ] **Step 1: Write the failing tests (port of `auth_controller_test.rb` and `otp_sign_in_test.rb`)**

`internal/server/auth_handlers_test.go`:

```go
package server

import (
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"rulinky/internal/auth"
)

func TestSignInFlow(t *testing.T) {
	srv, otp, _ := newTestServer(t)

	rec := serve(srv, jsonRequest("POST", "/auth", `{"email":"  Me@Example.com "}`))
	if rec.Code != 200 || decode[map[string]string](t, rec)["redirect"] != "/auth/verify" {
		t.Fatalf("POST /auth: %d %s", rec.Code, rec.Body.String())
	}
	if len(otp.codes) != 1 || !regexp.MustCompile(`^\d{6}$`).MatchString(otp.codes[0]) {
		t.Fatalf("codes sent: %v", otp.codes)
	}
	pending := cookieFrom(t, rec)

	req := httptest.NewRequest("GET", "/auth/verify", nil)
	req.AddCookie(pending)
	rec = serve(srv, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"component":"Auth/Verify"`) ||
		!strings.Contains(rec.Body.String(), `"email":"me@example.com"`) {
		t.Fatalf("verify form: %d %s", rec.Code, rec.Body.String())
	}

	req = jsonRequest("POST", "/auth/verify", `{"code":"000000x"}`)
	req.AddCookie(pending)
	rec = serve(srv, req)
	if rec.Code != 422 || decode[map[string]map[string]string](t, rec)["errors"]["code"] != "Invalid or expired code" {
		t.Fatalf("wrong code: %d %s", rec.Code, rec.Body.String())
	}

	req = jsonRequest("POST", "/auth/verify", `{"code":" `+otp.codes[0]+` "}`)
	req.AddCookie(pending)
	rec = serve(srv, req)
	if rec.Code != 200 || decode[map[string]string](t, rec)["redirect"] != "/" {
		t.Fatalf("right code: %d %s", rec.Code, rec.Body.String())
	}
	signedIn := cookieFrom(t, rec)

	req = httptest.NewRequest("GET", "/", nil)
	req.AddCookie(signedIn)
	if rec := serve(srv, req); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"component":"Links/Index"`) {
		t.Fatalf("GET / after sign-in: %d", rec.Code)
	}

	req = jsonRequest("DELETE", "/auth", "")
	req.AddCookie(signedIn)
	rec = serve(srv, req)
	if rec.Code != 200 || decode[map[string]string](t, rec)["redirect"] != "/auth/new" || cookieFrom(t, rec).MaxAge >= 0 {
		t.Fatalf("sign out: %d %s", rec.Code, rec.Body.String())
	}
}

func TestAuthCreateRejectsInvalidEmail(t *testing.T) {
	srv, otp, _ := newTestServer(t)
	rec := serve(srv, jsonRequest("POST", "/auth", `{"email":"nope"}`))
	if rec.Code != 422 || decode[map[string]map[string]string](t, rec)["errors"]["email"] != "Enter a valid email" || len(otp.codes) != 0 {
		t.Fatalf("%d %s codes=%v", rec.Code, rec.Body.String(), otp.codes)
	}
}

func TestPagesRedirectWhenSignedOut(t *testing.T) {
	srv, _, _ := newTestServer(t)

	rec := serve(srv, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 302 || rec.Header().Get("Location") != "/auth/new" {
		t.Fatalf("GET /: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept", "application/json")
	if rec := serve(srv, req); rec.Code != 401 {
		t.Fatalf("GET / json: %d", rec.Code)
	}
	rec = serve(srv, httptest.NewRequest("GET", "/auth/verify", nil))
	if rec.Code != 302 || rec.Header().Get("Location") != "/auth/new" {
		t.Fatalf("GET /auth/verify without pending email: %d", rec.Code)
	}
	rec = serve(srv, httptest.NewRequest("GET", "/auth/new", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"component":"Auth/New"`) {
		t.Fatalf("GET /auth/new: %d", rec.Code)
	}
}

func TestSessionForDeletedUserRedirects(t *testing.T) {
	srv, _, _ := newTestServer(t)
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(sessionCookie(t, srv, auth.Session{UserID: 424242}))
	if rec := serve(srv, req); rec.Code != 302 {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestCrossSiteFormPostIsRejected(t *testing.T) {
	srv, otp, _ := newTestServer(t)
	req := jsonRequest("POST", "/auth", `{"email":"me@example.com"}`)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	if rec := serve(srv, req); rec.Code != 403 || len(otp.codes) != 0 {
		t.Fatalf("got %d, codes %v", rec.Code, otp.codes)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/server/ -run 'SignIn|Auth|Redirect|Deleted|CrossSite' -v`
Expected: FAIL (the stubs answer 404).

- [ ] **Step 3: Implement (replace `internal/server/auth_handlers.go`)**

```go
package server

import (
	"net/http"
	"strings"

	"rulinky/internal/auth"
)

func (s *Server) handleAuthNew(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "Auth/New", map[string]any{})
}

func (s *Server) handleAuthCreate(w http.ResponseWriter, r *http.Request) {
	params, err := readParams(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid payload"})
		return
	}
	email := strings.TrimSpace(strings.ToLower(stringParam(params, "email")))
	if !auth.ValidEmail(email) {
		writeJSON(w, http.StatusUnprocessableEntity, errorsBody("email", "Enter a valid email"))
		return
	}
	ctx := r.Context()
	user, err := s.Store.FindOrCreateUserByEmail(ctx, email)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	code, err := auth.NewOTP()
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if err := s.Store.SetOTP(ctx, user.ID, code); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.Logger.Info("[OTP]", "email", email, "code", code)
	if err := s.OTP.SendOTP(ctx, code); err != nil {
		s.Logger.Error("OTP delivery failed", "err", err)
	}
	sess := s.Sessions.Read(r)
	sess.AuthEmail = email
	s.Sessions.Write(w, sess)
	writeJSON(w, http.StatusOK, map[string]string{"redirect": "/auth/verify"})
}

func (s *Server) handleAuthVerifyForm(w http.ResponseWriter, r *http.Request) {
	sess := s.Sessions.Read(r)
	if sess.AuthEmail == "" {
		http.Redirect(w, r, "/auth/new", http.StatusFound)
		return
	}
	s.render(w, r, "Auth/Verify", map[string]string{"email": sess.AuthEmail})
}

func (s *Server) handleAuthVerify(w http.ResponseWriter, r *http.Request) {
	params, err := readParams(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid payload"})
		return
	}
	sess := s.Sessions.Read(r)
	invalid := func() {
		writeJSON(w, http.StatusUnprocessableEntity, errorsBody("code", "Invalid or expired code"))
	}
	if sess.AuthEmail == "" {
		invalid()
		return
	}
	user, ok, err := s.Store.VerifyOTP(r.Context(), sess.AuthEmail, strings.TrimSpace(stringParam(params, "code")), clientIP(r))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if !ok {
		invalid()
		return
	}
	s.Sessions.Write(w, auth.Session{UserID: user.ID})
	writeJSON(w, http.StatusOK, map[string]string{"redirect": "/"})
}

func (s *Server) handleAuthDestroy(w http.ResponseWriter, r *http.Request) {
	s.Sessions.Clear(w)
	writeJSON(w, http.StatusOK, map[string]string{"redirect": "/auth/new"})
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/server/ -v`
Expected: PASS. `TestSignInFlow`'s `GET /` step needs the links page from Task 13. Until then the stub returns 404, so that one assertion fails. Implement Task 13 before you expect the whole suite to pass, or run `-run 'Auth|Redirect|Deleted|CrossSite'` for now.

- [ ] **Step 5: Commit**

```bash
git add internal/server/
git commit -m "feat(go): OTP sign-in, verify and sign-out handlers"
```

---

### Task 13: Links page and web create

**Files:**
- Replace: `internal/server/links_handlers.go`
- Test: `internal/server/links_handlers_test.go`

- [ ] **Step 1: Write the failing tests**

`internal/server/links_handlers_test.go`:

```go
package server

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"rulinky/internal/auth"
)

type pageLinkJSON struct {
	ID          string  `json:"id"`
	URL         string  `json:"url"`
	Title       string  `json:"title"`
	FullTitle   string  `json:"fullTitle"`
	Note        *string `json:"note"`
	Read        bool    `json:"read"`
	UpdatedAt   string  `json:"updatedAt"`
	UpdatedAtMs int64   `json:"updatedAtMs"`
}

type linksPropsJSON struct {
	Links       []pageLinkJSON `json:"links"`
	ReadCount   int            `json:"readCount"`
	UnreadCount int            `json:"unreadCount"`
	APIToken    string         `json:"apiToken"`
}

func TestLinksPageProps(t *testing.T) {
	srv, _, _ := newTestServer(t)
	ctx := context.Background()
	u := createUser(t, srv, "me@example.com")
	plain, _, _ := srv.Store.CreateLink(ctx, u.ID, "https://www.plain.com/x?q=1", "a note")
	titled, _, _ := srv.Store.CreateLink(ctx, u.ID, "https://titled.com", "")
	long := strings.Repeat("L", 60)
	_, _ = srv.Store.Pool().Exec(ctx, `UPDATE links SET raw_title = $2 WHERE id = $1`, titled.ID, long)
	time.Sleep(3 * time.Millisecond) // so marking read gives plain a strictly newer updated_at
	_, _ = srv.Store.SetLinkRead(ctx, u.ID, plain.ID, true)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept", "application/json")
	req.AddCookie(sessionCookie(t, srv, auth.Session{UserID: u.ID}))
	rec := serve(srv, req)
	if rec.Code != 200 {
		t.Fatalf("code %d", rec.Code)
	}
	props := decode[linksPropsJSON](t, rec)
	if props.APIToken != u.APIToken || props.ReadCount != 1 || props.UnreadCount != 1 || len(props.Links) != 2 {
		t.Fatalf("props %+v", props)
	}
	first, second := props.Links[0], props.Links[1] // plain was updated last (marked read)
	if first.ID != plain.ID || first.Title != "plain.com/x" || first.FullTitle != "plain.com/x" || !first.Read ||
		first.Note == nil || *first.Note != "a note" || first.UpdatedAtMs < plain.UpdatedAt || first.UpdatedAt == "" {
		t.Errorf("plain link %+v", first)
	}
	if second.ID != titled.ID || second.FullTitle != long || len(second.Title) != 50 || !strings.HasSuffix(second.Title, "...") {
		t.Errorf("titled link %+v", second)
	}
}

func TestLinksPageHTMLEscapesData(t *testing.T) {
	srv, _, _ := newTestServer(t)
	u := createUser(t, srv, "me@example.com")
	_, _, _ = srv.Store.CreateLink(context.Background(), u.ID, "https://a.com", "</script><b>x")

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(sessionCookie(t, srv, auth.Session{UserID: u.ID}))
	body := serve(srv, req).Body.String()
	if !strings.Contains(body, "window.__PAGE__ = ") || strings.Contains(body, "</script><b>") {
		t.Fatalf("unsafe or missing page data: %s", body)
	}
}

func TestCreateLinkFromWeb(t *testing.T) {
	srv, _, jobs := newTestServer(t)
	u := createUser(t, srv, "me@example.com")
	cookie := sessionCookie(t, srv, auth.Session{UserID: u.ID})

	req := jsonRequest("POST", "/links", `{"url":"   ","note":""}`)
	req.AddCookie(cookie)
	rec := serve(srv, req)
	if rec.Code != 422 || decode[map[string]map[string]string](t, rec)["errors"]["url"] != "Enter a link" {
		t.Fatalf("blank url: %d %s", rec.Code, rec.Body.String())
	}

	req = jsonRequest("POST", "/links", `{"url":" https://x.com ","note":"hi"}`)
	req.AddCookie(cookie)
	if rec := serve(srv, req); rec.Code != 201 || jobs.n != 1 {
		t.Fatalf("create: %d jobs=%d", rec.Code, jobs.n)
	}
	links, _ := srv.Store.ListLinks(context.Background(), u.ID)
	if len(links) != 1 || links[0].URL != "https://x.com" || *links[0].Note != "hi" {
		t.Fatalf("links %+v", links)
	}

	if rec := serve(srv, jsonRequest("POST", "/links", `{"url":"https://y.com"}`)); rec.Code != 401 {
		t.Fatalf("signed-out create: %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/server/ -run 'LinksPage|CreateLinkFromWeb' -v`
Expected: FAIL (stubs answer 404).

- [ ] **Step 3: Implement (replace `internal/server/links_handlers.go`)**

```go
package server

import (
	"net/http"
	"strings"
	"time"

	"rulinky/internal/store"
	"rulinky/internal/titles"
)

type pageLink struct {
	ID          string  `json:"id"`
	URL         string  `json:"url"`
	Title       string  `json:"title"`
	FullTitle   string  `json:"fullTitle"`
	Note        *string `json:"note"`
	Read        bool    `json:"read"`
	UpdatedAt   string  `json:"updatedAt"`
	UpdatedAtMs int64   `json:"updatedAtMs"`
}

type linksProps struct {
	Links       []pageLink `json:"links"`
	ReadCount   int        `json:"readCount"`
	UnreadCount int        `json:"unreadCount"`
	APIToken    string     `json:"apiToken"`
}

func buildLinksProps(links []store.Link, apiToken string) linksProps {
	props := linksProps{Links: make([]pageLink, 0, len(links)), APIToken: apiToken}
	for _, l := range links {
		read := l.Read == 1
		if read {
			props.ReadCount++
		} else {
			props.UnreadCount++
		}
		full := titles.Full(l.URL, l.RawTitle, l.Content)
		props.Links = append(props.Links, pageLink{
			ID: l.ID, URL: l.URL, Title: titles.Truncate(full, 50), FullTitle: full,
			Note: l.Note, Read: read, UpdatedAt: localTime(l.UpdatedAt), UpdatedAtMs: l.UpdatedAt,
		})
	}
	return props
}

// localTime formats epoch milliseconds like the Rails helper: "3/9/2026, 4:05:06 PM".
func localTime(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).Local().Format("1/2/2006, 3:04:05 PM")
}

// handleLinksPage renders the app, or returns just its props to
// Accept: application/json (the UI reloads the list that way).
func (s *Server) handleLinksPage(w http.ResponseWriter, r *http.Request, user store.User) {
	links, err := s.Store.ListLinks(r.Context(), user.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	props := buildLinksProps(links, user.APIToken)
	if wantsJSON(r) {
		writeJSON(w, http.StatusOK, props)
		return
	}
	s.render(w, r, "Links/Index", props)
}

func (s *Server) handleCreateLink(w http.ResponseWriter, r *http.Request, user store.User) {
	params, err := readParams(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid payload"})
		return
	}
	url := strings.TrimSpace(stringParam(params, "url"))
	if url == "" {
		writeJSON(w, http.StatusUnprocessableEntity, errorsBody("url", "Enter a link"))
		return
	}
	link, _, err := s.Store.CreateLink(r.Context(), user.ID, url, stringParam(params, "note"))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.Jobs.Notify()
	writeJSON(w, http.StatusCreated, map[string]string{"id": link.ID})
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/server/ -v`
Expected: PASS (the whole package, including `TestSignInFlow`).

- [ ] **Step 5: Commit**

```bash
git add internal/server/
git commit -m "feat(go): links page props and web link creation"
```

---

### Task 14: JSON API

**Files:**
- Replace: `internal/server/api_handlers.go`
- Test: `internal/server/api_handlers_test.go`

- [ ] **Step 1: Write the failing tests (port of `test/controllers/api/links_controller_test.rb`)**

`internal/server/api_handlers_test.go`:

```go
package server

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
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
	other := createUser(t, srv, "other@example.com")
	if rec := api(t, srv, "PATCH", `{"id":"`+l.ID+`","read":true}`, other.APIToken); rec.Code != 404 {
		t.Errorf("other user's link: %d", rec.Code)
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/server/ -run API -v`
Expected: FAIL (stubs answer 404).

- [ ] **Step 3: Implement (replace `internal/server/api_handlers.go`)**

```go
package server

import (
	"net/http"
	"strings"

	"rulinky/internal/store"
	"rulinky/internal/titles"
)

type apiLink struct {
	ID           string  `json:"id"`
	URL          string  `json:"url"`
	Note         *string `json:"note"`
	Read         int     `json:"read"`
	Timestamp    int64   `json:"timestamp"`
	UpdatedAt    int64   `json:"updated_at"`
	Title        string  `json:"title"`
	ContentTitle *string `json:"content_title"`
}

func (s *Server) handleAPIIndex(w http.ResponseWriter, r *http.Request, user store.User) {
	links, err := s.Store.ListLinks(r.Context(), user.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	out := make([]apiLink, 0, len(links))
	for _, l := range links {
		var contentTitle *string
		if t := titles.ContentTitle(l.RawTitle, l.Content); t != "" {
			contentTitle = &t
		}
		out = append(out, apiLink{
			ID: l.ID, URL: l.URL, Note: l.Note, Read: l.Read, Timestamp: l.Timestamp, UpdatedAt: l.UpdatedAt,
			Title: titles.Full(l.URL, l.RawTitle, l.Content), ContentTitle: contentTitle,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

type apiCreated struct {
	Message string `json:"message"`
	ID      string `json:"id"`
	Note    string `json:"note"`
	JobID   string `json:"job_id"`
	Title   string `json:"title"`
}

func (s *Server) handleAPICreate(w http.ResponseWriter, r *http.Request, user store.User) {
	params, err := readParams(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid link"})
		return
	}
	link, isString := params["link"].(string)
	if !isString || strings.TrimSpace(link) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid link"})
		return
	}
	note := stringParam(params, "note")
	created, jobID, err := s.Store.CreateLink(r.Context(), user.ID, link, note)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.Jobs.Notify()
	writeJSON(w, http.StatusCreated, apiCreated{
		Message: "Link saved", ID: created.ID, Note: note, JobID: jobID, Title: titles.Full(created.URL, nil, nil),
	})
}

func (s *Server) handleAPIUpdate(w http.ResponseWriter, r *http.Request, user store.User) {
	params, err := readParams(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid payload"})
		return
	}
	id, idOK := params["id"].(string)
	read, readOK := castBool(params["read"])
	if !idOK || strings.TrimSpace(id) == "" || !readOK {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid payload"})
		return
	}
	found, err := s.Store.SetLinkRead(r.Context(), user.ID, id, read)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Link not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Link updated"})
}

func (s *Server) handleAPIDestroy(w http.ResponseWriter, r *http.Request, user store.User) {
	params, err := readParams(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid payload"})
		return
	}
	id, idOK := params["id"].(string)
	if !idOK || strings.TrimSpace(id) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid payload"})
		return
	}
	found, err := s.Store.DeleteLink(r.Context(), user.ID, id)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Link not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Link deleted"})
}
```

- [ ] **Step 4: Run all Go tests**

Run: `go vet ./internal/... && go test ./internal/...`
Expected: `ok` for config, store, titles, auth, content and server.

- [ ] **Step 5: Commit**

```bash
git add internal/server/
git commit -m "feat(go): JSON API compatible with the Rails contract"
```

---

### Task 15: Frontend in `web/` (React + Vite, no Inertia)

**Files:**
- Create: `web/index.html`, `web/vite.config.js`, `web/embed.go`, `web/dist/.gitkeep`
- Create: `web/src/main.jsx`, `web/src/api.js`
- Create: `web/src/pages/AuthNew.jsx`, `web/src/pages/AuthVerify.jsx`, `web/src/pages/LinksIndex.jsx`, `web/src/pages/createShortcut.js`
- Create: `web/public/` (copies of the favicon set)
- Create: `web/test/links_index_shortcut.test.js`
- Modify: `package.json`, `.gitignore`

- [ ] **Step 1: Copy the static files and the shortcut helper**

```bash
mkdir -p web/public web/src/pages web/test web/dist
cp public/favicon.ico public/favicon.svg public/favicon-96x96.png public/apple-touch-icon.png \
   public/site.webmanifest public/web-app-manifest-192x192.png public/web-app-manifest-512x512.png \
   public/robots.txt public/icon.png public/icon.svg public/logo.png web/public/
cp app/javascript/Pages/Links/createShortcut.js web/src/pages/createShortcut.js
cp app/javascript/Pages/Links/Index.jsx web/src/pages/LinksIndex.jsx
touch web/dist/.gitkeep
```

- [ ] **Step 2: Write the JS test (it fails, because `LinksIndex.jsx` still uses Inertia's `post`)**

`web/test/links_index_shortcut.test.js`:

```js
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import test from 'node:test'
import { fileURLToPath, pathToFileURL } from 'node:url'

const webDir = resolve(dirname(fileURLToPath(import.meta.url)), '..')

test('recognizes Cmd+Enter and Ctrl+Enter as create shortcuts', async () => {
  const { isCreateShortcut } = await import(pathToFileURL(resolve(webDir, 'src/pages/createShortcut.js')))
  const baseEvent = { key: 'Enter', metaKey: false, ctrlKey: false, shiftKey: false, altKey: false }

  assert.equal(isCreateShortcut({ ...baseEvent, metaKey: true }), true)
  assert.equal(isCreateShortcut({ ...baseEvent, ctrlKey: true }), true)
  assert.equal(isCreateShortcut({ ...baseEvent, shiftKey: true }), false)
  assert.equal(isCreateShortcut({ ...baseEvent, altKey: true }), false)
  assert.equal(isCreateShortcut({ ...baseEvent, key: 'a', metaKey: true }), false)
})

test('wires the shortcut handler and submit button hotkey hint', () => {
  const source = readFileSync(resolve(webDir, 'src/pages/LinksIndex.jsx'), 'utf8')

  assert.match(source, /onKeyDown=\{handleCreateShortcut\}/)
  assert.match(source, /Add link/)
  assert.match(source, /Cmd/)
  assert.match(source, /Enter/)
})

test('does not depend on Inertia', () => {
  for (const file of ['src/pages/LinksIndex.jsx', 'src/pages/AuthNew.jsx', 'src/pages/AuthVerify.jsx', 'src/main.jsx']) {
    const source = readFileSync(resolve(webDir, file), 'utf8')
    assert.doesNotMatch(source, /@inertiajs/, file)
  }
})
```

Run: `node --test web/test/`
Expected: FAIL. `AuthNew.jsx` does not exist yet, and `LinksIndex.jsx` imports `@inertiajs/react`.

- [ ] **Step 3: Write the Vite entry, config and Go embed**

`web/index.html`:

```html
<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8">
    <title>Rulinky</title>
    <meta name="viewport" content="width=device-width,initial-scale=1">
    <meta name="apple-mobile-web-app-capable" content="yes">
    <meta name="application-name" content="Rulinky">
    <meta name="mobile-web-app-capable" content="yes">
    <link rel="icon" type="image/png" href="/favicon-96x96.png" sizes="96x96">
    <link rel="icon" type="image/svg+xml" href="/favicon.svg">
    <link rel="shortcut icon" href="/favicon.ico">
    <link rel="apple-touch-icon" sizes="180x180" href="/apple-touch-icon.png">
    <link rel="manifest" href="/site.webmanifest">
    <script src="https://cdn.tailwindcss.com"></script>
    <!-- PAGE_DATA -->
  </head>
  <body class="antialiased">
    <div id="app"></div>
    <script type="module" src="/src/main.jsx"></script>
  </body>
</html>
```

`web/vite.config.js`:

```js
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { fileURLToPath } from 'node:url'

export default defineConfig({
  root: fileURLToPath(new URL('.', import.meta.url)),
  plugins: [react()],
  build: { outDir: 'dist', emptyOutDir: true },
})
```

`web/embed.go`:

```go
// Package web embeds the Vite build (web/dist) into the binary.
// Run `pnpm run build` before `go build`.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

func Dist() (fs.FS, error) { return fs.Sub(dist, "dist") }
```

`web/src/main.jsx`:

```jsx
import { createRoot } from 'react-dom/client'
import AuthNew from './pages/AuthNew.jsx'
import AuthVerify from './pages/AuthVerify.jsx'
import LinksIndex from './pages/LinksIndex.jsx'

const pages = { 'Auth/New': AuthNew, 'Auth/Verify': AuthVerify, 'Links/Index': LinksIndex }
const page = window.__PAGE__ || { component: 'Auth/New', props: {} }
const Page = pages[page.component]

createRoot(document.getElementById('app')).render(<Page {...page.props} />)
```

`web/src/api.js`:

```js
import { useState } from 'react'

// JSON request to the Go server with the session cookie. Follows a
// { redirect } answer. Resolves to { ok, status, data }.
export async function sendJSON(method, path, body) {
  const response = await fetch(path, {
    method,
    headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
    credentials: 'same-origin',
  })
  let data = {}
  try {
    data = await response.json()
  } catch {
    data = {}
  }
  if (data.redirect) window.location.assign(data.redirect)
  return { ok: response.ok, status: response.status, data }
}

// Small replacement for Inertia's useForm: data, setData, errors,
// processing, reset, and submit(method, path) → sendJSON result.
export function useJSONForm(initial) {
  const [data, setDataState] = useState(initial)
  const [errors, setErrors] = useState({})
  const [processing, setProcessing] = useState(false)

  function setData(key, value) {
    setDataState(prev => ({ ...prev, [key]: value }))
  }

  function reset() {
    setDataState(initial)
  }

  async function submit(method, path) {
    setProcessing(true)
    try {
      const result = await sendJSON(method, path, data)
      setErrors(result.ok ? {} : (result.data.errors || {}))
      return result
    } finally {
      setProcessing(false)
    }
  }

  return { data, setData, errors, processing, reset, submit }
}
```

- [ ] **Step 4: Write the auth pages**

`web/src/pages/AuthNew.jsx`:

```jsx
import { useJSONForm } from '../api.js'

export default function AuthNew() {
  const { data, setData, submit, processing, errors } = useJSONForm({ email: '' })

  function onSubmit(e) {
    e.preventDefault()
    submit('POST', '/auth')
  }

  return (
    <div className="min-h-screen flex items-center justify-center p-4">
      <form onSubmit={onSubmit} className="w-full max-w-sm space-y-4">
        <h1 className="text-xl font-semibold">Sign in to Rulinky</h1>
        <input
          type="email"
          value={data.email}
          onChange={e => setData('email', e.target.value)}
          placeholder="you@example.com"
          className="w-full border rounded px-3 py-2"
          autoFocus
        />
        {errors.email && <p className="text-red-600 text-sm">{errors.email}</p>}
        <button
          type="submit"
          disabled={processing}
          className="w-full bg-black text-white rounded px-3 py-2 disabled:opacity-50"
        >
          Send code
        </button>
      </form>
    </div>
  )
}
```

`web/src/pages/AuthVerify.jsx`:

```jsx
import { useJSONForm } from '../api.js'

export default function AuthVerify({ email }) {
  const { data, setData, submit, processing, errors } = useJSONForm({ code: '' })

  function onSubmit(e) {
    e.preventDefault()
    submit('POST', '/auth/verify')
  }

  return (
    <div className="min-h-screen flex items-center justify-center p-4">
      <form onSubmit={onSubmit} className="w-full max-w-sm space-y-4">
        <h1 className="text-xl font-semibold">Enter your code</h1>
        <p className="text-sm text-gray-600">We sent a 6-digit code to {email}.</p>
        <input
          type="text"
          inputMode="numeric"
          pattern="\d*"
          maxLength={6}
          value={data.code}
          onChange={e => setData('code', e.target.value)}
          placeholder="123456"
          className="w-full border rounded px-3 py-2 tracking-widest text-center"
          autoFocus
        />
        {errors.code && <p className="text-red-600 text-sm">{errors.code}</p>}
        <button
          type="submit"
          disabled={processing}
          className="w-full bg-black text-white rounded px-3 py-2 disabled:opacity-50"
        >
          Verify
        </button>
        <a href="/auth/new" className="block text-center text-sm text-gray-600 underline">
          Use a different email / resend
        </a>
      </form>
    </div>
  )
}
```

- [ ] **Step 5: Port `LinksIndex.jsx` off Inertia with these exact replacements (Edit tool, one at a time)**

1. Replace:
```js
import { useForm, Link } from '@inertiajs/react'
```
with:
```js
import { useJSONForm, sendJSON } from '../api.js'
```

2. Replace:
```js
export default function LinksIndex({ links, readCount, unreadCount }) {
```
with:
```js
export default function LinksIndex({ links, readCount, unreadCount, apiToken }) {
```

3. Replace:
```js
  const { data, setData, post, processing, errors, reset } = useForm({
    url: '',
    note: ''
  })
```
with:
```js
  const { data, setData, submit, processing, errors, reset } = useJSONForm({
    url: '',
    note: ''
  })
```

4. Replace:
```js
  function authToken() {
    const meta = document.querySelector('meta[name="rulinky-auth-token"]')
    return (meta && meta.getAttribute('content')) || ''
  }
```
with:
```js
  function authToken() {
    return apiToken || ''
  }
```

5. Replace:
```js
  function handleCreate(e) {
    e.preventDefault()
    post('/links', {
      preserveScroll: true,
      onSuccess: () => {
        reset()
        setFilter('unread')
        setFormOpen(false)
      }
    })
  }
```
with:
```js
  async function reloadLinks() {
    const response = await fetch('/', { headers: { Accept: 'application/json' }, credentials: 'same-origin' })
    if (!response.ok) return
    const props = await response.json()
    setLinksState(props.links)
    setStickyReadIds(new Set())
  }

  async function handleCreate(e) {
    e.preventDefault()
    const result = await submit('POST', '/links')
    if (!result.ok) return
    reset()
    setFilter('unread')
    setFormOpen(false)
    await reloadLinks()
  }
```

6. Replace:
```jsx
                <Link
                  href="/auth"
                  method="delete"
                  as="button"
                  className="block w-full px-4 py-2 text-left text-sm text-gray-700 transition-colors hover:bg-gray-100 hover:cursor-pointer"
                >
                  Sign out
                </Link>
```
with:
```jsx
                <button
                  type="button"
                  onClick={() => sendJSON('DELETE', '/auth')}
                  className="block w-full px-4 py-2 text-left text-sm text-gray-700 transition-colors hover:bg-gray-100 hover:cursor-pointer"
                >
                  Sign out
                </button>
```

7. Replace:
```js
import { isCreateShortcut } from './createShortcut'
```
with:
```js
import { isCreateShortcut } from './createShortcut.js'
```

Then check that nothing is left: `grep -n "inertia\|useForm\|<Link\|post(" web/src/pages/LinksIndex.jsx` should print nothing.

- [ ] **Step 6: Switch `package.json` to the Go frontend build**

Replace `package.json` with:

```json
{
  "name": "rulinky",
  "private": true,
  "type": "module",
  "scripts": {
    "build": "vite build --config web/vite.config.js && touch web/dist/.gitkeep",
    "test": "node --test web/test/"
  },
  "dependencies": {
    "react": "^19.0.0",
    "react-dom": "^19.0.0"
  },
  "devDependencies": {
    "@vitejs/plugin-react": "^4.0.0",
    "vite": "^6.0.0"
  },
  "packageManager": "pnpm@11.20.0"
}
```

Append to `.gitignore`:

```
# Go build
/bin/rulinky
/web/dist/*
!/web/dist/.gitkeep
```

Run: `pnpm install` (this updates `pnpm-lock.yaml` and drops Inertia and vite-plugin-ruby).

- [ ] **Step 7: Build and run the JS tests**

Run: `pnpm test && pnpm run build && grep -c 'PAGE_DATA' web/dist/index.html && ls web/dist/assets web/dist/favicon.svg`
Expected: 3 passing tests, a successful Vite build, `1` (the marker survived the build), and the asset files listed. If the count is `0`, Vite stripped the comment. In that case, change the marker in both `web/index.html` and `internal/server/pages.go` to `<meta name="rulinky-page-data">`. Vite keeps meta tags.

- [ ] **Step 8: Commit**

```bash
git add web/ package.json pnpm-lock.yaml .gitignore
git commit -m "feat(web): React UI without Inertia, built by Vite into web/dist"
```

---

### Task 16: `main.go`, `-crawl` flag and the new `./serve`

**Files:**
- Create: `cmd/rulinky/main.go`
- Modify: `serve`

- [ ] **Step 1: Write `cmd/rulinky/main.go`**

```go
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

	worker := content.NewWorker(st, crawler, previewer, logger)
	if cfg.DisableWorker {
		logger.Warn("content worker disabled (RULINKY_DISABLE_WORKER=1)")
	} else {
		go func() {
			if err := worker.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				logger.Error("content worker stopped", "err", err)
			}
		}()
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
	httpServer := &http.Server{Addr: ":" + cfg.Port, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	logger.Info("rulinky listening", "port", cfg.Port, "production", cfg.Production)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
```

- [ ] **Step 2: Build everything**

Run: `pnpm run build && go vet ./... && go build -o bin/rulinky ./cmd/rulinky && ls -lh bin/rulinky`
Expected: no vet output and a binary of roughly 20–30 MB.

- [ ] **Step 3: Run it locally against the test database, with the worker off**

```bash
SESSION_SECRET=$(openssl rand -hex 32) DATABASE_URL=postgres://grillermo@192.168.1.1/rulinky_test \
  PORT=3013 RULINKY_DISABLE_WORKER=1 ./bin/rulinky
```

The `public` schema of `rulinky_test` may have no `users` table (the Rails tests load it). If requests fail with `relation "users" does not exist`, that is fine for this step. What matters is that `curl -s localhost:3013/up` returns the green page, `curl -sI localhost:3013/` returns `302` to `/auth/new`, and `curl -s localhost:3013/auth/new | grep -c __PAGE__` prints `1`. Stop it with Ctrl-C and confirm it logs nothing alarming on shutdown.

- [ ] **Step 4: Rewrite `./serve`**

Replace `serve` with:

```bash
#!/usr/bin/env bash
# (Re)start rulinky in tmux session "rulinky", in place, and return. Run by
# ../server/after-reboot.sh at boot and by ./activate after git-sync lands
# new code. The binary reads .env / .env.local from this directory.
set -euo pipefail
APP_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$APP_DIR"
SESSION=rulinky
# shellcheck source=/dev/null
. "$HOME/c/server/serve-in-place.sh"

echo "==> pnpm install";   pnpm install --frozen-lockfile
echo "==> pnpm run build"; pnpm run build
echo "==> go build";       go build -o bin/rulinky ./cmd/rulinky

serve_pane web 3002 "cd '$APP_DIR' && env RULINKY_ENV=production ./bin/rulinky" /up
serve_attach
```

Run: `shellcheck serve`
Expected: no output.

- [ ] **Step 5: Commit**

```bash
git add cmd/ serve
git commit -m "feat(go): main binary, -crawl debug flag, serve script for Go"
```

---

### Task 17: Smoke test on the macmini, beside production

Rails keeps serving on port 3002. The Go build runs from the branch on port **3012**, against the **production database**, with the worker **off**.

**Files:** none (machine state only)

- [ ] **Step 1: Put the branch on the macmini without touching `~/c/rulinky`**

```bash
command ssh macmini 'cd ~/c/rulinky && git fetch -q 2>/dev/null; git worktree add ~/c/rulinky-go go-rewrite 2>&1 | tail -1'
```

If git-sync has not carried the branch to the macmini, push it there instead: `git push macmini go-rewrite` if that remote exists, or ask the user how branches reach the macmini. Do not commit on the macmini.

- [ ] **Step 2: Build there and start on port 3012**

```bash
command ssh macmini 'export PATH=/opt/homebrew/bin:$HOME/Library/pnpm:$PATH; cd ~/c/rulinky-go && cp -n ../rulinky/.env . 2>/dev/null; pnpm install --frozen-lockfile >/dev/null && pnpm run build >/dev/null && go build -o bin/rulinky ./cmd/rulinky && echo built'
```

`cp -n` copies the git-ignored `.env` only when it is missing (see the rails-worktree-secrets rule). Next, add a session secret to **this worktree's** `.env` only if it has none. Never print the file:

```bash
command ssh macmini 'cd ~/c/rulinky-go && grep -q "^SESSION_SECRET=" .env || echo "SESSION_SECRET=$(openssl rand -hex 32)" >> .env'
command ssh macmini 'cd ~/c/rulinky-go && tmux new-session -d -s rulinky-go "env PORT=3012 RULINKY_DISABLE_WORKER=1 RULINKY_ENV=production ./bin/rulinky 2>&1 | tee /tmp/rulinky-go.log"'
```

- [ ] **Step 3: Check the API against production data with the real token**

Read the owner's token from the database inside one remote shell, use it there, and never print it:

```bash
command ssh macmini 'cd ~/c/rulinky-go && set -a && . ./.env && set +a && export PATH=/opt/homebrew/bin:$PATH && T=$(PGPASSWORD="$DATABASE_PASSWORD" psql -h "${DATABASE_HOST:-localhost}" -p "${DATABASE_PORT:-5432}" -U "$DATABASE_USER" -d rulinky -tA -c "select api_token from users order by id limit 1") && echo "token length: ${#T}" && for p in 3002 3012; do curl -s -H "Authorization: Bearer $T" localhost:$p/api/links | python3 -c "import json,sys; d=json.load(sys.stdin); print(len(d), sorted(d[0].keys()))"; done'
```

Expected: the same count and the same keys on both ports (`content_title, id, note, read, timestamp, title, updated_at, url`). Titles for links without a fetched title differ: Go shows the URL, Rails shows `""`. That is the intended fix.

- [ ] **Step 4: Check the UI by hand**

Open `http://192.168.1.1:3012/auth/new`. The cookie is `Secure` because `RULINKY_ENV=production`, so over plain HTTP on the LAN the browser drops it. If sign-in loops back to the form, restart without `RULINKY_ENV=production` for this check. Sign in with the OTP from Slack, then check: tabs and counts, search, the timeline buckets, toggling read, clicking a link (it stays dimmed in Unread), adding a link with Cmd+Enter (it appears), and delete. Any link you add here gets a **queued** job that nothing processes yet. Delete it afterwards, or let the Go worker pick it up after cutover.

- [ ] **Step 5: Check the crawler by hand (this kills Chrome on the macmini)**

Ask the user first: "This closes Chrome on the macmini for about a minute. OK?" Then:

```bash
command ssh macmini 'cd ~/c/rulinky-go && ./bin/rulinky -crawl https://example.com'
```

Expected: `title: Example Domain`, with `html_bytes` and `content_bytes` both above 0. If chromedp cannot find Chrome, set `CHROME_PATH` handling by adding `chromedp.ExecPath("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome")` to the allocator options in `crawler.go`, commit on the branch, and retry.

- [ ] **Step 6: Measure memory**

```bash
command ssh macmini 'for p in 3002 3012; do pid=$(lsof -ti tcp:$p -sTCP:LISTEN | head -1); echo "$p $(ps -o rss= -p $pid) KB"; done'
```

Write both numbers down for the final report.

- [ ] **Step 7: Stop the smoke instance**

```bash
command ssh macmini 'tmux kill-session -t rulinky-go'
```

---

### Task 18: Cutover

**Files:** none new. This task merges `go-rewrite` into `main`.

- [ ] **Step 1: Check for stale queued jobs before the Go worker sees them**

SuckerPunch lost queued jobs on every Rails restart, so production may hold many rows still marked `queued` or `running`. After cutover, the Go worker would crawl every one of them, killing Chrome each time.

```sql
SELECT status, count(*), min(created_at), max(created_at)
FROM link_content_jobs WHERE status IN ('queued', 'running') GROUP BY status;
```

If the count is more than a handful, **ask the user** whether to retire the old ones before cutover with:

```sql
UPDATE link_content_jobs SET status = 'failed', error_message = 'abandoned before Go cutover', updated_at = (now() AT TIME ZONE 'UTC')
WHERE status IN ('queued', 'running') AND created_at < (now() AT TIME ZONE 'UTC') - interval '1 day';
```

- [ ] **Step 2: Add `SESSION_SECRET` to the production `.env` on the macmini (only if missing, never printed)**

```bash
command ssh macmini 'cd ~/c/rulinky && grep -q "^SESSION_SECRET=" .env || echo "SESSION_SECRET=$(openssl rand -hex 32)" >> .env'
```

- [ ] **Step 3: Run the full test suite one last time on the branch**

Run: `pnpm test && pnpm run build && go vet ./... && go test ./...`
Expected: everything passes. Paste the output into the cutover message.

- [ ] **Step 4: Ask the user to confirm the cutover**

Merging into `main` restarts production through git-sync. Ask: "Ready to switch rulinky.chiq.me to the Go build? Rollback is `git revert -m 1 <merge>`." Wait for an explicit yes.

- [ ] **Step 5: Merge and let git-sync deploy**

```bash
git checkout main && git merge --no-ff go-rewrite -m "feat: replace Rails with the Go build of Rulinky

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

Watch the macmini pick it up: `command ssh macmini 'tmux capture-pane -pt rulinky -S -40'`. Expect `==> go build`, then the `rulinky listening port=3002 production=true` log line.

- [ ] **Step 6: Verify production**

- `curl -s https://rulinky.chiq.me/up` returns the green page.
- Sign in at `https://rulinky.chiq.me` (one fresh OTP; old Rails sessions are gone).
- Save a page with the **Chrome extension** and confirm `SAVED`. Within about a minute (Chrome on the macmini restarts), that link's title changes from the URL to the article title after a reload.
- RSS: `command ssh macmini 'ps -o rss= -p $(lsof -ti tcp:3002 -sTCP:LISTEN | head -1)'`. Compare it with the Task 0 baseline.

- [ ] **Step 7: Rollback, only if something is broken**

```bash
git revert -m 1 HEAD --no-edit && git log --oneline -1
```

git-sync carries the revert, and `./serve` rebuilds Rails.

---

### Task 19: Remove Rails (separate, after a few days on Go)

Ask the user before starting this task. It deletes the Rails app for good (it stays in git history).

**Files:**
- Delete: `app/`, `bin/` (Rails binstubs, but keep `bin/rulinky` ignored), `config/`, `db/`, `lib/`, `log/`, `public/`, `script/`, `test/`, `vendor/` (if present), `Gemfile`, `Gemfile.lock`, `Rakefile`, `config.ru`, `Procfile`, `Dockerfile`, `.ruby-version`, `vite.config.js`, `data/links.db`, `project/`, `.claude/skills/informant/`, `.claude/hooks/informant-alerts.sh`
- Modify: `.mcp.json` (remove the informant server), `.claude/settings.json` (remove the informant hook), `README.md`, `.gitignore` (drop the Rails sections)

- [ ] **Step 1: Delete the Rails files**

```bash
git rm -r -q app config db lib log public script test Gemfile Gemfile.lock Rakefile config.ru Procfile Dockerfile .ruby-version vite.config.js data/links.db .claude/skills/informant .claude/hooks/informant-alerts.sh
git rm -q bin/ci bin/dev bin/docker-entrypoint bin/rails bin/rake bin/setup bin/thrust
```

Then edit `.mcp.json` and `.claude/settings.json` to remove the entries that point at informant (read them first; remove only those entries).

- [ ] **Step 2: Rewrite `README.md`**

```markdown
# Rulinky

Personal read-it-later link saver, served at https://rulinky.chiq.me from the macmini.
One Go binary (`cmd/rulinky`) with the React UI (`web/`) embedded. Postgres.
What it does: `docs/PRODUCT.md`.

## Build and run

    pnpm install && pnpm run build      # web/dist
    go build -o bin/rulinky ./cmd/rulinky
    ./bin/rulinky                        # reads .env / .env.local, listens on $PORT (3002)

Production: `./serve` (tmux session `rulinky`), run by git-sync's `./activate`.

## Environment

DATABASE_URL or DATABASE_HOST/PORT/USER/PASSWORD/NAME, SESSION_SECRET (32+ chars),
SLACK_OTP_WEBHOOK_URL, READITSOON_EMAIL, CHROME_REAL_USER_DATA_DIR, CHROME_USER_DATA_DIR,
CHROME_PROFILE_DIRECTORY, RULINKY_ENV=production, RULINKY_DISABLE_WORKER=1.

## Tests

    pnpm test
    go test ./...      # needs TEST_DATABASE_URL (default postgres://grillermo@192.168.1.1/rulinky_test)

## Debug the crawler

    ./bin/rulinky -crawl https://example.com   # kills running Chrome
```

- [ ] **Step 3: Verify and commit**

Run: `pnpm run build && go build ./... && go test ./... && git status --short | head`
Expected: tests pass, and only deletions plus the edited files are listed.

```bash
git add -A && git commit -m "chore: remove the Rails app after the Go cutover

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

## Spec coverage check

| Spec section | Task |
|---|---|
| §2 data model, same DB | 3, 4, 5 (schema mirrored for tests, prod untouched) |
| §3.1 OTP over Slack, 10-day session, redirects, CSRF | 7, 12, 11 (`CrossOriginProtection`) |
| §3.2 bearer token | 11 (`requireToken`), 14 |
| §4.1 links page (all links, sorted, props, counts, timeline, search, toggles, Cmd+Enter, sign-out) | 13 (props), 15 (UI port keeps every behavior) |
| §4.2 title rules + `fullTitle` fix | 6, used in 13 and 14 |
| §4.3 keep only `POST /links` | 11 routes, 13 |
| §5 JSON API contract | 14 |
| §6 crawler + readitsoon, profile fix, serial, requeue | 8, 9, 10, 16 (`-crawl`) |
| §7 config/env | 1, 2 |
| §8 ops: binary, `./serve`, `/up`, logs | 11, 16, 18 |
| §9 drop informant | 19 |
| §10 drop scripts | 19 |
| §11 tests as acceptance cases | ported in 3–15 |
