# Rulinky — Product Spec

This document lists what Rulinky does today, the Rails 8.1 app, as of commit `c8383ec`. It is
the parity checklist for the Go rewrite. The goal of the rewrite is lower RAM use. Behavior
should stay the same unless a section marked **Rewrite decision** says otherwise.

## 1. What it is

Rulinky is a personal "read it later" link saver. You save URLs, from the web UI or with
an API token (Chrome extension, shortcuts). Each saved link gets a title fetched in the
background. You go through the list as Unread → Read.

It runs on the macmini at `https://rulinky.chiq.me`, on port **3002**, inside the tmux session
`rulinky`. Postgres is the database.

## 2. Data model (Postgres)

### `users`
| column | type | notes |
|---|---|---|
| `id` | bigint PK | |
| `email` | string, unique, not null | stored lowercased and stripped |
| `api_token` | string, unique | generated once per user (Rails `has_secure_token`, 24 chars base58) |
| `otp_code` | string | 6 digits, zero-padded; cleared after a successful verify |
| `otp_expires_at` | datetime | now + 10 minutes when the code is generated |
| `confirmed_at` | datetime | set the first time the user verifies an OTP |
| `remember_created_at` | datetime | Devise rememberable (unused in practice) |
| `sign_in_count`, `current_sign_in_at`, `last_sign_in_at`, `current_sign_in_ip`, `last_sign_in_ip` | | Devise trackable: updated on every sign-in |
| `created_at`, `updated_at` | datetime | |

### `links`
| column | type | notes |
|---|---|---|
| `id` | string PK (UUID v4) | made by the app, not the DB |
| `user_id` | bigint FK → users, not null | |
| `url` | text, not null | |
| `note` | text | optional, defaults to `""` |
| `read` | integer, default 0 | **0/1 integer**, not a boolean |
| `timestamp` | bigint, not null | creation time, **epoch milliseconds** |
| `updated_at` | bigint, not null | **epoch milliseconds**; set on create and on every read/unread change; the list is sorted by it |
| `content` | text | sanitized article HTML from readitsoon |
| `raw_title` | text | article title from readitsoon |

Indexes: `id` (unique), `read`, `updated_at`, `user_id`.

### `link_content_jobs`
| column | type | notes |
|---|---|---|
| `id` | string PK (UUID v4) | |
| `link_id` | string, not null | no FK constraint; rows are deleted along with their link (app-level `dependent: :destroy`) |
| `status` | string, default `queued` | `queued` → `running` → `finished` \| `failed` |
| `error_message` | text | set on failure |
| `created_at`, `updated_at` | datetime | |

### `informant_error_groups`, `informant_occurrences`
These belong to the `rails-informant` gem (error tracking, see §9). They are not app data.

**Rewrite decision:** the Go app should use the **same database and schema** as it is, so
it can replace the Rails app with no data migration. Who runs migrations from then on
(the Go app, or plain SQL files) is still to be decided.

## 3. Authentication

### 3.1 Web: email + one-time code (no passwords)
1. `GET /auth/new` shows a form with one field, "Sign in to Rulinky": email.
2. `POST /auth` with `email`:
   - The email is lowercased and stripped, then checked against an email regex. If it is
     invalid, it redirects back to `/auth/new` with the error `email: "Enter a valid email"`.
   - **Find or create** the user by email. Anyone can sign up just by entering an email.
   - Generates a random 6-digit code that expires in 10 minutes and stores it on the user.
   - Writes the code to the log: `[OTP] email -> code`.
   - Sends the code to **Slack** with the `SLACK_OTP_WEBHOOK_URL` webhook (JSON
     `{"text":"Your OTP for rulinky 123456"}`). If the env var is missing, nothing is sent.
   - Saves `auth_email` in the session and redirects to `/auth/verify`.
3. `GET /auth/verify` redirects to `/auth/new` when the session has no `auth_email`.
   Otherwise it shows "Enter your code", "We sent a 6-digit code to {email}", a numeric
   input (max length 6) and a link "Use a different email / resend" that goes to `/auth/new`.
