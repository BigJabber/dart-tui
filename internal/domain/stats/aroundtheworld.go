package stats

import "github.com/BigJabber/dart-tui/internal/domain/atw"

// Around the World figures are kept in their own type and never folded into the
// X01 or cricket ones.
//
// The reason is the same one that keeps X01 and cricket apart, only starker: an
// Around the World dart scores nothing at all in the X01 sense. Twenty-two
// darts spent working round the board would drag a three-dart average through
// the floor while saying nothing whatever about how the player scores. The two
// modes share a player and a dartboard and nothing else.

// ------------------------------------------------------- Around the World input

// ATWVisit is one recorded Around the World visit.
//
// The dart-level tallies come with it rather than being recomputed from the
// game, so a lifetime figure can be assembled from rows without replaying every
// game a player has ever thrown.
type ATWVisit struct {
	GameID   int64
	PlayerID int64
	// Darts is how many darts the visit actually counted, which is one, two or
	// three: a visit that completed a run part-way through counts only the
	// darts thrown.
	Darts int
	// Advanced is how many targets the visit moved the player along.
	Advanced int
	// Completed reports that this visit finished the player's run.
	Completed bool

	Hits    int
	Misses  int
	Singles int
	Doubles int
	Trebles int
}

// ATWRun is one participant's outcome in a finished Around the World game.
// Participants who never threw are still present, with zeroed figures.
type ATWRun struct {
	GameID   int64
	PlayerID int64
	// Participants is how many players took part, which decides whether the
	// game was a contest or solo practice.
	Participants int
	// Completed reports whether the player finished the run.
	Completed bool
	Won       bool
	// Darts is how many darts they threw. It is a result only when Completed;
	// otherwise it is just how far they got.
	Darts  int
	Visits int
}

// Training reports whether the run was thrown alone, with nobody to beat.
func (r ATWRun) Training() bool { return atw.IsTraining(r.Participants) }

// ------------------------------------------------------ Around the World output

// ATWStats are a player's lifetime Around the World figures.
type ATWStats struct {
	// GamesPlayed, GamesWon and WinPercent describe contested games only, on
	// the same rule X01 uses: a game nobody else entered cannot be won or lost.
	GamesPlayed int
	GamesWon    int
	WinPercent  float64
	// TrainingGames counts solo practice sessions. Every other figure here
	// includes them, because the darts were thrown either way.
	TrainingGames int

	// RunsCompleted is how many times the player has got all the way round.
	RunsCompleted int
	// CompletionPercent is that as a share of every game they started.
	CompletionPercent float64

	Darts  int
	Visits int

	// BestDarts is the fewest darts a run has been completed in — the record
	// worth beating. Zero means no run has been completed yet.
	BestDarts       int
	BestDartsGameID int64
	// AverageDartsPerRun averages only completed runs. An abandoned run has no
	// dart count worth averaging, so including it would flatter the figure.
	AverageDartsPerRun float64

	// The dart-by-dart record. These are kept because they are what any
	// finer-grained statistic would be built from, whether or not the profile
	// screen shows them all today.
	Hits    int
	Misses  int
	Singles int
	Doubles int
	Trebles int
	// TargetsAdvanced is every target completed across every game.
	TargetsAdvanced int
	// HitPercent is the share of darts that found the required target.
	HitPercent float64
	// ProgressPerDart is targets advanced per dart thrown: 1.0 means every dart
	// hit its target as a single, and more than that means doubles and trebles
	// have been skipping ahead.
	ProgressPerDart float64
}

// ComputeATW folds a player's Around the World visits and runs into their
// lifetime figures. Both are expected to be pre-filtered to the player.
//
// Visits supply the throwing detail; runs supply the game-level outcomes. Solo
// practice is treated exactly as it is in X01: it counts towards every figure
// that describes throwing, including the fewest-darts record, but is kept out
// of games played and won so that practising alone is not recorded as losing.
func ComputeATW(visits []ATWVisit, runs []ATWRun) ATWStats {
	var s ATWStats

	for _, v := range visits {
		s.Visits++
		s.Darts += v.Darts
		s.TargetsAdvanced += v.Advanced
		s.Hits += v.Hits
		s.Misses += v.Misses
		s.Singles += v.Singles
		s.Doubles += v.Doubles
		s.Trebles += v.Trebles
	}

	completedDarts := 0
	for _, r := range runs {
		if r.Training() {
			s.TrainingGames++
		} else {
			s.GamesPlayed++
			if r.Won {
				s.GamesWon++
			}
		}

		if !r.Completed {
			continue
		}
		s.RunsCompleted++
		completedDarts += r.Darts
		if s.BestDarts == 0 || r.Darts < s.BestDarts {
			s.BestDarts = r.Darts
			s.BestDartsGameID = r.GameID
		}
	}

	if s.GamesPlayed > 0 {
		s.WinPercent = float64(s.GamesWon) / float64(s.GamesPlayed) * 100
	}
	if started := s.GamesPlayed + s.TrainingGames; started > 0 {
		s.CompletionPercent = float64(s.RunsCompleted) / float64(started) * 100
	}
	if s.RunsCompleted > 0 {
		s.AverageDartsPerRun = float64(completedDarts) / float64(s.RunsCompleted)
	}
	if s.Darts > 0 {
		s.HitPercent = float64(s.Hits) / float64(s.Darts) * 100
		s.ProgressPerDart = float64(s.TargetsAdvanced) / float64(s.Darts)
	}
	return s
}
