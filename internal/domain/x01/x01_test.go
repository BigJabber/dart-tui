package x01

import (
	"errors"
	"math"
	"testing"
)

const (
	alice int64 = 1
	bob   int64 = 2
	carol int64 = 3
)

func newGame(t *testing.T, start int, ids ...int64) *Game {
	t.Helper()
	g, err := New(DefaultConfig(start), ids)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return g
}

func record(t *testing.T, g *Game, id int64, points, dartCount int) Visit {
	t.Helper()
	v, err := g.Record(VisitInput{PlayerID: id, Points: points, Darts: dartCount})
	if err != nil {
		t.Fatalf("Record(%d, %d, %d): %v", id, points, dartCount, err)
	}
	return v
}

func checkout(t *testing.T, g *Game, id int64, points, dartCount int) Visit {
	t.Helper()
	v, err := g.Record(VisitInput{PlayerID: id, Points: points, Darts: dartCount, ConfirmCheckout: true})
	if err != nil {
		t.Fatalf("checkout(%d, %d, %d): %v", id, points, dartCount, err)
	}
	return v
}

func state(t *testing.T, g *Game, id int64) PlayerState {
	t.Helper()
	s, ok := g.State(id)
	if !ok {
		t.Fatalf("no state for player %d", id)
	}
	return s
}

func TestNewValidation(t *testing.T) {
	if _, err := New(DefaultConfig(501), nil); !errors.Is(err, ErrNoPlayers) {
		t.Errorf("empty player list: got %v, want ErrNoPlayers", err)
	}
	if _, err := New(DefaultConfig(501), []int64{alice, alice}); !errors.Is(err, ErrDuplicatePlayer) {
		t.Errorf("duplicate player: got %v, want ErrDuplicatePlayer", err)
	}
	if _, err := New(DefaultConfig(0), []int64{alice}); !errors.Is(err, ErrBadStartScore) {
		t.Errorf("zero start score: got %v, want ErrBadStartScore", err)
	}
}

func TestStartingScores(t *testing.T) {
	for _, start := range []int{Start301, Start501} {
		g := newGame(t, start, alice, bob)
		for _, s := range g.States() {
			if s.Score != start {
				t.Errorf("start %d: player %d began on %d", start, s.PlayerID, s.Score)
			}
		}
	}
}

// Normal score reduction.
func TestNormalScoreReduction(t *testing.T) {
	g := newGame(t, Start501, alice)
	v := record(t, g, alice, 60, 3)
	if v.Bust || v.Checkout {
		t.Fatalf("60 off 501 should be an ordinary visit, got %+v", v)
	}
	if v.ScoreBefore != 501 || v.ScoreAfter != 441 || v.Credited != 60 {
		t.Errorf("got before=%d after=%d credited=%d, want 501/441/60", v.ScoreBefore, v.ScoreAfter, v.Credited)
	}
	s := state(t, g, alice)
	if s.Score != 441 || s.Darts != 3 || s.Visits != 1 || s.Credited != 60 {
		t.Errorf("state after one visit: %+v", s)
	}
}

// Bust restores previous score, and bust still counts darts.
func TestBustRestoresScoreAndCountsDarts(t *testing.T) {
	g := newGame(t, Start501, alice)
	record(t, g, alice, 180, 3) // 321
	record(t, g, alice, 180, 3) // 141
	record(t, g, alice, 100, 3) // 41

	v := record(t, g, alice, 60, 2) // would leave -19
	if !v.Bust {
		t.Fatalf("60 from 41 must bust, got %+v", v)
	}
	if v.Credited != 0 {
		t.Errorf("bust credited %d, want 0", v.Credited)
	}
	if v.ScoreAfter != 41 {
		t.Errorf("bust left score at %d, want the pre-visit 41", v.ScoreAfter)
	}

	s := state(t, g, alice)
	if s.Score != 41 {
		t.Errorf("score after bust = %d, want 41", s.Score)
	}
	if s.Darts != 11 {
		t.Errorf("darts after bust = %d, want 11 (9 + the 2 busted darts)", s.Darts)
	}
	if s.Visits != 4 {
		t.Errorf("visits = %d, want 4", s.Visits)
	}
	if s.Credited != 460 {
		t.Errorf("credited = %d, want 460 (bust credits nothing)", s.Credited)
	}
}

