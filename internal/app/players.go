package app

import (
	"context"
	"fmt"
	"time"

	"github.com/BigJabber/dart-tui/internal/domain/cricket"
	"github.com/BigJabber/dart-tui/internal/domain/player"
	"github.com/BigJabber/dart-tui/internal/persistence/sqlite"
)

// PlayerService manages the roster.
type PlayerService struct {
	store *sqlite.Store
}

// List returns the players available for a new game.
func (s *PlayerService) List(ctx context.Context, includeArchived bool) ([]player.Player, error) {
	return s.store.ListPlayers(ctx, includeArchived)
}

// Get loads a single player.
func (s *PlayerService) Get(ctx context.Context, id int64) (player.Player, error) {
	return s.store.GetPlayer(ctx, id)
}

// Create registers a player, assigning them a colour from the palette.
func (s *PlayerService) Create(ctx context.Context, name string) (player.Player, error) {
	return s.store.CreatePlayer(ctx, name)
}

// Rename changes a player's display name, keeping all their history.
func (s *PlayerService) Rename(ctx context.Context, id int64, name string) error {
	return s.store.RenamePlayer(ctx, id, name)
}

// SetColour changes a player's display colour.
func (s *PlayerService) SetColour(ctx context.Context, id int64, colour string) error {
	return s.store.SetPlayerColour(ctx, id, colour)
}

// SetArchived takes a player out of active play, or brings them back. It never
// deletes anything: their visits and statistics remain.
func (s *PlayerService) SetArchived(ctx context.Context, id int64, archived bool) error {
	return s.store.SetPlayerArchived(ctx, id, archived)
}

// RecentGame is a finished game as shown on a player's profile.
type RecentGame struct {
	GameID   int64
	Mode     string
	Label    string
	PlayedAt time.Time
	// Opponents are the other players, for a one-line description.
	Opponents []player.Player
	// Outcome is a short word: Won, Lost, Draw or Abandoned.
	Outcome string
	Won     bool
	// Detail is a mode-specific summary, such as "28 darts" or "60 runs".
	Detail string
}

// RecentGames returns a player's most recent finished games.
func (s *PlayerService) RecentGames(ctx context.Context, playerID int64, limit int) ([]RecentGame, error) {
	summaries, err := s.store.ListRecentGamesForPlayer(ctx, playerID, limit)
	if err != nil {
		return nil, err
	}

	out := make([]RecentGame, 0, len(summaries))
	for _, g := range summaries {
		rg := RecentGame{
			GameID: g.ID,
			Mode:   g.Mode,
			Label:  GameLabel(g),
		}
		if g.FinishedAt != nil {
			rg.PlayedAt = *g.FinishedAt
		} else {
			rg.PlayedAt = g.CreatedAt
		}

		others := make([]int64, 0, len(g.ParticipantIDs))
		for _, id := range g.ParticipantIDs {
			if id != playerID {
				others = append(others, id)
			}
		}
		byID, err := s.store.PlayersByID(ctx, others)
		if err != nil {
			return nil, err
		}
		for _, id := range others {
			if p, ok := byID[id]; ok {
				rg.Opponents = append(rg.Opponents, p)
			}
		}

		results, err := s.store.GameResults(ctx, g.ID)
		if err != nil {
			return nil, err
		}
		describeResult(&rg, g, results, playerID)
		out = append(out, rg)
	}
	return out, nil
}

// describeResult fills in the outcome and detail for one player's row.
func describeResult(rg *RecentGame, g sqlite.GameSummary, results []sqlite.ResultRow, playerID int64) {
	var mine *sqlite.ResultRow
	for i := range results {
		if results[i].PlayerID == playerID {
			mine = &results[i]
			break
		}
	}

	if g.Mode == sqlite.ModeCricket {
		switch cricket.ParseResult(g.Outcome) {
		case cricket.ResultDraw:
			rg.Outcome = "Draw"
		case cricket.ResultAbandoned:
			rg.Outcome = "Abandoned"
		default:
			if mine != nil && mine.Won {
				rg.Outcome, rg.Won = "Won", true
			} else {
				rg.Outcome = "Lost"
			}
		}
		if mine != nil && mine.Darts > 0 {
			rg.Detail = fmt.Sprintf("%d darts", mine.Darts)
		}
		return
	}

	switch {
	case mine == nil:
		rg.Outcome = "—"
	case mine.Won:
		rg.Outcome, rg.Won = "Won", true
	case g.WinnerPlayerID == nil:
		rg.Outcome = "No result"
	default:
		rg.Outcome = "Lost"
	}
	switch {
	case mine == nil:
	case mine.Completed:
		rg.Detail = fmt.Sprintf("out in %d darts", mine.Darts)
	case mine.Remaining != nil && mine.Visits > 0:
		rg.Detail = fmt.Sprintf("%d left", *mine.Remaining)
	case mine.Visits == 0:
		rg.Detail = "did not throw"
	}
}

// GameLabel is the short human name for a game, such as "501" or "Cricket".
func GameLabel(g sqlite.GameSummary) string {
	if g.Mode == sqlite.ModeCricket {
		return "Cricket"
	}
	if g.StartScore > 0 {
		return fmt.Sprintf("%d", g.StartScore)
	}
	return "X01"
}
