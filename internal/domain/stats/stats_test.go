package stats

import (
	"math"
	"testing"

	"github.com/BigJabber/dart-tui/internal/domain/cricket"
)

const player int64 = 1

func closeTo(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("%s = %v, want %v", label, got, want)
	}
}

// A full 501 leg won in 28 darts, plus a 301 leg that was not completed.
func TestComputeX01Lifetime(t *testing.T) {
	var visits []X01Visit
	for i := 0; i < 9; i++ {
		visits = append(visits, X01Visit{GameID: 1, StartScore: 501, PlayerID: player, Credited: 55, Darts: 3})
	}
	visits = append(visits, X01Visit{GameID: 1, StartScore: 501, PlayerID: player, Credited: 6, Darts: 1, Checkout: true})
	visits = append(visits,
		X01Visit{GameID: 2, StartScore: 301, PlayerID: player, Credited: 100, Darts: 3},
		X01Visit{GameID: 2, StartScore: 301, PlayerID: player, Credited: 140, Darts: 3},
	)

	legs := []X01Leg{
		{GameID: 1, StartScore: 501, PlayerID: player, Completed: true, Won: true, Darts: 28, Visits: 10, Credited: 501},
		{GameID: 2, StartScore: 301, PlayerID: player, Completed: false, Won: false, Darts: 6, Visits: 2, Credited: 240},
	}

	s := ComputeX01(visits, legs)

	if s.GamesPlayed != 2 {
		t.Errorf("GamesPlayed = %d, want 2", s.GamesPlayed)
	}
	if s.GamesWon != 1 {
		t.Errorf("GamesWon = %d, want 1", s.GamesWon)
	}
	closeTo(t, "WinPercent", s.WinPercent, 50)
	if s.LegsCompleted != 1 {
		t.Errorf("LegsCompleted = %d, want 1", s.LegsCompleted)
	}
	if s.Darts != 34 {
		t.Errorf("Darts = %d, want 34", s.Darts)
	}
	if s.Points != 741 {
		t.Errorf("Points = %d, want 741", s.Points)
	}
	closeTo(t, "Average", s.Average, 741.0/34*3)

	if s.HighestVisit != 140 {
		t.Errorf("HighestVisit = %d, want 140", s.HighestVisit)
	}
	if s.Tons != 2 {
		t.Errorf("Tons = %d, want 2 (100 and 140)", s.Tons)
	}
	if s.Ton40s != 1 {
		t.Errorf("Ton40s = %d, want 1", s.Ton40s)
	}
	if s.OneEighties != 0 {
		t.Errorf("OneEighties = %d, want 0", s.OneEighties)
	}

	// Best average comes from the completed 501 leg.
	closeTo(t, "BestGameAverage", s.BestGameAverage, 501.0/28*3)
	if s.BestGameAverageStart != 501 {
		t.Errorf("BestGameAverageStart = %d, want 501", s.BestGameAverageStart)
	}
	if s.BestGameAverageGameID != 1 {
		t.Errorf("BestGameAverageGameID = %d, want 1", s.BestGameAverageGameID)
	}
}

// 301 and 501 are reported separately, never merged into one average.
func TestComputeX01SplitsByStartingScore(t *testing.T) {
	visits := []X01Visit{
		{GameID: 1, StartScore: 501, PlayerID: player, Credited: 100, Darts: 3},
		{GameID: 2, StartScore: 301, PlayerID: player, Credited: 40, Darts: 3},
	}
	legs := []X01Leg{
		{GameID: 1, StartScore: 501, PlayerID: player, Completed: true, Won: true, Darts: 3, Credited: 100},
		{GameID: 2, StartScore: 301, PlayerID: player, Completed: false, Darts: 3, Credited: 40},
	}
	s := ComputeX01(visits, legs)

	five, ok := s.Variant(501)
	if !ok {
		t.Fatal("no 501 figures")
	}
	closeTo(t, "501 average", five.Average, 100)
	if five.GamesWon != 1 {
		t.Errorf("501 GamesWon = %d, want 1", five.GamesWon)
	}

	three, ok := s.Variant(301)
	if !ok {
		t.Fatal("no 301 figures")
	}
	closeTo(t, "301 average", three.Average, 40)
	if three.GamesWon != 0 {
		t.Errorf("301 GamesWon = %d, want 0", three.GamesWon)
	}

	if _, ok := s.Variant(701); ok {
		t.Error("a variant that was never played should not appear")
	}
}