// Under double-out, leaving exactly 1 is a bust.
func TestLeavingOneBustsUnderDoubleOut(t *testing.T) {
	g := newGame(t, Start501, alice)
	record(t, g, alice, 180, 3)
	record(t, g, alice, 180, 3)
	record(t, g, alice, 100, 3) // 41
	v := record(t, g, alice, 40, 3)
	if !v.Bust {
		t.Fatalf("leaving 1 must bust under double-out, got %+v", v)
	}
	if state(t, g, alice).Score != 41 {
		t.Errorf("score not restored after leaving 1")
	}
}

// With double-out disabled, leaving 1 is legal and zero needs no confirmation.
func TestStraightOutConfigurable(t *testing.T) {
	g, err := New(Config{StartScore: 101, DoubleOut: false}, []int64{alice})
	if err != nil {
		t.Fatal(err)
	}
	if v := record(t, g, alice, 100, 3); v.Bust {
		t.Fatalf("leaving 1 should be legal without double-out, got %+v", v)
	}
	// Reaching zero counts immediately, without operator confirmation.
	v := record(t, g, alice, 1, 1)
	if !v.Checkout || v.Bust {
		t.Fatalf("straight-out finish should check out unconfirmed, got %+v", v)
	}
	if !state(t, g, alice).Finished {
		t.Error("player should be finished")
	}
}

// A visit landing on zero that the operator does not confirm is a bust, because
// only the visit total is captured and the finish may not have been a double.
func TestUnconfirmedZeroIsBust(t *testing.T) {
	g := newGame(t, 40, alice)
	v, err := g.Record(VisitInput{PlayerID: alice, Points: 40, Darts: 3, ConfirmCheckout: false})
	if err != nil {
		t.Fatal(err)
	}
	if !v.Bust {
		t.Fatalf("unconfirmed zero must bust under double-out, got %+v", v)
	}
	if v.Checkout {
		t.Error("unconfirmed zero must not be recorded as a checkout")
	}
	s := state(t, g, alice)
	if s.Score != 40 || s.Finished {
		t.Errorf("state after unconfirmed zero: %+v", s)
	}
	if s.Darts != 3 {
		t.Errorf("darts = %d, want 3 — busted darts still count", s.Darts)
	}
}

func TestLandsOnZero(t *testing.T) {
	g := newGame(t, 40, alice)
	if !g.LandsOnZero(alice, 40) {
		t.Error("40 from 40 should land on zero")
	}
	if g.LandsOnZero(alice, 20) {
		t.Error("20 from 40 should not land on zero")
	}
	if g.LandsOnZero(carol, 40) {
		t.Error("unknown player should not land on zero")
	}
}

// One-, two- and three-dart checkouts are all recordable and store the exact
// dart count.
func TestCheckoutDartCounts(t *testing.T) {
	cases := []struct {
		name   string
		start  int
		points int
		darts  int
	}{
		{"one dart", 40, 40, 1},
		{"two darts", 60, 60, 2},
		{"three darts", 100, 100, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := newGame(t, tc.start, alice)
			v := checkout(t, g, alice, tc.points, tc.darts)
			if !v.Checkout || v.Bust {
				t.Fatalf("expected a checkout, got %+v", v)
			}
			s := state(t, g, alice)
			if !s.Finished {
				t.Fatal("player should be finished")
			}
			if s.FinishDarts != tc.darts {
				t.Errorf("FinishDarts = %d, want %d", s.FinishDarts, tc.darts)
			}
			if s.FinishVisits != 1 {
				t.Errorf("FinishVisits = %d, want 1", s.FinishVisits)
			}
			if s.Score != 0 {
				t.Errorf("score = %d, want 0", s.Score)
			}
		})
	}
}

