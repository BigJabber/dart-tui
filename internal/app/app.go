// Package app is the application layer the TUI talks to.
//
// It coordinates the domain engines and the store: it decides when a visit is
// persisted, keeps the in-memory engine and the database in step, and assembles
// the display-ready views the screens render. It contains no darts rules of its
// own — those live in internal/domain — and no SQL, which lives in the store.
package app

import (
	"context"

	"github.com/BigJabber/dart-tui/internal/persistence/sqlite"
)

// Services is the entry point the TUI is given.
type Services struct {
	Players *PlayerService
	Games   *GameService
	Stats   *StatsService

	store *sqlite.Store
}

// New wires the services onto a store.
func New(store *sqlite.Store) *Services {
	return &Services{
		Players: &PlayerService{store: store},
		Games:   &GameService{store: store},
		Stats:   &StatsService{store: store},
		store:   store,
	}
}

// Close releases the underlying database handle.
func (s *Services) Close() error { return s.store.Close() }

// Ping verifies the database is reachable, so a broken data directory is
// reported at start-up rather than on the first keystroke.
func (s *Services) Ping(ctx context.Context) error {
	_, err := s.store.Version(ctx)
	return err
}
