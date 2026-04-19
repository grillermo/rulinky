# Rulinky (Rails port of `eslabony`)

Simple link-saver Rails app that matches the `eslabony` Next.js behavior:

- Postgres via ActiveRecord (see `config/database.yml`)
- HTML UI at `/` with Read/Unread tabs
- JSON API at `/api/links`:
  - `GET` (no auth): list links
  - `POST` (auth): create link and enqueue a background scrape job
  - `GET /api/links/jobs/:id` (no auth): get the scrape job status for a link
  - `PATCH` (auth): `{ id, read: true|false }`
  - `DELETE` (auth): `{ id }`

## Ruby

Uses Ruby `3.4.7` (see `.ruby-version`).

## Auth token

Set `NEXT_PUBLIC_AUTH_TOKEN` (or `AUTH_TOKEN`) in the environment.
Set `FIRECRAWL_API_KEY` for general link scraping.
Set `APIFY_API_KEY` for `x.com` / `twitter.com` links, which now use Apify's `apidojo/twitter-scraper-lite` actor.

This repo includes a `.env` / `.env.local` loader at `config/initializers/load_env_local.rb` so `bin/rails s` will pick them up in development. `.env.local` overrides `.env`, and real shell environment variables still win.

## Run

```sh
cd /Users/grillermo/c/rulinky/rulinky
bin/rails db:create db:migrate
bin/rails s
```

## SQLite -> Postgres migration

1) Export existing SQLite data to CSV:

```sh
cd /Users/grillermo/c/rulinky/rulinky
/Users/grillermo/.rbenv/shims/ruby script/export_links_to_csv.rb tmp/links.csv
```

2) Import CSV into Postgres:

```sh
cd /Users/grillermo/c/rulinky/rulinky
bin/rails db:create db:migrate
bin/rails runner script/import_links_from_csv.rb tmp/links.csv
```