func TestFinishedPlayerCannotThrowAgain(t *testing.T) {
	g := newGame(t, 40, alice)
	checkout(t, g, alice, 40, 2)
	if _, err := g.Record(VisitInput{PlayerID: alice, Points: 20, Darts: 3}); !errors.Is(err, ErrPlayerFinished) {
		t.Errorf("got %v, want ErrPlayerFinished", err)
	}
}

func TestUnknownPlayerRejected(t *testing.T) {
	g := newGame(t, Start501, alice)
	if _, err := g.Record(VisitInput{PlayerID: carol, Points: 60, Darts: 3}); !errors.Is(err, ErrUnknownPlayer) {
		t.Errorf("got %v, want ErrUnknownPlayer", err)
	}
}

func TestInvalidInputRejected(t *testing.T) {
	g := newGame(t, Start501, alice)
	cases := []struct {
		name   string
		in     VisitInput
		target error
	}{
		{"zero darts", VisitInput{PlayerID: alice, Points: 60, Darts: 0}, ErrBadDartCount},
		{"four darts", VisitInput{PlayerID: alice, Points: 60, Darts: 4}, ErrBadDartCount},
		{"negative darts", VisitInput{PlayerID: alice, Points: 60, Darts: -1}, ErrBadDartCount},
		{"negative score", VisitInput{PlayerID: alice, Points: -10, Darts: 3}, ErrImpossibleScore},
		{"over 180", VisitInput{PlayerID: alice, Points: 181, Darts: 3}, ErrImpossibleScore},
		{"impossible 179", VisitInput{PlayerID: alice, Points: 179, Darts: 3}, ErrImpossibleScore},
		{"impossible for one dart", VisitInput{PlayerID: alice, Points: 61, Darts: 1}, ErrImpossibleScore},
		{"too high for two darts", VisitInput{PlayerID: alice, Points: 121, Darts: 2}, ErrImpossibleScore},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := g.Record(tc.in); !errors.Is(err, tc.target) {
				t.Errorf("got %v, want %v", err, tc.target)
			}
		})
	}
	if g.VisitCount() != 0 {
		t.Errorf("rejected input recorded %d visits, want 0", g.VisitCount())
	}
}

func TestZeroScoreVisitIsLegal(t *testing.T) {
	g := newGame(t, Start501, alice)
	v := record(t, g, alice, 0, 3)
	if v.Bust || v.Checkout {
		t.Fatalf("three misses is an ordinary visit, got %+v", v)
	}
	s := state(t, g, alice)
	if s.Score != 501 || s.Darts != 3 {
		t.Errorf("state after a blank visit: %+v", s)
	}
}

// Players may throw in any order, and one player may complete an entire leg
// before another has thrown a single dart. The engine must never advance to
// "the next player" on its own.
func TestArbitraryThrowOrder(t *testing.T) {
	g := newGame(t, Start501, alice, bob, carol)

	// Alice plays her whole leg first: nine 55s then a 6 checkout.
	for i := 0; i < 9; i++ {
		record(t, g, alice, 55, 3)
	}
	checkout(t, g, alice, 6, 2)

	if !state(t, g, alice).Finished {
		t.Fatal("alice should have finished")
	}
	if s := state(t, g, bob); s.Score != 501 || s.Darts != 0 || s.Visits != 0 {
		t.Errorf("bob should be untouched, got %+v", s)
	}
	if s := state(t, g, carol); s.Score != 501 {
		t.Errorf("carol should be untouched, got %+v", s)
	}

	// Now Bob plays, then Carol, then back to Bob — arbitrary interleaving.
	record(t, g, bob, 100, 3)
	record(t, g, carol, 60, 3)
	record(t, g, bob, 100, 3)

	if s := state(t, g, bob); s.Score != 301 {
		t.Errorf("bob score = %d, want 301", s.Score)
	}
	if s := state(t, g, carol); s.Score != 441 {
		t.Errorf("carol score = %d, want 441", s.Score)
	}

	// Alice is out of the active list; Bob and Carol remain, in selection order.
	active := g.ActivePlayers()
	if len(active) != 2 || active[0] != bob || active[1] != carol {
		t.Errorf("active players = %v, want [bob carol]", active)
	}
}