4. `POST /auth/verify` with `code` (stripped):
   - The code is valid if it is present, not expired, and equal to the stored one
     (constant-time comparison).
   - On success: clears the code and expiry, sets `confirmed_at` if it was empty, removes
     `auth_email` from the session, signs in (session cookie), records the trackable fields,
     and redirects to `/`.
   - On failure: redirects to `/auth/verify` with the error `code: "Invalid or expired code"`.
     There is no attempt limit or lockout.
5. `DELETE /auth` signs out and redirects to `/auth/new`. The UI menu has "Sign out".

Session rules:
- A cookie-based session (Rails encrypted cookie, Warden/Devise).
- **Inactivity timeout: 10 days** (Devise timeoutable).
- No "remember me" cookie is ever set.
- Every HTML route except `/auth/*` redirects to `/auth/new` when you are not signed in.
- Rails `allow_browser versions: :modern` blocks old browsers on HTML routes and serves
  `public/406-unsupported-browser.html`.
- CSRF: HTML form posts are CSRF-protected (Inertia sends the `X-XSRF-TOKEN` header).

An unused piece of code: `OtpMailer` (Mailgun) and the SMTP config exist, but nothing
calls them. OTP delivery goes **only through Slack** and the log.
**Rewrite decision:** drop the Mailgun/SMTP code.

### 3.2 API: bearer token
- All `/api/*` routes require `Authorization: <token>` or `Authorization: Bearer <token>`.
  Any `Bearer ` prefix is removed and the token is looked up in `users.api_token`.
- A missing or unknown token returns `401 {"error":"Unauthorized"}`.
- No CSRF on the API.
- There is no UI that shows or rotates a user's API token. The web page puts it in
  `<meta name="rulinky-auth-token">` so the page's own JS can call the API.

## 4. Web UI

A single-page React 19 app (Inertia.js on top of Rails, Vite build), styled with **Tailwind
from the Play CDN** (`cdn.tailwindcss.com`). The page title is "Rulinky". Favicons, the
apple-touch-icon and `site.webmanifest` (standalone display) are in `public/`.

### 4.1 Links page (`GET /`)
The server sends **all** of the user's links (no pagination), sorted by `updated_at` descending.
Each link has:
`id, url, title (display title cut to 50 chars), fullTitle, note, read (bool),
updatedAt (local time string "M/D/YYYY, H:MM:SS AM"), updatedAtMs`,
plus `readCount` and `unreadCount`.

> Quirk: `updatedAtMs` is sent as `updated_at.to_i`, and `updated_at` is already in
> milliseconds, so the client gets milliseconds and its `new Date(ms)` is correct.
> `updatedAt` divides by 1000 before formatting, so it is correct too.

Layout (one card, max width `md`, centered):
- **Header:** the logo image (hotlinked from S3) and a hamburger menu with **Sign out**.
- **"Add link"** panel, collapsed by default:
  - A URL input (`type=url`, autofocused when the panel opens) and an optional note textarea.
  - The submit button says "Add link" with a `Cmd+Enter` hint, and "Saving..." while the request runs.
  - **Cmd/Ctrl+Enter** inside the form submits it (Shift or Alt turns this off).
  - It posts `POST /links` (`url`, `note`). A blank URL brings back the error
    `url: "Enter a link"`. On success the form resets, the panel closes, the Unread tab is
    selected, and the list reloads.
- **Search box:** filters on the client. The query is lowercased and split on whitespace, and
  **every** term must appear in `fullTitle + note + url`. An × button clears it.
- **Tabs:** `Unread N` (default), `Read N`, `All N`. The counts are computed on the client from
  the current state, so they change right away when you toggle a link.
- **Timeline list:** links are grouped into consecutive "months ago" buckets by `updatedAtMs`:
  `This month`, `1 month ago` … `5 months ago`, `6+ months ago`. Each bucket has a sticky
  colored pill. The color fades from `rgb(173,27,26)` (this month) to `rgb(205,196,196)`
  (6+). A vertical line with a colored dot runs along the left side.
