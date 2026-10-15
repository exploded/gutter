#!/usr/bin/env bash
# Wipe pre-launch test data from the live database. Every booking, invoice,
# invoice line and customer goes, along with:
#
#   - ad clicks attached to a test booking, and
#   - ad clicks with no gclid, which only a test fetch produces while
#     auto-tagging is on (real ad clicks always carry one), plus any gclid we
#     made up ourselves while testing the deploy.
#
# Untouched: admin sign-ins (users), Google Calendar and Gmail connections,
# scheduler history, and real unbooked ad clicks. Invoice numbers start again
# at 1000, since the next number is MAX(number) + 1.
#
# Dry run by default - it prints exactly what it would delete and stops. Pass
# --apply to go ahead. Applying stops the service, backs the database up, and
# does the whole delete in one transaction.
#
#   sudo bash cleanup-test-data.sh            # look
#   sudo bash cleanup-test-data.sh --apply    # delete
set -euo pipefail

APP=gutter
DB=/var/www/$APP/app.db
TEST_GCLIDS="'SHIP-CHECK'"         # gclids we invented while testing, never real clicks
TEST_CLICKS="booking_id <> 0 OR gclid = '' OR gclid IN ($TEST_GCLIDS)"
BACKUP_DIR=/var/www/$APP/backups
APPLY=false
[ "${1:-}" = "--apply" ] && APPLY=true

if [ "$(id -u)" -ne 0 ]; then
    echo "Run me with sudo - the database belongs to www-data." >&2
    exit 1
fi
if ! command -v sqlite3 >/dev/null; then
    echo "sqlite3 is not installed. Run: sudo apt-get install -y sqlite3" >&2
    exit 1
fi
if [ ! -f "$DB" ]; then
    echo "No database at $DB" >&2
    exit 1
fi

q() { sqlite3 -header -column "$DB" "$1"; }

echo "=== Bookings to delete (all) ==="
q "SELECT id, name, suburb, status, start_at, created_at,
          CASE WHEN gcal_event_id = '' THEN '-' ELSE 'yes' END AS in_gcal
   FROM bookings ORDER BY id;"

echo
echo "=== Invoices to delete (all) ==="
q "SELECT id, number, booking_id, status, total_cents, issued_at FROM invoices ORDER BY id;"

echo
echo "=== Customers to delete (all) ==="
q "SELECT id, name, email, suburb, created_at FROM customers ORDER BY id;"

echo
echo "=== Test ad clicks to delete ==="
q "SELECT id, booking_id, source, gclid, keyword, landing, created_at
   FROM ad_clicks WHERE $TEST_CLICKS ORDER BY id;"

echo
echo "=== Ad clicks that will survive (real, unbooked) ==="
q "SELECT COUNT(*) AS kept, MIN(created_at) AS oldest, MAX(created_at) AS newest
   FROM ad_clicks WHERE NOT ($TEST_CLICKS);"

# A calendar event outlives its booking: Google keeps it, and deleting the row
# here leaves nothing to cancel it with.
GCAL=$(sqlite3 "$DB" "SELECT COUNT(*) FROM bookings WHERE gcal_event_id <> '';")
if [ "$GCAL" != "0" ]; then
    echo
    echo "!! $GCAL booking(s) above still have a Google Calendar event (in_gcal = yes)."
    echo "!! Delete those events in Google Calendar too, or they will linger with"
    echo "!! nothing in the app pointing at them."
fi

if [ "$APPLY" != true ]; then
    echo
    echo "Dry run - nothing deleted. Re-run with --apply to go ahead."
    exit 0
fi

echo
echo "Stopping $APP..."
systemctl stop "$APP"

mkdir -p "$BACKUP_DIR"
BACKUP="$BACKUP_DIR/app-before-cleanup-$(date +%Y%m%d-%H%M%S).db"
sqlite3 "$DB" ".backup '$BACKUP'"
echo "Backed up to $BACKUP"

sqlite3 "$DB" <<SQL
PRAGMA foreign_keys = ON;
BEGIN;

DELETE FROM ad_clicks WHERE $TEST_CLICKS;
DELETE FROM invoice_items;
DELETE FROM invoices;
DELETE FROM bookings;
DELETE FROM customers;

-- Restart the row ids too, so the first real booking is #1.
DELETE FROM sqlite_sequence WHERE name IN ('bookings', 'invoices', 'invoice_items', 'customers');

COMMIT;
VACUUM;
SQL

chown www-data:www-data "$DB"*
echo "Starting $APP..."
systemctl start "$APP"
sleep 2

echo
echo "=== What's left ==="
q "SELECT (SELECT COUNT(*) FROM bookings)  AS bookings,
          (SELECT COUNT(*) FROM invoices)  AS invoices,
          (SELECT COUNT(*) FROM customers) AS customers,
          (SELECT COUNT(*) FROM ad_clicks) AS ad_clicks;"

echo
echo -n "Health check: "
curl -sS -o /dev/null -w "%{http_code}\n" https://upthespout.com.au/health
echo "Backup kept at $BACKUP"