func TestActivePlayersAndAllFinished(t *testing.T) {
	g := newGame(t, 40, alice, bob)
	if g.AllFinished() {
		t.Error("nobody has finished yet")
	}
	checkout(t, g, alice, 40, 1)
	if g.AllFinished() {
		t.Error("bob has not finished")
	}
	checkout(t, g, bob, 40, 3)
	if !g.AllFinished() {
		t.Error("both players have finished")
	}
	if len(g.ActivePlayers()) != 0 {
		t.Errorf("active players = %v, want none", g.ActivePlayers())
	}
}

// The headline requirement: 28 darts must beat 30 darts. Both players score the
// same 501 points; only the dart count separates them.
func Test28DartLegBeats30DartLeg(t *testing.T) {
	g := newGame(t, Start501, alice, bob)

	// Alice: nine visits of 55 (495), then 6 with three darts = 30 darts.
	for i := 0; i < 9; i++ {
		record(t, g, alice, 55, 3)
	}
	checkout(t, g, alice, 6, 3)

	// Bob: nine visits of 55 (495), then 6 with a single dart = 28 darts.
	for i := 0; i < 9; i++ {
		record(t, g, bob, 55, 3)
	}
	checkout(t, g, bob, 6, 1)

	as, bs := state(t, g, alice), state(t, g, bob)
	if as.FinishDarts != 30 {
		t.Errorf("alice finished in %d darts, want 30", as.FinishDarts)
	}
	if bs.FinishDarts != 28 {
		t.Errorf("bob finished in %d darts, want 28", bs.FinishDarts)
	}
	if as.FinishDarts == bs.FinishDarts {
		t.Fatal("the two legs must not be considered equivalent")
	}

	// Both took ten visits, so a visits-based comparison would call it a tie.
	if as.FinishVisits != bs.FinishVisits {
		t.Fatalf("precondition: both should have taken 10 visits, got %d and %d",
			as.FinishVisits, bs.FinishVisits)
	}

	// Bob is ranked first and suggested as the winner despite throwing second.
	results := g.Results()
	if results[0].PlayerID != bob {
		t.Errorf("results ranked %d first, want bob (fewer darts)", results[0].PlayerID)
	}
	winner, ok := g.SuggestWinner()
	if !ok || winner != bob {
		t.Errorf("SuggestWinner = (%d, %v), want (bob, true)", winner, ok)
	}

	// And the averages differ in Bob's favour.
	if !(bs.Average() > as.Average()) {
		t.Errorf("bob's average %.3f should beat alice's %.3f", bs.Average(), as.Average())
	}
}

func TestThreeDartAverageUsesDartsNotVisits(t *testing.T) {
	g := newGame(t, Start501, alice)
	for i := 0; i < 9; i++ {
		record(t, g, alice, 55, 3)
	}
	checkout(t, g, alice, 6, 1)

	s := state(t, g, alice)
	// 501 credited points over 28 darts.
	want := 501.0 / 28.0 * 3
	if math.Abs(s.Average()-want) > 1e-9 {
		t.Errorf("average = %.6f, want %.6f", s.Average(), want)
	}

	// The naive visits×3 calculation would give 501/30*3; the real figure must
	// be higher because the final visit used a single dart.
	naive := 501.0 / float64(s.Visits*3) * 3
	if !(s.Average() > naive) {
		t.Errorf("average %.6f should exceed the visits-based %.6f", s.Average(), naive)
	}
}