- **Each link card:**
  - The title (display title, with the full title as a tooltip) links to the URL in a **new tab**.
  - **Clicking the link marks it read**, optimistically, with a `keepalive` request. While you
    stay on the Unread tab, the card is "sticky": it stays visible, dimmed, until the next
    reload, so the list does not jump.
  - 🗑️ **Delete**: optimistic, calls `DELETE /api/links`, and rolls back on error. No confirmation.
  - ✅ / 📩 **Toggle read/unread**: optimistic, calls `PATCH /api/links`, and rolls back on error.
  - The note, if there is one, and the `updatedAt` timestamp.
  - Read cards are dimmed (gray background, 60% opacity).
- Empty state: "No links found."

### 4.2 Display title rules (shared by the UI and the API)
1. If `raw_title` is set, use it.
2. Otherwise, if `content` has a `<title>` element, use its whitespace-collapsed text.
3. Otherwise, use the URL without its scheme: host with `www.` removed, plus `:port` when it is
   not the default, plus the path, plus `#fragment`. The query string is dropped. For URLs that
   don't parse, strip `http(s)://(www.)` and anything after `?`.
- `title` in the page props is cut to 50 chars (Rails `truncate`, ending in `...`).
- `fullTitle` / the API's `title`: **bug today.** `link_content_title(link).to_s || url` never
  falls back to the URL, because `"".to_s` is truthy in Ruby, so links with no title get `""`.
  **Rewrite decision:** fix it and fall back to the scheme-less URL.

### 4.3 Server-rendered HTML link routes (the UI doesn't use them)
These are still routed and need a session:
- `POST /links`: create (used by the Add form).
- `DELETE /links/:id`: delete, then redirect to `/`.
- `PATCH /links/:id/read` and `PATCH /links/:id/unread`: set read to 1 or 0, bump `updated_at`,
  then redirect back.
  **Rewrite decision:** keep only `POST /links`. The UI uses the JSON API for the others.

## 5. JSON API (`/api/links`)

All endpoints need the bearer token (§3.2). They take JSON bodies.

| Method | Body | Success | Errors |
|---|---|---|---|
| `GET /api/links` | – | `200` array of the user's links, sorted by `updated_at` desc: `{id, url, note, read (0/1 int), timestamp (ms), updated_at (ms), title, content_title}` | `500 {"error":"Internal Server Error"}` |
| `POST /api/links` | `{link: string, note?: string}` | `201 {message:"Link saved", id, note, job_id, title}`, then a content fetch job is queued | `400 {"error":"Invalid link"}` if `link` is not a non-blank string; `500` |
| `PATCH /api/links` | `{id: string, read: bool-ish}` | `200 {message:"Link updated"}`; sets read 0/1 and bumps `updated_at` | `400 {"error":"Invalid payload"}`; `404 {"error":"Link not found"}`; `500` |
| `DELETE /api/links` | `{id: string}` | `200 {message:"Link deleted"}`; also deletes its jobs | `400 {"error":"Invalid payload"}`; `404 {"error":"Link not found"}`; `500` |

Details:
- `read` accepts what Rails' boolean cast accepts: `true/false`, `"true"/"false"`, `1/0`,
  `"1"/"0"`, `"t"/"f"`, `"on"/"off"`, and so on. An empty string or null is invalid.
- `title` on create is computed right after the insert, before any content is fetched,
  so in practice it is `""` today (see the bug in §4.2).
- Every query is scoped to the token's user. Another user's link gets a 404.
- The README mentions `GET /api/links/jobs/:id` and an unauthenticated `GET`. **Neither exists.**
  The README is out of date.

### Clients of the API
- **Chrome extension** (`chrome-extension/rulinky-save-link`, MV3): clicking the toolbar button
  sends `POST https://rulinky.chiq.me/api/links {link: tab.url}` with a **hardcoded token**.
  It shows an `alert()` and a badge `SAVED` (green, 4 s) or `ERR` (red). Only http(s) tabs work.
- The web UI itself (PATCH/DELETE).
- Other clients (iOS Shortcuts and similar) that POST with a token. The API contract must stay
  byte-compatible.

## 6. Background content fetching

Each link created (from the web or the API) gets a `link_content_jobs` row (`queued`), and an
in-process async job starts (SuckerPunch: an in-memory thread pool, **not persisted**, so jobs
queued before a restart are lost and stay `queued` forever).

What the job does:
1. Loads the job (it logs a warning and stops if the job is missing), then sets
   `status=running` and clears `error_message`.
