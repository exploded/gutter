# Up The Spout

Website and booking system for **Up The Spout**: residential gutter
cleaning across Melbourne's north-east, run by Vin from Warrandyte VIC 3113. Customers see fixed prices, book
online, and get a photo report with every clean. Vin runs bookings, his Google
Calendar, invoices, receipts and review requests from `/admin`.

Go + `html/template` + SQLite (sqlc). Forked from
[exploded/localithelp](https://github.com/exploded/localithelp), which documents
the shared mechanics in more depth.

- Repo: `exploded/gutter`, Go module `gutter`
- Live: https://upthespout.com.au (port 8997, systemd unit `gutter`)

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
   `https://upthespout.com.au/auth/google/callback` and
   `https://upthespout.com.au/auth/google/calendar/callback` and
   `https://upthespout.com.au/auth/google/mail/callback`, enable the Calendar
   and Gmail APIs, add the `calendar` and `gmail.send` scopes, and set the
   consent screen to **In production**.
3. **Admin accounts**: `ADMIN_EMAIL=upthespoutguttercleaning@gmail.com,james67@gmail.com`. Connect
   Google Calendar from `/admin/calendar/settings` and Gmail from `/admin/email`,
   both with the business Gmail. Once Gmail is connected, customer emails go from
   it, and so do admin notices. The business doesn't use Amazon SES.
4. **Backups**: `AWS_PROFILE=… scripts/s3-backup-setup.sh`.

## Domains

`upthespout.com.au` (VentraIP, DNS on Cloudflare) is the only site. Both zones
point `@` and `www` at the Linode box, DNS-only. Caddy serves the site on one
name and redirects the rest; `canonicalHost` (seo.go) also 301s any host that
isn't the one in `BASE_URL`.

```
upthespout.com.au {
    import access_log
    reverse_proxy 127.0.0.1:8997 {
        import go_proxy
    }
}

www.upthespout.com.au, upthespout.com, www.upthespout.com {
    redir https://upthespout.com.au{uri} permanent
}
```

`upthespout.com` is registered with Cloudflare Registrar and exists only to
redirect. The old staging name, `gutter.mchugh.au`, was retired on
7 October 2026.

Still to do for the domain: in Search Console, add `upthespout.com.au` as a
domain property and submit `/sitemap.xml`. Use the new URL on the Google
Business Profile and in Ads.

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