func TestAverageExcludesBustPointsButIncludesBustDarts(t *testing.T) {
	g := newGame(t, Start501, alice)
	record(t, g, alice, 60, 3)  // 441, credited 60
	record(t, g, alice, 180, 3) // 261, credited 180
	// A bust: 261 - 300 is impossible, so use a score that overshoots.
	record(t, g, alice, 180, 3) // 81
	record(t, g, alice, 100, 3) // would leave -19 → bust

	s := state(t, g, alice)
	if s.Credited != 420 {
		t.Fatalf("credited = %d, want 420", s.Credited)
	}
	if s.Darts != 12 {
		t.Fatalf("darts = %d, want 12", s.Darts)
	}
	want := 420.0 / 12.0 * 3
	if math.Abs(s.Average()-want) > 1e-9 {
		t.Errorf("average = %.6f, want %.6f", s.Average(), want)
	}
}

func TestUndoOrdinaryVisit(t *testing.T) {
	g := newGame(t, Start501, alice, bob)
	record(t, g, alice, 60, 3)
	record(t, g, bob, 100, 3)

	removed, err := g.Undo()
	if err != nil {
		t.Fatal(err)
	}
	if removed.PlayerID != bob || removed.Points != 100 {
		t.Errorf("undid the wrong visit: %+v", removed)
	}
	if s := state(t, g, bob); s.Score != 501 || s.Darts != 0 || s.Visits != 0 || s.Credited != 0 {
		t.Errorf("bob not restored: %+v", s)
	}
	if s := state(t, g, alice); s.Score != 441 {
		t.Errorf("alice should be untouched by undoing bob's visit: %+v", s)
	}
	if g.VisitCount() != 1 {
		t.Errorf("visit count = %d, want 1", g.VisitCount())
	}
}

// Undoing a checkout must unwind the finished position too.
func TestUndoCompletedCheckout(t *testing.T) {
	g := newGame(t, 100, alice, bob)
	record(t, g, alice, 60, 3) // 40 left
	checkout(t, g, alice, 40, 2)

	if !state(t, g, alice).Finished {
		t.Fatal("precondition: alice should be finished")
	}
	if _, ok := g.SuggestWinner(); !ok {
		t.Fatal("precondition: a winner should be suggested")
	}

	if _, err := g.Undo(); err != nil {
		t.Fatal(err)
	}

	s := state(t, g, alice)
	if s.Finished {
		t.Error("alice should no longer be finished")
	}
	if s.Score != 40 {
		t.Errorf("score = %d, want 40", s.Score)
	}
	if s.Darts != 3 {
		t.Errorf("darts = %d, want 3", s.Darts)
	}
	if s.FinishDarts != 0 || s.FinishVisits != 0 {
		t.Errorf("finish figures not cleared: %+v", s)
	}
	if _, ok := g.SuggestWinner(); ok {
		t.Error("no winner should be suggested once the checkout is undone")
	}
	// Alice can throw again.
	if _, err := g.Record(VisitInput{PlayerID: alice, Points: 20, Darts: 3}); err != nil {
		t.Errorf("alice should be able to throw again: %v", err)
	}
}

func TestUndoBustRestoresDartCount(t *testing.T) {
	g := newGame(t, 50, alice)
	record(t, g, alice, 20, 3) // 30 left
	record(t, g, alice, 40, 2) // bust
	if s := state(t, g, alice); s.Darts != 5 {
		t.Fatalf("precondition darts = %d, want 5", s.Darts)
	}
	if _, err := g.Undo(); err != nil {
		t.Fatal(err)
	}
	s := state(t, g, alice)
	if s.Darts != 3 || s.Busts != 0 || s.Score != 30 {
		t.Errorf("state after undoing a bust: %+v", s)
	}
}

