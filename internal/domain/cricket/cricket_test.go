package cricket

import (
	"errors"
	"math"
	"testing"
)

// Team A: Alice, Bob. Team B: Charlie, David, Eve. Deliberately unequal.
const (
	alice   int64 = 1
	bob     int64 = 2
	charlie int64 = 3
	david   int64 = 4
	eve     int64 = 5
)

func teams() (Team, Team) {
	return Team{Name: "Team A", Players: []int64{alice, bob}},
		Team{Name: "Team B", Players: []int64{charlie, david, eve}}
}

func newGame(t *testing.T, wickets int, firstBatting Side) *Game {
	t.Helper()
	a, b := teams()
	g, err := New(Config{WicketLimit: wickets, FirstBatting: firstBatting}, a, b)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return g
}

// bat records a batting visit for whoever is due to bat.
func bat(t *testing.T, g *Game, raw, dartCount int) Event {
	t.Helper()
	turn, ok := g.CurrentTurn()
	if !ok {
		t.Fatal("game is complete; no batting visit possible")
	}
	if turn.Kind != Batting {
		t.Fatalf("expected a batting turn, got %v", turn.Kind)
	}
	e, err := g.RecordBatting(turn.PlayerID, raw, dartCount)
	if err != nil {
		t.Fatalf("RecordBatting(%d, %d): %v", raw, dartCount, err)
	}
	return e
}

// bowl records a bowling visit for whoever is due to bowl.
func bowl(t *testing.T, g *Game, notation string) Event {
	t.Helper()
	turn, ok := g.CurrentTurn()
	if !ok {
		t.Fatal("game is complete; no bowling visit possible")
	}
	if turn.Kind != Bowling {
		t.Fatalf("expected a bowling turn, got %v", turn.Kind)
	}
	bowls, err := ParseBowls(notation)
	if err != nil {
		t.Fatalf("ParseBowls(%q): %v", notation, err)
	}
	e, err := g.RecordBowling(turn.PlayerID, bowls)
	if err != nil {
		t.Fatalf("RecordBowling(%q): %v", notation, err)
	}
	return e
}

// ---------------------------------------------------------------- setup

