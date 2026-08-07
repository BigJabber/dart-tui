// Package stats derives player statistics from recorded visits.
//
// Nothing here is stored as a pre-calculated aggregate. The persistence layer
// hands over the raw visit and event rows and these functions fold them into
// the figures the UI shows, so a corrected visit or an undone checkout changes
// every statistic that depends on it without any separate repair step.
//
// X01 and Cricket figures are kept strictly apart. Averaging a three-dart X01
// average together with a cricket batting average would produce a number that
// means nothing, so the two never mix.
package stats

import (
	"github.com/BigJabber/dart-tui/internal/domain/cricket"
	"github.com/BigJabber/dart-tui/internal/domain/darts"
)

// ---------------------------------------------------------------- X01 input

// X01Visit is one recorded X01 visit, with the game context needed to attribute
// it to a variant.
type X01Visit struct {
	GameID     int64
	StartScore int
	PlayerID   int64
	// Credited is the score actually deducted: zero for a busted visit.
	Credited int
	Darts    int
	Bust     bool
	Checkout bool
}

// X01Leg is one participant's outcome in a finished X01 game. Participants who
// never threw are still present, with zeroed figures.
type X01Leg struct {
	GameID     int64
	StartScore int
	PlayerID   int64
	// Completed reports whether the player checked out.
	Completed bool
	Won       bool
	Darts     int
	Visits    int
	Credited  int
}

// Average is the leg's three-dart average.
func (l X01Leg) Average() float64 { return darts.ThreeDartAverage(l.Credited, l.Darts) }

// --------------------------------------------------------------- X01 output

// X01VariantStats are the figures for a single starting score, such as 501.
type X01VariantStats struct {
	StartScore    int
	GamesPlayed   int
	GamesWon      int
	LegsCompleted int
	Darts         int
	Points        int
	Average       float64
	// FewestDarts is the quickest completed leg at this starting score. Zero
	// means no leg has been completed yet.
	FewestDarts       int
	FewestDartsGameID int64
	// BestAverage is the best three-dart average in a completed leg.
	BestAverage       float64
	BestAverageGameID int64
}

// X01Stats are a player's lifetime X01 figures.
type X01Stats struct {
	GamesPlayed   int
	GamesWon      int
	WinPercent    float64
	LegsCompleted int
	Darts         int
	Points        int
	// Average is the lifetime three-dart average: credited points per dart
	// thrown, scaled to three darts. It is never derived from visit counts.
	Average float64

	HighestVisit int
	Tons         int // visits of 100 or more
	Ton40s       int // visits of 140 or more
	OneEighties  int // maximum visits

	// BestGameAverage is the best three-dart average across all completed legs.
	BestGameAverage       float64
	BestGameAverageStart  int
	BestGameAverageGameID int64

	// ByStart holds the same figures split by starting score, so 301 and 501
	// are never averaged together.
	ByStart map[int]X01VariantStats
}

// Variant returns the figures for one starting score, and whether any exist.
func (s X01Stats) Variant(startScore int) (X01VariantStats, bool) {
	v, ok := s.ByStart[startScore]
	return v, ok
}

// FewestDarts returns the quickest completed leg at a starting score.
func (s X01Stats) FewestDarts(startScore int) (int, bool) {
	v, ok := s.ByStart[startScore]
	if !ok || v.FewestDarts == 0 {
		return 0, false
	}
	return v.FewestDarts, true
}

// ComputeX01 folds a player's visits and legs into their lifetime X01 figures.
//
// Visits supply the scoring detail — darts, points, high-score counts — while
// legs supply the game-level outcomes. Both are expected to be pre-filtered to
// the player in question.
func ComputeX01(visits []X01Visit, legs []X01Leg) X01Stats {
	s := X01Stats{ByStart: map[int]X01VariantStats{}}

	variant := func(start int) X01VariantStats {
		v, ok := s.ByStart[start]
		if !ok {
			v = X01VariantStats{StartScore: start}
		}
		return v
	}

	for _, v := range visits {
		s.Darts += v.Darts
		s.Points += v.Credited

		// High-score counts use credited points, so a visit that busted counts
		// as nothing scored — which is exactly what happened at the board.
		if v.Credited > s.HighestVisit {
			s.HighestVisit = v.Credited
		}
		switch {
		case v.Credited >= 180:
			s.OneEighties++
			s.Ton40s++
			s.Tons++
		case v.Credited >= 140:
			s.Ton40s++
			s.Tons++
		case v.Credited >= 100:
			s.Tons++
		}

		vs := variant(v.StartScore)
		vs.Darts += v.Darts
		vs.Points += v.Credited
		s.ByStart[v.StartScore] = vs
	}

	for _, l := range legs {
		s.GamesPlayed++
		if l.Won {
			s.GamesWon++
		}
		if l.Completed {
			s.LegsCompleted++
		}

		vs := variant(l.StartScore)
		vs.GamesPlayed++
		if l.Won {
			vs.GamesWon++
		}
		if l.Completed {
			vs.LegsCompleted++

			if vs.FewestDarts == 0 || l.Darts < vs.FewestDarts {
				vs.FewestDarts = l.Darts
				vs.FewestDartsGameID = l.GameID
			}
			avg := l.Average()
			if avg > vs.BestAverage {
				vs.BestAverage = avg
				vs.BestAverageGameID = l.GameID
			}
			if avg > s.BestGameAverage {
				s.BestGameAverage = avg
				s.BestGameAverageStart = l.StartScore
				s.BestGameAverageGameID = l.GameID
			}
		}
		s.ByStart[l.StartScore] = vs
	}

	s.Average = darts.ThreeDartAverage(s.Points, s.Darts)
	if s.GamesPlayed > 0 {
		s.WinPercent = float64(s.GamesWon) / float64(s.GamesPlayed) * 100
	}
	for start, vs := range s.ByStart {
		vs.Average = darts.ThreeDartAverage(vs.Points, vs.Darts)
		s.ByStart[start] = vs
	}
	return s
}

