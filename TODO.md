# TODO

In priority order. `[ ]` to do · ✅ done.

## Before launch (bookings open 19 October 2026)

The full list for James and Vin is the launch checklist doc (private; not in
this public repo). The code-side items:

- [ ] Send email from `bookings@upthespout.com.au` (SES) with `Reply-To` the business Gmail, once it exists
- [ ] Fill `.env`: `PHONE`, `CONTACT_EMAIL`, `ADMIN_EMAIL`, bank details
- [ ] Business cards (`tools/bizcard`) once the phone number is final
- [ ] Replace stock suburb photos with Vin's own (`tools/areaphoto`); add a photo of Vin to the home page
- [ ] `REVIEW_URL` and `SAME_AS` once the Google Business Profile is verified
- [x] `GA4_ID` (live 7 October 2026)
- [ ] `GOOGLE_ADS_ID`, `GOOGLE_ADS_BOOKING_LABEL`, `GOOGLE_ADS_CALL_LABEL`
- [ ] Delete the `warrandyte-gutters-backups` bucket once backups land in `upthespout-backups`

## Features

1. [ ] **Job photos** — upload before/after photos from the phone on the booking page;
   store on disk under `APP_DIR/photos/{booking}`; attach to the receipt email and
   show on the public invoice page. Included in the nightly backup or its own S3 prefix.
2. [ ] **Fire-ready plan reminders** — scheduler job: for paid bookings with
   `on_plan = 1`, email ~1 month before the next season (early September and
   mid April) with a prefilled `/book?plan=1&property=…` link; stamp the booking.
3. [ ] **GST invoices** — `GST_REGISTERED` flag: GST line, "Tax invoice" title,
   prices treated as GST-inclusive. Needed before turnover reaches $75,000.
4. [ ] **Fire danger day reschedule** — admin button that emails every customer
   booked for a given day with a "moved because of the fire danger rating" note.
5. [ ] **Neighbour deal from a booking** — "Book the street" admin action that
   prints door-hanger copy with the address and a prefilled link.
6. [ ] **SMS reminders** (ClickSend/Twilio) for customers without email.
7. [ ] **Public reviews + AggregateRating** once there are real Google reviews.
8. [ ] **Revenue by suburb and source** report for the 90-day review.
