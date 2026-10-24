package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/BigJabber/dart-tui/internal/domain/x01"
)

// openAtVersion opens a database with only the migrations up to and including
// version n, so an upgrade can be exercised against a database as an earlier
// release would actually have left it.
func openAtVersion(t *testing.T, path string, n int) *Store {
	t.Helper()

	full := migrations
	t.Cleanup(func() { migrations = full })

	var upTo []migration
	for _, m := range full {
		if m.Version <= n {
			upTo = append(upTo, m)
		}
	}
	migrations = upTo

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open at version %d: %v", n, err)
	}
	migrations = full
	return s
}

// Adding Around the World meant rebuilding the games table, because its mode is
// constrained by a CHECK that SQLite cannot widen in place. Dropping a table
// that the whole history references is the one operation here that could
// silently destroy an office's records, so the upgrade is tested against a
// database with real games in it.
func TestUpgradeFromVersionOneKeepsEveryGame(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.db")
	ctx := context.Background()

	old := openAtVersion(t, path, 1)

	alice := mustPlayer(t, old, ctx, "Alice")
	bob := mustPlayer(t, old, ctx, "Bob")

	// A finished X01 game, with visits and a stored result.
	cfg := x01.DefaultConfig(101)
	finishedID, err := old.CreateX01Game(ctx, cfg, []int64{alice.ID, bob.ID})
	if err != nil {
		t.Fatal(err)
	}
	live, _ := x01.New(cfg, []int64{alice.ID, bob.ID})
	v, err := live.Record(x01.VisitInput{PlayerID: alice.ID, Points: 101, Darts: 3, ConfirmCheckout: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := old.AppendX01Visit(ctx, finishedID, v); err != nil {
		t.Fatal(err)
	}
	winner := alice.ID
	if err := live.Finish(&winner); err != nil {
		t.Fatal(err)
	}
	if err := old.FinishX01Game(ctx, finishedID, &winner, live.Results()); err != nil {
		t.Fatal(err)
	}

	// And a game still in progress, which must still be resumable afterwards.
	openID, err := old.CreateX01Game(ctx, cfg, []int64{bob.ID})
	if err != nil {
		t.Fatal(err)
	}
	open, _ := x01.New(cfg, []int64{bob.ID})
	v2, _ := open.Record(x01.VisitInput{PlayerID: bob.ID, Points: 60, Darts: 3})
	if err := old.AppendX01Visit(ctx, openID, v2); err != nil {
		t.Fatal(err)
	}

	if got, err := old.Version(ctx); err != nil || got != 1 {
		t.Fatalf("version = %d (%v), want 1", got, err)
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen with the full migration set: this is the upgrade.
	upgraded, err := Open(path)
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	defer func() { _ = upgraded.Close() }()

	if got, err := upgraded.Version(ctx); err != nil || got != SchemaVersion() {
		t.Fatalf("version after upgrade = %d (%v), want %d", got, err, SchemaVersion())
	}

	// The finished game, its participants, visits and result all survived.
	rec, err := upgraded.LoadX01Game(ctx, finishedID)
	if err != nil {
		t.Fatalf("load the finished game: %v", err)
	}
	if rec.Status != StatusFinished {
		t.Errorf("status = %q, want finished", rec.Status)
	}
	if rec.Winner == nil || *rec.Winner != alice.ID {
		t.Errorf("winner = %v, want Alice", rec.Winner)
	}
	if len(rec.PlayerIDs) != 2 {
		t.Errorf("participants = %v, want two", rec.PlayerIDs)
	}
	if len(rec.Visits) != 1 {
		t.Errorf("visits = %d, want 1", len(rec.Visits))
	}
	// Games from before auto-swap existed were played without it.
	if rec.AutoSwap {
		t.Error("an upgraded game came back with auto-swap on")
	}
	results, err := upgraded.GameResults(ctx, finishedID)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Errorf("stored results = %d, want 2", len(results))
	}

	// The in-progress game is still resumable.
	resumable, err := upgraded.ListInProgressGames(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(resumable) != 1 || resumable[0].ID != openID {
		t.Fatalf("resumable = %+v, want the one open game", resumable)
	}
	if resumable[0].VisitCount != 1 {
		t.Errorf("visit count = %d, want 1", resumable[0].VisitCount)
	}

	// Statistics computed from the migrated rows are unchanged.
	stats, err := upgraded.PlayerStats(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stats.X01.GamesWon != 1 {
		t.Errorf("Alice's X01 wins = %d, want 1", stats.X01.GamesWon)
	}

	// And the widened constraint now accepts the new mode.
	if _, err := upgraded.CreateATWGame(ctx, []int64{alice.ID, bob.ID}); err != nil {
		t.Errorf("create an Around the World game after upgrading: %v", err)
	}
}

// Foreign keys must be enforced again once the rebuild is done, or the next
// deletion would leave orphaned rows behind.
func TestForeignKeysAreEnforcedAfterTheRebuild(t *testing.T) {
	s, ctx := newStore(t)
	alice := mustPlayer(t, s, ctx, "Alice")

	gameID, err := s.CreateATWGame(ctx, []int64{alice.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteGame(ctx, gameID); err != nil {
		t.Fatal(err)
	}

	// The cascade only happens with enforcement on.
	var participants int
	if err := s.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM game_participants WHERE game_id = ?`, gameID).
		Scan(&participants); err != nil {
		t.Fatal(err)
	}
	if participants != 0 {
		t.Errorf("%d participant rows survived the deleted game — foreign keys are off", participants)
	}
}
