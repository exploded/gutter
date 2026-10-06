# gutter — Up The Spout

Website and booking system for **Up The Spout**, Vin's residential gutter
cleaning business across Melbourne's north-east (Manningham, Nillumbik,
Maroondah), based in Warrandyte VIC 3113.
A fork of `C:\Projects\go\localithelp` (Local IT Help), so most mechanics —
bookings, calendar sync, invoices, receipts, Zeller links, Ads attribution,
review card, scheduler, backups — match that project. Check there first when
something looks unfamiliar.

The business plan (market research, name, ABN decision, launch timeline) is a
Claude Doc: https://claude.ai/code/artifact/28260734-ee63-4958-898a-1f82ff0cc04e.
It is deliberately not in this repo: the repo is public.

## Skills
- Always invoke `go-htmx-skill` for UI changes (htmx 4, admin pages only).
- Invoke `sqlc-sqlite` before touching `db/queries.sql`; run `sqlc generate` after.
- Write prose (copy, docs, commits) with the `google-style` skill: Australian spelling.

## Brand
Easy-going, sunny, tradie signwriting. Navy ink `#12284A`, tangerine `#FF7A21`
(the spout, primary buttons), sky `#4DB0FF` (water), sun `#FFD449` (labels,
highlights), cream `#FFF7EA` ground. Tangerine, sky and sun are fills only; use
`--accent` / `--sky-text` for coloured text. Cards and buttons are "stickers":
2px ink outline plus a hard offset shadow (`--pop`). Speak as Vin, first person.
Describe the area as "Melbourne's north-east"; Warrandyte is only the base.
The mark is `static/img/logo-mark.svg` (gutter, tangerine downpipe, drop, leaf);
`go run ./tools/genassets` and `go run ./tools/bizcard -logos` re-render
everything from it.

## Stack
Go 1.25, `net/http` ServeMux, `html/template` (read from `templates/` on disk at
startup, not embedded), modernc/sqlite + sqlc, htmx 4 on `/admin` only (public
pages are script-light: `book.js`, `address.js`, `areamap.js`, `reveal.js`).
Module `gutter`. Fonts: Lilita One (headings, wordmark) + Figtree (body), self-hosted.

## Where things live
- `cmd/server/pricing.go` — **the price list**. Every price on the site, the
  booking form's live total, invoice prefill, llms.txt and `/api/pricing` read
  from here. Change prices here only.
- `cmd/server/services.go` — the six services (copy + meta).
- `cmd/server/suburbs.go` — the service area (15 suburbs, unique blurbs; `TestSuburbs` rejects thin copy).
- `cmd/server/guides.go` — `/guides` articles.
- `cmd/server/pages.go` — public handlers incl. `/book`; `admin_*.go` — admin.
- `tools/bizcard` — Officeworks business-card PDF; `tools/genassets` — icons/OG image; `tools/brand` — logo files and fonts.

## Business rules the code encodes (don't drift)
- **No repairs.** Fixing or replacing gutters/downpipes is licensed plumbing work
  in Victoria. Copy must never offer repairs or gutter guard installation.
- **Make no claims we can't back up**: no "fully insured", years of experience,
  ratings or review counts until they're true.
- No GST is charged (sole trader, under the threshold). Invoices must not say
  "tax invoice" until Vin registers for GST.
- `businessName` (main.go) must match the ASIC registration and the Google
  Business Profile exactly.
- Hours (`openHours` in main.go) must match the Business Profile.

## Local dev
`build.bat` (deletes `app.db`, runs `sqlc generate`, builds, runs on :8080).
That's James's server — never start, restart or kill it. To test a change, run
your own instance on another port: `PORT=8999 go run ./cmd/server`, then stop it.
Booking needs `MAPPIFY_API_KEY` for the address autocomplete.

## Deploy
Push to `main` → GitHub Actions (vet + test, build, scp, `deploy-gutter`).
Server: `/var/www/gutter`, systemd unit `gutter`, port **8997**, staged at
`https://gutter.mchugh.au` until `upthespout.com.au` is registered.