2. **ChromeCrawler**, which drives the user's real Chrome:
   - Copies `Cookies`, and *tries* to copy Local Storage, Session Storage and IndexedDB, from
     the real profile (`~/Library/Application Support/Google/Chrome/<profile>`) into a separate
     automation dir (`.../ChromeRulinky/<profile>`). Chrome refuses DevTools on its default
     data dir, which is why it copies.
     > Bug: `%w[Cookies "Local Storage" ...]` splits the names on spaces, so only `Cookies`
     > and `"IndexedDB"` (quotes included, which never exists) are tried. In practice
     > **only Cookies** get copied.
   - **Kills every running Google Chrome** (`pkill -i "Google Chrome"`), waits up to 15 s for
     it to exit, then waits another 0.5 s.
   - Starts Chrome through Selenium + chromedriver with the automation user-data-dir and profile,
     and the flags `--no-first-run --no-default-browser-check --no-restore-last-session
     --disable-session-crashed-bubble --disable-sync`. Page-load timeout is 60 s, HTTP client
     timeouts 120 s, and the chromedriver log goes to `/tmp/chromedriver.log`.
   - Navigates to the URL, then runs a cleaning script (a port of the readitsoon bookmarklet):
     - On x.com / twitter.com, if `[data-testid="twitterArticleReadView"]` exists, it keeps only that element.
     - Otherwise it clones the whole document.
     - It removes `script, style, canvas, select, textarea` and adds `<h1>{document.title}</h1>`
       at the top of the body.
     - It returns `"<!DOCTYPE html>\n" + outerHTML`.
   - Always quits the driver.
   - Env: `CHROME_REAL_USER_DATA_DIR`, `CHROME_USER_DATA_DIR`, `CHROME_PROFILE_DIRECTORY`
     (default `Default`).
3. **ReaditsoonPreview**: `POST https://readitsoon.app/preview` as a form with
   `inputhtml=<html>`, `url=<url>` and `email=$READITSOON_EMAIL` (default
   `guillermo.siliceo@kindle.com`). Open timeout 10 s, read timeout 90 s.
   - On a non-2xx response it raises the JSON `error`, or the `errors` joined, or
     `readitsoon preview failed (<code>)`.
   - It parses the HTML: finds `form[x-data^="previewSendForm"]` (if it is missing, it raises
     "readitsoon preview form not found in response"). The title comes from that form's
     `input[name=title]` value and the content from `input[name=html_content]` value, searched
     anywhere in the document.
4. Saves `links.content` and `links.raw_title`, then sets the job to `status=finished`.
   **`updated_at` is not changed**, so the link keeps its place in the list.
5. On any error it sets the job to `status=failed` with `error_message`, and logs the error
   class, message and the first 5 backtrace lines.

Jobs run in parallel up to SuckerPunch's default pool size (2 workers). Two jobs at once would
each kill the other's Chrome.
**Rewrite decision:** run content jobs **one at a time** (a single worker goroutine fed by a
channel). At startup, re-queue any jobs left in `queued` or `running`.

Nothing in the UI shows job status. The only visible result is that the title changes from the
URL to the article title after the next reload.

## 7. Configuration (env)

The app loads `.env`, then `.env.local` (which overrides `.env`). Real environment variables
always win. Format: `KEY=VALUE`, `#` comments, optional double quotes around the value.

| var | used for |
|---|---|
| `DATABASE_HOST`, `DATABASE_PORT` (5432), `DATABASE_USER`, `DATABASE_PASSWORD` | Postgres connection; the database name is `rulinky` in production |
| `PORT` | HTTP port (`./serve` passes 3002) |
| `HOSTS` | comma-separated extra allowed `Host` headers (Rails host authorization) |
| `SLACK_OTP_WEBHOOK_URL` | OTP delivery |
| `READITSOON_EMAIL` | readitsoon preview form field |
| `CHROME_REAL_USER_DATA_DIR`, `CHROME_USER_DATA_DIR`, `CHROME_PROFILE_DIRECTORY` | crawler |
| `INFORMANT_API_TOKEN` | rails-informant MCP access |
| `RAILS_LOG_LEVEL`, `RAILS_MAX_THREADS` | Rails-specific |
| `SECRET_KEY_BASE` / `config/master.key` | session cookie encryption |
| `MAILGUN_*`, `SMTP_HOST`, `DEFAULT_EMAIL` | unused (see §3.1) |
| `NEXT_PUBLIC_AUTH_TOKEN`, `AUTH_TOKEN`, `FIRECRAWL_API_KEY`, `APIFY_API_KEY` | listed in the README, **not used** by any code anymore |

