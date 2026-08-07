package sqlite

import (
	"context"
	"fmt"

	"github.com/BigJabber/dart-tui/internal/domain/cricket"
	"github.com/BigJabber/dart-tui/internal/domain/stats"
)

// The statistics queries below only ever read finished games.
//
// An in-progress leg is not a result yet: counting its visits towards a lifetime
// average while its game outcome is still unknown would make "games played" and
// "total darts" disagree with each other. The live game screen shows current-game
// figures from the engine instead, which is the number the operator actually
// wants while standing at the board.

// X01VisitsForPlayer returns every X01 visit a player has thrown in a finished
// game, as the raw material for their lifetime figures.
func (s *Store) X01VisitsForPlayer(ctx context.Context, playerID int64) ([]stats.X01Visit, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT v.game_id, g.start_score, v.player_id, v.credited, v.darts, v.bust, v.checkout
FROM x01_visits v
JOIN games g ON g.id = v.game_id
WHERE v.player_id = ? AND g.status = 'finished'
ORDER BY v.game_id ASC, v.seq ASC`, playerID)
	if err != nil {
		return nil, fmt.Errorf("load X01 visits: %w", err)
	}
	defer rows.Close()

	var out []stats.X01Visit
	for rows.Next() {
		var (
			v              stats.X01Visit
			bust, checkout int
		)
		if err := rows.Scan(&v.GameID, &v.StartScore, &v.PlayerID,
			&v.Credited, &v.Darts, &bust, &checkout); err != nil {
			return nil, err
		}
		v.Bust = bust != 0
		v.Checkout = checkout != 0
		out = append(out, v)
	}
	return out, rows.Err()
}

// X01LegsForPlayer returns a player's outcome in each finished X01 game.
func (s *Store) X01LegsForPlayer(ctx context.Context, playerID int64) ([]stats.X01Leg, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT r.game_id, g.start_score, r.player_id, r.completed, r.won, r.darts, r.visits, r.points
FROM game_results r
JOIN games g ON g.id = r.game_id
WHERE r.player_id = ? AND g.mode = 'x01' AND g.status = 'finished'
ORDER BY r.game_id ASC`, playerID)
	if err != nil {
		return nil, fmt.Errorf("load X01 results: %w", err)
	}
	defer rows.Close()

	var out []stats.X01Leg
	for rows.Next() {
		var (
			l              stats.X01Leg
			completed, won int
		)
		if err := rows.Scan(&l.GameID, &l.StartScore, &l.PlayerID,
			&completed, &won, &l.Darts, &l.Visits, &l.Credited); err != nil {
			return nil, err
		}
		l.Completed = completed != 0
		l.Won = won != 0
		out = append(out, l)
	}
	return out, rows.Err()
}

// CricketVisitsForPlayer returns every cricket visit a player has taken in a
// finished game. Outer and inner bulls are counted from the individual darts, so
// they stay distinguishable rather than collapsing into a wicket total.
func (s *Store) CricketVisitsForPlayer(ctx context.Context, playerID int64) ([]stats.CricketVisit, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT e.game_id, e.player_id, e.kind, e.raw_points, e.runs, e.darts, e.wickets,
       (SELECT COUNT(*) FROM bowling_darts d WHERE d.event_id = e.id AND d.result = 'O'),
       (SELECT COUNT(*) FROM bowling_darts d WHERE d.event_id = e.id AND d.result = 'B')
FROM cricket_events e
JOIN games g ON g.id = e.game_id
WHERE e.player_id = ? AND g.status = 'finished'
ORDER BY e.game_id ASC, e.seq ASC`, playerID)
	if err != nil {
		return nil, fmt.Errorf("load cricket visits: %w", err)
	}
	defer rows.Close()

	var out []stats.CricketVisit
	for rows.Next() {
		var (
			v       stats.CricketVisit
			kindStr string
		)
		if err := rows.Scan(&v.GameID, &v.PlayerID, &kindStr,
			&v.RawPoints, &v.Runs, &v.Darts, &v.Wickets,
			&v.OuterBulls, &v.InnerBulls); err != nil {
			return nil, err
		}
		kind, err := cricket.ParseKind(kindStr)
		if err != nil {
			return nil, err
		}
		v.Kind = kind
		out = append(out, v)
	}
	return out, rows.Err()
}

// CricketEntriesForPlayer returns a player's participation in each finished
// cricket game.
func (s *Store) CricketEntriesForPlayer(ctx context.Context, playerID int64) ([]stats.CricketEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT r.game_id, r.player_id, r.won
FROM game_results r
JOIN games g ON g.id = r.game_id
WHERE r.player_id = ? AND g.mode = 'cricket' AND g.status = 'finished'
ORDER BY r.game_id ASC`, playerID)
	if err != nil {
		return nil, fmt.Errorf("load cricket results: %w", err)
	}
	defer rows.Close()

	var out []stats.CricketEntry
	for rows.Next() {
		var (
			e   stats.CricketEntry
			won int
		)
		if err := rows.Scan(&e.GameID, &e.PlayerID, &won); err != nil {
			return nil, err
		}
		e.Won = won != 0
		out = append(out, e)
	}
	return out, rows.Err()
}

// PlayerStats assembles a player's complete statistics by handing the recorded
// rows to the domain's derivation functions. No aggregate is read from a stored
// total: everything here is recomputed from the visits as they happened.
func (s *Store) PlayerStats(ctx context.Context, playerID int64) (stats.PlayerStats, error) {
	x01Visits, err := s.X01VisitsForPlayer(ctx, playerID)
	if err != nil {
		return stats.PlayerStats{}, err
	}
	x01Legs, err := s.X01LegsForPlayer(ctx, playerID)
	if err != nil {
		return stats.PlayerStats{}, err
	}
	cricketVisits, err := s.CricketVisitsForPlayer(ctx, playerID)
	if err != nil {
		return stats.PlayerStats{}, err
	}
	cricketEntries, err := s.CricketEntriesForPlayer(ctx, playerID)
	if err != nil {
		return stats.PlayerStats{}, err
	}

	return stats.PlayerStats{
		PlayerID: playerID,
		X01:      stats.ComputeX01(x01Visits, x01Legs),
		Cricket:  stats.ComputeCricket(cricketVisits, cricketEntries),
	}, nil
}
