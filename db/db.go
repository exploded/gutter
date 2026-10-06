package db

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"time"

	"gutter/db/sqlc"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

var (
	conn *sql.DB
	q    *sqlc.Queries
)

// ── Domain types (preserve existing API for main.go / templates) ──

type User struct {
	ID        int64
	GoogleID  string
	Email     string
	Name      string
	Picture   string
	CreatedAt string
}

// ── Open / Close ──

func Open(path string) error {
	var err error
	conn, err = sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	conn.SetMaxOpenConns(1)

	// Columns added after launch go here as "ALTER TABLE … ADD COLUMN …"
	// statements (constant defaults only — SQLite ALTER can't use
	// datetime('now')), and into schema.sql for fresh databases. They run before
	// the schema so a CREATE INDEX on a new column succeeds; "duplicate column"
	// (or "no such column" for a drop) errors on an up-to-date database are
	// expected and ignored.
	for _, stmt := range []string{
		"ALTER TABLE bookings DROP COLUMN has_guard", // gutter guard is no longer offered
	} {
		conn.Exec(stmt)
	}

	// Run embedded schema (idempotent CREATE TABLE/INDEX IF NOT EXISTS statements)
	if _, err := conn.Exec(schemaSQL); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}

	if _, err := conn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("enable foreign keys: %w", err)
	}

	q = sqlc.New(conn)

	if err := BackfillCustomers(); err != nil {
		return fmt.Errorf("backfill customers: %w", err)
	}
	return nil
}

// parseUTC parses the datetime('now') text SQLite stores (UTC, no zone).
func parseUTC(s string) time.Time {
	t, _ := time.Parse("2006-01-02 15:04:05", s)
	return t
}

// BackupTo writes a consistent snapshot of the live database to path using
// VACUUM INTO, which runs on the app's own connection so it never races a
// writer (the pool is capped at one connection). Any file already at path is
// removed first — VACUUM INTO refuses to overwrite.
func BackupTo(ctx context.Context, path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove old backup: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		return fmt.Errorf("vacuum into: %w", err)
	}
	return nil
}

// Ping runs a trivial query so /health reports a real DB failure (locked,
// corrupt, disk gone) rather than just "the process is up".
func Ping(ctx context.Context) error {
	var one int
	return conn.QueryRowContext(ctx, "SELECT 1").Scan(&one)
}

func Close() error {
	if conn != nil {
		return conn.Close()
	}
	return nil
}

// ── Users ──

func UpsertUser(googleID, email, name, picture string) (*User, error) {
	ctx := context.Background()

	if err := q.UpsertUser(ctx, sqlc.UpsertUserParams{
		GoogleID: googleID,
		Email:    email,
		Name:     name,
		Picture:  picture,
	}); err != nil {
		return nil, err
	}

	row, err := q.GetUserByGoogleID(ctx, googleID)
	if err != nil {
		return nil, err
	}
	return &User{
		ID:        row.ID,
		GoogleID:  row.GoogleID,
		Email:     row.Email,
		Name:      row.Name,
		Picture:   row.Picture,
		CreatedAt: row.CreatedAt,
	}, nil
}
