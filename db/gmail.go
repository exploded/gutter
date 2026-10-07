package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"gutter/db/sqlc"
)

// GoogleMail holds the Gmail account customer emails are sent from. There is
// at most one row; nil means "not connected" and email goes through SES.
type GoogleMail struct {
	AccountEmail string
	RefreshToken string
	ConnectedAt  time.Time // UTC
	LastSentAt   time.Time // UTC; zero until Gmail first accepts a message
	LastError    string
}

// Connected reports whether a usable connection is stored.
func (g *GoogleMail) Connected() bool {
	return g != nil && g.RefreshToken != ""
}

// GetGoogleMail returns the stored connection, or nil when Gmail has never been
// connected (or was disconnected). A missing row is not an error.
func GetGoogleMail() (*GoogleMail, error) {
	r, err := q.GetGoogleMail(context.Background())
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &GoogleMail{
		AccountEmail: r.AccountEmail, RefreshToken: r.RefreshToken,
		ConnectedAt: parseUTC(r.ConnectedAt), LastSentAt: parseUTC(r.LastSentAt),
		LastError: r.LastError,
	}, nil
}

// SaveGoogleMail stores (or replaces) the connection and clears any error.
func SaveGoogleMail(accountEmail, refreshToken string) error {
	return q.SaveGoogleMail(context.Background(), sqlc.SaveGoogleMailParams{
		AccountEmail: accountEmail, RefreshToken: refreshToken,
	})
}

// MarkGoogleMailSent stamps a successful send and clears the last error.
func MarkGoogleMailSent() error {
	return q.MarkGoogleMailSent(context.Background())
}

// SetGoogleMailError records the most recent failure for the settings page.
func SetGoogleMailError(msg string) error {
	return q.SetGoogleMailError(context.Background(), msg)
}

// DisconnectGoogleMail drops the stored token.
func DisconnectGoogleMail() error {
	return q.DeleteGoogleMail(context.Background())
}
