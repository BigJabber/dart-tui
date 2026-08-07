package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/BigJabber/dart-tui/internal/domain/cricket"
	"github.com/BigJabber/dart-tui/internal/domain/player"
	"github.com/BigJabber/dart-tui/internal/domain/x01"
)

func newStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	// A real file rather than :memory: so the tests exercise the same WAL and
	// pragma setup the application uses on a Windows desktop.
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, context.Background()
}

func mustPlayer(t *testing.T, s *Store, ctx context.Context, name string) player.Player {
	t.Helper()
	p, err := s.CreatePlayer(ctx, name)
	if err != nil {
		t.Fatalf("CreatePlayer(%q): %v", name, err)
	}
	return p
}

// ------------------------------------------------------------------ schema

func TestMigrationsApplyAndAreIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	ctx := context.Background()
	v, err := s.Version(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v != SchemaVersion() {
		t.Errorf("schema version = %d, want %d", v, SchemaVersion())
	}
	p := mustPlayer(t, s, ctx, "Alice")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopening must not re-run migrations or lose data.
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer s2.Close()

	got, err := s2.GetPlayer(ctx, p.ID)
	if err != nil {
		t.Fatalf("player did not survive reopen: %v", err)
	}
	if got.Name != "Alice" {
		t.Errorf("name = %q, want Alice", got.Name)
	}
	v2, _ := s2.Version(ctx)
	if v2 != SchemaVersion() {
		t.Errorf("version after reopen = %d, want %d", v2, SchemaVersion())
	}
}

func TestForeignKeysAreEnforced(t *testing.T) {
	s, ctx := newStore(t)
	_, err := s.DB().ExecContext(ctx,
		`INSERT INTO game_participants (game_id, player_id, position) VALUES (999, 999, 0)`)
	if err == nil {
		t.Fatal("inserting a participant for a non-existent game should fail")
	}
}

// ----------------------------------------------------------------- players

func TestCreatePlayerAssignsDistinctColours(t *testing.T) {
	s, ctx := newStore(t)

	seen := map[string]bool{}
	for _, name := range []string{"Alice", "Bob", "Carol", "Dave"} {
		p := mustPlayer(t, s, ctx, name)
		if p.ID == 0 {
			t.Fatalf("%s got no ID", name)
		}
		if _, err := player.ValidateColour(p.Colour); err != nil {
			t.Errorf("%s got an invalid colour %q", name, p.Colour)
		}
		if seen[p.Colour] {
			t.Errorf("%s reused colour %q while unused colours remained", name, p.Colour)
		}
		seen[p.Colour] = true
		if p.CreatedAt.IsZero() {
			t.Errorf("%s has no created date", name)
		}
		if p.Archived() {
			t.Errorf("%s should not start archived", name)
		}
	}
}

func TestCreatePlayerNormalisesAndValidatesName(t *testing.T) {
	s, ctx := newStore(t)

	p, err := s.CreatePlayer(ctx, "  Bob   Smith  ")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "Bob Smith" {
		t.Errorf("name = %q, want %q", p.Name, "Bob Smith")
	}
	if _, err := s.CreatePlayer(ctx, "   "); !errors.Is(err, player.ErrEmptyName) {
		t.Errorf("blank name: got %v, want ErrEmptyName", err)
	}
}

func TestDuplicateNamesRejectedCaseInsensitively(t *testing.T) {
	s, ctx := newStore(t)
	mustPlayer(t, s, ctx, "Alice")

	if _, err := s.CreatePlayer(ctx, "Alice"); !errors.Is(err, ErrDuplicateName) {
		t.Errorf("exact duplicate: got %v, want ErrDuplicateName", err)
	}
	if _, err := s.CreatePlayer(ctx, "alice"); !errors.Is(err, ErrDuplicateName) {
		t.Errorf("case-variant duplicate: got %v, want ErrDuplicateName", err)
	}
}