func TestNewValidation(t *testing.T) {
	a, b := teams()
	cases := []struct {
		name    string
		cfg     Config
		a, b    Team
		wantErr error
	}{
		{"zero wickets", Config{WicketLimit: 0}, a, b, ErrBadWicketLimit},
		{"negative wickets", Config{WicketLimit: -3}, a, b, ErrBadWicketLimit},
		{"absurd wickets", Config{WicketLimit: MaxWickets + 1}, a, b, ErrBadWicketLimit},
		{"empty team A", Config{WicketLimit: 5}, Team{}, b, ErrEmptyTeam},
		{"empty team B", Config{WicketLimit: 5}, a, Team{}, ErrEmptyTeam},
		{
			"player on both teams",
			Config{WicketLimit: 5},
			Team{Players: []int64{alice}},
			Team{Players: []int64{alice}},
			ErrDuplicatePlayer,
		},
		{
			"player twice in one team",
			Config{WicketLimit: 5},
			Team{Players: []int64{alice, alice}},
			b,
			ErrDuplicatePlayer,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.cfg, tc.a, tc.b); !errors.Is(err, tc.wantErr) {
				t.Errorf("got %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestNewAcceptsSinglePlayerTeams(t *testing.T) {
	g, err := New(
		Config{WicketLimit: 3, FirstBatting: TeamA},
		Team{Players: []int64{alice}},
		Team{Players: []int64{bob}},
	)
	if err != nil {
		t.Fatalf("one-player teams should be legal: %v", err)
	}
	turn, _ := g.CurrentTurn()
	if turn.PlayerID != alice {
		t.Errorf("first batter = %d, want alice", turn.PlayerID)
	}
}

func TestDefaultTeamNames(t *testing.T) {
	g, err := New(Config{WicketLimit: 3}, Team{Players: []int64{alice}}, Team{Players: []int64{bob}})
	if err != nil {
		t.Fatal(err)
	}
	if g.Team(TeamA).Name != "Team A" || g.Team(TeamB).Name != "Team B" {
		t.Errorf("default names = %q / %q", g.Team(TeamA).Name, g.Team(TeamB).Name)
	}
}

// ---------------------------------------------------------------- batting

// The headline batting rule: 40 or below is nothing, every point above is a run.
func TestRunsFor(t *testing.T) {
	cases := map[int]int{
		0:   0,
		26:  0,
		39:  0,
		40:  0,  // exactly the threshold still scores nothing
		41:  1,  // the first scoring point
		57:  17, // worked example from the house rules
		60:  20,
		100: 60, // worked example from the house rules
		140: 100,
		180: 140,
	}
	for raw, want := range cases {
		if got := RunsFor(raw); got != want {
			t.Errorf("RunsFor(%d) = %d, want %d", raw, got, want)
		}
	}
}

func TestBattingRecordsRawAndRuns(t *testing.T) {
	g := newGame(t, 5, TeamA)
	e := bat(t, g, 57, 3)
	if e.RawPoints != 57 {
		t.Errorf("RawPoints = %d, want 57", e.RawPoints)
	}
	if e.Runs != 17 {
		t.Errorf("Runs = %d, want 17", e.Runs)
	}
	if e.Darts != 3 {
		t.Errorf("Darts = %d, want 3", e.Darts)
	}
	if g.CurrentInnings().Runs != 17 {
		t.Errorf("innings runs = %d, want 17", g.CurrentInnings().Runs)
	}
}

func TestBattingBelowThresholdScoresNothing(t *testing.T) {
	g := newGame(t, 5, TeamA)
	e := bat(t, g, 26, 3)
	if e.Runs != 0 {
		t.Errorf("26 scored %d runs, want 0", e.Runs)
	}
	if e.RawPoints != 26 {
		t.Errorf("raw points must still be stored, got %d", e.RawPoints)
	}
	if g.CurrentInnings().Runs != 0 {
		t.Errorf("innings runs = %d, want 0", g.CurrentInnings().Runs)
	}
}

func TestBattingSupportsFewerThanThreeDarts(t *testing.T) {
	g := newGame(t, 5, TeamA)
	e := bat(t, g, 60, 1)
	if e.Darts != 1 {
		t.Errorf("Darts = %d, want 1", e.Darts)
	}
	if e.Runs != 20 {
		t.Errorf("Runs = %d, want 20", e.Runs)
	}
}

func TestBattingRejectsImpossibleInput(t *testing.T) {
	g := newGame(t, 5, TeamA)
	turn, _ := g.CurrentTurn()
	cases := []struct {
		name      string
		raw, nDar int
		wantErr   error
	}{
		{"negative score", -1, 3, ErrImpossibleScore},
		{"over 180", 181, 3, ErrImpossibleScore},
		{"impossible 179", 179, 3, ErrImpossibleScore},
		{"too high for one dart", 61, 1, ErrImpossibleScore},
		{"zero darts", 60, 0, ErrBadDartCount},
		{"four darts", 60, 4, ErrBadDartCount},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := g.RecordBatting(turn.PlayerID, tc.raw, tc.nDar); !errors.Is(err, tc.wantErr) {
				t.Errorf("got %v, want %v", err, tc.wantErr)
			}
		})
	}
	if g.EventCount() != 0 {
		t.Errorf("rejected input recorded %d events, want 0", g.EventCount())
	}
}

// ---------------------------------------------------------------- bowling

func TestBowlWickets(t *testing.T) {
	if Miss.Wickets() != 0 {
		t.Errorf("a miss takes %d wickets, want 0", Miss.Wickets())
	}
	if Outer.Wickets() != 1 {
		t.Errorf("outer bull takes %d wickets, want 1", Outer.Wickets())
	}
	if Inner.Wickets() != 2 {
		t.Errorf("inner bull takes %d wickets, want 2", Inner.Wickets())
	}
}

func TestParseBowls(t *testing.T) {
	got, err := ParseBowls("moB")
	if err != nil {
		t.Fatal(err)
	}
	want := []Bowl{Miss, Outer, Inner}
	if len(got) != len(want) {
		t.Fatalf("got %d darts, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("dart %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestParseBowlsRejectsBadNotation(t *testing.T) {
	cases := []struct {
		in      string
		wantErr error
	}{
		{"", ErrNoDarts},
		{"MOBM", ErrTooManyDarts},
		{"X", ErrBadBowlNotation},
		{"MOX", ErrBadBowlNotation},
		{"0", ErrBadBowlNotation},
		{"4", ErrBadBowlNotation},
		{"1234", ErrTooManyDarts},
	}
	for _, tc := range cases {
		if _, err := ParseBowls(tc.in); !errors.Is(err, tc.wantErr) {
			t.Errorf("ParseBowls(%q) = %v, want %v", tc.in, err, tc.wantErr)
		}
	}
}

func TestBowlingCountsBullsSeparately(t *testing.T) {
	g := newGame(t, 10, TeamA)
	bat(t, g, 45, 3)
	e := bowl(t, g, "MOB")
	if e.Wickets != 3 {
		t.Errorf("Wickets = %d, want 3 (one outer + one inner)", e.Wickets)
	}
	if e.OuterBulls() != 1 {
		t.Errorf("OuterBulls = %d, want 1", e.OuterBulls())
	}
	if e.InnerBulls() != 1 {
		t.Errorf("InnerBulls = %d, want 1", e.InnerBulls())
	}
	if e.Darts != 3 {
		t.Errorf("Darts = %d, want 3", e.Darts)
	}
}

// When the wicket limit is hit part-way through a visit, only the darts that
// were actually thrown are recorded.
func TestBowlingTruncatesAtWicketLimit(t *testing.T) {
	g := newGame(t, 2, TeamA)
	bat(t, g, 50, 3)
	e := bowl(t, g, "OOM") // the second dart reaches the two-wicket limit
	if e.Darts != 2 {
		t.Errorf("Darts = %d, want 2 — the third dart was never thrown", e.Darts)
	}
	if len(e.Bowls) != 2 {
		t.Errorf("stored %d darts, want 2", len(e.Bowls))
	}
	if e.Wickets != 2 {
		t.Errorf("Wickets = %d, want 2", e.Wickets)
	}
	if g.InningsNumber() != 2 {
		t.Errorf("innings = %d, want 2 — the innings should have ended", g.InningsNumber())
	}
}

func TestBowlingTruncatesOnInnerBullOverrun(t *testing.T) {
	// Limit 3, one wicket already down; an inner bull takes two and ends it.
	g := newGame(t, 3, TeamA)
	bat(t, g, 50, 3)
	bowl(t, g, "OMM") // one wicket
	bat(t, g, 50, 3)
	e := bowl(t, g, "BOM") // inner bull on the first dart ends the innings
	if e.Darts != 1 {
		t.Errorf("Darts = %d, want 1", e.Darts)
	}
	if e.Wickets != 2 {
		t.Errorf("Wickets = %d, want 2", e.Wickets)
	}
	if g.InningsNumber() != 2 {
		t.Errorf("innings = %d, want 2", g.InningsNumber())
	}
}

func TestBowlingRejectsBadDartCounts(t *testing.T) {
	g := newGame(t, 5, TeamA)
	bat(t, g, 45, 3)
	turn, _ := g.CurrentTurn()
	if _, err := g.RecordBowling(turn.PlayerID, nil); !errors.Is(err, ErrNoDarts) {
		t.Errorf("no darts: got %v, want ErrNoDarts", err)
	}
	four := []Bowl{Miss, Miss, Miss, Miss}
	if _, err := g.RecordBowling(turn.PlayerID, four); !errors.Is(err, ErrTooManyDarts) {
		t.Errorf("four darts: got %v, want ErrTooManyDarts", err)
	}
}

// ---------------------------------------------------------------- turn order

// The batter always throws first.
func TestBatterThrowsFirst(t *testing.T) {
	for _, first := range []Side{TeamA, TeamB} {
		g := newGame(t, 5, first)
		turn, ok := g.CurrentTurn()
		if !ok {
			t.Fatal("a new game must have a turn")
		}
		if turn.Kind != Batting {
			t.Errorf("first turn kind = %v, want Batting", turn.Kind)
		}
		if turn.Side != first {
			t.Errorf("first turn side = %v, want %v", turn.Side, first)
		}
	}
}

func TestBattingAndBowlingAlternate(t *testing.T) {
	g := newGame(t, 20, TeamA)
	for i := 0; i < 6; i++ {
		turn, _ := g.CurrentTurn()
		wantKind := Batting
		if i%2 == 1 {
			wantKind = Bowling
		}
		if turn.Kind != wantKind {
			t.Fatalf("visit %d kind = %v, want %v", i, turn.Kind, wantKind)
		}
		if wantKind == Batting {
			bat(t, g, 45, 3)
		} else {
			bowl(t, g, "MMM")
		}
	}
}

// The worked example from the house rules: Team A (Alice, Bob) batting against
// Team B (Charlie, David, Eve) bowling, with each team cycling independently.
func TestPlayerCyclingWithUnequalTeams(t *testing.T) {
	g := newGame(t, 20, TeamA)
	want := []struct {
		player int64
		kind   Kind
	}{
		{alice, Batting},
		{charlie, Bowling},
		{bob, Batting},
		{david, Bowling},
		{alice, Batting},
		{eve, Bowling},
		{bob, Batting},
		{charlie, Bowling}, // Team B has wrapped back to the top of its order
	}
	for i, w := range want {
		turn, ok := g.CurrentTurn()
		if !ok {
			t.Fatalf("visit %d: game unexpectedly complete", i)
		}
		if turn.PlayerID != w.player || turn.Kind != w.kind {
			t.Fatalf("visit %d: got player %d %v, want player %d %v",
				i, turn.PlayerID, turn.Kind, w.player, w.kind)
		}
		if w.kind == Batting {
			bat(t, g, 45, 3)
		} else {
			bowl(t, g, "MMM")
		}
	}
}

// Cricket, unlike X01, enforces order.
func TestOutOfTurnVisitsRejected(t *testing.T) {
	g := newGame(t, 5, TeamA)

	// Bob cannot bat before Alice.
	if _, err := g.RecordBatting(bob, 45, 3); !errors.Is(err, ErrWrongPlayer) {
		t.Errorf("wrong batter: got %v, want ErrWrongPlayer", err)
	}
	// A bowling visit is not what the game is waiting for.
	if _, err := g.RecordBowling(charlie, []Bowl{Miss}); !errors.Is(err, ErrNotBowling) {
		t.Errorf("bowling first: got %v, want ErrNotBowling", err)
	}

	bat(t, g, 45, 3)

	// Now the game wants a bowler, not a batter.
	if _, err := g.RecordBatting(bob, 45, 3); !errors.Is(err, ErrNotBatting) {
		t.Errorf("batting twice: got %v, want ErrNotBatting", err)
	}
	// And it must be Charlie, not David.
	if _, err := g.RecordBowling(david, []Bowl{Miss}); !errors.Is(err, ErrWrongPlayer) {
		t.Errorf("wrong bowler: got %v, want ErrWrongPlayer", err)
	}
}

// ---------------------------------------------------------------- innings

// inningsOf plays a complete innings: the side batting makes one visit worth
// raw points, then is bowled out with blank visits in between.
func inningsOf(t *testing.T, g *Game, raw int) {
	t.Helper()
	bat(t, g, raw, 3)
	number := g.InningsNumber()
	for !g.Complete() && g.InningsNumber() == number {
		turn, ok := g.CurrentTurn()
		if !ok {
			return
		}
		if turn.Kind == Batting {
			bat(t, g, 0, 3) // a blank visit, so only the first visit scores
		} else {
			bowl(t, g, "3") // inner bull: two wickets
		}
	}
}

func TestWicketLimitEndsInnings(t *testing.T) {
	g := newGame(t, 3, TeamA)
	bat(t, g, 100, 3) // 60 runs
	bowl(t, g, "2")   // 1 wicket
	bat(t, g, 50, 3)  // 10 runs
	bowl(t, g, "2")   // 2 wickets

	if g.InningsNumber() != 1 {
		t.Fatalf("innings ended early at %d wickets", g.CurrentInnings().Wickets)
	}
	bat(t, g, 45, 3) // 5 runs, total 75
	bowl(t, g, "2")  // 3 wickets — innings over

	if g.InningsNumber() != 2 {
		t.Fatalf("innings = %d, want 2", g.InningsNumber())
	}
	first := g.Innings()[0]
	if !first.Complete {
		t.Error("first innings should be marked complete")
	}
	if first.Runs != 75 {
		t.Errorf("first innings runs = %d, want 75", first.Runs)
	}
	if first.Wickets != 3 {
		t.Errorf("first innings wickets = %d, want 3", first.Wickets)
	}
	if first.BattingSide != TeamA {
		t.Errorf("first innings batting side = %v, want TeamA", first.BattingSide)
	}
}

// A match is two legs of two innings each.
func TestLegNumbering(t *testing.T) {
	cases := map[int]int{1: 1, 2: 1, 3: 2, 4: 2}
	for number, wantLeg := range cases {
		if got := (Innings{Number: number}).Leg(); got != wantLeg {
			t.Errorf("innings %d is in leg %d, want %d", number, got, wantLeg)
		}
	}
}

func TestTeamsSwapForTheSecondInnings(t *testing.T) {
	g := newGame(t, 1, TeamA)
	bat(t, g, 100, 3)
	bowl(t, g, "3") // 2 wickets, past a 1-wicket limit: innings over

	if g.InningsNumber() != 2 {
		t.Fatalf("innings = %d, want 2", g.InningsNumber())
	}
	if g.Leg() != 1 {
		t.Errorf("leg = %d, want 1 — innings 2 is still leg 1", g.Leg())
	}
	inn := g.CurrentInnings()
	if inn.BattingSide != TeamB {
		t.Errorf("second innings batting side = %v, want TeamB", inn.BattingSide)
	}
	if inn.Runs != 0 || inn.Wickets != 0 {
		t.Errorf("second innings should start fresh, got %+v", inn)
	}

	turn, ok := g.CurrentTurn()
	if !ok {
		t.Fatal("second innings should have a turn")
	}
	// Team B bats, starting from the top of its own order.
	if turn.Kind != Batting || turn.PlayerID != charlie {
		t.Errorf("second innings opens with %v by %d, want Batting by charlie", turn.Kind, turn.PlayerID)
	}

	// And Team A bowls, also from the top of its order.
	bat(t, g, 26, 3)
	turn, _ = g.CurrentTurn()
	if turn.PlayerID != alice {
		t.Errorf("second innings first bowler = %d, want alice", turn.PlayerID)
	}
}

// Leg 1 is played out in full. Passing the other side's score does not end an
// innings, because the match continues into leg 2 regardless.
func TestLegOneIsPlayedOutInFull(t *testing.T) {
	g := newGame(t, 4, TeamA)
	inningsOf(t, g, 60) // Team A: 20 runs

	if g.InningsNumber() != 2 {
		t.Fatalf("innings = %d, want 2", g.InningsNumber())
	}

	// Team B passes 20 but must keep batting.
	bat(t, g, 100, 3) // 60 runs, already ahead
	if g.Complete() {
		t.Fatal("leg 1 must not end the match, however many runs are scored")
	}
	if g.InningsNumber() != 2 {
		t.Fatalf("innings = %d, want 2 — passing the score must not end the innings", g.InningsNumber())
	}
	if _, ok := g.Target(); ok {
		t.Error("there is no target in leg 1")
	}

	// It ends only on wickets.
	bowl(t, g, "3") // 2 of the 4 wickets
	bat(t, g, 0, 3)
	bowl(t, g, "3") // 4 wickets: bowled out
	if g.InningsNumber() != 3 {
		t.Fatalf("innings = %d, want 3 once Team B is bowled out", g.InningsNumber())
	}
	if g.Leg() != 2 {
		t.Errorf("leg = %d, want 2", g.Leg())
	}
}

func TestAggregateAndLead(t *testing.T) {
	g := newGame(t, 2, TeamA)
	inningsOf(t, g, 100) // A: 60
	inningsOf(t, g, 60)  // B: 20

	if got := g.Aggregate(TeamA); got != 60 {
		t.Errorf("Team A aggregate = %d, want 60", got)
	}
	if got := g.Aggregate(TeamB); got != 20 {
		t.Errorf("Team B aggregate = %d, want 20", got)
	}
	leader, by, ok := g.Lead()
	if !ok || leader != TeamA || by != 40 {
		t.Errorf("Lead() = %v by %d (%v), want TeamA by 40", leader, by, ok)
	}
}

// The side behind on aggregate bats first in leg 2, which leaves the leading
// side batting last.
func TestTrailingSideBatsFirstInLegTwo(t *testing.T) {
	g := newGame(t, 2, TeamA)
	inningsOf(t, g, 100) // A: 60
	inningsOf(t, g, 60)  // B: 20 — B trails

	if g.InningsNumber() != 3 {
		t.Fatalf("innings = %d, want 3", g.InningsNumber())
	}
	if got := g.CurrentInnings().BattingSide; got != TeamB {
		t.Errorf("leg 2 opens with %v batting, want TeamB who are behind", got)
	}
	turn, _ := g.CurrentTurn()
	if turn.PlayerID != charlie {
		t.Errorf("leg 2 first batter = %d, want charlie at the top of Team B's order", turn.PlayerID)
	}
}

// The same rule applies when the side that batted second is ahead: the trailing
// side bats first whichever way round it is.
func TestTrailingSideBatsFirstWhenTheSecondTeamLeads(t *testing.T) {
	g := newGame(t, 2, TeamA)
	inningsOf(t, g, 60)  // A: 20
	inningsOf(t, g, 100) // B: 60 — now A trails

	if got := g.CurrentInnings().BattingSide; got != TeamA {
		t.Errorf("leg 2 opens with %v batting, want TeamA who are behind", got)
	}
}

// Level after leg 1 keeps the original order, so the choice stays predictable.
func TestLevelAfterLegOneKeepsTheOriginalOrder(t *testing.T) {
	g := newGame(t, 2, TeamB) // Team B chosen to bat first
	inningsOf(t, g, 100)      // B: 60
	inningsOf(t, g, 100)      // A: 60 — level

	if _, _, ok := g.Lead(); ok {
		t.Fatal("the sides should be level")
	}
	if got := g.CurrentInnings().BattingSide; got != TeamB {
		t.Errorf("leg 2 opens with %v, want TeamB who batted first", got)
	}
}

// ---------------------------------------------------------------- results

// The headline case: the trailing side is bowled out in leg 2 with a combined
// total still short of the opposition's, so the leaders never bat again.
func TestLeadingSideNeverBatsAgainWhenTheChaseFallsShort(t *testing.T) {
	g := newGame(t, 2, TeamA)
	inningsOf(t, g, 100) // A: 60
	inningsOf(t, g, 60)  // B: 20, B trails
	inningsOf(t, g, 60)  // B again: 20 more, B aggregate 40

	if !g.Complete() {
		t.Fatal("the match should be decided once Team B is bowled out short")
	}
	if g.Result() != ResultTeamA {
		t.Errorf("result = %v, want ResultTeamA", g.Result())
	}
	if got := len(g.Innings()); got != 3 {
		t.Errorf("innings played = %d, want 3 — Team A never needed to bat again", got)
	}
	if g.Aggregate(TeamA) != 60 || g.Aggregate(TeamB) != 40 {
		t.Errorf("aggregates = %d v %d, want 60 v 40", g.Aggregate(TeamA), g.Aggregate(TeamB))
	}
	if _, ok := g.CurrentTurn(); ok {
		t.Error("a decided match must not offer another turn")
	}
}

// If the trailing side overtakes, the leaders must bat the final innings.
func TestLeadingSideBatsLastWhenOvertaken(t *testing.T) {
	g := newGame(t, 2, TeamA)
	inningsOf(t, g, 100) // A: 60
	inningsOf(t, g, 60)  // B: 20
	inningsOf(t, g, 140) // B again: 100 more, B aggregate 120

	if g.Complete() {
		t.Fatal("Team A must get their second innings after being overtaken")
	}
	if g.InningsNumber() != FinalInnings {
		t.Fatalf("innings = %d, want %d", g.InningsNumber(), FinalInnings)
	}
	if got := g.CurrentInnings().BattingSide; got != TeamA {
		t.Errorf("final innings batting side = %v, want TeamA", got)
	}

	target, ok := g.Target()
	if !ok {
		t.Fatal("the final innings should have a target")
	}
	if target != 120 {
		t.Errorf("target = %d, want Team B's aggregate of 120", target)
	}
	needed, _ := g.RunsNeeded()
	if needed != 61 {
		t.Errorf("runs needed = %d, want 61 (120 - 60 + 1)", needed)
	}
}

// Only the final innings is a chase, and it must be won by exceeding the target.
func TestFinalInningsChaseMustExceedTheTarget(t *testing.T) {
	g := newGame(t, 2, TeamA)
	inningsOf(t, g, 100) // A: 60
	inningsOf(t, g, 60)  // B: 20
	inningsOf(t, g, 140) // B: 120 aggregate

	// Team A draws level, which is not enough.
	bat(t, g, 100, 3) // 60 runs, aggregate 120
	if g.Complete() {
		t.Fatal("matching the target must not win the match")
	}
	if g.Aggregate(TeamA) != 120 {
		t.Fatalf("Team A aggregate = %d, want 120", g.Aggregate(TeamA))
	}

	bowl(t, g, "1")  // a miss, so the innings continues
	bat(t, g, 41, 3) // one more run: 121 beats 120

	if !g.Complete() {
		t.Fatal("exceeding the target must end the match immediately")
	}
	if g.Result() != ResultTeamA {
		t.Errorf("result = %v, want ResultTeamA", g.Result())
	}
	if g.Aggregate(TeamA) != 121 {
		t.Errorf("Team A aggregate = %d, want 121", g.Aggregate(TeamA))
	}
}

// Bowled out short in the final innings hands the match to the other side.
func TestFinalInningsBowledOutShort(t *testing.T) {
	g := newGame(t, 2, TeamA)
	inningsOf(t, g, 100) // A: 60
	inningsOf(t, g, 60)  // B: 20
	inningsOf(t, g, 140) // B: 120 aggregate
	inningsOf(t, g, 60)  // A: 20 more, aggregate 80 — short of 120

	if !g.Complete() {
		t.Fatal("the match should be over")
	}
	if g.Result() != ResultTeamB {
		t.Errorf("result = %v, want ResultTeamB", g.Result())
	}
	if got := len(g.Innings()); got != FinalInnings {
		t.Errorf("innings played = %d, want %d", got, FinalInnings)
	}
}

// Bowled out exactly level on aggregate is a draw.
func TestBowledOutLevelOnAggregateIsADraw(t *testing.T) {
	g := newGame(t, 2, TeamA)
	inningsOf(t, g, 100) // A: 60
	inningsOf(t, g, 100) // B: 60 — level, so Team A bat first in leg 2
	inningsOf(t, g, 60)  // A: 20 more, aggregate 80
	inningsOf(t, g, 60)  // B: 20 more, aggregate 80 — level again

	if !g.Complete() {
		t.Fatal("the match should be over")
	}
	if g.Result() != ResultDraw {
		t.Errorf("result = %v, want ResultDraw", g.Result())
	}
	if _, ok := g.WinningSide(); ok {
		t.Error("a draw has no winning side")
	}
	if g.Aggregate(TeamA) != g.Aggregate(TeamB) {
		t.Errorf("aggregates should be level, got %d v %d",
			g.Aggregate(TeamA), g.Aggregate(TeamB))
	}
}

// Drawing level after three innings still gives the other side its innings, so
// a tie can be broken rather than declared early.
func TestLevelAfterThreeInningsStillPlaysTheFourth(t *testing.T) {
	g := newGame(t, 2, TeamA)
	inningsOf(t, g, 100) // A: 60
	inningsOf(t, g, 60)  // B: 20
	inningsOf(t, g, 80)  // B again: 40 more — aggregate 60, exactly level

	if g.Aggregate(TeamA) != g.Aggregate(TeamB) {
		t.Fatalf("precondition: aggregates should be level, got %d v %d",
			g.Aggregate(TeamA), g.Aggregate(TeamB))
	}
	if g.Complete() {
		t.Fatal("level is not a decided match; Team A must still bat")
	}
	if g.InningsNumber() != FinalInnings {
		t.Errorf("innings = %d, want %d", g.InningsNumber(), FinalInnings)
	}
	// Team A bat needing a single run to win.
	needed, _ := g.RunsNeeded()
	if needed != 1 {
		t.Errorf("runs needed = %d, want 1", needed)
	}
}

func TestTargetOnlyExistsInTheFinalInnings(t *testing.T) {
	g := newGame(t, 2, TeamA)
	if _, ok := g.Target(); ok {
		t.Error("no target in the first innings")
	}
	inningsOf(t, g, 100)
	if _, ok := g.Target(); ok {
		t.Error("no target in the second innings — leg 1 is played out in full")
	}
	inningsOf(t, g, 60)
	if _, ok := g.Target(); ok {
		t.Error("no target in the third innings — the leaders still have an innings")
	}
	inningsOf(t, g, 140) // B overtakes, so Team A bat last
	if _, ok := g.Target(); !ok {
		t.Error("the final innings should have a target")
	}
}

func TestWinningSide(t *testing.T) {
	g := newGame(t, 2, TeamA)
	inningsOf(t, g, 100) // A: 60
	inningsOf(t, g, 60)  // B: 20
	inningsOf(t, g, 60)  // B: 40 aggregate, bowled out short

	side, ok := g.WinningSide()
	if !ok {
		t.Fatal("there should be a winning side")
	}
	if side != TeamA {
		t.Errorf("winning side = %v, want TeamA", side)
	}
}

func TestVisitsRejectedOnceComplete(t *testing.T) {
	g := newGame(t, 2, TeamA)
	inningsOf(t, g, 100)
	inningsOf(t, g, 60)
	inningsOf(t, g, 60)
	if !g.Complete() {
		t.Fatal("precondition: game should be complete")
	}
	if _, err := g.RecordBatting(alice, 45, 3); !errors.Is(err, ErrGameComplete) {
		t.Errorf("got %v, want ErrGameComplete", err)
	}
	if _, err := g.RecordBowling(charlie, []Bowl{Miss}); !errors.Is(err, ErrGameComplete) {
		t.Errorf("got %v, want ErrGameComplete", err)
	}
}

func TestAbandon(t *testing.T) {
	g := newGame(t, 5, TeamA)
	bat(t, g, 100, 3)
	if err := g.Abandon(); err != nil {
		t.Fatal(err)
	}
	if !g.Complete() {
		t.Error("an abandoned game is complete")
	}
	if g.Result() != ResultAbandoned {
		t.Errorf("result = %v, want ResultAbandoned", g.Result())
	}
	if err := g.Abandon(); !errors.Is(err, ErrGameComplete) {
		t.Errorf("double abandon: got %v, want ErrGameComplete", err)
	}
}

// ---------------------------------------------------------------- undo

func TestUndoBattingVisit(t *testing.T) {
	g := newGame(t, 5, TeamA)
	bat(t, g, 100, 3) // 60 runs
	bowl(t, g, "1")
	bat(t, g, 60, 3) // 20 runs, total 80

	if g.CurrentInnings().Runs != 80 {
		t.Fatalf("precondition runs = %d, want 80", g.CurrentInnings().Runs)
	}
	removed, err := g.Undo()
	if err != nil {
		t.Fatal(err)
	}
	if removed.RawPoints != 60 {
		t.Errorf("undid the wrong event: %+v", removed)
	}
	if g.CurrentInnings().Runs != 60 {
		t.Errorf("runs after undo = %d, want 60", g.CurrentInnings().Runs)
	}
	turn, _ := g.CurrentTurn()
	if turn.Kind != Batting || turn.PlayerID != bob {
		t.Errorf("turn after undo = %v player %d, want Batting bob", turn.Kind, turn.PlayerID)
	}
}

func TestUndoWicket(t *testing.T) {
	g := newGame(t, 5, TeamA)
	bat(t, g, 100, 3)
	bowl(t, g, "23") // outer then inner: 3 wickets

	if g.CurrentInnings().Wickets != 3 {
		t.Fatalf("precondition wickets = %d, want 3", g.CurrentInnings().Wickets)
	}
	if _, err := g.Undo(); err != nil {
		t.Fatal(err)
	}
	if got := g.CurrentInnings().Wickets; got != 0 {
		t.Errorf("wickets after undo = %d, want 0", got)
	}
	turn, _ := g.CurrentTurn()
	if turn.Kind != Bowling || turn.PlayerID != charlie {
		t.Errorf("turn after undo = %v player %d, want Bowling charlie", turn.Kind, turn.PlayerID)
	}
}

// Undoing the visit that ended an innings must return the match to that innings.
func TestUndoAcrossInningsTransition(t *testing.T) {
	g := newGame(t, 2, TeamA)
	bat(t, g, 100, 3) // 60 runs
	bowl(t, g, "22")  // 2 wickets: first innings over

	if g.InningsNumber() != 2 {
		t.Fatalf("precondition: innings = %d, want 2", g.InningsNumber())
	}

	if _, err := g.Undo(); err != nil {
		t.Fatal(err)
	}

	if g.InningsNumber() != 1 {
		t.Fatalf("innings = %d, want 1 after undoing the innings-ending visit", g.InningsNumber())
	}
	inn := g.CurrentInnings()
	if inn.Complete {
		t.Error("the first innings should no longer be complete")
	}
	if inn.Runs != 60 {
		t.Errorf("runs = %d, want 60", inn.Runs)
	}
	if inn.Wickets != 0 {
		t.Errorf("wickets = %d, want 0", inn.Wickets)
	}
	turn, _ := g.CurrentTurn()
	if turn.Kind != Bowling || turn.Side != TeamB || turn.PlayerID != charlie {
		t.Errorf("turn = %+v, want Charlie of Team B bowling", turn)
	}
}

// Undoing across the boundary between the two legs must put the match back into
// leg 1, with the leg 2 batting order forgotten.
func TestUndoAcrossTheLegBoundary(t *testing.T) {
	g := newGame(t, 2, TeamA)
	inningsOf(t, g, 100) // A: 60
	inningsOf(t, g, 60)  // B: 20 — leg 1 over, B bats first in leg 2

	if g.Leg() != 2 || g.InningsNumber() != 3 {
		t.Fatalf("precondition: leg %d innings %d, want leg 2 innings 3",
			g.Leg(), g.InningsNumber())
	}

	if _, err := g.Undo(); err != nil {
		t.Fatal(err)
	}

	if g.Leg() != 1 {
		t.Errorf("leg = %d, want 1", g.Leg())
	}
	if g.InningsNumber() != 2 {
		t.Fatalf("innings = %d, want 2", g.InningsNumber())
	}
	if got := g.CurrentInnings().BattingSide; got != TeamB {
		t.Errorf("batting side = %v, want TeamB back in their first innings", got)
	}
	if g.Aggregate(TeamB) != 20 {
		t.Errorf("Team B aggregate = %d, want 20", g.Aggregate(TeamB))
	}
	if _, ok := g.Target(); ok {
		t.Error("there should be no target back in leg 1")
	}
}

// Undoing the visit that decided the match must reopen it, including when the
// match ended without the leaders batting a second time.
func TestUndoTheMatchDecidingVisit(t *testing.T) {
	g := newGame(t, 2, TeamA)
	inningsOf(t, g, 100) // A: 60
	inningsOf(t, g, 60)  // B: 20
	inningsOf(t, g, 60)  // B: 40 aggregate, bowled out short — Team A win

	if !g.Complete() || len(g.Innings()) != 3 {
		t.Fatalf("precondition: complete=%v innings=%d", g.Complete(), len(g.Innings()))
	}

	if _, err := g.Undo(); err != nil {
		t.Fatal(err)
	}
	if g.Complete() {
		t.Error("the match should be back in progress")
	}
	if g.Result() != ResultNone {
		t.Errorf("result = %v, want ResultNone", g.Result())
	}
	if g.InningsNumber() != 3 {
		t.Errorf("innings = %d, want 3", g.InningsNumber())
	}
	if _, ok := g.CurrentTurn(); !ok {
		t.Error("a reopened match must offer a turn")
	}
}

func TestUndoTheChaseWinningVisit(t *testing.T) {
	g := newGame(t, 2, TeamA)
	inningsOf(t, g, 100) // A: 60
	inningsOf(t, g, 60)  // B: 20
	inningsOf(t, g, 140) // B: 120 aggregate
	bat(t, g, 140, 3)    // A: 100 more → 160 beats 120

	if !g.Complete() || g.Result() != ResultTeamA {
		t.Fatalf("precondition: complete=%v result=%v", g.Complete(), g.Result())
	}
	if _, err := g.Undo(); err != nil {
		t.Fatal(err)
	}
	if g.Complete() {
		t.Error("the match should be back in progress")
	}
	if g.InningsNumber() != FinalInnings {
		t.Errorf("innings = %d, want %d", g.InningsNumber(), FinalInnings)
	}
	if g.Aggregate(TeamA) != 60 {
		t.Errorf("Team A aggregate = %d, want 60", g.Aggregate(TeamA))
	}
}

func TestUndoBackToEmpty(t *testing.T) {
	g := newGame(t, 5, TeamA)
	bat(t, g, 100, 3)
	bowl(t, g, "2")
	for i := 0; i < 2; i++ {
		if _, err := g.Undo(); err != nil {
			t.Fatalf("undo %d: %v", i, err)
		}
	}
	if _, err := g.Undo(); !errors.Is(err, ErrNothingToUndo) {
		t.Errorf("got %v, want ErrNothingToUndo", err)
	}
	inn := g.CurrentInnings()
	if inn.Runs != 0 || inn.Wickets != 0 || g.InningsNumber() != 1 {
		t.Errorf("game not fully reset: %+v", inn)
	}
	turn, _ := g.CurrentTurn()
	if turn.PlayerID != alice || turn.Kind != Batting {
		t.Errorf("turn = %+v, want Alice batting", turn)
	}
}

// ---------------------------------------------------------------- restore

func TestRestoreReplaysToIdenticalState(t *testing.T) {
	g := newGame(t, 3, TeamB)
	bat(t, g, 100, 3)
	bowl(t, g, "OMM")
	bat(t, g, 57, 2)
	bowl(t, g, "MOM")
	bat(t, g, 41, 3)

	a, b := teams()
	restored, err := Restore(g.Config(), a, b, g.Events())
	if err != nil {
		t.Fatal(err)
	}
	if restored.InningsNumber() != g.InningsNumber() {
		t.Errorf("innings = %d, want %d", restored.InningsNumber(), g.InningsNumber())
	}
	if restored.CurrentInnings() != g.CurrentInnings() {
		t.Errorf("innings state = %+v, want %+v", restored.CurrentInnings(), g.CurrentInnings())
	}
	gotTurn, ok1 := restored.CurrentTurn()
	wantTurn, ok2 := g.CurrentTurn()
	if ok1 != ok2 || gotTurn != wantTurn {
		t.Errorf("turn = %+v (%v), want %+v (%v)", gotTurn, ok1, wantTurn, ok2)
	}
	if restored.Complete() != g.Complete() || restored.Result() != g.Result() {
		t.Errorf("completion mismatch")
	}
}

func TestRestoreCarriesCompletedGame(t *testing.T) {
	g := newGame(t, 2, TeamA)
	inningsOf(t, g, 100) // A: 60
	inningsOf(t, g, 60)  // B: 20
	inningsOf(t, g, 60)  // B: 40 aggregate, bowled out short — Team A win

	if !g.Complete() {
		t.Fatal("precondition: the match should be decided")
	}

	a, b := teams()
	restored, err := Restore(g.Config(), a, b, g.Events())
	if err != nil {
		t.Fatal(err)
	}
	if !restored.Complete() {
		t.Error("restored game should be complete")
	}
	if restored.Result() != ResultTeamA {
		t.Errorf("restored result = %v, want ResultTeamA", restored.Result())
	}
	// A match decided in three innings must restore that way too, not replay
	// into a fourth.
	if got := len(restored.Innings()); got != 3 {
		t.Errorf("restored innings = %d, want 3", got)
	}
	if restored.Aggregate(TeamA) != 60 || restored.Aggregate(TeamB) != 40 {
		t.Errorf("restored aggregates = %d v %d, want 60 v 40",
			restored.Aggregate(TeamA), restored.Aggregate(TeamB))
	}
}

// ---------------------------------------------------------------- stats

func TestStatsForSeparatesBattingAndBowling(t *testing.T) {
	g := newGame(t, 20, TeamA)
	bat(t, g, 100, 3) // alice: 60 runs
	bowl(t, g, "OMM") // charlie: 1 wicket
	bat(t, g, 57, 3)  // bob: 17 runs
	bowl(t, g, "BB")  // david: 4 wickets
	bat(t, g, 26, 3)  // alice: 0 runs
	bowl(t, g, "MMM") // eve
	bat(t, g, 41, 2)  // bob: 1 run
	bowl(t, g, "OB")  // charlie: 3 more wickets

	as := g.StatsFor(alice)
	if as.BattingVisits != 2 {
		t.Errorf("alice batting visits = %d, want 2", as.BattingVisits)
	}
	if as.RawPoints != 126 {
		t.Errorf("alice raw points = %d, want 126", as.RawPoints)
	}
	if as.Runs != 60 {
		t.Errorf("alice runs = %d, want 60", as.Runs)
	}
	if as.HighestRaw != 100 {
		t.Errorf("alice highest raw = %d, want 100", as.HighestRaw)
	}
	if as.HighestRuns != 60 {
		t.Errorf("alice highest runs = %d, want 60", as.HighestRuns)
	}
	if as.BattingDarts != 6 {
		t.Errorf("alice batting darts = %d, want 6", as.BattingDarts)
	}
	if as.BowlingVisits != 0 {
		t.Errorf("alice bowled %d visits, want 0", as.BowlingVisits)
	}
	if got, want := as.AverageRawPerVisit(), 63.0; math.Abs(got-want) > 1e-9 {
		t.Errorf("alice avg raw = %v, want %v", got, want)
	}
	if got, want := as.AverageRunsPerVisit(), 30.0; math.Abs(got-want) > 1e-9 {
		t.Errorf("alice avg runs = %v, want %v", got, want)
	}

	cs := g.StatsFor(charlie)
	if cs.BowlingVisits != 2 {
		t.Errorf("charlie bowling visits = %d, want 2", cs.BowlingVisits)
	}
	if cs.Wickets != 4 {
		t.Errorf("charlie wickets = %d, want 4", cs.Wickets)
	}
	if cs.OuterBulls != 2 {
		t.Errorf("charlie outer bulls = %d, want 2", cs.OuterBulls)
	}
	if cs.InnerBulls != 1 {
		t.Errorf("charlie inner bulls = %d, want 1", cs.InnerBulls)
	}
	if cs.BowlingDarts != 5 {
		t.Errorf("charlie bowling darts = %d, want 5", cs.BowlingDarts)
	}
	if cs.BattingVisits != 0 {
		t.Errorf("charlie batted %d visits, want 0", cs.BattingVisits)
	}
	if got, want := cs.WicketsPerVisit(), 2.0; math.Abs(got-want) > 1e-9 {
		t.Errorf("charlie wickets per visit = %v, want %v", got, want)
	}
}

func TestEmptyStatsAvoidDivisionByZero(t *testing.T) {
	var s PlayerStats
	if s.AverageRawPerVisit() != 0 || s.AverageRunsPerVisit() != 0 || s.WicketsPerVisit() != 0 {
		t.Error("a player who never played should average zero, not NaN")
	}
}

func TestSideOfAndPlayers(t *testing.T) {
	g := newGame(t, 5, TeamA)
	if s, ok := g.SideOf(david); !ok || s != TeamB {
		t.Errorf("SideOf(david) = %v %v, want TeamB true", s, ok)
	}
	if _, ok := g.SideOf(99); ok {
		t.Error("an unknown player should have no side")
	}
	if got := g.Players(); len(got) != 5 || got[0] != alice || got[4] != eve {
		t.Errorf("Players() = %v", got)
	}
}

// ---------------------------------------------------------------- encoding

func TestSideRoundTrip(t *testing.T) {
	for _, s := range []Side{TeamA, TeamB} {
		got, err := ParseSide(s.String())
		if err != nil || got != s {
			t.Errorf("ParseSide(%q) = %v %v, want %v", s.String(), got, err, s)
		}
	}
	if _, err := ParseSide("C"); !errors.Is(err, ErrBadSide) {
		t.Errorf("got %v, want ErrBadSide", err)
	}
	if TeamA.Other() != TeamB || TeamB.Other() != TeamA {
		t.Error("Other() should swap the sides")
	}
}

func TestKindRoundTrip(t *testing.T) {
	for _, k := range []Kind{Batting, Bowling} {
		got, err := ParseKind(k.String())
		if err != nil || got != k {
			t.Errorf("ParseKind(%q) = %v %v, want %v", k.String(), got, err, k)
		}
	}
	if _, err := ParseKind("throw"); !errors.Is(err, ErrBadKind) {
		t.Errorf("got %v, want ErrBadKind", err)
	}
}

func TestResultRoundTrip(t *testing.T) {
	for _, r := range []Result{ResultNone, ResultTeamA, ResultTeamB, ResultDraw, ResultAbandoned} {
		if got := ParseResult(r.String()); got != r {
			t.Errorf("ParseResult(%q) = %v, want %v", r.String(), got, r)
		}
	}
}

func TestBowlNotationRoundTrip(t *testing.T) {
	for _, b := range []Bowl{Miss, Outer, Inner} {
		got, err := ParseBowl([]rune(b.String())[0])
		if err != nil || got != b {
			t.Errorf("ParseBowl(%q) = %v %v, want %v", b.String(), got, err, b)
		}
		if b.Label() == "" {
			t.Errorf("%v has no label", b)
		}
	}
}

// The keypad digits are the notation the operator types; the letters are what
// the database stores. Both must parse, and the two must not be confused.
func TestKeypadNotation(t *testing.T) {
	cases := []struct {
		key  rune
		want Bowl
	}{
		{'1', Miss},
		{'2', Outer},
		{'3', Inner},
	}
	for _, tc := range cases {
		got, err := ParseBowl(tc.key)
		if err != nil {
			t.Errorf("ParseBowl(%q): %v", string(tc.key), err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseBowl(%q) = %v, want %v", string(tc.key), got, tc.want)
		}
		if got.Key() != string(tc.key) {
			t.Errorf("%v.Key() = %q, want %q", got, got.Key(), string(tc.key))
		}
	}

	// Wickets ascend with the digit, which is what makes the keys memorable.
	if Miss.Wickets() != 0 || Outer.Wickets() != 1 || Inner.Wickets() != 2 {
		t.Error("the keypad digits should map to 0, 1 and 2 wickets in order")
	}

	// Storage notation is unchanged, so existing rows still load.
	if Miss.String() != "M" || Outer.String() != "O" || Inner.String() != "B" {
		t.Error("storage notation must stay M/O/B")
	}

	// A whole visit typed on the keypad.
	got, err := ParseBowls("123")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != Miss || got[1] != Outer || got[2] != Inner {
		t.Errorf("ParseBowls(\"123\") = %v", got)
	}
}
