package stats

import "testing"

func TestComputeATWLifetime(t *testing.T) {
	const player int64 = 1
	visits := []ATWVisit{
		{GameID: 1, PlayerID: player, Darts: 3, Advanced: 4, Hits: 2, Misses: 1, Singles: 1, Trebles: 1},
		{GameID: 1, PlayerID: player, Darts: 2, Advanced: 3, Hits: 2, Singles: 1, Doubles: 1, Completed: true},
		{GameID: 2, PlayerID: player, Darts: 3, Advanced: 1, Hits: 1, Misses: 2, Singles: 1},
	}
	runs := []ATWRun{
		{GameID: 1, PlayerID: player, Participants: 2, Completed: true, Won: true, Darts: 5, Visits: 2},
		{GameID: 2, PlayerID: player, Participants: 2, Completed: false, Darts: 3, Visits: 1},
	}
	s := ComputeATW(visits, runs)

	if s.GamesPlayed != 2 || s.GamesWon != 1 {
		t.Errorf("played/won = %d/%d, want 2/1", s.GamesPlayed, s.GamesWon)
	}
	if s.WinPercent != 50 {
		t.Errorf("win percent = %v, want 50", s.WinPercent)
	}
	if s.RunsCompleted != 1 {
		t.Errorf("runs completed = %d, want 1", s.RunsCompleted)
	}
	if s.CompletionPercent != 50 {
		t.Errorf("completion percent = %v, want 50", s.CompletionPercent)
	}
	if s.Darts != 8 || s.Visits != 3 {
		t.Errorf("darts/visits = %d/%d, want 8/3", s.Darts, s.Visits)
	}
	if s.BestDarts != 5 || s.BestDartsGameID != 1 {
		t.Errorf("best = %d darts (game %d), want 5 (game 1)", s.BestDarts, s.BestDartsGameID)
	}
	if s.AverageDartsPerRun != 5 {
		t.Errorf("average darts per run = %v, want 5", s.AverageDartsPerRun)
	}
	if s.Hits != 5 || s.Misses != 3 {
		t.Errorf("hits/misses = %d/%d, want 5/3", s.Hits, s.Misses)
	}
	if s.Singles != 3 || s.Doubles != 1 || s.Trebles != 1 {
		t.Errorf("singles/doubles/trebles = %d/%d/%d, want 3/1/1",
			s.Singles, s.Doubles, s.Trebles)
	}
	if s.TargetsAdvanced != 8 {
		t.Errorf("targets advanced = %d, want 8", s.TargetsAdvanced)
	}
	closeTo(t, "hit percent", s.HitPercent, 62.5)
	closeTo(t, "progress per dart", s.ProgressPerDart, 1.0)
}

// Only completed runs have a dart count worth averaging. An abandoned run would
// flatter the figure by contributing a small number that was never a result.
func TestATWAverageIgnoresUnfinishedRuns(t *testing.T) {
	const player int64 = 1
	s := ComputeATW(nil, []ATWRun{
		{GameID: 1, PlayerID: player, Participants: 2, Completed: true, Darts: 30},
		{GameID: 2, PlayerID: player, Participants: 2, Completed: false, Darts: 3},
		{GameID: 3, PlayerID: player, Participants: 2, Completed: true, Darts: 26},
	})

	if s.RunsCompleted != 2 {
		t.Fatalf("runs completed = %d, want 2", s.RunsCompleted)
	}
	closeTo(t, "average darts per run", s.AverageDartsPerRun, 28)
	if s.BestDarts != 26 {
		t.Errorf("best darts = %d, want 26 — not the 3 from the abandoned run", s.BestDarts)
	}
}

// Solo practice counts as throwing but not as a contest, exactly as it does in
// X01.
func TestATWTrainingIsExcludedFromTheWinRate(t *testing.T) {
	const player int64 = 1
	s := ComputeATW(
		[]ATWVisit{{GameID: 2, PlayerID: player, Darts: 22, Advanced: 22, Hits: 22, Singles: 22}},
		[]ATWRun{
			{GameID: 1, PlayerID: player, Participants: 3, Completed: true, Won: true, Darts: 30},
			{GameID: 2, PlayerID: player, Participants: 1, Completed: true, Darts: 22},
		})

	if s.GamesPlayed != 1 || s.GamesWon != 1 || s.WinPercent != 100 {
		t.Errorf("played/won/percent = %d/%d/%v, want 1/1/100",
			s.GamesPlayed, s.GamesWon, s.WinPercent)
	}
	if s.TrainingGames != 1 {
		t.Errorf("training games = %d, want 1", s.TrainingGames)
	}
	// The practice run is still a completed run and still the record.
	if s.RunsCompleted != 2 {
		t.Errorf("runs completed = %d, want 2", s.RunsCompleted)
	}
	if s.BestDarts != 22 {
		t.Errorf("best darts = %d, want 22 — a record set alone still counts", s.BestDarts)
	}
	if s.Darts != 22 {
		t.Errorf("darts = %d, want 22", s.Darts)
	}
}

// A win stored against a solo game is ignored rather than counted, as the
// engine now refuses to create one.
func TestATWTrainingCannotContributeAWin(t *testing.T) {
	s := ComputeATW(nil, []ATWRun{
		{GameID: 1, PlayerID: 1, Participants: 1, Completed: true, Won: true, Darts: 22},
	})
	if s.GamesWon != 0 || s.GamesPlayed != 0 {
		t.Errorf("played/won = %d/%d, want 0/0", s.GamesPlayed, s.GamesWon)
	}
	if s.TrainingGames != 1 {
		t.Errorf("training games = %d, want 1", s.TrainingGames)
	}
}

func TestATWRunKnowsWhetherItWasTraining(t *testing.T) {
	if !(ATWRun{Participants: 1}).Training() {
		t.Error("a one-participant run is training")
	}
	if (ATWRun{Participants: 2}).Training() {
		t.Error("a two-participant run is a contest")
	}
}

func TestEmptyATWStats(t *testing.T) {
	s := ComputeATW(nil, nil)
	if s != (ATWStats{}) {
		t.Errorf("empty input produced %+v, want a zero value", s)
	}
}

// Counting games is meaningful across modes; averaging them is not, which is
// why only the counts are summed.
func TestPlayerTotalsIncludeAroundTheWorld(t *testing.T) {
	p := PlayerStats{
		X01:     X01Stats{GamesPlayed: 2, GamesWon: 1, TrainingGames: 1},
		Cricket: CricketStats{GamesPlayed: 3, GamesWon: 2},
		ATW:     ATWStats{GamesPlayed: 5, GamesWon: 4, TrainingGames: 2},
	}
	if got := p.TotalGamesPlayed(); got != 10 {
		t.Errorf("total played = %d, want 10", got)
	}
	if got := p.TotalGamesWon(); got != 7 {
		t.Errorf("total won = %d, want 7", got)
	}
	if got := p.TotalTrainingGames(); got != 3 {
		t.Errorf("total training = %d, want 3", got)
	}
	closeTo(t, "total win percent", p.TotalWinPercent(), 70)
}