func TestFewestDartsPerVariant(t *testing.T) {
	legs := []X01Leg{
		{GameID: 1, StartScore: 501, PlayerID: player, Completed: true, Darts: 30, Credited: 501},
		{GameID: 2, StartScore: 501, PlayerID: player, Completed: true, Darts: 28, Credited: 501},
		{GameID: 3, StartScore: 501, PlayerID: player, Completed: false, Darts: 12, Credited: 200},
		{GameID: 4, StartScore: 301, PlayerID: player, Completed: true, Darts: 21, Credited: 301},
	}
	s := ComputeX01(nil, legs)

	got, ok := s.FewestDarts(501)
	if !ok {
		t.Fatal("no 501 record")
	}
	if got != 28 {
		t.Errorf("fewest 501 darts = %d, want 28", got)
	}
	if v, _ := s.Variant(501); v.FewestDartsGameID != 2 {
		t.Errorf("fewest darts game = %d, want 2", v.FewestDartsGameID)
	}

	if got, ok := s.FewestDarts(301); !ok || got != 21 {
		t.Errorf("fewest 301 darts = %d (%v), want 21", got, ok)
	}
	if _, ok := s.FewestDarts(701); ok {
		t.Error("an unplayed variant has no record")
	}
}

// An incomplete leg must never set the fewest-darts record, however few darts
// were thrown in it.
func TestIncompleteLegDoesNotSetRecords(t *testing.T) {
	legs := []X01Leg{
		{GameID: 1, StartScore: 501, PlayerID: player, Completed: false, Darts: 3, Credited: 180},
	}
	s := ComputeX01(nil, legs)
	if _, ok := s.FewestDarts(501); ok {
		t.Error("an unfinished leg must not count as a fewest-darts record")
	}
	if s.BestGameAverage != 0 {
		t.Errorf("BestGameAverage = %v, want 0 — the leg was not completed", s.BestGameAverage)
	}
}

func TestHighScoreCountsUseCreditedPoints(t *testing.T) {
	visits := []X01Visit{
		{StartScore: 501, Credited: 180, Darts: 3},
		{StartScore: 501, Credited: 140, Darts: 3},
		{StartScore: 501, Credited: 100, Darts: 3},
		{StartScore: 501, Credited: 99, Darts: 3},
		// A busted visit credits nothing, so it counts towards no band even
		// though 180 was thrown at the board.
		{StartScore: 501, Credited: 0, Darts: 3, Bust: true},
	}
	s := ComputeX01(visits, nil)
	if s.OneEighties != 1 {
		t.Errorf("OneEighties = %d, want 1", s.OneEighties)
	}
	if s.Ton40s != 2 {
		t.Errorf("Ton40s = %d, want 2 (180 and 140)", s.Ton40s)
	}
	if s.Tons != 3 {
		t.Errorf("Tons = %d, want 3 (180, 140 and 100)", s.Tons)
	}
	if s.HighestVisit != 180 {
		t.Errorf("HighestVisit = %d, want 180", s.HighestVisit)
	}
	// The busted darts still count towards the dart total and drag the average
	// down, which is the correct behaviour.
	if s.Darts != 15 {
		t.Errorf("Darts = %d, want 15", s.Darts)
	}
	closeTo(t, "Average", s.Average, 519.0/15*3)
}

func TestComputeX01Empty(t *testing.T) {
	s := ComputeX01(nil, nil)
	if s.GamesPlayed != 0 || s.Average != 0 || s.WinPercent != 0 {
		t.Errorf("empty stats should be zeroed, got %+v", s)
	}
	if s.ByStart == nil {
		t.Error("ByStart should be an empty map, not nil")
	}
	if _, ok := s.FewestDarts(501); ok {
		t.Error("no records for a player who never played")
	}
}

func TestX01LegAverage(t *testing.T) {
	l := X01Leg{Credited: 501, Darts: 28}
	closeTo(t, "leg average", l.Average(), 501.0/28*3)
	empty := X01Leg{}
	closeTo(t, "empty leg average", empty.Average(), 0)
}

// -------------------------------------------------------------- cricket

