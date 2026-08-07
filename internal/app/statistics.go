package app

import (
	"context"
	"sort"

	"github.com/pourmoi/dart-tui/internal/domain/player"
	"github.com/pourmoi/dart-tui/internal/domain/stats"
	"github.com/pourmoi/dart-tui/internal/persistence/sqlite"
)

// StatsService produces the figures shown on the profile and leaderboard.
type StatsService struct {
	store *sqlite.Store
}

// ForPlayer returns a player's complete statistics, recomputed from their
// recorded visits rather than read from any stored total.
func (s *StatsService) ForPlayer(ctx context.Context, playerID int64) (stats.PlayerStats, error) {
	return s.store.PlayerStats(ctx, playerID)
}

// LeaderboardRow is one player's line on the leaderboard.
type LeaderboardRow struct {
	Player player.Player
	Stats  stats.PlayerStats
}

// Played reports whether the player has any recorded games, so the UI can keep
// newcomers out of the ranking rather than showing a table of zeroes.
func (r LeaderboardRow) Played() bool { return r.Stats.TotalGamesPlayed() > 0 }

// SortKey names a leaderboard ordering.
type SortKey int

// Leaderboard orderings. Each is a sensible way to answer "who is best", and
// they are separate because X01 and cricket cannot be ranked by one number.
const (
	// SortByWins ranks by total games won, then by win rate.
	SortByWins SortKey = iota
	// SortByX01Average ranks by lifetime three-dart average.
	SortByX01Average
	// SortByCricketRuns ranks by total cricket runs scored.
	SortByCricketRuns
	// SortByCricketWickets ranks by total cricket wickets taken.
	SortByCricketWickets
	// SortByName is alphabetical, for finding somebody quickly.
	SortByName
)

// Label is the column heading a sort corresponds to.
func (k SortKey) Label() string {
	switch k {
	case SortByX01Average:
		return "X01 average"
	case SortByCricketRuns:
		return "cricket runs"
	case SortByCricketWickets:
		return "cricket wickets"
	case SortByName:
		return "name"
	default:
		return "wins"
	}
}

// Next cycles to the following sort order, for a single-key toggle in the UI.
func (k SortKey) Next() SortKey {
	if k >= SortByName {
		return SortByWins
	}
	return k + 1
}

// Leaderboard returns every active player with their statistics, ordered by the
// requested key. Archived players are excluded from the ranking but keep their
// history, which remains visible on their profile.
func (s *StatsService) Leaderboard(ctx context.Context, key SortKey) ([]LeaderboardRow, error) {
	players, err := s.store.ListPlayers(ctx, false)
	if err != nil {
		return nil, err
	}
	rows := make([]LeaderboardRow, 0, len(players))
	for _, p := range players {
		ps, err := s.store.PlayerStats(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		rows = append(rows, LeaderboardRow{Player: p, Stats: ps})
	}
	sortLeaderboard(rows, key)
	return rows, nil
}

func sortLeaderboard(rows []LeaderboardRow, key SortKey) {
	byName := func(i, j int) bool {
		return lessFold(rows[i].Player.Name, rows[j].Player.Name)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		switch key {
		case SortByX01Average:
			// Players with no darts thrown sort last rather than tying at zero.
			if (a.Stats.X01.Darts == 0) != (b.Stats.X01.Darts == 0) {
				return b.Stats.X01.Darts == 0
			}
			if a.Stats.X01.Average != b.Stats.X01.Average {
				return a.Stats.X01.Average > b.Stats.X01.Average
			}
		case SortByCricketRuns:
			if a.Stats.Cricket.Batting.Runs != b.Stats.Cricket.Batting.Runs {
				return a.Stats.Cricket.Batting.Runs > b.Stats.Cricket.Batting.Runs
			}
		case SortByCricketWickets:
			if a.Stats.Cricket.Bowling.Wickets != b.Stats.Cricket.Bowling.Wickets {
				return a.Stats.Cricket.Bowling.Wickets > b.Stats.Cricket.Bowling.Wickets
			}
		case SortByName:
			return byName(i, j)
		default:
			if a.Stats.TotalGamesWon() != b.Stats.TotalGamesWon() {
				return a.Stats.TotalGamesWon() > b.Stats.TotalGamesWon()
			}
			if a.Stats.TotalWinPercent() != b.Stats.TotalWinPercent() {
				return a.Stats.TotalWinPercent() > b.Stats.TotalWinPercent()
			}
		}
		return byName(i, j)
	})
}

// lessFold compares names case-insensitively without allocating.
func lessFold(a, b string) bool {
	ar, br := []rune(a), []rune(b)
	for i := 0; i < len(ar) && i < len(br); i++ {
		x, y := toLower(ar[i]), toLower(br[i])
		if x != y {
			return x < y
		}
	}
	return len(ar) < len(br)
}

func toLower(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + ('a' - 'A')
	}
	return r
}
