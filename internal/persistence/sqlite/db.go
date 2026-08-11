// Package sqlite is the persistence layer. All SQL lives here; no darts rules
// do. The store hands the domain engines their recorded visits and stores what
// they produce, but it never decides what a bust is or who won.
//
// The driver is modernc.org/sqlite, a pure-Go implementation, so Windows builds
// need no CGO, no MinGW and no Visual Studio build tools.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, registered as "sqlite"
)

// Store is a handle on the darts database.
type Store struct {
	db *sql.DB
}

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("not found")

// Open opens (creating if necessary) the database at path and brings its schema
// up to date.
func Open(path string) (*Store, error) {
	dsn := buildDSN(path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// A TUI is single-user, and SQLite writes are serialised anyway. Holding one
	// connection avoids "database is locked" surprises between goroutines.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// buildDSN assembles a connection string with the pragmas the application needs.
//
//   - foreign_keys enforces the ON DELETE CASCADE relationships.
//   - journal_mode=WAL keeps an in-progress game durable if the application is
//     killed mid-leg, which is the whole point of persisting live games.
//   - busy_timeout stops a transient lock surfacing as an error in the UI.
func buildDSN(path string) string {
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "synchronous(NORMAL)")
	return "file:" + path + "?" + q.Encode()
}

// Close releases the database handle.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// DB exposes the underlying handle for tests and diagnostics.
func (s *Store) DB() *sql.DB { return s.db }

// migrate applies any migrations the database has not yet seen.
func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    applied_at TEXT NOT NULL
)`); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}

	applied := map[int]bool{}
	rows, err := s.db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("read applied migrations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return fmt.Errorf("scan migration version: %w", err)
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read applied migrations: %w", err)
	}

	for _, m := range migrations {
		if applied[m.Version] {
			continue
		}
		if err := s.applyMigration(ctx, m); err != nil {
			return fmt.Errorf("migration %d (%s): %w", m.Version, m.Name, err)
		}
	}
	return nil
}

func (s *Store) applyMigration(ctx context.Context, m migration) error {
	// A migration that rebuilds a referenced table has to run with foreign key
	// enforcement off, and the pragma is ignored inside a transaction, so it is
	// set on the connection first. There is only ever one connection, and it is
	// restored below whatever happens.
	if m.WithoutForeignKeys {
		if _, err := s.db.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
			return fmt.Errorf("suspend foreign keys: %w", err)
		}
		defer func() { _, _ = s.db.ExecContext(ctx, `PRAGMA foreign_keys = ON`) }()
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
		return err
	}
	if m.WithoutForeignKeys {
		// Nothing is committed until the rebuilt table has been checked against
		// every reference to it, so a migration cannot leave orphaned history.
		if err := checkForeignKeys(ctx, tx); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
		m.Version, m.Name, nowString(),
	); err != nil {
		return err
	}
	return tx.Commit()
}

// checkForeignKeys fails if the database holds a row pointing at one that is no
// longer there. PRAGMA foreign_key_check returns one row per violation and no
// rows at all when everything is sound.
func checkForeignKeys(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("check foreign keys: %w", err)
	}
	defer rows.Close()

	if rows.Next() {
		return errors.New("migration left rows referring to a table that no longer holds them")
	}
	return rows.Err()
}

// Version reports the highest migration applied to this database.
func (s *Store) Version(ctx context.Context) (int, error) {
	var v sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&v)
	if err != nil {
		return 0, err
	}
	return int(v.Int64), nil
}

// withTx runs fn inside a transaction, rolling back on error.
func (s *Store) withTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// ---------------------------------------------------------------- time helpers

const timeLayout = time.RFC3339Nano

// nowString is the canonical timestamp format: UTC, RFC 3339, so timestamps
// sort lexically in SQL and survive a move between machines.
func nowString() string { return time.Now().UTC().Format(timeLayout) }

func formatTime(t time.Time) string { return t.UTC().Format(timeLayout) }

func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(timeLayout, s)
	if err != nil {
		// Tolerate timestamps written by an older or hand-edited build rather
		// than failing to load a game over a formatting detail.
		if t2, err2 := time.Parse(time.RFC3339, s); err2 == nil {
			return t2, nil
		}
		return time.Time{}, err
	}
	return t, nil
}

func parseNullTime(ns sql.NullString) *time.Time {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	t, err := parseTime(ns.String)
	if err != nil {
		return nil
	}
	return &t
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
