<p align="center">
  <img src="assets/logo-readme.jpg" alt="GoTree — Go-powered genealogy software" width="360">
</p>

# GoTree

A lightweight, self-hosted family tree app. One Go binary with an embedded React UI and a single
SQLite file — no database server, no cloud account.

**Status:** usable, pre-1.0 — there is no tagged release yet, so expect changes.

## Features

**Tree and people**
- People, families, events, places, sources and citations, with GEDCOM dates, alternate names,
  shared events with roles and disputed or disproven facts
- Pedigree, descendant, hourglass and family-group charts with pan & zoom, also as lists for
  screen readers and small screens
- Relationship calculator ("how is A related to B?") and a print-ready wall chart (A4 to A0, PDF
  or SVG)

**Records and evidence**
- Photos, documents and recordings attached to people, with face tags and portraits
- Source-first transcription of census, baptism, marriage and other records into the tree
- Research tasks and a research log; data-quality checks and "Tidy up dates"

**Places and history**
- Migration map with a year slider, place coordinates and OpenStreetMap lookup
- Statistics, heirlooms and an "On this day" list

**Sharing and data**
- Read-only share links (whole tree or one branch, living people hidden) and a calendar feed of
  birthdays and anniversaries
- GEDCOM 5.5, 5.5.1 and 7.0 import and export, also zipped with media; backup as one zip
- Light, dark or system theme and a first-run onboarding
- Single user today, data model ready for multiple users and trees (there is no multi-user UI yet)

## Stack

- Backend: Go, chi, SQLite (`modernc.org/sqlite`, no CGO), goose, sqlc
- Frontend: React, TypeScript, Vite, Tailwind CSS, React Flow
- Deployment: single binary (`go:embed`) or Docker image (`ghcr.io/soa4work/gotree`)

## Running

```bash
docker run -d -p 8080:8080 -v gotree-data:/data ghcr.io/soa4work/gotree
```

or download a binary from the releases page and run `./gotree`. Open http://localhost:8080.

| Setting | Flag | Environment | Default |
|---|---|---|---|
| Data directory (database, media) | `-data-dir` | `GOTREE_DATA_DIR` | `./data` (`/data` in Docker) |
| Listen address | `-listen` | `GOTREE_LISTEN_ADDR` | `:8080` |
| First admin account (only used while there is none) | | `GOTREE_ADMIN_USER`, `GOTREE_ADMIN_PASSWORD` | `admin`, unset |
| Largest upload (MB) | | `GOTREE_MAX_UPLOAD_MB` | `100` |

On first start GoTree asks you in the browser to create the admin account and name your tree.
For unattended installs, set `GOTREE_ADMIN_PASSWORD` instead.

Behind a reverse proxy, terminate TLS there and forward `X-Forwarded-Proto: https` so the session
cookie is marked secure. Failed logins are throttled per client address; GoTree sees the proxy's
address, so the limit then applies to all clients together.

Behind a reverse proxy, allow request bodies as large as the upload limit.

The migration map loads its background tiles from openstreetmap.org in the browser. The "Look up
on OpenStreetMap" button next to a place without coordinates sends that place's name and
your GoTree server's address (as the referrer) to nominatim.openstreetmap.org, only when you click it. Everything else GoTree shows comes from your
own server.

### Backup and restore

Everything lives in the data directory: the database `gotree.db` and `media/` with the uploaded
files. Administrators can download both as one zip under **Import & export → Backup**; the
database in it is a consistent snapshot taken while GoTree keeps running. Copying the data
directory works too, but only while GoTree is stopped, since the database may have changes in
its `-wal` file that a copy taken mid-write would miss.

GoTree finishes running requests for up to 30 seconds on shutdown. `docker stop` sends SIGKILL
after 10 seconds by default, so use `--stop-timeout 30` (Compose: `stop_grace_period: 30s`) to
shut down cleanly during a long import.

To restore, stop GoTree, unzip the backup into an empty data directory (so that `gotree.db` and
`media/` sit directly in it) and start GoTree again. Thumbnails are not in the backup; they are
made again when first shown.

## Development

Requires Go, a C compiler (for the race detector) and Node.js (LTS).

```bash
make dev     # API on :8080, Vite on :5173 with hot reload (proxies /api)
make build   # frontend + single binary ./gotree
make test    # ./run-tests.sh — the same gate the pre-push hook and CI use
```

The end-to-end tests (`web/e2e`, Playwright) use the installed Chrome or Chromium; set
`CHROME_PATH` if it is not found in the usual places.

Layout: `cmd/gotree` (entry point), `internal/` (`config`, `db` + migrations, `gendate` GEDCOM
dates, `store` domain logic, `api` HTTP handlers), `web/` (React app, embedded into the binary
via `web/embed.go`).

## Releasing

`./release.sh` derives the next version from the Conventional Commits since the last tag, moves
the `[Unreleased]` section of `CHANGELOG.md` into the release, updates `VERSION`, commits and
tags. `./release.sh --dry-run` shows what it would do. Pushing the tag
(`git push origin main --follow-tags`) triggers the release workflow, which publishes binaries
for Linux, macOS and Windows and a multi-arch Docker image.

## License

[AGPL-3.0](LICENSE)

The logo builds on the Go gopher, designed by [Renée French](https://reneefrench.blogspot.com/) and
licensed under [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/). See [NOTICE](NOTICE); the app
shows the same credit on the login page, in the sidebar and on share links.