func TestUndoRepeatedlyBackToStart(t *testing.T) {
	g := newGame(t, Start301, alice, bob)
	record(t, g, alice, 100, 3)
	record(t, g, bob, 60, 3)
	record(t, g, alice, 100, 3)
	for i := 0; i < 3; i++ {
		if _, err := g.Undo(); err != nil {
			t.Fatalf("undo %d: %v", i, err)
		}
	}
	if _, err := g.Undo(); !errors.Is(err, ErrNothingToUndo) {
		t.Errorf("got %v, want ErrNothingToUndo", err)
	}
	for _, s := range g.States() {
		if s.Score != Start301 || s.Darts != 0 || s.Visits != 0 {
			t.Errorf("player %d not fully reset: %+v", s.PlayerID, s)
		}
	}
}

// Reaching zero must not end the game — the other players may still play on.
func TestGameDoesNotAutoFinishWhenAPlayerCheckouts(t *testing.T) {
	g := newGame(t, 40, alice, bob)
	checkout(t, g, alice, 40, 1)
	if g.Finished() {
		t.Fatal("the game must not finish just because a player reached zero")
	}
	// Bob can still play his leg out.
	if _, err := g.Record(VisitInput{PlayerID: bob, Points: 20, Darts: 3}); err != nil {
		t.Fatalf("bob should still be able to throw: %v", err)
	}
}

// The operator can finish a game with participants who never threw; those
// players are stored as incomplete rather than given an invented finish.
func TestManualFinishWithIncompletePlayers(t *testing.T) {
	g := newGame(t, Start501, alice, bob, carol)
	for i := 0; i < 9; i++ {
		record(t, g, alice, 55, 3)
	}
	checkout(t, g, alice, 6, 3)
	record(t, g, bob, 100, 3) // Bob part-way through; Carol never threw.

	winner, ok := g.SuggestWinner()
	if !ok || winner != alice {
		t.Fatalf("SuggestWinner = (%d, %v), want (alice, true)", winner, ok)
	}
	if err := g.Finish(&winner); err != nil {
		t.Fatal(err)
	}
	if !g.Finished() {
		t.Fatal("game should be finished")
	}

	results := g.Results()
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	byID := map[int64]ParticipantResult{}
	for _, r := range results {
		byID[r.PlayerID] = r
	}
	if a := byID[alice]; !a.Completed || !a.Won || a.Remaining != 0 || a.Darts != 30 {
		t.Errorf("alice result: %+v", a)
	}
	if b := byID[bob]; b.Completed || b.Won || b.Remaining != 401 || b.Darts != 3 || b.Visits != 1 {
		t.Errorf("bob result: %+v", b)
	}
	c := byID[carol]
	if c.Completed || c.Won || c.Remaining != Start501 || c.Darts != 0 || c.Visits != 0 {
		t.Errorf("carol result should be an honest zero, not a fabricated finish: %+v", c)
	}
	if c.Average != 0 {
		t.Errorf("carol average = %v, want 0", c.Average)
	}

	// Bob is ranked ahead of Carol because he has less left on the board.
	if results[1].PlayerID != bob || results[2].PlayerID != carol {
		t.Errorf("unfinished players ranked %d then %d, want bob then carol",
			results[1].PlayerID, results[2].PlayerID)
	}
}

func TestFinishWithNoWinner(t *testing.T) {
	g := newGame(t, Start501, alice, bob)
	record(t, g, alice, 60, 3)
	if _, ok := g.SuggestWinner(); ok {
		t.Error("no completed leg means no suggestion")
	}
	if err := g.Finish(nil); err != nil {
		t.Fatal(err)
	}
	if g.Winner() != nil {
		t.Errorf("winner = %v, want nil", g.Winner())
	}
}

// The operator may override the suggestion — the winner is never inferred from
// throwing order alone.
func TestOperatorCanOverrideSuggestedWinner(t *testing.T) {
	g := newGame(t, 40, alice, bob)
	checkout(t, g, alice, 40, 1)
	suggested, _ := g.SuggestWinner()
	if suggested != alice {
		t.Fatalf("precondition: suggestion should be alice, got %d", suggested)
	}
	chosen := bob
	if err := g.Finish(&chosen); err != nil {
		t.Fatal(err)
	}
	if w := g.Winner(); w == nil || *w != bob {
		t.Errorf("winner = %v, want bob", w)
	}
}

