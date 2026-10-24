package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/BigJabber/dart-tui/internal/domain/atw"
	"github.com/BigJabber/dart-tui/internal/domain/stats"
)

// ATWGameRecord is a persisted Around the World game, ready to be replayed.
type ATWGameRecord struct {
	ID         int64
	PlayerIDs  []int64
	Visits     []atw.Visit
	Status     string
	CreatedAt  time.Time
	FinishedAt *time.Time
	Winners    []int64
	// AutoSwap is whether the scoreboard moves to the next player in sequence
	// after each visit.
	AutoSwap bool
}

// Restore turns the record back into a live engine by replaying its darts.
func (r *ATWGameRecord) Restore() (*atw.Game, error) {
	return atw.Restore(r.PlayerIDs, r.Visits, r.Status == StatusFinished, r.Winners)
}

// CreateATWGame records a new Around the World game and its participants.
//
// As with the other modes the game is written before the first dart is thrown,
// so an accidental exit loses nothing.
func (s *Store) CreateATWGame(ctx context.Context, playerIDs []int64) (int64, error) {
	if len(playerIDs) == 0 {
		return 0, atw.ErrNoPlayers
	}
	var gameID int64
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
INSERT INTO games (mode, status, created_at) VALUES (?, ?, ?)`,
			ModeATW, StatusInProgress, nowString())
		if err != nil {
			return fmt.Errorf("create game: %w", err)
		}
		if gameID, err = res.LastInsertId(); err != nil {
			return err
		}
		return insertParticipants(ctx, tx, gameID, playerIDs, nil)
	})
	return gameID, err
}

// AppendATWVisit stores one visit and its darts.
//
// The visit and its darts go in together: a visit whose darts failed to write
// would be replayed as an empty one, quietly forgiving the misses and losing
// the progress.
func (s *Store) AppendATWVisit(ctx context.Context, gameID int64, v atw.Visit) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
INSERT INTO atw_visits
    (game_id, seq, player_id, darts, advanced, target_before, target_after, completed)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			gameID, v.Seq, v.PlayerID, v.DartCount(), v.Advanced(),
			v.TargetBefore.Index(), v.TargetAfter.Index(), boolToInt(v.Completed))
		if err != nil {
			return fmt.Errorf("record visit: %w", err)
		}
		visitID, err := res.LastInsertId()
		if err != nil {
			return err
		}

		stmt, err := tx.PrepareContext(ctx,
			`INSERT INTO atw_darts (visit_id, idx, segment) VALUES (?, ?, ?)`)
		if err != nil {
			return err
		}
		defer stmt.Close()

		for i, d := range v.Darts {
			if _, err := stmt.ExecContext(ctx, visitID, i, d.Notation()); err != nil {
				return fmt.Errorf("record dart %d: %w", i+1, err)
			}
		}
		return nil
	})
}

// DeleteLastATWVisit removes the most recently recorded visit, mirroring an
// undo in the engine. Its darts go with it through the cascade.
func (s *Store) DeleteLastATWVisit(ctx context.Context, gameID int64) error {
	_, err := s.db.ExecContext(ctx, `
DELETE FROM atw_visits
WHERE game_id = ?
  AND seq = (SELECT MAX(seq) FROM atw_visits WHERE game_id = ?)`, gameID, gameID)
	if err != nil {
		return fmt.Errorf("undo visit: %w", err)
	}
	return nil
}

// LoadATWGame reads a game and its visits back from storage.
func (s *Store) LoadATWGame(ctx context.Context, gameID int64) (*ATWGameRecord, error) {
	rec := &ATWGameRecord{ID: gameID}

	var (
		mode       string
		created    string
		finishedAt sql.NullString
		autoSwap   int
	)
	err := s.db.QueryRowContext(ctx, `
SELECT mode, status, created_at, finished_at, auto_swap FROM games WHERE id = ?`, gameID).
		Scan(&mode, &rec.Status, &created, &finishedAt, &autoSwap)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: game %d", ErrNotFound, gameID)
	}
	if err != nil {
		return nil, fmt.Errorf("load game: %w", err)
	}
	if mode != ModeATW {
		return nil, fmt.Errorf("%w: game %d is %s", ErrWrongMode, gameID, mode)
	}

	if rec.CreatedAt, err = parseTime(created); err != nil {
		return nil, fmt.Errorf("game %d has an unreadable created date: %w", gameID, err)
	}
	rec.FinishedAt = parseNullTime(finishedAt)
	rec.AutoSwap = autoSwap != 0

	if rec.PlayerIDs, err = participantIDs(ctx, s.db, gameID); err != nil {
		return nil, err
	}
	if rec.Visits, err = s.atwVisits(ctx, gameID); err != nil {
		return nil, err
	}
	if rec.Winners, err = s.atwWinners(ctx, gameID); err != nil {
		return nil, err
	}
	return rec, nil
}

// atwVisits reads a game's visits with their darts attached.
//
// Only the player and the darts are read back. Everything else about a visit —
// which target it started on, how far it carried the player, whether it
// completed the run — is recomputed by the engine from those darts, so the
// stored copies can never be the thing a resumed game disagrees with.
func (s *Store) atwVisits(ctx context.Context, gameID int64) ([]atw.Visit, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, seq, player_id FROM atw_visits WHERE game_id = ? ORDER BY seq ASC`, gameID)
	if err != nil {
		return nil, fmt.Errorf("load visits: %w", err)
	}
	defer rows.Close()

	var (
		out []atw.Visit
		ids []int64
	)
	for rows.Next() {
		var (
			id int64
			v  atw.Visit
		)
		if err := rows.Scan(&id, &v.Seq, &v.PlayerID); err != nil {
			return nil, err
		}
		out = append(out, v)
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i, id := range ids {
		if out[i].Darts, err = s.atwDarts(ctx, id); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Store) atwDarts(ctx context.Context, visitID int64) ([]atw.Dart, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT segment FROM atw_darts WHERE visit_id = ? ORDER BY idx ASC`, visitID)
	if err != nil {
		return nil, fmt.Errorf("load darts: %w", err)
	}
	defer rows.Close()

	var out []atw.Dart
	for rows.Next() {
		var notation string
		if err := rows.Scan(&notation); err != nil {
			return nil, err
		}
		d, err := atw.ParseDart(notation)
		if err != nil {
			return nil, fmt.Errorf("visit %d holds an unreadable dart %q: %w", visitID, notation, err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// atwWinners reads back who won. There may be more than one: two players
// completing a run in the same number of darts is a tie, and the games table
// has room for only one winner, so the result rows are what carry it.
func (s *Store) atwWinners(ctx context.Context, gameID int64) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT player_id FROM game_results WHERE game_id = ? AND won = 1 ORDER BY player_id ASC`,
		gameID)
	if err != nil {
		return nil, fmt.Errorf("load winners: %w", err)
	}
	defer rows.Close()

	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// FinishATWGame closes a game with the winner or winners the operator
// confirmed, and stores each participant's outcome including those who never
// threw.
//
// A tie is stored as a win against each tied player and no single winner on the
// game row, because there genuinely is not one. Everything that reports a win
// reads the per-player rows, so a tie is reported as two wins rather than as
// one arbitrary victory.
func (s *Store) FinishATWGame(ctx context.Context, gameID int64, winners []int64, results []atw.ParticipantResult) error {
	won := make(map[int64]bool, len(winners))
	for _, id := range winners {
		won[id] = true
	}

	return s.withTx(ctx, func(tx *sql.Tx) error {
		var winnerArg any
		if len(winners) == 1 {
			winnerArg = winners[0]
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE games SET status = ?, finished_at = ?, winner_player_id = ? WHERE id = ?`,
			StatusFinished, nowString(), winnerArg, gameID); err != nil {
			return fmt.Errorf("finish game: %w", err)
		}

		// Rewrite rather than append, so finishing is safely repeatable.
		if _, err := tx.ExecContext(ctx, `DELETE FROM game_results WHERE game_id = ?`, gameID); err != nil {
			return err
		}
		stmt, err := tx.PrepareContext(ctx, `
INSERT INTO game_results
    (game_id, player_id, completed, won, remaining, darts, visits, points, average)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, r := range results {
			// "points" carries the targets advanced. It is the only per-player
			// number the shared results table has left, and how far somebody got
			// is the Around the World equivalent of a score.
			if _, err := stmt.ExecContext(ctx, gameID, r.PlayerID,
				boolToInt(r.Completed), boolToInt(won[r.PlayerID]), r.Remaining,
				r.Darts, r.Visits, r.Advanced, 0.0); err != nil {
				return fmt.Errorf("record result for player %d: %w", r.PlayerID, err)
			}
		}
		return nil
	})
}

// ------------------------------------------------------------- statistics

// ATWVisitsForPlayer returns every Around the World visit a player has thrown in
// a finished game, with its darts counted by kind.
//
// The tallies are computed in SQL from the stored segments rather than from a
// column recording what the operator typed, so they describe the board rather
// than the keyboard.
func (s *Store) ATWVisitsForPlayer(ctx context.Context, playerID int64) ([]stats.ATWVisit, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT v.game_id, v.player_id, v.darts, v.advanced, v.completed,
       (SELECT COUNT(*) FROM atw_darts d WHERE d.visit_id = v.id AND d.segment <> '-'),
       (SELECT COUNT(*) FROM atw_darts d WHERE d.visit_id = v.id AND d.segment = '-'),
       (SELECT COUNT(*) FROM atw_darts d WHERE d.visit_id = v.id AND d.segment LIKE 'D%'),
       (SELECT COUNT(*) FROM atw_darts d WHERE d.visit_id = v.id AND d.segment LIKE 'T%')
FROM atw_visits v
JOIN games g ON g.id = v.game_id
WHERE v.player_id = ? AND g.status = 'finished'
ORDER BY v.game_id ASC, v.seq ASC`, playerID)
	if err != nil {
		return nil, fmt.Errorf("load Around the World visits: %w", err)
	}
	defer rows.Close()

	var out []stats.ATWVisit
	for rows.Next() {
		var (
			v         stats.ATWVisit
			completed int
		)
		if err := rows.Scan(&v.GameID, &v.PlayerID, &v.Darts, &v.Advanced, &completed,
			&v.Hits, &v.Misses, &v.Doubles, &v.Trebles); err != nil {
			return nil, err
		}
		v.Completed = completed != 0
		// Every hit that is not a double or a treble advances one target: a
		// plain single, the outer bull, or the bullseye.
		v.Singles = v.Hits - v.Doubles - v.Trebles
		out = append(out, v)
	}
	return out, rows.Err()
}

// ATWRunsForPlayer returns a player's outcome in each finished Around the World
// game. As with X01, the participant count comes from the game's own roster so
// solo practice is recognised without a stored flag.
func (s *Store) ATWRunsForPlayer(ctx context.Context, playerID int64) ([]stats.ATWRun, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT r.game_id, r.player_id, r.completed, r.won, r.darts, r.visits,
       (SELECT COUNT(*) FROM game_participants p WHERE p.game_id = g.id)
FROM game_results r
JOIN games g ON g.id = r.game_id
WHERE r.player_id = ? AND g.mode = 'atw' AND g.status = 'finished'
ORDER BY r.game_id ASC`, playerID)
	if err != nil {
		return nil, fmt.Errorf("load Around the World results: %w", err)
	}
	defer rows.Close()

	var out []stats.ATWRun
	for rows.Next() {
		var (
			r              stats.ATWRun
			completed, won int
		)
		if err := rows.Scan(&r.GameID, &r.PlayerID, &completed, &won,
			&r.Darts, &r.Visits, &r.Participants); err != nil {
			return nil, err
		}
		r.Completed = completed != 0
		r.Won = won != 0
		out = append(out, r)
	}
	return out, rows.Err()
}