Production behavior: `force_ssl` and `assume_ssl` are on (TLS ends at the Cloudflare tunnel),
so cookies are `Secure` and HSTS is sent.

## 8. Operations

- `./serve`: (re)starts the app in place in tmux session `rulinky` using
  `~/c/server/serve-in-place.sh`. It runs bundle install, pnpm install + build,
  `db:migrate`, then `rails server -p 3002` with `RAILS_ENV=production`.
  `~/c/server/after-reboot.sh` calls it at boot.
- `./activate`: the git-sync hook. If the tmux session `rulinky` exists (only on the macmini),
  it re-runs `./serve`.
- `GET /up`: health check, `200` when the app boots (Rails health controller; returns a small
  HTML page with a green background).
- The process title is `rulinky`.
- Logs: `log/<env>.log`, rotated at 1 MB (1 file kept). Production also logs to STDOUT,
  tagged with the request id. `/up` is not logged.
- `Procfile`: `server: rails s -p 3002`. A `Dockerfile` and `bin/docker-entrypoint` exist
  (Rails boilerplate, Thruster), but production doesn't use them.

**Rewrite decision:** one static Go binary. Build the frontend with Vite at build time and
embed it in the binary (`embed.FS`). `./serve` becomes: build, then run the binary on 3002.

## 9. Error tracking (rails-informant)

- The `rails-informant` gem is mounted at `/informant`. In production it records uncaught
  exceptions into the `informant_*` tables. Access needs `INFORMANT_API_TOKEN`.
- `.mcp.json` and the `.claude/skills/informant` skill plus `.claude/hooks/informant-alerts.sh`
  read from it.

**Rewrite decision:** drop it. Uncaught errors get logged instead. The informant tables stay in
the DB and are no longer used.

## 10. One-off scripts (not product features)

- `script/export_links_to_csv.rb`: legacy SQLite (`data/links.db`) → CSV.
- `script/import_links_from_csv.rb`: CSV → Postgres `links` (upsert by id, batch size 1000).
- `script/backfill_unread_link_content_jobs.rb`: queues content jobs for unread links with no
  content. **It is broken**: it references `FetchLinkContentJob::TWITTER_HOSTS`, which no
  longer exists.

**Rewrite decision:** don't port these. Their one-time migrations are done.

## 11. Tests that encode behavior

`test/` covers: API links controller (auth, CRUD, error codes), auth controller and the OTP
sign-in integration flow, the HTML links controller, the links helper (title and URL formatting),
the fetch job, `User` OTP logic, `ReaditsoonPreview` parsing, the Slack notifier, and
`createShortcut.js`. Treat these as the acceptance cases to port to Go tests.

## 12. Rewrite summary (proposed)

- **Keep:** the Postgres schema and data, every route and the JSON contract in §3–§5, the
  OTP-over-Slack login, a 10-day session, the React UI (same JSX, served as a static Vite build
  with the page data embedded or fetched as JSON instead of through Inertia), the Chrome crawler
  plus readitsoon pipeline (Go: `chromedp` or `rod` over the DevTools protocol instead of
  Selenium/chromedriver), and `/up`.
- **Drop:** Inertia/Rails/Devise, Mailgun/SMTP, rails-informant, the unused HTML link mutation
  routes, the Dockerfile, the legacy scripts, and the stale README auth claims.
- **Fix:** the `fullTitle` fallback (§4.2), the copy list of profile files (§6), and job
  concurrency plus persistence (§6).
- **Open question:** the frontend. Either keep React + Vite (a small JSON endpoint replaces
  Inertia props, and the HTML redirects become `fetch` calls), or server-render with Go
  `html/template` plus a little vanilla JS. Server rendering removes the Node build step
  entirely.