func TestFinishRejectsOutsider(t *testing.T) {
	g := newGame(t, Start501, alice, bob)
	outsider := carol
	if err := g.Finish(&outsider); !errors.Is(err, ErrWinnerNotPlaying) {
		t.Errorf("got %v, want ErrWinnerNotPlaying", err)
	}
	if g.Finished() {
		t.Error("a rejected finish must leave the game open")
	}
}

func TestFinishedGameIsClosed(t *testing.T) {
	g := newGame(t, Start501, alice)
	record(t, g, alice, 60, 3)
	if err := g.Finish(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Record(VisitInput{PlayerID: alice, Points: 60, Darts: 3}); !errors.Is(err, ErrGameFinished) {
		t.Errorf("record after finish: got %v, want ErrGameFinished", err)
	}
	if _, err := g.Undo(); !errors.Is(err, ErrGameFinished) {
		t.Errorf("undo after finish: got %v, want ErrGameFinished", err)
	}
	if err := g.Finish(nil); !errors.Is(err, ErrGameFinished) {
		t.Errorf("double finish: got %v, want ErrGameFinished", err)
	}
}

// Restoring from persisted visits must reproduce the live game exactly.
func TestRestoreReplaysToIdenticalState(t *testing.T) {
	g := newGame(t, Start501, alice, bob)
	record(t, g, alice, 100, 3)
	record(t, g, bob, 140, 3)
	record(t, g, alice, 180, 3)
	record(t, g, alice, 180, 3) // busts: 221 - 180 = 41, fine; then check
	record(t, g, bob, 60, 3)

	restored, err := Restore(g.Config(), g.Order(), g.Visits(), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range g.States() {
		got, ok := restored.State(want.PlayerID)
		if !ok {
			t.Fatalf("restored game is missing player %d", want.PlayerID)
		}
		if got != want {
			t.Errorf("player %d: restored %+v, want %+v", want.PlayerID, got, want)
		}
	}
	if restored.VisitCount() != g.VisitCount() {
		t.Errorf("visit count %d, want %d", restored.VisitCount(), g.VisitCount())
	}
}

func TestRestoreCarriesFinishedStateAndWinner(t *testing.T) {
	g := newGame(t, 40, alice, bob)
	checkout(t, g, alice, 40, 2)
	w := alice
	if err := g.Finish(&w); err != nil {
		t.Fatal(err)
	}
	restored, err := Restore(g.Config(), g.Order(), g.Visits(), true, &w)
	if err != nil {
		t.Fatal(err)
	}
	if !restored.Finished() {
		t.Error("restored game should be finished")
	}
	if got := restored.Winner(); got == nil || *got != alice {
		t.Errorf("restored winner = %v, want alice", got)
	}
}

func TestPreviewDoesNotRecord(t *testing.T) {
	g := newGame(t, 40, alice)
	v, err := g.Preview(VisitInput{PlayerID: alice, Points: 40, Darts: 2, ConfirmCheckout: true})
	if err != nil {
		t.Fatal(err)
	}
	if !v.Checkout {
		t.Errorf("preview should report a checkout, got %+v", v)
	}
	if g.VisitCount() != 0 {
		t.Errorf("preview recorded %d visits, want 0", g.VisitCount())
	}
	if state(t, g, alice).Score != 40 {
		t.Error("preview must not change the score")
	}
}

func TestWinnerReturnsCopy(t *testing.T) {
	g := newGame(t, 40, alice)
	w := alice
	if err := g.Finish(&w); err != nil {
		t.Fatal(err)
	}
	got := g.Winner()
	*got = 999
	if second := g.Winner(); second == nil || *second != alice {
		t.Errorf("mutating the returned winner changed the game: %v", second)
	}
}
