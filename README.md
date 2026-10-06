# Up The Spout

Website and booking system for **Up The Spout**: residential gutter
cleaning across Melbourne's north-east, run by Vin from Warrandyte VIC 3113. Customers see fixed prices, book
online, and get a photo report with every clean. Vin runs bookings, his Google
Calendar, invoices, receipts and review requests from `/admin`.

Go + `html/template` + SQLite (sqlc). Forked from
[exploded/localithelp](https://github.com/exploded/localithelp), which documents
the shared mechanics in more depth.

- Repo: `exploded/gutter`, Go module `gutter`
- Staging: https://gutter.mchugh.au (port 8997, systemd unit `gutter`)
- Production domain (once registered): https://upthespout.com.au

## Structure

- `cmd/server/main.go` — server, routes, Google sign-in for `/admin`, site config, trading hours
- `cmd/server/pricing.go` — **the price list** (property types, downpipes, Fire-ready plan, neighbour and referral offers). Everything that shows or charges a price reads from here.
- `cmd/server/services.go` — the five services and their copy
- `cmd/server/suburbs.go` — the service area: one entry per suburb with a unique blurb → `/areas` and `/areas/{slug}`
- `cmd/server/guides.go` — `/guides` articles
- `cmd/server/pages.go` — public pages and the `/book` form (property, plan, live price)
- `cmd/server/admin_bookings.go`, `admin_invoices.go` — admin bookings, calendar, invoices, customers
- `cmd/server/gcal*.go` — Google Calendar sync; `scheduler.go` — reminders, digest, backups
- `cmd/server/seo.go`, `llms.go` — robots, sitemap, JSON-LD helpers, `/llms.txt`, `/api/pricing`
- `templates/` — layouts and one file per page; `static/` — CSS, JS, fonts, images
- `db/schema.sql`, `db/queries.sql` → `sqlc generate` → `db/sqlc/`
- `tools/genassets` — favicon/icon set and social images; `tools/bizcard` — print-ready business cards; `tools/brand` — logo files and fonts; `tools/areaphoto` — suburb photos

## Run locally

```
sqlc generate
go run ./cmd/server          # http://localhost:8080
# or build.bat (deletes app.db, regenerates, builds, runs)
```

Copy `.env.example` to `.env` for local settings. The booking form needs
`MAPPIFY_API_KEY`, because the address must be picked from the autocomplete.

## Prices

Edit `cmd/server/pricing.go` and redeploy. Current list:

| Property | Price |
| --- | --- |
| Unit or townhouse | $219 |
| Single-storey house | $289 |
| Large single storey or split level | $389 |
| Double-storey house | $489 |
| Blocked downpipe jetted (with the customer's OK) | $90 each |

The Fire-ready plan takes 12% off each of two cleans a year. No GST is charged.

## Bookings → invoices

1. **Booking** (`/book`): the customer picks a property type and the
   plan, sees the price, and submits. The booking stores the property, extras,
   the price shown, and a default duration from the price list.
2. **Schedule** it on the booking page or the week calendar. The customer gets a
   confirmation with an `.ics` invite; it also lands in Vin's Google Calendar.
3. **Done** → **Create invoice**: prefilled from the price list (clean,
   plan discount), so the total matches what the customer saw. Add downpipe
   jetting or the neighbour deal with the editor buttons.
4. **Zeller**: there's no public Zeller API for payment links. Create a payment
   link in the Zeller app for the amount and paste it into the invoice, or take
   Tap to Pay on the day and mark the invoice paid.
5. **Mark paid** emails the receipt and, if `REVIEW_URL` is set, can ask for a
   Google review.

The scheduler (on when `PROD` is set) sends the day-before reminder, the
1-hour heads-up, a 7:30 am digest, and runs the nightly S3 backup.

## First-time setup

1. **Server**:
   `curl -fsSL https://raw.githubusercontent.com/exploded/gutter/main/scripts/server-setup.sh | sudo bash`,
   then edit `/var/www/gutter/.env`, add the printed Caddy block,
   `sudo systemctl reload caddy`, and `sudo systemctl enable --now gutter`.
2. **Google Cloud**: create a project for this app (separate from the shared
   mchugh.au project, as Local IT Help does), an OAuth client with redirect URIs
   `https://gutter.mchugh.au/auth/google/callback` and
   `https://gutter.mchugh.au/auth/google/calendar/callback`, enable the Calendar
   API, add the `calendar` scope, and set the consent screen to **In production**.
3. **Admin accounts**: `ADMIN_EMAIL=<Vin's Gmail>,james67@gmail.com`. Vin connects
   Google Calendar from `/admin/calendar/settings` with his own account.
4. **Email**: after the domain is registered, `AWS_PROFILE=… CF_TOKEN=… scripts/ses-setup.sh`.
5. **Backups**: `AWS_PROFILE=… scripts/s3-backup-setup.sh`.

## Domain cutover (to do)

When `upthespout.com.au` is registered and on Cloudflare:

1. DNS: `A`/`AAAA` for `@` and `www` → the Linode box, DNS-only.
2. Caddy: a `upthespout.com.au` block proxying to port 8997, with
   `www.` and `gutter.mchugh.au` redirecting to it.
3. `.env`: `BASE_URL=https://upthespout.com.au`; restart.
4. Google Cloud: add the new redirect URIs.
5. Search Console: add the property and submit `/sitemap.xml`.

## Brand assets

- Logo: `static/img/logo-mark.svg` (mark). Lockups and PNGs: `tools/brand/out/`.
- Icons and social images: `go run ./tools/genassets`.
- Business cards for Officeworks (90 × 55 mm, 5 mm bleed, PDF):
  `go run ./tools/bizcard -name "Vin …" -phone "04xx xxx xxx"`.

## Deploy

GitHub Actions (`.github/workflows/deploy.yml`) runs `go vet` and `go test`,
builds a static Linux binary, copies it with `templates/`, `static/` and
`scripts/deploy-gutter` to the server, and runs the deploy script. Repo secrets:
`DEPLOY_HOST`, `DEPLOY_USER`, `DEPLOY_PORT`, `DEPLOY_SSH_KEY`.

`GET /health` returns `200 ok` when the app and database answer.