func TestListPlayersExcludesArchivedByDefault(t *testing.T) {
	s, ctx := newStore(t)
	alice := mustPlayer(t, s, ctx, "Alice")
	mustPlayer(t, s, ctx, "Bob")

	if err := s.SetPlayerArchived(ctx, alice.ID, true); err != nil {
		t.Fatal(err)
	}

	active, err := s.ListPlayers(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].Name != "Bob" {
		t.Errorf("active players = %+v, want just Bob", active)
	}

	all, err := s.ListPlayers(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("all players = %d, want 2", len(all))
	}
	// Archiving is not deletion: the record and its date are still there.
	got, err := s.GetPlayer(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Archived() {
		t.Error("Alice should be archived")
	}

	// And she can be brought back.
	if err := s.SetPlayerArchived(ctx, alice.ID, false); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetPlayer(ctx, alice.ID); got.Archived() {
		t.Error("Alice should be active again")
	}
}

func TestRenameAndRecolourPlayer(t *testing.T) {
	s, ctx := newStore(t)
	p := mustPlayer(t, s, ctx, "Alice")

	if err := s.RenamePlayer(ctx, p.ID, "Alicia"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPlayerColour(ctx, p.ID, "#123abc"); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetPlayer(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Alicia" {
		t.Errorf("name = %q, want Alicia", got.Name)
	}
	if got.Colour != "#123ABC" {
		t.Errorf("colour = %q, want #123ABC", got.Colour)
	}

	if err := s.SetPlayerColour(ctx, p.ID, "purple"); err == nil {
		t.Error("an invalid colour should be rejected")
	}
	if err := s.RenamePlayer(ctx, p.ID, ""); err == nil {
		t.Error("an empty name should be rejected")
	}
}

func TestRenameToExistingNameRejected(t *testing.T) {
	s, ctx := newStore(t)
	mustPlayer(t, s, ctx, "Alice")
	bob := mustPlayer(t, s, ctx, "Bob")

	if err := s.RenamePlayer(ctx, bob.ID, "Alice"); !errors.Is(err, ErrDuplicateName) {
		t.Errorf("got %v, want ErrDuplicateName", err)
	}
}

func TestMissingPlayerReportsNotFound(t *testing.T) {
	s, ctx := newStore(t)
	if _, err := s.GetPlayer(ctx, 404); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetPlayer: got %v, want ErrNotFound", err)
	}
	if err := s.RenamePlayer(ctx, 404, "Ghost"); !errors.Is(err, ErrNotFound) {
		t.Errorf("RenamePlayer: got %v, want ErrNotFound", err)
	}
	if err := s.SetPlayerArchived(ctx, 404, true); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetPlayerArchived: got %v, want ErrNotFound", err)
	}
}

func TestPlayersByID(t *testing.T) {
	s, ctx := newStore(t)
	a := mustPlayer(t, s, ctx, "Alice")
	b := mustPlayer(t, s, ctx, "Bob")

	got, err := s.PlayersByID(ctx, []int64{a.ID, b.ID, 999})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("got %d players, want 2", len(got))
	}
	if got[a.ID].Name != "Alice" || got[b.ID].Name != "Bob" {
		t.Errorf("wrong players returned: %+v", got)
	}

	empty, err := s.PlayersByID(ctx, nil)
	if err != nil || len(empty) != 0 {
		t.Errorf("empty lookup = %v, %v", empty, err)
	}
}

// --------------------------------------------------------------- X01 games

// The headline persistence requirement: an in-progress game survives an exit.
func TestX01GameSavesAndResumes(t *testing.T) {
	s, ctx := newStore(t)
	alice := mustPlayer(t, s, ctx, "Alice")
	bob := mustPlayer(t, s, ctx, "Bob")

	cfg := x01.DefaultConfig(501)
	gameID, err := s.CreateX01Game(ctx, cfg, []int64{alice.ID, bob.ID})
	if err != nil {
		t.Fatal(err)
	}

	// Play a few visits, saving each one as the engine accepts it.
	live, err := x01.New(cfg, []int64{alice.ID, bob.ID})
	if err != nil {
		t.Fatal(err)
	}
	inputs := []x01.VisitInput{
		{PlayerID: alice.ID, Points: 140, Darts: 3},
		{PlayerID: alice.ID, Points: 100, Darts: 3},
		{PlayerID: bob.ID, Points: 180, Darts: 3},
		{PlayerID: alice.ID, Points: 60, Darts: 2}, // partial visit
	}
	for _, in := range inputs {
		v, err := live.Record(in)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.AppendX01Visit(ctx, gameID, v); err != nil {
			t.Fatal(err)
		}
	}

	// Simulate the application being closed and reopened.
	rec, err := s.LoadX01Game(ctx, gameID)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != StatusInProgress {
		t.Errorf("status = %q, want in_progress", rec.Status)
	}
	if rec.Config != cfg {
		t.Errorf("config = %+v, want %+v", rec.Config, cfg)
	}
	if len(rec.PlayerIDs) != 2 || rec.PlayerIDs[0] != alice.ID || rec.PlayerIDs[1] != bob.ID {
		t.Errorf("participants = %v, want [%d %d]", rec.PlayerIDs, alice.ID, bob.ID)
	}
	if len(rec.Visits) != len(inputs) {
		t.Fatalf("loaded %d visits, want %d", len(rec.Visits), len(inputs))
	}

	resumed, err := rec.Restore()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range live.States() {
		got, ok := resumed.State(want.PlayerID)
		if !ok {
			t.Fatalf("resumed game is missing player %d", want.PlayerID)
		}
		if got != want {
			t.Errorf("player %d: resumed %+v, want %+v", want.PlayerID, got, want)
		}
	}
	if resumed.Finished() {
		t.Error("the resumed game should still be in progress")
	}

	// And play can continue from where it stopped.
	if _, err := resumed.Record(x01.VisitInput{PlayerID: bob.ID, Points: 60, Darts: 3}); err != nil {
		t.Errorf("could not continue the resumed game: %v", err)
	}
}

