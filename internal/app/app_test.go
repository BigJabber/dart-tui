package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/BigJabber/dart-tui/internal/domain/atw"
	"github.com/BigJabber/dart-tui/internal/domain/cricket"
	"github.com/BigJabber/dart-tui/internal/domain/player"
	"github.com/BigJabber/dart-tui/internal/domain/x01"
	"github.com/BigJabber/dart-tui/internal/persistence/sqlite"
)

func newServices(t *testing.T) (*Services, context.Context) {
	t.Helper()
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	svc := New(store)
	t.Cleanup(func() { _ = svc.Close() })
	return svc, context.Background()
}

func mustCreate(t *testing.T, svc *Services, ctx context.Context, name string) player.Player {
	t.Helper()
	p, err := svc.Players.Create(ctx, name)
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	return p
}

func TestPingReportsAHealthyDatabase(t *testing.T) {
	svc, ctx := newServices(t)
	if err := svc.Ping(ctx); err != nil {
		t.Errorf("Ping: %v", err)
	}
}

// A game played through the session layer must be resumable from storage alone.
func TestX01SessionRecordUndoResumeFinish(t *testing.T) {
	svc, ctx := newServices(t)
	alice := mustCreate(t, svc, ctx, "Alice")
	bob := mustCreate(t, svc, ctx, "Bob")

	sess, err := svc.Games.StartX01(ctx, x01.DefaultConfig(501), []int64{alice.ID, bob.ID})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := sess.Record(ctx, x01.VisitInput{PlayerID: alice.ID, Points: 140, Darts: 3}); err != nil {
		t.Fatal(err)
	}
	if _, err := sess.Record(ctx, x01.VisitInput{PlayerID: bob.ID, Points: 100, Darts: 3}); err != nil {
		t.Fatal(err)
	}
	// A mistyped visit, corrected immediately.
	if _, err := sess.Record(ctx, x01.VisitInput{PlayerID: alice.ID, Points: 180, Darts: 3}); err != nil {
		t.Fatal(err)
	}
	if _, err := sess.Undo(ctx); err != nil {
		t.Fatal(err)
	}

	// Reopen the game as a fresh session, as a restart would.
	resumed, err := svc.Games.ResumeX01(ctx, sess.GameID())
	if err != nil {
		t.Fatal(err)
	}
	st, ok := resumed.Game().State(alice.ID)
	if !ok {
		t.Fatal("Alice missing from the resumed game")
	}
	if st.Score != 361 {
		t.Errorf("Alice's resumed score = %d, want 361 (the undone visit must not persist)", st.Score)
	}
	if resumed.Game().VisitCount() != 2 {
		t.Errorf("resumed visit count = %d, want 2", resumed.Game().VisitCount())
	}
	if got := resumed.Player(alice.ID); got.Name != "Alice" || got.Colour != alice.Colour {
		t.Errorf("resumed player details = %+v, want Alice's", got)
	}

	// Finish from the resumed session with no completed leg and no winner.
	if err := resumed.Finish(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if !resumed.Game().Finished() {
		t.Error("game should be finished")
	}

	// It is no longer resumable.
	open, err := svc.Games.Resumable(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 0 {
		t.Errorf("resumable games = %d, want 0", len(open))
	}
}

func TestX01SessionRejectsInvalidVisitWithoutStoringIt(t *testing.T) {
	svc, ctx := newServices(t)
	alice := mustCreate(t, svc, ctx, "Alice")

	sess, err := svc.Games.StartX01(ctx, x01.DefaultConfig(301), []int64{alice.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sess.Record(ctx, x01.VisitInput{PlayerID: alice.ID, Points: 179, Darts: 3}); err == nil {
		t.Fatal("an impossible score should be rejected")
	}
	resumed, err := svc.Games.ResumeX01(ctx, sess.GameID())
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Game().VisitCount() != 0 {
		t.Errorf("stored %d visits, want 0", resumed.Game().VisitCount())
	}
}

func TestResumableListsInProgressGames(t *testing.T) {
	svc, ctx := newServices(t)
	alice := mustCreate(t, svc, ctx, "Alice")
	bob := mustCreate(t, svc, ctx, "Bob")

	sess, err := svc.Games.StartX01(ctx, x01.DefaultConfig(501), []int64{alice.ID, bob.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sess.Record(ctx, x01.VisitInput{PlayerID: alice.ID, Points: 60, Darts: 3}); err != nil {
		t.Fatal(err)
	}

	open, err := svc.Games.Resumable(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 1 {
		t.Fatalf("resumable games = %d, want 1", len(open))
	}
	g := open[0]
	if g.Label != "501" {
		t.Errorf("label = %q, want 501", g.Label)
	}
	if g.VisitCount != 1 {
		t.Errorf("visit count = %d, want 1", g.VisitCount)
	}
	if len(g.Players) != 2 || g.Players[0].Name != "Alice" {
		t.Errorf("players = %+v", g.Players)
	}

	// Discarding removes it from the list.
	if err := svc.Games.Discard(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	open, _ = svc.Games.Resumable(ctx)
	if len(open) != 0 {
		t.Errorf("resumable games after discard = %d, want 0", len(open))
	}
}

// playCricketInnings plays one complete innings through the session: a single
// scoring visit, then the batting side is bowled out.
func playCricketInnings(t *testing.T, ctx context.Context, sess *CricketSession, raw int) {
	t.Helper()
	turn, ok := sess.Game().CurrentTurn()
	if !ok {
		t.Fatal("the match is already complete")
	}
	if _, err := sess.RecordBatting(ctx, turn.PlayerID, raw, 3); err != nil {
		t.Fatal(err)
	}
	number := sess.Game().InningsNumber()
	for !sess.Game().Complete() && sess.Game().InningsNumber() == number {
		turn, ok := sess.Game().CurrentTurn()
		if !ok {
			return
		}
		if turn.Kind == cricket.Batting {
			if _, err := sess.RecordBatting(ctx, turn.PlayerID, 0, 3); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if _, err := sess.RecordBowling(ctx, turn.PlayerID, []cricket.Bowl{cricket.Inner}); err != nil {
			t.Fatal(err)
		}
	}
}

// startCricket opens a two-player, two-team match with a short innings.
func startCricket(t *testing.T, ctx context.Context, svc *Services, a, b player.Player) *CricketSession {
	t.Helper()
	sess, err := svc.Games.StartCricket(ctx,
		cricket.Config{WicketLimit: 2, FirstBatting: cricket.TeamA},
		cricket.Team{Players: []int64{a.ID}},
		cricket.Team{Players: []int64{b.ID}})
	if err != nil {
		t.Fatal(err)
	}
	return sess
}

// Cricket runs to two legs and ends itself, and the session must record that
// result at the moment it happens.
func TestCricketSessionAutoRecordsResult(t *testing.T) {
	svc, ctx := newServices(t)
	alice := mustCreate(t, svc, ctx, "Alice")
	bob := mustCreate(t, svc, ctx, "Bob")

	sess := startCricket(t, ctx, svc, alice, bob)

	playCricketInnings(t, ctx, sess, 60)  // Team A: 20
	playCricketInnings(t, ctx, sess, 100) // Team B: 60, and lead

	if sess.Game().Leg() != 2 || sess.Game().InningsNumber() != 3 {
		t.Fatalf("after leg 1: leg %d innings %d, want leg 2 innings 3",
			sess.Game().Leg(), sess.Game().InningsNumber())
	}
	// Team A trails, so they bat first in leg 2.
	if got := sess.Game().CurrentInnings().BattingSide; got != cricket.TeamA {
		t.Fatalf("leg 2 opens with %v, want the trailing TeamA", got)
	}

	playCricketInnings(t, ctx, sess, 60) // Team A: 40 aggregate, short of 60

	if !sess.Game().Complete() {
		t.Fatal("Team B should have won without batting again")
	}
	if got := len(sess.Game().Innings()); got != 3 {
		t.Errorf("innings played = %d, want 3", got)
	}

	// The result is stored without any further action from the operator.
	bobStats, err := svc.Stats.ForPlayer(ctx, bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	if bobStats.Cricket.GamesWon != 1 {
		t.Errorf("Bob's cricket wins = %d, want 1", bobStats.Cricket.GamesWon)
	}
	aliceStats, _ := svc.Stats.ForPlayer(ctx, alice.ID)
	if aliceStats.Cricket.GamesPlayed != 1 || aliceStats.Cricket.GamesWon != 0 {
		t.Errorf("Alice = %d played / %d won, want 1/0",
			aliceStats.Cricket.GamesPlayed, aliceStats.Cricket.GamesWon)
	}
}

// Undoing the visit that decided a match must reopen the stored game too,
// otherwise the database would keep claiming a result that no longer exists.
func TestCricketUndoReopensAFinishedGame(t *testing.T) {
	svc, ctx := newServices(t)
	alice := mustCreate(t, svc, ctx, "Alice")
	bob := mustCreate(t, svc, ctx, "Bob")

	sess := startCricket(t, ctx, svc, alice, bob)
	playCricketInnings(t, ctx, sess, 60)  // Team A: 20
	playCricketInnings(t, ctx, sess, 100) // Team B: 60
	playCricketInnings(t, ctx, sess, 60)  // Team A bowled out short — Team B win

	if !sess.Game().Complete() {
		t.Fatal("precondition: game should be complete")
	}

	if _, err := sess.Undo(ctx); err != nil {
		t.Fatal(err)
	}
	if sess.Game().Complete() {
		t.Fatal("undo should have reopened the game")
	}
	if sess.Game().InningsNumber() != 3 {
		t.Errorf("innings = %d, want 3", sess.Game().InningsNumber())
	}

	// The stored game is back in the resumable list, with no result recorded.
	open, err := svc.Games.Resumable(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 1 || open[0].ID != sess.GameID() {
		t.Errorf("resumable games = %+v, want the reopened game", open)
	}
	bobStats, _ := svc.Stats.ForPlayer(ctx, bob.ID)
	if bobStats.Cricket.GamesWon != 0 {
		t.Errorf("Bob still has %d recorded cricket wins, want 0", bobStats.Cricket.GamesWon)
	}

	// And the game can be resumed and played on.
	resumed, err := svc.Games.ResumeCricket(ctx, sess.GameID())
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Game().Complete() {
		t.Error("the resumed game should be in progress")
	}
	if _, ok := resumed.Game().CurrentTurn(); !ok {
		t.Error("a reopened match must offer a turn")
	}
	if resumed.Game().Aggregate(cricket.TeamB) != 60 {
		t.Errorf("Team B aggregate = %d, want 60", resumed.Game().Aggregate(cricket.TeamB))
	}
}

func TestCricketAbandonRecordsNoWinner(t *testing.T) {
	svc, ctx := newServices(t)
	alice := mustCreate(t, svc, ctx, "Alice")
	bob := mustCreate(t, svc, ctx, "Bob")

	sess, err := svc.Games.StartCricket(ctx,
		cricket.Config{WicketLimit: 5, FirstBatting: cricket.TeamA},
		cricket.Team{Players: []int64{alice.ID}},
		cricket.Team{Players: []int64{bob.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sess.RecordBatting(ctx, alice.ID, 100, 3); err != nil {
		t.Fatal(err)
	}
	if err := sess.Abandon(ctx); err != nil {
		t.Fatal(err)
	}

	for _, p := range []player.Player{alice, bob} {
		st, err := svc.Stats.ForPlayer(ctx, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		if st.Cricket.GamesWon != 0 {
			t.Errorf("%s has %d wins from an abandoned game, want 0", p.Name, st.Cricket.GamesWon)
		}
	}
	// The abandoned game reloads as complete rather than reappearing to resume.
	open, _ := svc.Games.Resumable(ctx)
	if len(open) != 0 {
		t.Errorf("abandoned game is still resumable: %+v", open)
	}
}

func TestCricketSessionEnforcesTurnOrder(t *testing.T) {
	svc, ctx := newServices(t)
	alice := mustCreate(t, svc, ctx, "Alice")
	bob := mustCreate(t, svc, ctx, "Bob")
	carol := mustCreate(t, svc, ctx, "Carol")

	sess, err := svc.Games.StartCricket(ctx,
		cricket.Config{WicketLimit: 5, FirstBatting: cricket.TeamA},
		cricket.Team{Players: []int64{alice.ID, bob.ID}},
		cricket.Team{Players: []int64{carol.ID}})
	if err != nil {
		t.Fatal(err)
	}
	// Bob cannot bat before Alice.
	if _, err := sess.RecordBatting(ctx, bob.ID, 60, 3); err == nil {
		t.Error("out-of-order batting should be rejected")
	}
	// And nothing was stored.
	if sess.Game().EventCount() != 0 {
		t.Errorf("stored %d events, want 0", sess.Game().EventCount())
	}
}

// -------------------------------------------------------------- profile data

func TestRecentGamesDescribesOutcomes(t *testing.T) {
	svc, ctx := newServices(t)
	alice := mustCreate(t, svc, ctx, "Alice")
	bob := mustCreate(t, svc, ctx, "Bob")

	sess, err := svc.Games.StartX01(ctx, x01.DefaultConfig(501), []int64{alice.ID, bob.ID})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 9; i++ {
		if _, err := sess.Record(ctx, x01.VisitInput{PlayerID: alice.ID, Points: 55, Darts: 3}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := sess.Record(ctx, x01.VisitInput{
		PlayerID: alice.ID, Points: 6, Darts: 1, ConfirmCheckout: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := sess.Record(ctx, x01.VisitInput{PlayerID: bob.ID, Points: 100, Darts: 3}); err != nil {
		t.Fatal(err)
	}

	winner, ok := sess.Game().SuggestWinner()
	if !ok || winner != alice.ID {
		t.Fatalf("suggested winner = %d, want Alice", winner)
	}
	if err := sess.Finish(ctx, &winner); err != nil {
		t.Fatal(err)
	}

	games, err := svc.Players.RecentGames(ctx, alice.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 1 {
		t.Fatalf("Alice's recent games = %d, want 1", len(games))
	}
	g := games[0]
	if g.Label != "501" {
		t.Errorf("label = %q, want 501", g.Label)
	}
	if !g.Won || g.Outcome != "Won" {
		t.Errorf("outcome = %q (won=%v), want Won", g.Outcome, g.Won)
	}
	if g.Detail != "out in 28 darts" {
		t.Errorf("detail = %q, want %q", g.Detail, "out in 28 darts")
	}
	if len(g.Opponents) != 1 || g.Opponents[0].Name != "Bob" {
		t.Errorf("opponents = %+v, want just Bob", g.Opponents)
	}
	if g.PlayedAt.IsZero() {
		t.Error("recent game has no date")
	}

	bobGames, err := svc.Players.RecentGames(ctx, bob.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(bobGames) != 1 || bobGames[0].Won || bobGames[0].Outcome != "Lost" {
		t.Errorf("Bob's game = %+v, want a loss", bobGames)
	}
	if bobGames[0].Detail != "401 left" {
		t.Errorf("Bob's detail = %q, want %q", bobGames[0].Detail, "401 left")
	}
}

func TestRecentGamesMarksPlayersWhoNeverThrew(t *testing.T) {
	svc, ctx := newServices(t)
	alice := mustCreate(t, svc, ctx, "Alice")
	bob := mustCreate(t, svc, ctx, "Bob")

	sess, err := svc.Games.StartX01(ctx, x01.DefaultConfig(301), []int64{alice.ID, bob.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sess.Record(ctx, x01.VisitInput{PlayerID: alice.ID, Points: 100, Darts: 3}); err != nil {
		t.Fatal(err)
	}
	if err := sess.Finish(ctx, nil); err != nil {
		t.Fatal(err)
	}

	games, err := svc.Players.RecentGames(ctx, bob.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 1 {
		t.Fatalf("got %d games, want 1", len(games))
	}
	if games[0].Detail != "did not throw" {
		t.Errorf("detail = %q, want %q", games[0].Detail, "did not throw")
	}
	if games[0].Outcome != "No result" {
		t.Errorf("outcome = %q, want %q", games[0].Outcome, "No result")
	}
}

// ------------------------------------------------------------- leaderboard

func TestLeaderboardSorting(t *testing.T) {
	svc, ctx := newServices(t)
	alice := mustCreate(t, svc, ctx, "Alice")
	bob := mustCreate(t, svc, ctx, "Bob")
	carol := mustCreate(t, svc, ctx, "Carol")

	// Alice wins a 501 leg with a strong average; Bob throws poorly.
	sess, err := svc.Games.StartX01(ctx, x01.DefaultConfig(501), []int64{alice.ID, bob.ID})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 9; i++ {
		if _, err := sess.Record(ctx, x01.VisitInput{PlayerID: alice.ID, Points: 55, Darts: 3}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := sess.Record(ctx, x01.VisitInput{
		PlayerID: alice.ID, Points: 6, Darts: 1, ConfirmCheckout: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := sess.Record(ctx, x01.VisitInput{PlayerID: bob.ID, Points: 26, Darts: 3}); err != nil {
		t.Fatal(err)
	}
	if err := sess.Finish(ctx, &alice.ID); err != nil {
		t.Fatal(err)
	}

	byWins, err := svc.Stats.Leaderboard(ctx, SortByWins)
	if err != nil {
		t.Fatal(err)
	}
	if len(byWins) != 3 {
		t.Fatalf("leaderboard rows = %d, want 3", len(byWins))
	}
	if byWins[0].Player.ID != alice.ID {
		t.Errorf("top by wins = %s, want Alice", byWins[0].Player.Name)
	}

	byAverage, err := svc.Stats.Leaderboard(ctx, SortByX01Average)
	if err != nil {
		t.Fatal(err)
	}
	if byAverage[0].Player.ID != alice.ID {
		t.Errorf("top by average = %s, want Alice", byAverage[0].Player.Name)
	}
	// Carol has thrown nothing, so she sorts below Bob rather than tying at zero.
	if byAverage[len(byAverage)-1].Player.ID != carol.ID {
		t.Errorf("bottom by average = %s, want Carol who never threw",
			byAverage[len(byAverage)-1].Player.Name)
	}
	if byAverage[len(byAverage)-1].Played() {
		t.Error("Carol should not count as having played")
	}

	byName, err := svc.Stats.Leaderboard(ctx, SortByName)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Alice", "Bob", "Carol"}
	for i, name := range want {
		if byName[i].Player.Name != name {
			t.Errorf("alphabetical row %d = %s, want %s", i, byName[i].Player.Name, name)
		}
	}
}

func TestLeaderboardExcludesArchivedPlayers(t *testing.T) {
	svc, ctx := newServices(t)
	alice := mustCreate(t, svc, ctx, "Alice")
	mustCreate(t, svc, ctx, "Bob")

	if err := svc.Players.SetArchived(ctx, alice.ID, true); err != nil {
		t.Fatal(err)
	}
	rows, err := svc.Stats.Leaderboard(ctx, SortByWins)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Player.Name != "Bob" {
		t.Errorf("leaderboard = %+v, want just Bob", rows)
	}
}

func TestSortKeyCycles(t *testing.T) {
	seen := map[SortKey]bool{}
	k := SortByWins
	for i := 0; i < 5; i++ {
		if seen[k] {
			t.Fatalf("sort key %v repeated after %d steps", k, i)
		}
		seen[k] = true
		if k.Label() == "" {
			t.Errorf("sort key %v has no label", k)
		}
		k = k.Next()
	}
	if k != SortByWins {
		t.Errorf("cycling returned to %v, want SortByWins", k)
	}
}

// A solo X01 session is described as training rather than as a game that failed
// to produce a result, and it never claims a win.
func TestSoloX01SessionIsRecordedAsTraining(t *testing.T) {
	svc, ctx := newServices(t)
	alice := mustCreate(t, svc, ctx, "Alice")

	sess, err := svc.Games.StartX01(ctx, x01.DefaultConfig(101), []int64{alice.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !sess.Game().Training() {
		t.Fatal("a one-player session should be training")
	}
	if _, err := sess.Record(ctx, x01.VisitInput{
		PlayerID: alice.ID, Points: 101, Darts: 3, ConfirmCheckout: true,
	}); err != nil {
		t.Fatal(err)
	}

	// The session refuses to award a win even if a caller asks for one.
	winner := alice.ID
	if err := sess.Finish(ctx, &winner); !errors.Is(err, x01.ErrTrainingNoWinner) {
		t.Fatalf("Finish(alice) error = %v, want ErrTrainingNoWinner", err)
	}
	if err := sess.Finish(ctx, nil); err != nil {
		t.Fatal(err)
	}

	games, err := svc.Players.RecentGames(ctx, alice.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 1 {
		t.Fatalf("recent games = %d, want 1", len(games))
	}
	g := games[0]
	if g.Won || g.Outcome != "Training" {
		t.Errorf("outcome = %q (won=%v), want Training", g.Outcome, g.Won)
	}
	if g.Detail != "out in 3 darts" {
		t.Errorf("detail = %q, want %q — the leg is still described", g.Detail, "out in 3 darts")
	}
	if len(g.Opponents) != 0 {
		t.Errorf("opponents = %+v, want none", g.Opponents)
	}

	// And the leaderboard shows the darts without crediting a win.
	stats, err := svc.Stats.ForPlayer(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stats.X01.GamesWon != 0 || stats.X01.GamesPlayed != 0 {
		t.Errorf("played/won = %d/%d, want 0/0", stats.X01.GamesPlayed, stats.X01.GamesWon)
	}
	if stats.X01.TrainingGames != 1 {
		t.Errorf("TrainingGames = %d, want 1", stats.X01.TrainingGames)
	}
	if stats.X01.Darts != 3 || stats.X01.Points != 101 {
		t.Errorf("darts/points = %d/%d, want 3/101 — practice still counts",
			stats.X01.Darts, stats.X01.Points)
	}
}

// ---------------------------------------------------- Around the World

func atwThrow(t *testing.T, sess *ATWSession, ctx context.Context, id int64, notation string) atw.Visit {
	t.Helper()
	calls, err := atw.ParseCalls(notation)
	if err != nil {
		t.Fatalf("ParseCalls(%q): %v", notation, err)
	}
	v, err := sess.Record(ctx, atw.VisitInput{PlayerID: id, Calls: calls})
	if err != nil {
		t.Fatalf("Record(%q): %v", notation, err)
	}
	return v
}

func TestATWSessionRecordUndoResumeFinish(t *testing.T) {
	svc, ctx := newServices(t)
	alice := mustCreate(t, svc, ctx, "Alice")
	bob := mustCreate(t, svc, ctx, "Bob")

	sess, err := svc.Games.StartATW(ctx, []int64{alice.ID, bob.ID})
	if err != nil {
		t.Fatal(err)
	}
	gameID := sess.GameID()

	atwThrow(t, sess, ctx, alice.ID, "031") // → 5
	atwThrow(t, sess, ctx, bob.ID, "1")     // → 2
	atwThrow(t, sess, ctx, alice.ID, "33")  // 5 → 8 → 11

	st, _ := sess.Game().State(alice.ID)
	if st.Target.String() != "11" || st.Darts != 5 {
		t.Fatalf("alice = %v with %d darts, want 11 with 5", st.Target, st.Darts)
	}

	// Undo takes back only the last visit.
	if _, err := sess.Undo(ctx); err != nil {
		t.Fatal(err)
	}
	if st, _ := sess.Game().State(alice.ID); st.Target.String() != "5" {
		t.Errorf("after undo alice is on %v, want 5", st.Target)
	}
	if st, _ := sess.Game().State(bob.ID); st.Target.String() != "2" {
		t.Errorf("bob's visit was disturbed: %v", st.Target)
	}

	// Everything survives a reload from disk.
	resumed, err := svc.Games.ResumeATW(ctx, gameID)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Game().VisitCount() != 2 {
		t.Errorf("resumed with %d visits, want 2", resumed.Game().VisitCount())
	}
	if st, _ := resumed.Game().State(alice.ID); st.Target.String() != "5" || st.Darts != 3 {
		t.Errorf("resumed alice = %v with %d darts, want 5 with 3", st.Target, st.Darts)
	}

	// Finish it early, with nobody round the board.
	if err := resumed.Finish(ctx, nil); err != nil {
		t.Fatal(err)
	}
	games, err := svc.Games.Resumable(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range games {
		if g.ID == gameID {
			t.Error("a finished game is still offered for resuming")
		}
	}
}

func TestATWSessionAppearsInTheResumeListWithItsLabel(t *testing.T) {
	svc, ctx := newServices(t)
	alice := mustCreate(t, svc, ctx, "Alice")

	sess, err := svc.Games.StartATW(ctx, []int64{alice.ID})
	if err != nil {
		t.Fatal(err)
	}
	atwThrow(t, sess, ctx, alice.ID, "111")

	games, err := svc.Games.Resumable(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 1 {
		t.Fatalf("resumable = %d, want 1", len(games))
	}
	if games[0].Label != "Around the World" {
		t.Errorf("label = %q, want %q", games[0].Label, "Around the World")
	}
	if games[0].VisitCount != 1 {
		t.Errorf("visit count = %d, want 1", games[0].VisitCount)
	}
}

// An unfinished run is described by how far it got, never by a dart count that
// could be mistaken for a result.
func TestATWRecentGamesDescribeRunsHonestly(t *testing.T) {
	svc, ctx := newServices(t)
	alice := mustCreate(t, svc, ctx, "Alice")
	bob := mustCreate(t, svc, ctx, "Bob")
	chris := mustCreate(t, svc, ctx, "Chris")

	sess, err := svc.Games.StartATW(ctx, []int64{alice.ID, bob.ID, chris.ID})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < atw.TargetCount; i++ {
		atwThrow(t, sess, ctx, alice.ID, "1")
	}
	atwThrow(t, sess, ctx, bob.ID, "11")
	// Chris never throws.

	winners := sess.Game().SuggestWinners()
	if len(winners) != 1 || winners[0] != alice.ID {
		t.Fatalf("suggested %v, want alice", winners)
	}
	if err := sess.Finish(ctx, winners); err != nil {
		t.Fatal(err)
	}

	check := func(id int64, wantOutcome, wantDetail string) {
		t.Helper()
		games, err := svc.Players.RecentGames(ctx, id, 5)
		if err != nil {
			t.Fatal(err)
		}
		if len(games) != 1 {
			t.Fatalf("player %d has %d recent games, want 1", id, len(games))
		}
		g := games[0]
		if g.Label != "Around the World" {
			t.Errorf("label = %q", g.Label)
		}
		if g.Outcome != wantOutcome {
			t.Errorf("player %d outcome = %q, want %q", id, g.Outcome, wantOutcome)
		}
		if g.Detail != wantDetail {
			t.Errorf("player %d detail = %q, want %q", id, g.Detail, wantDetail)
		}
	}

	check(alice.ID, "Won", "round in 22 darts")
	check(bob.ID, "Lost", "20 targets left")
	check(chris.ID, "Lost", "did not throw")
}

func TestATWTieIsReportedAsAWinForBoth(t *testing.T) {
	svc, ctx := newServices(t)
	alice := mustCreate(t, svc, ctx, "Alice")
	bob := mustCreate(t, svc, ctx, "Bob")

	sess, err := svc.Games.StartATW(ctx, []int64{alice.ID, bob.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{alice.ID, bob.ID} {
		for i := 0; i < atw.TargetCount; i++ {
			atwThrow(t, sess, ctx, id, "1")
		}
	}

	winners := sess.Game().SuggestWinners()
	if err := sess.Finish(ctx, winners); err != nil {
		t.Fatal(err)
	}

	for _, id := range []int64{alice.ID, bob.ID} {
		games, err := svc.Players.RecentGames(ctx, id, 5)
		if err != nil {
			t.Fatal(err)
		}
		if !games[0].Won || games[0].Outcome != "Won" {
			t.Errorf("player %d = %q (won=%v), want a win", id, games[0].Outcome, games[0].Won)
		}
	}
}

func TestSoloATWSessionIsTraining(t *testing.T) {
	svc, ctx := newServices(t)
	alice := mustCreate(t, svc, ctx, "Alice")

	sess, err := svc.Games.StartATW(ctx, []int64{alice.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !sess.Game().Training() {
		t.Fatal("a one-player game should be training")
	}
	for i := 0; i < atw.TargetCount; i++ {
		atwThrow(t, sess, ctx, alice.ID, "1")
	}

	if err := sess.Finish(ctx, []int64{alice.ID}); !errors.Is(err, atw.ErrTrainingNoWinner) {
		t.Fatalf("error = %v, want ErrTrainingNoWinner", err)
	}
	if err := sess.Finish(ctx, nil); err != nil {
		t.Fatal(err)
	}

	games, err := svc.Players.RecentGames(ctx, alice.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if games[0].Outcome != "Training" || games[0].Won {
		t.Errorf("outcome = %q (won=%v), want Training", games[0].Outcome, games[0].Won)
	}

	ps, err := svc.Stats.ForPlayer(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ps.ATW.GamesPlayed != 0 || ps.ATW.GamesWon != 0 || ps.ATW.TrainingGames != 1 {
		t.Errorf("played/won/training = %d/%d/%d, want 0/0/1",
			ps.ATW.GamesPlayed, ps.ATW.GamesWon, ps.ATW.TrainingGames)
	}
	// The run itself still counts.
	if ps.ATW.RunsCompleted != 1 || ps.ATW.BestDarts != atw.TargetCount {
		t.Errorf("runs/best = %d/%d, want 1/%d",
			ps.ATW.RunsCompleted, ps.ATW.BestDarts, atw.TargetCount)
	}
}
