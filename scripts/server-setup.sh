#!/bin/bash
set -euo pipefail

# One-time server setup for the Warrandyte Gutters Go web app (repo exploded/gutter).
# Usage: curl -fsSL https://raw.githubusercontent.com/exploded/gutter/main/scripts/server-setup.sh | sudo bash
#
# Staged at gutter.mchugh.au until warrandytegutters.com.au is registered; the
# README's "Domain cutover" section covers the move.

APP_DIR="/var/www/gutter"
SERVICE="gutter"
DEPLOY_USER="deploy"
PORT="8997"

echo "Setting up $SERVICE..."

# Create app directory
mkdir -p "$APP_DIR"

# .env template (only if absent — never overwrite real secrets)
if [ ! -f "$APP_DIR/.env" ]; then
cat > "$APP_DIR/.env" <<EOF
# ── Warrandyte Gutters ──
PORT=$PORT
PROD=true
APP_DIR=$APP_DIR
# Staging origin; change to https://warrandytegutters.com.au at the domain cutover.
BASE_URL=https://gutter.mchugh.au

# Contact details shown on the site (PHONE empty = phone UI hidden).
# CONTACT_EMAIL is also the SES sender + notification address.
PHONE=
CONTACT_EMAIL=vin@warrandytegutters.com.au
OWNER_NAME=Vin

# Invoices: ABN and bank transfer details (both empty = hidden). Prices live in cmd/server/pricing.go.
ABN=
BANK_ACCOUNT_NAME=
BANK_BSB=
BANK_ACCOUNT_NO=
SENIORS_DISCOUNT_PCT=0

# Google tag. Nothing loads until an ID is set. SAME_AS is a comma-separated list
# of profile URLs (Google Business Profile, Facebook, ...) for the home-page JSON-LD.
GA4_ID=
GOOGLE_ADS_ID=
GOOGLE_ADS_BOOKING_LABEL=
GOOGLE_ADS_CALL_LABEL=
SAME_AS=
# Google review link from the Business Profile (enables /review and the review card).
REVIEW_URL=

# Admin: comma-separated Google accounts allowed into /admin. The FIRST one is
# the account Google Calendar sync connects to (Vin's).
ADMIN_EMAIL=james67@gmail.com
GOOGLE_CLIENT_ID=
GOOGLE_CLIENT_SECRET=

# Address autocomplete on /book (Mappify). Without it the booking form can't be submitted.
MAPPIFY_API_KEY=

# Email notifications via Amazon SES (scripts/ses-setup.sh prints these). Empty = disabled.
AWS_REGION=ap-southeast-2
AWS_ACCESS_KEY_ID=
AWS_SECRET_ACCESS_KEY=

# Background scheduler (reminders, digest, nightly backup). Runs when PROD is set; 0 disables.
SCHEDULER=

# Nightly DB backup to S3 (scripts/s3-backup-setup.sh prints these). Empty = no backups.
BACKUP_S3_BUCKET=gutter-backups
BACKUP_AWS_ACCESS_KEY_ID=
BACKUP_AWS_SECRET_ACCESS_KEY=
EOF
    chmod 640 "$APP_DIR/.env"
    echo "Created $APP_DIR/.env template — edit it before starting the service."
fi

chown -R www-data:www-data "$APP_DIR"

# Install deploy script if a deploy bundle is present
cp /tmp/gutter-deploy/scripts/deploy-gutter /usr/local/bin/deploy-gutter 2>/dev/null || true
chmod +x /usr/local/bin/deploy-gutter 2>/dev/null || true

# Create systemd service
cat > /etc/systemd/system/${SERVICE}.service <<EOF
[Unit]
Description=Warrandyte Gutters web app
After=network.target

[Service]
Type=simple
User=www-data
WorkingDirectory=$APP_DIR
ExecStart=$APP_DIR/gutter
EnvironmentFile=$APP_DIR/.env
Environment=PORT=$PORT
Environment=PROD=true
Environment=APP_DIR=$APP_DIR
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable "$SERVICE"

# Sudoers for deploy user
cat > /etc/sudoers.d/${SERVICE} <<EOF
$DEPLOY_USER ALL=(ALL) NOPASSWD: /usr/local/bin/deploy-gutter
$DEPLOY_USER ALL=(ALL) NOPASSWD: /usr/bin/systemctl stop $SERVICE
EOF
chmod 440 /etc/sudoers.d/${SERVICE}

cat <<EOF

Setup complete.

Next steps:
  1. Edit $APP_DIR/.env (PHONE, ADMIN_EMAIL, Google OAuth, Mappify, SES).
  2. Add this to /etc/caddy/Caddyfile and run: sudo systemctl reload caddy

     gutter.mchugh.au {
         import access_log
         reverse_proxy 127.0.0.1:$PORT {
             import go_proxy
         }
     }

  3. Deploy from GitHub Actions (or: sudo /usr/local/bin/deploy-gutter), then
     sudo systemctl enable --now $SERVICE
EOF