func TestX01UndoRemovesTheStoredVisit(t *testing.T) {
	s, ctx := newStore(t)
	alice := mustPlayer(t, s, ctx, "Alice")

	cfg := x01.DefaultConfig(501)
	gameID, err := s.CreateX01Game(ctx, cfg, []int64{alice.ID})
	if err != nil {
		t.Fatal(err)
	}
	live, _ := x01.New(cfg, []int64{alice.ID})
	for _, pts := range []int{100, 140} {
		v, _ := live.Record(x01.VisitInput{PlayerID: alice.ID, Points: pts, Darts: 3})
		if err := s.AppendX01Visit(ctx, gameID, v); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := live.Undo(); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteLastX01Visit(ctx, gameID); err != nil {
		t.Fatal(err)
	}

	rec, err := s.LoadX01Game(ctx, gameID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Visits) != 1 {
		t.Fatalf("stored %d visits after undo, want 1", len(rec.Visits))
	}
	if rec.Visits[0].Points != 100 {
		t.Errorf("wrong visit survived: %+v", rec.Visits[0])
	}

	resumed, _ := rec.Restore()
	st, _ := resumed.State(alice.ID)
	if st.Score != 401 {
		t.Errorf("resumed score = %d, want 401", st.Score)
	}

	// A further undo on an empty game must not error at the storage layer.
	if err := s.DeleteLastX01Visit(ctx, gameID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteLastX01Visit(ctx, gameID); err != nil {
		t.Fatal(err)
	}
}

// Finishing stores every participant, including one who never threw.
func TestFinishX01GameStoresAllResults(t *testing.T) {
	s, ctx := newStore(t)
	alice := mustPlayer(t, s, ctx, "Alice")
	bob := mustPlayer(t, s, ctx, "Bob")
	carol := mustPlayer(t, s, ctx, "Carol")

	cfg := x01.DefaultConfig(501)
	gameID, err := s.CreateX01Game(ctx, cfg, []int64{alice.ID, bob.ID, carol.ID})
	if err != nil {
		t.Fatal(err)
	}
	live, _ := x01.New(cfg, []int64{alice.ID, bob.ID, carol.ID})

	// Alice completes in 28 darts; Bob throws once; Carol never throws.
	for i := 0; i < 9; i++ {
		v, _ := live.Record(x01.VisitInput{PlayerID: alice.ID, Points: 55, Darts: 3})
		if err := s.AppendX01Visit(ctx, gameID, v); err != nil {
			t.Fatal(err)
		}
	}
	v, err := live.Record(x01.VisitInput{PlayerID: alice.ID, Points: 6, Darts: 1, ConfirmCheckout: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AppendX01Visit(ctx, gameID, v); err != nil {
		t.Fatal(err)
	}
	v, _ = live.Record(x01.VisitInput{PlayerID: bob.ID, Points: 100, Darts: 3})
	if err := s.AppendX01Visit(ctx, gameID, v); err != nil {
		t.Fatal(err)
	}

	winner, ok := live.SuggestWinner()
	if !ok || winner != alice.ID {
		t.Fatalf("suggested winner = %d (%v), want Alice", winner, ok)
	}
	if err := live.Finish(&winner); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishX01Game(ctx, gameID, &winner, live.Results()); err != nil {
		t.Fatal(err)
	}

	legs, err := s.X01LegsForPlayer(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(legs) != 1 {
		t.Fatalf("Alice has %d legs, want 1", len(legs))
	}
	if !legs[0].Completed || !legs[0].Won || legs[0].Darts != 28 {
		t.Errorf("Alice's leg = %+v, want completed, won, 28 darts", legs[0])
	}

	bobLegs, _ := s.X01LegsForPlayer(ctx, bob.ID)
	if len(bobLegs) != 1 || bobLegs[0].Completed || bobLegs[0].Won {
		t.Errorf("Bob's leg = %+v, want an incomplete loss", bobLegs)
	}

	// Carol never threw but must still have an honest recorded result.
	carolLegs, _ := s.X01LegsForPlayer(ctx, carol.ID)
	if len(carolLegs) != 1 {
		t.Fatalf("Carol has %d legs, want 1", len(carolLegs))
	}
	if carolLegs[0].Completed || carolLegs[0].Darts != 0 || carolLegs[0].Visits != 0 {
		t.Errorf("Carol's leg = %+v, want zeroed and incomplete", carolLegs[0])
	}

	// The finished game reloads as finished, with its winner.
	rec, err := s.LoadX01Game(ctx, gameID)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != StatusFinished || rec.Winner == nil || *rec.Winner != alice.ID {
		t.Errorf("reloaded game = %s winner %v", rec.Status, rec.Winner)
	}
	if rec.FinishedAt == nil {
		t.Error("finished game should have a finish time")
	}
	restored, err := rec.Restore()
	if err != nil {
		t.Fatal(err)
	}
	if !restored.Finished() {
		t.Error("restored game should be finished")
	}
}

func TestFinishX01GameWithNoWinner(t *testing.T) {
	s, ctx := newStore(t)
	alice := mustPlayer(t, s, ctx, "Alice")
	cfg := x01.DefaultConfig(301)
	gameID, _ := s.CreateX01Game(ctx, cfg, []int64{alice.ID})
	live, _ := x01.New(cfg, []int64{alice.ID})
	v, _ := live.Record(x01.VisitInput{PlayerID: alice.ID, Points: 60, Darts: 3})
	_ = s.AppendX01Visit(ctx, gameID, v)
	_ = live.Finish(nil)

	if err := s.FinishX01Game(ctx, gameID, nil, live.Results()); err != nil {
		t.Fatal(err)
	}
	rec, err := s.LoadX01Game(ctx, gameID)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Winner != nil {
		t.Errorf("winner = %v, want nil", rec.Winner)
	}
}

func TestLoadX01GameRejectsWrongMode(t *testing.T) {
	s, ctx := newStore(t)
	alice := mustPlayer(t, s, ctx, "Alice")
	bob := mustPlayer(t, s, ctx, "Bob")
	gameID, err := s.CreateCricketGame(ctx,
		cricket.Config{WicketLimit: 3, FirstBatting: cricket.TeamA},
		cricket.Team{Side: cricket.TeamA, Name: "A", Players: []int64{alice.ID}},
		cricket.Team{Side: cricket.TeamB, Name: "B", Players: []int64{bob.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadX01Game(ctx, gameID); !errors.Is(err, ErrWrongMode) {
		t.Errorf("got %v, want ErrWrongMode", err)
	}
	if _, err := s.LoadX01Game(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

// ----------------------------------------------------------- cricket games

func TestCricketGameSavesAndResumes(t *testing.T) {
	s, ctx := newStore(t)
	alice := mustPlayer(t, s, ctx, "Alice")
	bob := mustPlayer(t, s, ctx, "Bob")
	charlie := mustPlayer(t, s, ctx, "Charlie")
	david := mustPlayer(t, s, ctx, "David")
	eve := mustPlayer(t, s, ctx, "Eve")

	teamA := cricket.Team{Side: cricket.TeamA, Name: "Sales", Players: []int64{alice.ID, bob.ID}}
	teamB := cricket.Team{Side: cricket.TeamB, Name: "Warehouse", Players: []int64{charlie.ID, david.ID, eve.ID}}
	cfg := cricket.Config{WicketLimit: 4, FirstBatting: cricket.TeamA}

	gameID, err := s.CreateCricketGame(ctx, cfg, teamA, teamB)
	if err != nil {
		t.Fatal(err)
	}

	live, err := cricket.New(cfg, teamA, teamB)
	if err != nil {
		t.Fatal(err)
	}
	play := func(raw int, notation string) {
		t.Helper()
		turn, ok := live.CurrentTurn()
		if !ok {
			t.Fatal("game already complete")
		}
		var e cricket.Event
		var err error
		if turn.Kind == cricket.Batting {
			e, err = live.RecordBatting(turn.PlayerID, raw, 3)
		} else {
			bowls, perr := cricket.ParseBowls(notation)
			if perr != nil {
				t.Fatal(perr)
			}
			e, err = live.RecordBowling(turn.PlayerID, bowls)
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := s.AppendCricketEvent(ctx, gameID, e); err != nil {
			t.Fatal(err)
		}
	}

	play(100, "")  // Alice bats: 60 runs
	play(0, "OMM") // Charlie bowls: 1 wicket
	play(57, "")   // Bob bats: 17 runs
	play(0, "MBM") // David bowls: 2 wickets, 3 total
	play(45, "")   // Alice bats: 5 runs

	rec, err := s.LoadCricketGame(ctx, gameID)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Config != cfg {
		t.Errorf("config = %+v, want %+v", rec.Config, cfg)
	}
	if rec.TeamA.Name != "Sales" || rec.TeamB.Name != "Warehouse" {
		t.Errorf("team names = %q / %q", rec.TeamA.Name, rec.TeamB.Name)
	}
	// Player order within each team must survive the round trip exactly.
	if len(rec.TeamA.Players) != 2 || rec.TeamA.Players[0] != alice.ID || rec.TeamA.Players[1] != bob.ID {
		t.Errorf("team A order = %v, want [%d %d]", rec.TeamA.Players, alice.ID, bob.ID)
	}
	if len(rec.TeamB.Players) != 3 || rec.TeamB.Players[0] != charlie.ID || rec.TeamB.Players[2] != eve.ID {
		t.Errorf("team B order = %v", rec.TeamB.Players)
	}
	if len(rec.Events) != 5 {
		t.Fatalf("loaded %d events, want 5", len(rec.Events))
	}

	resumed, err := rec.Restore()
	if err != nil {
		t.Fatal(err)
	}
	if resumed.CurrentInnings() != live.CurrentInnings() {
		t.Errorf("resumed innings = %+v, want %+v", resumed.CurrentInnings(), live.CurrentInnings())
	}
	gotTurn, ok1 := resumed.CurrentTurn()
	wantTurn, ok2 := live.CurrentTurn()
	if ok1 != ok2 || gotTurn != wantTurn {
		t.Errorf("resumed turn = %+v (%v), want %+v (%v)", gotTurn, ok1, wantTurn, ok2)
	}

	// The individual bowling darts survive, so bull types remain distinguishable.
	charlieStats := resumed.StatsFor(charlie.ID)
	if charlieStats.OuterBulls != 1 || charlieStats.InnerBulls != 0 {
		t.Errorf("Charlie's bulls = %d outer / %d inner, want 1 / 0",
			charlieStats.OuterBulls, charlieStats.InnerBulls)
	}
	davidStats := resumed.StatsFor(david.ID)
	if davidStats.InnerBulls != 1 || davidStats.OuterBulls != 0 {
		t.Errorf("David's bulls = %d outer / %d inner, want 0 / 1",
			davidStats.OuterBulls, davidStats.InnerBulls)
	}
}

func TestCricketUndoRemovesStoredEventAndItsDarts(t *testing.T) {
	s, ctx := newStore(t)
	alice := mustPlayer(t, s, ctx, "Alice")
	bob := mustPlayer(t, s, ctx, "Bob")

	teamA := cricket.Team{Side: cricket.TeamA, Players: []int64{alice.ID}}
	teamB := cricket.Team{Side: cricket.TeamB, Players: []int64{bob.ID}}
	cfg := cricket.Config{WicketLimit: 5, FirstBatting: cricket.TeamA}

	gameID, _ := s.CreateCricketGame(ctx, cfg, teamA, teamB)
	live, _ := cricket.New(cfg, teamA, teamB)

	e, _ := live.RecordBatting(alice.ID, 100, 3)
	_ = s.AppendCricketEvent(ctx, gameID, e)
	bowls, _ := cricket.ParseBowls("OMB")
	e, _ = live.RecordBowling(bob.ID, bowls)
	_ = s.AppendCricketEvent(ctx, gameID, e)

	var dartCount int
	if err := s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM bowling_darts`).Scan(&dartCount); err != nil {
		t.Fatal(err)
	}
	if dartCount != 3 {
		t.Fatalf("stored %d bowling darts, want 3", dartCount)
	}

	if _, err := live.Undo(); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteLastCricketEvent(ctx, gameID); err != nil {
		t.Fatal(err)
	}

	if err := s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM bowling_darts`).Scan(&dartCount); err != nil {
		t.Fatal(err)
	}
	if dartCount != 0 {
		t.Errorf("bowling darts left after undo = %d, want 0 (cascade)", dartCount)
	}

	rec, _ := s.LoadCricketGame(ctx, gameID)
	if len(rec.Events) != 1 {
		t.Errorf("events after undo = %d, want 1", len(rec.Events))
	}
}

func TestFinishCricketGame(t *testing.T) {
	s, ctx := newStore(t)
	alice := mustPlayer(t, s, ctx, "Alice")
	bob := mustPlayer(t, s, ctx, "Bob")

	teamA := cricket.Team{Side: cricket.TeamA, Players: []int64{alice.ID}}
	teamB := cricket.Team{Side: cricket.TeamB, Players: []int64{bob.ID}}
	cfg := cricket.Config{WicketLimit: 2, FirstBatting: cricket.TeamA}

	gameID, _ := s.CreateCricketGame(ctx, cfg, teamA, teamB)
	live, _ := cricket.New(cfg, teamA, teamB)

	// Play a complete two-leg match, saving every visit as it happens.
	playInnings := func(raw int) {
		t.Helper()
		turn, ok := live.CurrentTurn()
		if !ok {
			return
		}
		e, err := live.RecordBatting(turn.PlayerID, raw, 3)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.AppendCricketEvent(ctx, gameID, e); err != nil {
			t.Fatal(err)
		}
		number := live.InningsNumber()
		for !live.Complete() && live.InningsNumber() == number {
			turn, ok := live.CurrentTurn()
			if !ok {
				return
			}
			if turn.Kind == cricket.Batting {
				e, err = live.RecordBatting(turn.PlayerID, 0, 3)
			} else {
				e, err = live.RecordBowling(turn.PlayerID, []cricket.Bowl{cricket.Inner})
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := s.AppendCricketEvent(ctx, gameID, e); err != nil {
				t.Fatal(err)
			}
		}
	}

	playInnings(60)  // Team A: 20
	playInnings(100) // Team B: 60, and lead
	playInnings(60)  // Team A bowled out on 40 — Team B win without batting again

	if !live.Complete() || live.Result() != cricket.ResultTeamB {
		t.Fatalf("precondition: complete=%v result=%v", live.Complete(), live.Result())
	}

	entries := []CricketResultEntry{
		{PlayerID: alice.ID, TeamSide: "A", Won: false, Darts: 6, Visits: 2, Points: 120},
		{PlayerID: bob.ID, TeamSide: "B", Won: true, Darts: 6, Visits: 2, Points: 100},
	}
	if err := s.FinishCricketGame(ctx, gameID, live.Result(), entries); err != nil {
		t.Fatal(err)
	}

	rec, err := s.LoadCricketGame(ctx, gameID)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != StatusFinished {
		t.Errorf("status = %q, want finished", rec.Status)
	}
	if rec.StoredResult != cricket.ResultTeamB {
		t.Errorf("stored result = %v, want ResultTeamB", rec.StoredResult)
	}

	// The whole two-leg match replays from storage, including the innings that
	// was never needed.
	restored, err := rec.Restore()
	if err != nil {
		t.Fatal(err)
	}
	if got := len(restored.Innings()); got != 3 {
		t.Errorf("restored innings = %d, want 3", got)
	}
	if restored.Aggregate(cricket.TeamA) != 40 || restored.Aggregate(cricket.TeamB) != 60 {
		t.Errorf("restored aggregates = %d v %d, want 40 v 60",
			restored.Aggregate(cricket.TeamA), restored.Aggregate(cricket.TeamB))
	}

	bobEntries, _ := s.CricketEntriesForPlayer(ctx, bob.ID)
	if len(bobEntries) != 1 || !bobEntries[0].Won {
		t.Errorf("Bob's cricket entries = %+v, want one win", bobEntries)
	}
	aliceEntries, _ := s.CricketEntriesForPlayer(ctx, alice.ID)
	if len(aliceEntries) != 1 || aliceEntries[0].Won {
		t.Errorf("Alice's cricket entries = %+v, want one loss", aliceEntries)
	}
}

// An abandoned game cannot be re-derived from its events, so the stored result
// has to be reapplied on load.
func TestAbandonedCricketGameRestoresAsComplete(t *testing.T) {
	s, ctx := newStore(t)
	alice := mustPlayer(t, s, ctx, "Alice")
	bob := mustPlayer(t, s, ctx, "Bob")

	teamA := cricket.Team{Side: cricket.TeamA, Players: []int64{alice.ID}}
	teamB := cricket.Team{Side: cricket.TeamB, Players: []int64{bob.ID}}
	cfg := cricket.Config{WicketLimit: 5, FirstBatting: cricket.TeamA}

	gameID, _ := s.CreateCricketGame(ctx, cfg, teamA, teamB)
	live, _ := cricket.New(cfg, teamA, teamB)
	e, _ := live.RecordBatting(alice.ID, 100, 3)
	_ = s.AppendCricketEvent(ctx, gameID, e)
	_ = live.Abandon()

	if err := s.FinishCricketGame(ctx, gameID, cricket.ResultAbandoned, nil); err != nil {
		t.Fatal(err)
	}

	rec, err := s.LoadCricketGame(ctx, gameID)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := rec.Restore()
	if err != nil {
		t.Fatal(err)
	}
	if !restored.Complete() {
		t.Error("an abandoned game must restore as complete")
	}
	if restored.Result() != cricket.ResultAbandoned {
		t.Errorf("result = %v, want ResultAbandoned", restored.Result())
	}
}

// ----------------------------------------------------------------- listing

func TestListInProgressAndRecentGames(t *testing.T) {
	s, ctx := newStore(t)
	alice := mustPlayer(t, s, ctx, "Alice")
	bob := mustPlayer(t, s, ctx, "Bob")

	cfg := x01.DefaultConfig(501)
	openGame, _ := s.CreateX01Game(ctx, cfg, []int64{alice.ID, bob.ID})
	live, _ := x01.New(cfg, []int64{alice.ID, bob.ID})
	v, _ := live.Record(x01.VisitInput{PlayerID: alice.ID, Points: 100, Darts: 3})
	_ = s.AppendX01Visit(ctx, openGame, v)

	doneGame, _ := s.CreateX01Game(ctx, cfg, []int64{alice.ID})
	done, _ := x01.New(cfg, []int64{alice.ID})
	_ = done.Finish(nil)
	_ = s.FinishX01Game(ctx, doneGame, nil, done.Results())

	inProgress, err := s.ListInProgressGames(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(inProgress) != 1 {
		t.Fatalf("in-progress games = %d, want 1", len(inProgress))
	}
	g := inProgress[0]
	if g.ID != openGame {
		t.Errorf("resumable game = %d, want %d", g.ID, openGame)
	}
	if !g.InProgress() {
		t.Error("game should report itself in progress")
	}
	if g.StartScore != 501 || !g.DoubleOut {
		t.Errorf("config not summarised: %+v", g)
	}
	if g.VisitCount != 1 {
		t.Errorf("visit count = %d, want 1", g.VisitCount)
	}
	if len(g.ParticipantIDs) != 2 {
		t.Errorf("participants = %v, want 2", g.ParticipantIDs)
	}

	recent, err := s.ListRecentGames(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 1 || recent[0].ID != doneGame {
		t.Errorf("recent games = %+v, want just the finished game", recent)
	}

	forAlice, err := s.ListRecentGamesForPlayer(ctx, alice.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(forAlice) != 1 {
		t.Errorf("Alice's recent games = %d, want 1", len(forAlice))
	}
	forBob, err := s.ListRecentGamesForPlayer(ctx, bob.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(forBob) != 0 {
		t.Errorf("Bob's recent games = %d, want 0 (his only game is unfinished)", len(forBob))
	}
}

func TestDeleteGameRemovesItsVisits(t *testing.T) {
	s, ctx := newStore(t)
	alice := mustPlayer(t, s, ctx, "Alice")
	cfg := x01.DefaultConfig(501)
	gameID, _ := s.CreateX01Game(ctx, cfg, []int64{alice.ID})
	live, _ := x01.New(cfg, []int64{alice.ID})
	v, _ := live.Record(x01.VisitInput{PlayerID: alice.ID, Points: 100, Darts: 3})
	_ = s.AppendX01Visit(ctx, gameID, v)

	if err := s.DeleteGame(ctx, gameID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM x01_visits`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("visits left after deleting the game = %d, want 0", n)
	}
	if err := s.DeleteGame(ctx, gameID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting twice: got %v, want ErrNotFound", err)
	}
	// The player is untouched: deleting a game is not deleting people.
	if _, err := s.GetPlayer(ctx, alice.ID); err != nil {
		t.Errorf("player should survive game deletion: %v", err)
	}
}

// -------------------------------------------------------------- statistics

func TestPlayerStatsDerivedFromStoredVisits(t *testing.T) {
	s, ctx := newStore(t)
	alice := mustPlayer(t, s, ctx, "Alice")
	bob := mustPlayer(t, s, ctx, "Bob")

	// A finished 501 game Alice wins in 28 darts.
	cfg := x01.DefaultConfig(501)
	gameID, _ := s.CreateX01Game(ctx, cfg, []int64{alice.ID, bob.ID})
	live, _ := x01.New(cfg, []int64{alice.ID, bob.ID})
	for i := 0; i < 9; i++ {
		v, _ := live.Record(x01.VisitInput{PlayerID: alice.ID, Points: 55, Darts: 3})
		_ = s.AppendX01Visit(ctx, gameID, v)
	}
	v, _ := live.Record(x01.VisitInput{PlayerID: alice.ID, Points: 6, Darts: 1, ConfirmCheckout: true})
	_ = s.AppendX01Visit(ctx, gameID, v)
	v, _ = live.Record(x01.VisitInput{PlayerID: bob.ID, Points: 180, Darts: 3})
	_ = s.AppendX01Visit(ctx, gameID, v)

	winner := alice.ID
	_ = live.Finish(&winner)
	_ = s.FinishX01Game(ctx, gameID, &winner, live.Results())

	got, err := s.PlayerStats(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.X01.GamesPlayed != 1 || got.X01.GamesWon != 1 {
		t.Errorf("Alice played/won = %d/%d, want 1/1", got.X01.GamesPlayed, got.X01.GamesWon)
	}
	if got.X01.LegsCompleted != 1 {
		t.Errorf("legs completed = %d, want 1", got.X01.LegsCompleted)
	}
	if got.X01.Darts != 28 {
		t.Errorf("darts = %d, want 28", got.X01.Darts)
	}
	if got.X01.Points != 501 {
		t.Errorf("points = %d, want 501", got.X01.Points)
	}
	if fewest, ok := got.X01.FewestDarts(501); !ok || fewest != 28 {
		t.Errorf("fewest 501 darts = %d (%v), want 28", fewest, ok)
	}

	bobStats, err := s.PlayerStats(ctx, bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	if bobStats.X01.OneEighties != 1 {
		t.Errorf("Bob's 180s = %d, want 1", bobStats.X01.OneEighties)
	}
	if bobStats.X01.GamesWon != 0 {
		t.Errorf("Bob's wins = %d, want 0", bobStats.X01.GamesWon)
	}
	if bobStats.X01.HighestVisit != 180 {
		t.Errorf("Bob's highest visit = %d, want 180", bobStats.X01.HighestVisit)
	}
}

// Visits from an unfinished game must not leak into lifetime figures, or the
// dart total would disagree with the games-played count.
func TestInProgressGamesExcludedFromStats(t *testing.T) {
	s, ctx := newStore(t)
	alice := mustPlayer(t, s, ctx, "Alice")

	cfg := x01.DefaultConfig(501)
	gameID, _ := s.CreateX01Game(ctx, cfg, []int64{alice.ID})
	live, _ := x01.New(cfg, []int64{alice.ID})
	v, _ := live.Record(x01.VisitInput{PlayerID: alice.ID, Points: 180, Darts: 3})
	_ = s.AppendX01Visit(ctx, gameID, v)

	got, err := s.PlayerStats(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.X01.Darts != 0 || got.X01.OneEighties != 0 || got.X01.GamesPlayed != 0 {
		t.Errorf("unfinished game leaked into stats: %+v", got.X01)
	}
}

func TestCricketStatsDerivedFromStoredEvents(t *testing.T) {
	s, ctx := newStore(t)
	alice := mustPlayer(t, s, ctx, "Alice")
	bob := mustPlayer(t, s, ctx, "Bob")

	teamA := cricket.Team{Side: cricket.TeamA, Players: []int64{alice.ID}}
	teamB := cricket.Team{Side: cricket.TeamB, Players: []int64{bob.ID}}
	cfg := cricket.Config{WicketLimit: 1, FirstBatting: cricket.TeamA}

	gameID, _ := s.CreateCricketGame(ctx, cfg, teamA, teamB)
	live, _ := cricket.New(cfg, teamA, teamB)

	e, _ := live.RecordBatting(alice.ID, 100, 3)
	_ = s.AppendCricketEvent(ctx, gameID, e)
	bowls, _ := cricket.ParseBowls("O")
	e, _ = live.RecordBowling(bob.ID, bowls)
	_ = s.AppendCricketEvent(ctx, gameID, e)
	e, _ = live.RecordBatting(bob.ID, 140, 3)
	_ = s.AppendCricketEvent(ctx, gameID, e)

	_ = s.FinishCricketGame(ctx, gameID, live.Result(), []CricketResultEntry{
		{PlayerID: alice.ID, TeamSide: "A", Won: false},
		{PlayerID: bob.ID, TeamSide: "B", Won: true},
	})

	aliceStats, err := s.PlayerStats(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	b := aliceStats.Cricket.Batting
	if b.Visits != 1 || b.RawPoints != 100 || b.Runs != 60 {
		t.Errorf("Alice's batting = %+v, want 1 visit, 100 raw, 60 runs", b)
	}
	if aliceStats.Cricket.Bowling.Visits != 0 {
		t.Errorf("Alice bowled %d visits, want 0", aliceStats.Cricket.Bowling.Visits)
	}
	// X01 figures must stay empty: the two modes never mix.
	if aliceStats.X01.GamesPlayed != 0 || aliceStats.X01.Darts != 0 {
		t.Errorf("cricket leaked into X01 stats: %+v", aliceStats.X01)
	}

	bobStats, _ := s.PlayerStats(ctx, bob.ID)
	if bobStats.Cricket.Bowling.Wickets != 1 || bobStats.Cricket.Bowling.OuterBulls != 1 {
		t.Errorf("Bob's bowling = %+v, want 1 wicket from an outer bull", bobStats.Cricket.Bowling)
	}
	if bobStats.Cricket.GamesWon != 1 {
		t.Errorf("Bob's cricket wins = %d, want 1", bobStats.Cricket.GamesWon)
	}
	if bobStats.Cricket.Batting.HighestVisitRuns != 100 {
		t.Errorf("Bob's highest runs = %d, want 100", bobStats.Cricket.Batting.HighestVisitRuns)
	}
}

func TestPlayerStatsForNewPlayerIsEmpty(t *testing.T) {
	s, ctx := newStore(t)
	p := mustPlayer(t, s, ctx, "Newcomer")

	got, err := s.PlayerStats(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.TotalGamesPlayed() != 0 || got.X01.Average != 0 {
		t.Errorf("a new player should have empty stats, got %+v", got)
	}
}
