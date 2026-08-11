package atw

import (
	"errors"
	"testing"

	"github.com/BigJabber/dart-tui/internal/domain/darts"
)

const (
	alice int64 = 1
	bob   int64 = 2
	carol int64 = 3
)

func newGame(t *testing.T, ids ...int64) *Game {
	t.Helper()
	g, err := New(ids)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return g
}

// throw records a visit from its keyed notation, such as "031".
func throw(t *testing.T, g *Game, id int64, notation string) Visit {
	t.Helper()
	calls, err := ParseCalls(notation)
	if err != nil {
		t.Fatalf("ParseCalls(%q): %v", notation, err)
	}
	v, err := g.Record(VisitInput{PlayerID: id, Calls: calls})
	if err != nil {
		t.Fatalf("Record(%d, %q): %v", id, notation, err)
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

func wantTarget(t *testing.T, g *Game, id int64, want Target) {
	t.Helper()
	if got := state(t, g, id).Target; got != want {
		t.Errorf("player %d target = %v, want %v", id, got, want)
	}
}

// ------------------------------------------------------------ progression

func TestEverybodyStartsOnOne(t *testing.T) {
	g := newGame(t, alice, bob)
	for _, id := range []int64{alice, bob} {
		st := state(t, g, id)
		if st.Target != number(1) {
			t.Errorf("player %d starts on %v, want 1", id, st.Target)
		}
		if st.Darts != 0 || st.Visits != 0 || st.Finished {
			t.Errorf("player %d starts with figures already set: %+v", id, st)
		}
	}
}

func TestSingleDoubleAndTrebleFromOne(t *testing.T) {
	cases := map[string]Target{
		"1": number(2),
		"2": number(3),
		"3": number(4),
	}
	for notation, want := range cases {
		g := newGame(t, alice)
		throw(t, g, alice, notation)
		wantTarget(t, g, alice, want)
	}
}

func TestAMissLeavesTheTargetAlone(t *testing.T) {
	g := newGame(t, alice)
	throw(t, g, alice, "111") // 1 → 4
	wantTarget(t, g, alice, number(4))

	v := throw(t, g, alice, "0")
	wantTarget(t, g, alice, number(4))
	if v.Advanced() != 0 {
		t.Errorf("a miss advanced %d targets, want 0", v.Advanced())
	}
	if st := state(t, g, alice); st.Darts != 4 {
		t.Errorf("darts = %d, want 4 — a miss is still a dart thrown", st.Darts)
	}
}

// The worked example from the specification: each dart is scored against the
// target as it stands, not against the one the visit started on.
func TestVisitIsScoredDartByDart(t *testing.T) {
	g := newGame(t, alice)
	v := throw(t, g, alice, "031")

	wantTarget(t, g, alice, number(5))
	if st := state(t, g, alice); st.Darts != 3 {
		t.Errorf("darts = %d, want 3", st.Darts)
	}

	// The darts recorded are the segments actually struck, in order.
	want := []Dart{Miss(), HitOn(darts.T(1)), HitOn(darts.S(4))}
	if len(v.Darts) != len(want) {
		t.Fatalf("recorded %d darts, want %d", len(v.Darts), len(want))
	}
	for i := range want {
		if v.Darts[i] != want[i] {
			t.Errorf("dart %d = %v, want %v", i+1, v.Darts[i], want[i])
		}
	}
	if v.TargetBefore != number(1) || v.TargetAfter != number(5) {
		t.Errorf("visit ran %v → %v, want 1 → 5", v.TargetBefore, v.TargetAfter)
	}
	if v.Advanced() != 4 {
		t.Errorf("advanced %d, want 4", v.Advanced())
	}
}

// The same visit scored all three darts against target 1 would leave the player
// on 2. This is the mistake the test exists to catch.
func TestVisitIsNotScoredAgainstTheOpeningTarget(t *testing.T) {
	g := newGame(t, alice)
	throw(t, g, alice, "031")
	if got := state(t, g, alice).Target; got == number(2) {
		t.Fatal("all three darts were scored against the opening target")
	}
}

func TestConsecutiveSingles(t *testing.T) {
	g := newGame(t, alice)
	throw(t, g, alice, "111")
	wantTarget(t, g, alice, number(4))
}

func TestTrebleSkipsTwoTargets(t *testing.T) {
	g := newGame(t, alice)
	throw(t, g, alice, "33") // 1 → 4 → 7
	wantTarget(t, g, alice, number(7))
}

// ------------------------------------------------- the end of the sequence

// advanceTo walks a player to a target using single hits, so a test can start
// from wherever it needs to without hard-coding a dart count.
func advanceTo(t *testing.T, g *Game, id int64, target Target) {
	t.Helper()
	for state(t, g, id).Target.Index() < target.Index() {
		throw(t, g, id, "1")
	}
	wantTarget(t, g, id, target)
}

func TestProgressionFromNineteen(t *testing.T) {
	cases := map[string]Target{
		"1": number(20),
		"2": outer,
		"3": bull,
	}
	for notation, want := range cases {
		g := newGame(t, alice)
		advanceTo(t, g, alice, number(19))
		throw(t, g, alice, notation)
		wantTarget(t, g, alice, want)
	}
}

func TestProgressionFromTwenty(t *testing.T) {
	cases := map[string]Target{
		"1": outer,
		"2": bull,
	}
	for notation, want := range cases {
		g := newGame(t, alice)
		advanceTo(t, g, alice, number(20))
		throw(t, g, alice, notation)
		wantTarget(t, g, alice, want)
	}

	// A treble 20 carries the player through 20, the outer bull and the
	// bullseye, and out the other side.
	g := newGame(t, alice)
	advanceTo(t, g, alice, number(20))
	v := throw(t, g, alice, "3")
	if !v.Completed {
		t.Fatal("treble 20 from target 20 should complete the run")
	}
	if !state(t, g, alice).Finished {
		t.Error("the player should be finished")
	}
}

func TestProgressionThroughTheBulls(t *testing.T) {
	// Outer bull hit → bullseye.
	g := newGame(t, alice)
	advanceTo(t, g, alice, outer)
	throw(t, g, alice, "1")
	wantTarget(t, g, alice, bull)

	// Bullseye hit while the outer bull was required → finished.
	g2 := newGame(t, alice)
	advanceTo(t, g2, alice, outer)
	v := throw(t, g2, alice, "2")
	if !v.Completed || !state(t, g2, alice).Finished {
		t.Error("a bullseye from the outer bull should complete the run")
	}

	// Bullseye hit on the bullseye → finished.
	g3 := newGame(t, alice)
	advanceTo(t, g3, alice, bull)
	if v := throw(t, g3, alice, "2"); !v.Completed {
		t.Error("a bullseye on the bullseye should complete the run")
	}
}

func TestTrebleIsRefusedOnBothBulls(t *testing.T) {
	for _, target := range []Target{outer, bull} {
		g := newGame(t, alice)
		advanceTo(t, g, alice, target)
		before := state(t, g, alice)

		_, err := g.Record(VisitInput{PlayerID: alice, Calls: []Call{CallTreble}})
		if !errors.Is(err, ErrNoTrebleBull) {
			t.Errorf("at %v: error = %v, want ErrNoTrebleBull", target, err)
		}
		if after := state(t, g, alice); after != before {
			t.Errorf("at %v: a refused visit changed the state", target)
		}
		if g.VisitCount() != before.Visits {
			t.Errorf("at %v: a refused visit was recorded", target)
		}
	}
}

// A visit is refused as a whole when any dart in it is impossible, so a bad
// keystroke never half-applies.
func TestARefusedVisitRecordsNothing(t *testing.T) {
	g := newGame(t, alice)
	advanceTo(t, g, alice, number(19))
	before := state(t, g, alice)

	// The second dart takes the player to the outer bull; the third is a treble
	// bull, which does not exist.
	_, err := g.Record(VisitInput{PlayerID: alice, Calls: []Call{CallSingle, CallSingle, CallTreble}})
	if !errors.Is(err, ErrNoTrebleBull) {
		t.Fatalf("error = %v, want ErrNoTrebleBull", err)
	}
	if after := state(t, g, alice); after != before {
		t.Errorf("state changed despite the refusal: %+v", after)
	}
}

// ------------------------------------------------------------ dart counting

// A run completed part-way through a visit costs only the darts thrown.
func TestPartialFinalVisitCountsOnlyTheDartsThrown(t *testing.T) {
	g := newGame(t, alice)
	advanceTo(t, g, alice, bull) // 21 single hits, so 21 darts
	if got := state(t, g, alice).Darts; got != 21 {
		t.Fatalf("darts before the final visit = %d, want 21", got)
	}

	// Miss, then the bullseye. The third dart was never thrown.
	v := throw(t, g, alice, "022")
	if len(v.Darts) != 2 {
		t.Fatalf("recorded %d darts, want 2 — the third was never thrown", len(v.Darts))
	}
	st := state(t, g, alice)
	if st.Darts != 23 || st.FinishDarts != 23 {
		t.Errorf("darts = %d (finish %d), want 23", st.Darts, st.FinishDarts)
	}
	if !st.Finished {
		t.Error("the player should be finished")
	}
}

func TestFullVisitsCountEveryDart(t *testing.T) {
	g := newGame(t, alice)
	for i := 0; i < 7; i++ {
		throw(t, g, alice, "000")
	}
	if got := state(t, g, alice).Darts; got != 21 {
		t.Errorf("seven complete visits = %d darts, want 21", got)
	}
	if got := state(t, g, alice).Visits; got != 7 {
		t.Errorf("visits = %d, want 7", got)
	}
}

func TestDartOutcomesAreTallied(t *testing.T) {
	g := newGame(t, alice)
	throw(t, g, alice, "031") // miss, treble, single
	throw(t, g, alice, "20")  // double, miss

	st := state(t, g, alice)
	if st.Misses != 2 {
		t.Errorf("misses = %d, want 2", st.Misses)
	}
	if st.Hits != 3 {
		t.Errorf("hits = %d, want 3", st.Hits)
	}
	if st.Singles != 1 || st.Doubles != 1 || st.Trebles != 1 {
		t.Errorf("singles/doubles/trebles = %d/%d/%d, want 1/1/1",
			st.Singles, st.Doubles, st.Trebles)
	}
	if st.Advanced != 6 {
		t.Errorf("advanced = %d, want 6", st.Advanced)
	}
}

// ------------------------------------------------------------- player order

func TestPlayersMayThrowInAnyOrder(t *testing.T) {
	g := newGame(t, alice, bob, carol)

	// The office's actual pattern: one player several times in a row, others
	// interleaved as they wander over.
	for _, id := range []int64{alice, alice, carol, bob, alice} {
		throw(t, g, id, "1")
	}

	if got := state(t, g, alice).Darts; got != 3 {
		t.Errorf("alice darts = %d, want 3", got)
	}
	if got := state(t, g, alice).Target; got != number(4) {
		t.Errorf("alice target = %v, want 4", got)
	}
	if got := state(t, g, bob).Target; got != number(2) {
		t.Errorf("bob target = %v, want 2", got)
	}
	if got := state(t, g, carol).Target; got != number(2) {
		t.Errorf("carol target = %v, want 2", got)
	}
}

func TestPlayersProgressIndependently(t *testing.T) {
	g := newGame(t, alice, bob)
	throw(t, g, alice, "333") // alice races ahead

	if got := state(t, g, bob).Target; got != number(1) {
		t.Errorf("bob target = %v, want 1 — he has not thrown", got)
	}
	if got := state(t, g, bob).Darts; got != 0 {
		t.Errorf("bob darts = %d, want 0", got)
	}
	if got := state(t, g, alice).Target; got != number(10) {
		t.Errorf("alice target = %v, want 10", got)
	}
}

func TestUnknownPlayerIsRefused(t *testing.T) {
	g := newGame(t, alice, bob)
	_, err := g.Record(VisitInput{PlayerID: carol, Calls: []Call{CallSingle}})
	if !errors.Is(err, ErrUnknownPlayer) {
		t.Errorf("error = %v, want ErrUnknownPlayer", err)
	}
}

func TestEmptyAndOversizedVisitsAreRefused(t *testing.T) {
	g := newGame(t, alice)
	if _, err := g.Record(VisitInput{PlayerID: alice}); !errors.Is(err, ErrNoDarts) {
		t.Errorf("empty visit error = %v, want ErrNoDarts", err)
	}
	four := []Call{CallMiss, CallMiss, CallMiss, CallMiss}
	if _, err := g.Record(VisitInput{PlayerID: alice, Calls: four}); err == nil {
		t.Error("a four-dart visit should be refused")
	}
}

// ------------------------------------------------------------- completion

func TestAFinishedPlayerCannotThrowAgain(t *testing.T) {
	g := newGame(t, alice, bob)
	finish(t, g, alice)

	_, err := g.Record(VisitInput{PlayerID: alice, Calls: []Call{CallSingle}})
	if !errors.Is(err, ErrPlayerFinished) {
		t.Errorf("error = %v, want ErrPlayerFinished", err)
	}

	// The others carry on as normal.
	throw(t, g, bob, "1")
	if got := state(t, g, bob).Target; got != number(2) {
		t.Errorf("bob target = %v, want 2", got)
	}
	if g.Finished() {
		t.Error("one player finishing must not end the game")
	}
}

// finish walks a player all the way round with single hits: 22 darts.
func finish(t *testing.T, g *Game, id int64) {
	t.Helper()
	for !state(t, g, id).Finished {
		throw(t, g, id, "1")
	}
}

func TestCompletedRunKeepsItsDartCount(t *testing.T) {
	g := newGame(t, alice)
	finish(t, g, alice)

	st := state(t, g, alice)
	if st.FinishDarts != TargetCount {
		t.Errorf("finish darts = %d, want %d", st.FinishDarts, TargetCount)
	}
	if !st.Target.Done() {
		t.Errorf("target = %v, want finished", st.Target)
	}
	if st.Target.Remaining() != 0 {
		t.Errorf("remaining = %d, want 0", st.Target.Remaining())
	}
}

func TestAllFinished(t *testing.T) {
	g := newGame(t, alice, bob)
	finish(t, g, alice)
	if g.AllFinished() {
		t.Error("only one player has finished")
	}
	if got := g.ActivePlayers(); len(got) != 1 || got[0] != bob {
		t.Errorf("active players = %v, want just bob", got)
	}
	finish(t, g, bob)
	if !g.AllFinished() {
		t.Error("everybody has finished")
	}
}

// --------------------------------------------------------------- results

func TestResultsRankByFewestDarts(t *testing.T) {
	g := newGame(t, alice, bob, carol)

	// Alice goes round in 22 single hits.
	finish(t, g, alice)
	// Bob wastes a visit first, so he takes three more darts.
	throw(t, g, bob, "000")
	finish(t, g, bob)
	// Carol throws once and stops.
	throw(t, g, carol, "1")

	results := g.Results()
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}
	if results[0].PlayerID != alice || !results[0].Completed {
		t.Errorf("first = %+v, want alice completed", results[0])
	}
	if results[1].PlayerID != bob || results[1].Darts <= results[0].Darts {
		t.Errorf("second = %+v, want bob with more darts", results[1])
	}
	if results[2].PlayerID != carol || results[2].Completed {
		t.Errorf("third = %+v, want carol incomplete", results[2])
	}
	// An unfinished player has no numeric result to compare, only how far they
	// got.
	if results[2].Remaining != TargetCount-1 {
		t.Errorf("carol remaining = %d, want %d", results[2].Remaining, TargetCount-1)
	}
}

func TestResultsIncludePlayersWhoNeverThrew(t *testing.T) {
	g := newGame(t, alice, bob)
	finish(t, g, alice)

	results := g.Results()
	var found bool
	for _, r := range results {
		if r.PlayerID != bob {
			continue
		}
		found = true
		if r.Completed || r.Darts != 0 || r.Visits != 0 {
			t.Errorf("bob = %+v, want an honest zero", r)
		}
		if r.Remaining != TargetCount {
			t.Errorf("bob remaining = %d, want %d", r.Remaining, TargetCount)
		}
	}
	if !found {
		t.Error("a player who never threw was dropped from the results")
	}
}

// Unfinished players are ranked by how far round they got, never by darts
// alone — thirty darts spent reaching target 12 is worse than three spent
// reaching target 4.
func TestUnfinishedPlayersRankByProgress(t *testing.T) {
	g := newGame(t, alice, bob)
	throw(t, g, alice, "333") // target 10, 3 darts
	throw(t, g, bob, "100")   // target 2, 3 darts

	results := g.Results()
	if results[0].PlayerID != alice {
		t.Errorf("first = %d, want alice, who got further", results[0].PlayerID)
	}
}

func TestSuggestedWinnerIsTheFewestDarts(t *testing.T) {
	g := newGame(t, alice, bob, carol)

	// Bob wastes three darts, so Alice finishes in fewer.
	throw(t, g, bob, "000")
	finish(t, g, bob)
	finish(t, g, alice)
	throw(t, g, carol, "1") // never finishes

	winners := g.SuggestWinners()
	if len(winners) != 1 || winners[0] != alice {
		t.Fatalf("suggested %v, want just alice", winners)
	}
}

// Finishing first says nothing when players throw in any order.
func TestSuggestedWinnerIgnoresWhoFinishedFirst(t *testing.T) {
	g := newGame(t, alice, bob)
	throw(t, g, alice, "000") // alice starts badly
	finish(t, g, bob)         // bob finishes first, in 22
	finish(t, g, alice)       // alice finishes later, in 25

	if state(t, g, bob).FinishSeq >= state(t, g, alice).FinishSeq {
		t.Fatal("this test needs bob to have finished first")
	}
	winners := g.SuggestWinners()
	if len(winners) != 1 || winners[0] != bob {
		t.Fatalf("suggested %v, want bob — fewest darts, and he also happens to be first", winners)
	}

	// And the other way round: alice finishes first but takes more darts.
	g2 := newGame(t, alice, bob)
	finish(t, g2, alice)
	throw(t, g2, bob, "3") // a flying start
	finish(t, g2, bob)
	if got := g2.SuggestWinners(); len(got) != 1 || got[0] != bob {
		t.Errorf("suggested %v, want bob — he used fewer darts despite finishing second", got)
	}
}

func TestEqualDartsAreATie(t *testing.T) {
	g := newGame(t, alice, bob)
	finish(t, g, alice)
	finish(t, g, bob)

	if a, b := state(t, g, alice).FinishDarts, state(t, g, bob).FinishDarts; a != b {
		t.Fatalf("this test needs equal dart counts, got %d and %d", a, b)
	}
	winners := g.SuggestWinners()
	if len(winners) != 2 {
		t.Fatalf("suggested %v, want both players tied", winners)
	}
}

func TestNoWinnerIsSuggestedWhenNobodyFinished(t *testing.T) {
	g := newGame(t, alice, bob)
	throw(t, g, alice, "111")
	if got := g.SuggestWinners(); len(got) != 0 {
		t.Errorf("suggested %v, want nobody", got)
	}
}

// --------------------------------------------------------------- finishing

func TestFinishWithAWinner(t *testing.T) {
	g := newGame(t, alice, bob)
	finish(t, g, alice)

	if err := g.Finish([]int64{alice}); err != nil {
		t.Fatal(err)
	}
	if !g.Finished() {
		t.Error("the game should be finished")
	}
	if w := g.Winners(); len(w) != 1 || w[0] != alice {
		t.Errorf("winners = %v, want alice", w)
	}
	for _, r := range g.Results() {
		if r.PlayerID == alice && !r.Won {
			t.Error("alice's result should be marked as won")
		}
		if r.PlayerID == bob && r.Won {
			t.Error("bob's result should not be marked as won")
		}
	}
}

func TestFinishWithATie(t *testing.T) {
	g := newGame(t, alice, bob)
	finish(t, g, alice)
	finish(t, g, bob)

	if err := g.Finish([]int64{alice, bob}); err != nil {
		t.Fatal(err)
	}
	won := 0
	for _, r := range g.Results() {
		if r.Won {
			won++
		}
	}
	if won != 2 {
		t.Errorf("%d results marked as won, want 2", won)
	}
}

// The office stopping early is a normal outcome, not a failure.
func TestFinishEarlyWithNoWinner(t *testing.T) {
	g := newGame(t, alice, bob, carol)
	throw(t, g, alice, "111")
	// Bob and Carol never throw at all.

	if err := g.Finish(nil); err != nil {
		t.Fatal(err)
	}
	if !g.Finished() {
		t.Error("the game should be finished")
	}
	if len(g.Winners()) != 0 {
		t.Errorf("winners = %v, want none", g.Winners())
	}
	if len(g.Results()) != 3 {
		t.Error("every selected player should still have a result")
	}
}

func TestFinishEarlyWithSomeCompleteAndSomeNot(t *testing.T) {
	g := newGame(t, alice, bob, carol)
	finish(t, g, alice)
	throw(t, g, bob, "11")
	// Carol never throws.

	if err := g.Finish([]int64{alice}); err != nil {
		t.Fatal(err)
	}
	results := g.Results()
	if !results[0].Completed || results[0].PlayerID != alice {
		t.Errorf("first result = %+v, want alice completed", results[0])
	}
	if results[1].Completed || results[2].Completed {
		t.Error("bob and carol did not complete their runs")
	}
}

func TestFinishRejectsAnOutsider(t *testing.T) {
	g := newGame(t, alice, bob)
	if err := g.Finish([]int64{carol}); !errors.Is(err, ErrWinnerNotPlaying) {
		t.Errorf("error = %v, want ErrWinnerNotPlaying", err)
	}
	if g.Finished() {
		t.Error("a refused finish must leave the game open")
	}
}

func TestRecordingAfterTheGameIsFinished(t *testing.T) {
	g := newGame(t, alice)
	if err := g.Finish(nil); err != nil {
		t.Fatal(err)
	}
	_, err := g.Record(VisitInput{PlayerID: alice, Calls: []Call{CallSingle}})
	if !errors.Is(err, ErrGameFinished) {
		t.Errorf("error = %v, want ErrGameFinished", err)
	}
}

// ------------------------------------------------------------- training

func TestSoloGameIsTraining(t *testing.T) {
	if !newGame(t, alice).Training() {
		t.Error("one player is a training session")
	}
	if newGame(t, alice, bob).Training() {
		t.Error("two players is a contest")
	}
}

func TestTrainingGameHasNoWinner(t *testing.T) {
	g := newGame(t, alice)
	finish(t, g, alice)

	if got := g.SuggestWinners(); len(got) != 0 {
		t.Errorf("suggested %v, want nobody in a training game", got)
	}
	if err := g.Finish([]int64{alice}); !errors.Is(err, ErrTrainingNoWinner) {
		t.Fatalf("error = %v, want ErrTrainingNoWinner", err)
	}
	if err := g.Finish(nil); err != nil {
		t.Fatal(err)
	}
	// The run itself is still recorded in full.
	r := g.Results()[0]
	if !r.Completed || r.Darts != TargetCount || r.Won {
		t.Errorf("training result = %+v, want a completed run with no win", r)
	}
}

// ------------------------------------------------------------------ undo

func TestUndoANormalVisit(t *testing.T) {
	g := newGame(t, alice)
	throw(t, g, alice, "111")
	throw(t, g, alice, "100")

	before := state(t, g, alice)
	if before.Target != number(5) || before.Darts != 6 {
		t.Fatalf("setup wrong: %+v", before)
	}

	if _, err := g.Undo(); err != nil {
		t.Fatal(err)
	}
	st := state(t, g, alice)
	if st.Target != number(4) {
		t.Errorf("target = %v, want 4", st.Target)
	}
	if st.Darts != 3 || st.Visits != 1 {
		t.Errorf("darts/visits = %d/%d, want 3/1", st.Darts, st.Visits)
	}
	if g.VisitCount() != 1 {
		t.Errorf("visit count = %d, want 1", g.VisitCount())
	}
}

func TestUndoATrebleSkip(t *testing.T) {
	g := newGame(t, alice)
	throw(t, g, alice, "1") // → 2
	throw(t, g, alice, "3") // → 5

	if _, err := g.Undo(); err != nil {
		t.Fatal(err)
	}
	wantTarget(t, g, alice, number(2))
	if st := state(t, g, alice); st.Trebles != 0 {
		t.Errorf("trebles = %d, want 0 after undo", st.Trebles)
	}
}

func TestUndoReachingTheOuterBull(t *testing.T) {
	g := newGame(t, alice)
	advanceTo(t, g, alice, number(20))
	throw(t, g, alice, "1") // → outer bull
	wantTarget(t, g, alice, outer)

	if _, err := g.Undo(); err != nil {
		t.Fatal(err)
	}
	wantTarget(t, g, alice, number(20))
}

func TestUndoReachingTheBullseye(t *testing.T) {
	g := newGame(t, alice)
	advanceTo(t, g, alice, outer)
	throw(t, g, alice, "1") // → bullseye
	wantTarget(t, g, alice, bull)

	if _, err := g.Undo(); err != nil {
		t.Fatal(err)
	}
	wantTarget(t, g, alice, outer)
}

// Undoing the visit that completed a run has to put the player back in the
// game, dart count and all.
func TestUndoAFinishedRun(t *testing.T) {
	g := newGame(t, alice, bob)
	advanceTo(t, g, alice, bull)
	dartsBefore := state(t, g, alice).Darts

	throw(t, g, alice, "02")
	if !state(t, g, alice).Finished {
		t.Fatal("the run should have completed")
	}

	if _, err := g.Undo(); err != nil {
		t.Fatal(err)
	}
	st := state(t, g, alice)
	if st.Finished {
		t.Error("the player should be active again")
	}
	if st.Target != bull {
		t.Errorf("target = %v, want the bullseye", st.Target)
	}
	if st.Darts != dartsBefore {
		t.Errorf("darts = %d, want %d", st.Darts, dartsBefore)
	}
	if st.FinishDarts != 0 || st.FinishSeq != 0 {
		t.Errorf("completion figures survived the undo: %+v", st)
	}
	if got := g.SuggestWinners(); len(got) != 0 {
		t.Errorf("suggested %v after undoing the only finish, want nobody", got)
	}

	// And they can throw again.
	throw(t, g, alice, "2")
	if !state(t, g, alice).Finished {
		t.Error("the player should be able to finish again")
	}
}

func TestUndoOnlyTouchesTheLastVisit(t *testing.T) {
	g := newGame(t, alice, bob)
	throw(t, g, alice, "111")
	throw(t, g, bob, "22")
	throw(t, g, alice, "1")

	if _, err := g.Undo(); err != nil {
		t.Fatal(err)
	}
	if got := state(t, g, alice).Target; got != number(4) {
		t.Errorf("alice target = %v, want 4", got)
	}
	if got := state(t, g, bob).Target; got != number(5) {
		t.Errorf("bob target = %v, want 5 — his visit was not touched", got)
	}
}

func TestNothingToUndo(t *testing.T) {
	g := newGame(t, alice)
	if _, err := g.Undo(); !errors.Is(err, ErrNothingToUndo) {
		t.Errorf("error = %v, want ErrNothingToUndo", err)
	}
}

// ---------------------------------------------------------------- replay

// A restored game must agree with the one that was saved, dart for dart.
func TestRestoreReplaysTheLog(t *testing.T) {
	g := newGame(t, alice, bob, carol)
	throw(t, g, alice, "031")
	throw(t, g, bob, "000")
	throw(t, g, alice, "23")
	throw(t, g, carol, "1")

	restored, err := Restore(g.Order(), g.Visits(), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range g.Order() {
		want := state(t, g, id)
		got := state(t, restored, id)
		if got != want {
			t.Errorf("player %d restored as %+v, want %+v", id, got, want)
		}
	}
	if restored.VisitCount() != g.VisitCount() {
		t.Errorf("visit count = %d, want %d", restored.VisitCount(), g.VisitCount())
	}
}

func TestRestoreAFinishedGame(t *testing.T) {
	g := newGame(t, alice, bob)
	finish(t, g, alice)
	if err := g.Finish([]int64{alice}); err != nil {
		t.Fatal(err)
	}

	restored, err := Restore(g.Order(), g.Visits(), true, g.Winners())
	if err != nil {
		t.Fatal(err)
	}
	if !restored.Finished() {
		t.Error("the restored game should be finished")
	}
	if w := restored.Winners(); len(w) != 1 || w[0] != alice {
		t.Errorf("winners = %v, want alice", w)
	}
	if !state(t, restored, alice).Finished {
		t.Error("alice's completed run did not survive the round trip")
	}
}

// A log holding a dart that could not have been thrown at its target is
// reported rather than quietly scored as something else.
func TestRestoreRejectsACorruptLog(t *testing.T) {
	visits := []Visit{{
		Seq:      1,
		PlayerID: alice,
		// The player is on 1; a treble 19 is not a dart they could have thrown.
		Darts: []Dart{HitOn(darts.T(19))},
	}}
	if _, err := Restore([]int64{alice}, visits, false, nil); !errors.Is(err, ErrCorruptLog) {
		t.Errorf("error = %v, want ErrCorruptLog", err)
	}
}

// ------------------------------------------------------------ construction

func TestNewRejectsBadSelections(t *testing.T) {
	if _, err := New(nil); !errors.Is(err, ErrNoPlayers) {
		t.Errorf("error = %v, want ErrNoPlayers", err)
	}
	if _, err := New([]int64{alice, alice}); !errors.Is(err, ErrDuplicatePlayer) {
		t.Errorf("error = %v, want ErrDuplicatePlayer", err)
	}
}

func TestPreviewDoesNotRecord(t *testing.T) {
	g := newGame(t, alice)
	v, err := g.Preview(VisitInput{PlayerID: alice, Calls: []Call{CallMiss, CallTreble, CallSingle}})
	if err != nil {
		t.Fatal(err)
	}
	if v.TargetAfter != number(5) {
		t.Errorf("preview target = %v, want 5", v.TargetAfter)
	}
	if g.VisitCount() != 0 {
		t.Errorf("preview recorded %d visits, want 0", g.VisitCount())
	}
	if state(t, g, alice).Target != number(1) {
		t.Error("preview must not move the player")
	}
}

func TestStatesAndVisitAccessors(t *testing.T) {
	g := newGame(t, alice, bob)
	v := throw(t, g, alice, "031")

	if v.DartCount() != 3 {
		t.Errorf("DartCount() = %d, want 3", v.DartCount())
	}

	states := g.States()
	if len(states) != 2 {
		t.Fatalf("States() = %d, want 2", len(states))
	}
	// Selection order, not any ranking.
	if states[0].PlayerID != alice || states[1].PlayerID != bob {
		t.Errorf("States() = %v, want alice then bob", states)
	}
	if states[0].Target != number(5) {
		t.Errorf("alice's state = %v, want target 5", states[0].Target)
	}
}

func TestFinishRejectsTheSameWinnerTwice(t *testing.T) {
	g := newGame(t, alice, bob)
	finish(t, g, alice)
	if err := g.Finish([]int64{alice, alice}); !errors.Is(err, ErrDuplicatePlayer) {
		t.Errorf("error = %v, want ErrDuplicatePlayer", err)
	}
	if g.Finished() {
		t.Error("a refused finish must leave the game open")
	}
}

func TestUndoIsRefusedOnAFinishedGame(t *testing.T) {
	g := newGame(t, alice)
	throw(t, g, alice, "1")
	if err := g.Finish(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Undo(); !errors.Is(err, ErrGameFinished) {
		t.Errorf("error = %v, want ErrGameFinished", err)
	}
}

// A log naming somebody who is not in the game is skipped rather than guessed
// at, so one bad row cannot take the rest of the game with it.
func TestReplaySkipsAVisitByAnOutsider(t *testing.T) {
	visits := []Visit{
		{Seq: 1, PlayerID: alice, Darts: []Dart{HitOn(darts.S(1))}},
		{Seq: 2, PlayerID: carol, Darts: []Dart{HitOn(darts.S(1))}},
		{Seq: 3, PlayerID: alice, Darts: []Dart{HitOn(darts.S(2))}},
	}
	g, err := Restore([]int64{alice}, visits, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := state(t, g, alice).Target; got != number(3) {
		t.Errorf("alice target = %v, want 3", got)
	}
	if got := state(t, g, alice).Darts; got != 2 {
		t.Errorf("alice darts = %d, want 2", got)
	}
}

// Storage holding a dart thrown after a run was complete is dropped, not
// counted: it cannot have happened.
func TestReplayDropsDartsAfterCompletion(t *testing.T) {
	g := newGame(t, alice)
	advanceTo(t, g, alice, bull)
	throw(t, g, alice, "2")

	visits := g.Visits()
	last := &visits[len(visits)-1]
	last.Darts = append(last.Darts, HitOn(darts.Bull()))

	restored, err := Restore(g.Order(), visits, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	st := state(t, restored, alice)
	if st.Darts != TargetCount {
		t.Errorf("darts = %d, want %d — the extra dart must not be counted", st.Darts, TargetCount)
	}
}
