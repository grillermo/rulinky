# Rulinky

Personal read-it-later link saver, served at https://rulinky.chiq.me from the macmini.
One Go binary (`cmd/rulinky`) with the React UI (`web/`) embedded. Postgres.
What it does: `docs/PRODUCT.md`.

## Build and run

    pnpm install && pnpm run build      # web/dist
    go build -o bin/rulinky ./cmd/rulinky
    ./bin/rulinky                        # reads the dotenv files, listens on $PORT (3002)

Production: `./serve` (tmux session `rulinky`), run by git-sync's `./activate`.

## Environment

DATABASE_URL or DATABASE_HOST/PORT/USER/PASSWORD/NAME, SESSION_SECRET (32+ chars),
SLACK_OTP_WEBHOOK_URL, READITSOON_EMAIL, EGO_BROWSER_BIN, RULINKY_ENV=production,
RULINKY_DISABLE_WORKER=1.

## Tests

    pnpm test
    go test ./...      # needs TEST_DATABASE_URL, e.g. postgres://grillermo@localhost:5432/rulinky_test?sslmode=disable

## Debug the crawler

    ./bin/rulinky -crawl https://example.com   # opens the page in ego-browser (your logged-in browser)
