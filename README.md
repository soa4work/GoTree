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
- Russian (default) and Kazakh interface: the first visit follows the browser language, otherwise
  Russian, and the choice is saved in the browser. Calendar feeds, dates and server messages follow
  the language. Kazakh is still to be proofread by a native speaker.
- Several users and trees: administrators create accounts, owners add members as owner, editor or
  viewer, and everyone can switch between their trees

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
| Keep the GPS position in served and exported photos | | `GOTREE_PHOTO_LOCATION` | `strip` |
| Trusted reverse proxies (their X-Forwarded-For names the client) | | `GOTREE_TRUSTED_PROXIES` | unset |

`gotree -healthcheck` asks the running server on the listen address for `/api/health` and exits 0
when it is healthy; the Docker image uses it as its HEALTHCHECK.

Photos are served and exported without their GPS position (EXIF and XMP in JPEG, PNG and WebP);
the position read on upload stays visible to members of the tree. `GOTREE_PHOTO_LOCATION=keep`
serves the originals instead, but a GEDZIP export without living people strips it anyway.
Backups always hold the originals.

On first start GoTree asks you in the browser to create the admin account and name your tree.
For unattended installs, set `GOTREE_ADMIN_PASSWORD` instead.

### Accounts and members

Only administrators manage accounts, under **Accounts** in the menu: they create an account, change
its name and role, reset its password (which signs it out on every device) and delete it. They
cannot delete or reset themselves, and the last administrator can be neither deleted nor made a
regular user. Being an administrator gives no access to the contents of a tree.

Tree members are managed by the owners of that tree, under **Members**: they add an existing
account by its username as owner, editor or viewer, change roles and remove members. A tree always
keeps at least one owner, so the last owner can neither be demoted nor removed, and an account that
is the last owner of a tree cannot be deleted. Any member can leave a tree. Editors change the
tree, viewers only read it, and viewers can export it only without living people. A person who is
not a member of a tree cannot see that it exists.

Behind a reverse proxy, terminate TLS there and forward `X-Forwarded-Proto: https` so the session
cookie is marked secure. Failed logins are limited per username and per client address. Behind a
reverse proxy, set `GOTREE_TRUSTED_PROXIES` to the proxy's address or range (e.g. `127.0.0.1` or
`172.16.0.0/12`) and have it set `X-Forwarded-For`; otherwise GoTree sees only the proxy's address
and the per-address limit applies to all clients together. Never list addresses that clients can
reach GoTree from directly. A username's limit does not lock out a client address that logged in as that
user within the last 120 days (remembered in memory only, so a restart forgets it), so someone
guessing the owner's name cannot lock the owner out of their usual device; the per-address limit
still applies to it.

Behind a reverse proxy, allow request bodies as large as the upload limit.

The migration map loads its background tiles from openstreetmap.org in the browser. The "Look up
on OpenStreetMap" button next to a place without coordinates sends that place's name and
your GoTree server's address (as the referrer) to nominatim.openstreetmap.org, only when you click it. Everything else GoTree shows comes from your
own server.

### Backup and restore

Everything lives in the data directory: the database `gotree.db` and `media/` with the uploaded
files. Administrators can download both as one zip under **Import & export → Backup**; the
database in it is a consistent snapshot taken while GoTree keeps running. The backup holds every
tree and every account; to share one tree, export it as GEDCOM. Copying the data
directory works too, but only while GoTree is stopped, since the database may have changes in
its `-wal` file that a copy taken mid-write would miss.

GoTree finishes running requests for up to 30 seconds on shutdown. `docker stop` sends SIGKILL
after 10 seconds by default, so use `--stop-timeout 30` (Compose: `stop_grace_period: 30s`) to
shut down cleanly during a long import.

To restore, stop GoTree, unzip the backup into an empty data directory (so that `gotree.db` and
`media/` sit directly in it) and start GoTree again. Thumbnails are not in the backup; they are
made again when first shown.

## Development

Requires Go and Node.js (LTS). `GOTREE_RACE=1 ./run-tests.sh` runs the Go tests with the race
detector, which also needs a C compiler (CI does this).

```bash
make dev     # API on :8080, Vite on :5173 with hot reload (proxies /api)
make build   # frontend + single binary ./gotree
make test    # ./run-tests.sh — the same gate the pre-push hook and CI use
```

The end-to-end tests (`web/e2e`, Playwright) use the installed Chrome or Chromium; set
`CHROME_PATH` if it is not found in the usual places. `GOTREE_AUDIT=1 ./run-tests.sh` also runs
govulncheck and `npm audit` (CI does this on every PR and nightly).

### Languages

`web/src/locales/en.json` is the source: `ru.json` and `kk.json` must have the same keys
(`locales.test.ts` checks it), and Russian needs the plural forms `_one`, `_few`, `_many` and
`_other`. English is a hidden fallback used by the unit tests and Playwright (`gotree-lang=en`).
`npm run e2e -- e2e/i18n.spec.ts` checks the main pages in Russian and Kazakh, including the
phone width.

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