// ------------------------------------------------------------ Cricket input

// CricketVisit is one recorded cricket visit. Batting fields are meaningful when
// Kind is Batting, bowling fields when it is Bowling.
type CricketVisit struct {
	GameID   int64
	PlayerID int64
	Kind     cricket.Kind

	RawPoints int
	Runs      int
	Darts     int

	Wickets    int
	OuterBulls int
	InnerBulls int
}

// CricketEntry is a player's participation in a finished cricket game.
type CricketEntry struct {
	GameID   int64
	PlayerID int64
	Won      bool
}

// ----------------------------------------------------------- Cricket output

// BattingStats are a player's lifetime cricket batting figures.
type BattingStats struct {
	Visits int
	Darts  int
	// RawPoints is the total raw dartboard score, before the runs rule.
	RawPoints int
	// Runs is the total awarded runs.
	Runs int
	// AverageRawPerVisit and AverageRunsPerVisit are per batting visit, not per
	// dart, because runs are a property of the visit total.
	AverageRawPerVisit  float64
	AverageRunsPerVisit float64
	HighestVisitRaw     int
	HighestVisitRuns    int
}

// BowlingStats are a player's lifetime cricket bowling figures.
type BowlingStats struct {
	Visits          int
	Darts           int
	Wickets         int
	OuterBulls      int
	InnerBulls      int
	WicketsPerVisit float64
}

// CricketStats are a player's lifetime cricket figures. Batting and bowling are
// separate because they measure unrelated skills.
type CricketStats struct {
	GamesPlayed int
	GamesWon    int
	WinPercent  float64
	Batting     BattingStats
	Bowling     BowlingStats
}

// ComputeCricket folds a player's cricket visits and game entries into their
// lifetime figures. Both are expected to be pre-filtered to the player.
func ComputeCricket(visits []CricketVisit, entries []CricketEntry) CricketStats {
	var s CricketStats

	for _, v := range visits {
		switch v.Kind {
		case cricket.Batting:
			s.Batting.Visits++
			s.Batting.Darts += v.Darts
			s.Batting.RawPoints += v.RawPoints
			s.Batting.Runs += v.Runs
			if v.RawPoints > s.Batting.HighestVisitRaw {
				s.Batting.HighestVisitRaw = v.RawPoints
			}
			if v.Runs > s.Batting.HighestVisitRuns {
				s.Batting.HighestVisitRuns = v.Runs
			}
		case cricket.Bowling:
			s.Bowling.Visits++
			s.Bowling.Darts += v.Darts
			s.Bowling.Wickets += v.Wickets
			s.Bowling.OuterBulls += v.OuterBulls
			s.Bowling.InnerBulls += v.InnerBulls
		}
	}

	if s.Batting.Visits > 0 {
		s.Batting.AverageRawPerVisit = float64(s.Batting.RawPoints) / float64(s.Batting.Visits)
		s.Batting.AverageRunsPerVisit = float64(s.Batting.Runs) / float64(s.Batting.Visits)
	}
	if s.Bowling.Visits > 0 {
		s.Bowling.WicketsPerVisit = float64(s.Bowling.Wickets) / float64(s.Bowling.Visits)
	}

	for _, e := range entries {
		s.GamesPlayed++
		if e.Won {
			s.GamesWon++
		}
	}
	if s.GamesPlayed > 0 {
		s.WinPercent = float64(s.GamesWon) / float64(s.GamesPlayed) * 100
	}
	return s
}

// ---------------------------------------------------------------- summaries

// PlayerStats is everything known about one player, ready for the profile
// screen. The two modes stay in their own fields; there is deliberately no
// combined "overall average".
type PlayerStats struct {
	PlayerID int64
	X01      X01Stats
	Cricket  CricketStats
}

// TotalGamesPlayed counts games across both modes. Counting games is meaningful
// across modes even though averaging scores is not.
func (p PlayerStats) TotalGamesPlayed() int {
	return p.X01.GamesPlayed + p.Cricket.GamesPlayed
}

// TotalGamesWon counts wins across both modes.
func (p PlayerStats) TotalGamesWon() int {
	return p.X01.GamesWon + p.Cricket.GamesWon
}

// TotalWinPercent is the win rate across both modes.
func (p PlayerStats) TotalWinPercent() float64 {
	played := p.TotalGamesPlayed()
	if played == 0 {
		return 0
	}
	return float64(p.TotalGamesWon()) / float64(played) * 100
}