func TestComputeCricketSeparatesBattingAndBowling(t *testing.T) {
	visits := []CricketVisit{
		{GameID: 1, PlayerID: player, Kind: cricket.Batting, RawPoints: 100, Runs: 60, Darts: 3},
		{GameID: 1, PlayerID: player, Kind: cricket.Batting, RawPoints: 26, Runs: 0, Darts: 3},
		{GameID: 1, PlayerID: player, Kind: cricket.Batting, RawPoints: 45, Runs: 5, Darts: 2},
		{GameID: 1, PlayerID: player, Kind: cricket.Bowling, Darts: 3, Wickets: 1, OuterBulls: 1},
		{GameID: 1, PlayerID: player, Kind: cricket.Bowling, Darts: 2, Wickets: 2, InnerBulls: 1},
	}
	entries := []CricketEntry{
		{GameID: 1, PlayerID: player, Won: true},
		{GameID: 2, PlayerID: player, Won: false},
	}

	s := ComputeCricket(visits, entries)

	b := s.Batting
	if b.Visits != 3 {
		t.Errorf("batting visits = %d, want 3", b.Visits)
	}
	if b.Darts != 8 {
		t.Errorf("batting darts = %d, want 8", b.Darts)
	}
	if b.RawPoints != 171 {
		t.Errorf("raw points = %d, want 171", b.RawPoints)
	}
	if b.Runs != 65 {
		t.Errorf("runs = %d, want 65", b.Runs)
	}
	if b.HighestVisitRaw != 100 {
		t.Errorf("highest raw = %d, want 100", b.HighestVisitRaw)
	}
	if b.HighestVisitRuns != 60 {
		t.Errorf("highest runs = %d, want 60", b.HighestVisitRuns)
	}
	closeTo(t, "avg raw per visit", b.AverageRawPerVisit, 171.0/3)
	closeTo(t, "avg runs per visit", b.AverageRunsPerVisit, 65.0/3)

	w := s.Bowling
	if w.Visits != 2 {
		t.Errorf("bowling visits = %d, want 2", w.Visits)
	}
	if w.Darts != 5 {
		t.Errorf("bowling darts = %d, want 5", w.Darts)
	}
	if w.Wickets != 3 {
		t.Errorf("wickets = %d, want 3", w.Wickets)
	}
	if w.OuterBulls != 1 {
		t.Errorf("outer bulls = %d, want 1", w.OuterBulls)
	}
	if w.InnerBulls != 1 {
		t.Errorf("inner bulls = %d, want 1", w.InnerBulls)
	}
	closeTo(t, "wickets per visit", w.WicketsPerVisit, 1.5)

	if s.GamesPlayed != 2 {
		t.Errorf("games played = %d, want 2", s.GamesPlayed)
	}
	if s.GamesWon != 1 {
		t.Errorf("games won = %d, want 1", s.GamesWon)
	}
	closeTo(t, "win percent", s.WinPercent, 50)
}

func TestComputeCricketEmpty(t *testing.T) {
	s := ComputeCricket(nil, nil)
	if s.Batting.AverageRawPerVisit != 0 || s.Bowling.WicketsPerVisit != 0 || s.WinPercent != 0 {
		t.Errorf("empty cricket stats should be zeroed, got %+v", s)
	}
}

// A bowler's batting figures must stay empty, and vice versa.
func TestCricketRolesDoNotLeak(t *testing.T) {
	visits := []CricketVisit{
		{Kind: cricket.Bowling, Darts: 3, Wickets: 2, InnerBulls: 1},
	}
	s := ComputeCricket(visits, nil)
	if s.Batting.Visits != 0 || s.Batting.RawPoints != 0 || s.Batting.Darts != 0 {
		t.Errorf("bowling leaked into batting figures: %+v", s.Batting)
	}
	if s.Bowling.Visits != 1 {
		t.Errorf("bowling visits = %d, want 1", s.Bowling.Visits)
	}
}

// -------------------------------------------------------------- combined

func TestPlayerStatsTotals(t *testing.T) {
	p := PlayerStats{
		PlayerID: player,
		X01:      X01Stats{GamesPlayed: 4, GamesWon: 3},
		Cricket:  CricketStats{GamesPlayed: 2, GamesWon: 0},
	}
	if p.TotalGamesPlayed() != 6 {
		t.Errorf("total played = %d, want 6", p.TotalGamesPlayed())
	}
	if p.TotalGamesWon() != 3 {
		t.Errorf("total won = %d, want 3", p.TotalGamesWon())
	}
	closeTo(t, "total win percent", p.TotalWinPercent(), 50)

	empty := PlayerStats{}
	closeTo(t, "empty win percent", empty.TotalWinPercent(), 0)
}
