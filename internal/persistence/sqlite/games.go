package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/BigJabber/dart-tui/internal/domain/cricket"
	"github.com/BigJabber/dart-tui/internal/domain/x01"
)

// Game modes as stored in the games table.
const (
	ModeX01     = "x01"
	ModeCricket = "cricket"
	ModeATW     = "atw"
)

// Game statuses as stored in the games table.
const (
	StatusInProgress = "in_progress"
	StatusFinished   = "finished"
)

// ErrWrongMode is returned when a game is loaded as the wrong kind of game.
var ErrWrongMode = errors.New("that game is not of the expected type")

// GameSummary is the row-level view of a game used by the resume and history
// lists. It carries just enough to describe the game without replaying it.
type GameSummary struct {
	ID         int64
	Mode       string
	Status     string
	CreatedAt  time.Time
	FinishedAt *time.Time

	// X01 configuration.
	StartScore int
	DoubleOut  bool

	// Cricket configuration.
	WicketLimit  int
	FirstBatting string

	WinnerPlayerID *int64
	Outcome        string

	// ParticipantIDs is every player in the game, in selection order.
	ParticipantIDs []int64
	// VisitCount is how many visits have been recorded, so a resumable game can
	// show its progress.
	VisitCount int
}

// InProgress reports whether the game can be resumed.
func (g GameSummary) InProgress() bool { return g.Status == StatusInProgress }

// ---------------------------------------------------------------- X01 games

// X01GameRecord is a persisted X01 game, ready to be replayed by the engine.
type X01GameRecord struct {
	ID         int64
	Config     x01.Config
	PlayerIDs  []int64
	Visits     []x01.Visit
	Status     string
	CreatedAt  time.Time
	FinishedAt *time.Time
	Winner     *int64
	// AutoSwap is whether the scoreboard moves to the next player in sequence
	// after each visit. It is a scoring convenience, not a rule: the engine
	// never sees it.
	AutoSwap bool
}

// Restore turns the record back into a live engine by replaying its visits.
func (r *X01GameRecord) Restore() (*x01.Game, error) {
	return x01.Restore(r.Config, r.PlayerIDs, r.Visits, r.Status == StatusFinished, r.Winner)
}

// CreateX01Game records a new X01 game and its participants.
//
// The game is written before the first dart is thrown so that an accidental
// exit — a closed terminal, a dead battery — loses nothing.
func (s *Store) CreateX01Game(ctx context.Context, cfg x01.Config, playerIDs []int64) (int64, error) {
	if len(playerIDs) == 0 {
		return 0, x01.ErrNoPlayers
	}
	var gameID int64
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
INSERT INTO games (mode, status, created_at, start_score, double_out)
VALUES (?, ?, ?, ?, ?)`,
			ModeX01, StatusInProgress, nowString(), cfg.StartScore, boolToInt(cfg.DoubleOut))
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

// insertParticipants records who is playing, in order. sides may be nil for X01,
// where the recorded position is display metadata and never restricts play.
func insertParticipants(ctx context.Context, tx *sql.Tx, gameID int64, playerIDs []int64, sides map[int64]string) error {
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO game_participants (game_id, player_id, position, team_side) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i, id := range playerIDs {
		var side any
		if sides != nil {
			if s, ok := sides[id]; ok {
				side = s
			}
		}
		if _, err := stmt.ExecContext(ctx, gameID, id, i, side); err != nil {
			return fmt.Errorf("add participant %d: %w", id, err)
		}
	}
	return nil
}

// AppendX01Visit stores one visit. It is called immediately after the engine
// accepts a visit, so the database is never more than one entry behind the board.
func (s *Store) AppendX01Visit(ctx context.Context, gameID int64, v x01.Visit) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO x01_visits
    (game_id, seq, player_id, points, darts, confirm_checkout,
     credited, bust, checkout, score_before, score_after)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		gameID, v.Seq, v.PlayerID, v.Points, v.Darts, boolToInt(v.ConfirmCheckout),
		v.Credited, boolToInt(v.Bust), boolToInt(v.Checkout), v.ScoreBefore, v.ScoreAfter)
	if err != nil {
		return fmt.Errorf("record visit: %w", err)
	}
	return nil
}

// DeleteLastX01Visit removes the most recently recorded visit, mirroring an
// undo in the engine.
func (s *Store) DeleteLastX01Visit(ctx context.Context, gameID int64) error {
	_, err := s.db.ExecContext(ctx, `
DELETE FROM x01_visits
WHERE game_id = ?
  AND seq = (SELECT MAX(seq) FROM x01_visits WHERE game_id = ?)`, gameID, gameID)
	if err != nil {
		return fmt.Errorf("undo visit: %w", err)
	}
	return nil
}

// LoadX01Game reads a game and its visits back from storage.
func (s *Store) LoadX01Game(ctx context.Context, gameID int64) (*X01GameRecord, error) {
	rec := &X01GameRecord{ID: gameID}

	var (
		mode       string
		created    string
		finishedAt sql.NullString
		startScore sql.NullInt64
		doubleOut  sql.NullInt64
		winner     sql.NullInt64
		autoSwap   int
	)
	err := s.db.QueryRowContext(ctx, `
SELECT mode, status, created_at, finished_at, start_score, double_out, winner_player_id, auto_swap
FROM games WHERE id = ?`, gameID).
		Scan(&mode, &rec.Status, &created, &finishedAt, &startScore, &doubleOut, &winner, &autoSwap)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: game %d", ErrNotFound, gameID)
	}
	if err != nil {
		return nil, fmt.Errorf("load game: %w", err)
	}
	if mode != ModeX01 {
		return nil, fmt.Errorf("%w: game %d is %s", ErrWrongMode, gameID, mode)
	}

	if rec.CreatedAt, err = parseTime(created); err != nil {
		return nil, fmt.Errorf("game %d has an unreadable created date: %w", gameID, err)
	}
	rec.FinishedAt = parseNullTime(finishedAt)
	rec.AutoSwap = autoSwap != 0
	rec.Config = x01.Config{
		StartScore: int(startScore.Int64),
		DoubleOut:  doubleOut.Int64 != 0,
	}
	if winner.Valid {
		w := winner.Int64
		rec.Winner = &w
	}

	if rec.PlayerIDs, err = participantIDs(ctx, s.db, gameID); err != nil {
		return nil, err
	}
	if rec.Visits, err = s.x01Visits(ctx, gameID); err != nil {
		return nil, err
	}
	return rec, nil
}

func (s *Store) x01Visits(ctx context.Context, gameID int64) ([]x01.Visit, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT seq, player_id, points, darts, confirm_checkout,
       credited, bust, checkout, score_before, score_after
FROM x01_visits WHERE game_id = ? ORDER BY seq ASC`, gameID)
	if err != nil {
		return nil, fmt.Errorf("load visits: %w", err)
	}
	defer rows.Close()

	var out []x01.Visit
	for rows.Next() {
		var (
			v                           x01.Visit
			confirm, bust, checkoutFlag int
		)
		if err := rows.Scan(&v.Seq, &v.PlayerID, &v.Points, &v.Darts, &confirm,
			&v.Credited, &bust, &checkoutFlag, &v.ScoreBefore, &v.ScoreAfter); err != nil {
			return nil, err
		}
		v.ConfirmCheckout = confirm != 0
		v.Bust = bust != 0
		v.Checkout = checkoutFlag != 0
		out = append(out, v)
	}
	return out, rows.Err()
}

// FinishX01Game closes a game with the winner the operator confirmed and stores
// each participant's outcome, including those who never threw.
func (s *Store) FinishX01Game(ctx context.Context, gameID int64, winner *int64, results []x01.ParticipantResult) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		var winnerArg any
		if winner != nil {
			winnerArg = *winner
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
			won := winner != nil && *winner == r.PlayerID
			if _, err := stmt.ExecContext(ctx, gameID, r.PlayerID,
				boolToInt(r.Completed), boolToInt(won), r.Remaining,
				r.Darts, r.Visits, r.Credited, r.Average); err != nil {
				return fmt.Errorf("record result for player %d: %w", r.PlayerID, err)
			}
		}
		return nil
	})
}

// ------------------------------------------------------------ cricket games

// CricketGameRecord is a persisted cricket game, ready to be replayed.
type CricketGameRecord struct {
	ID           int64
	Config       cricket.Config
	TeamA        cricket.Team
	TeamB        cricket.Team
	Events       []cricket.Event
	Status       string
	CreatedAt    time.Time
	FinishedAt   *time.Time
	StoredResult cricket.Result
}

// Restore turns the record back into a live engine by replaying its events.
func (r *CricketGameRecord) Restore() (*cricket.Game, error) {
	g, err := cricket.Restore(r.Config, r.TeamA, r.TeamB, r.Events)
	if err != nil {
		return nil, err
	}
	// A game the operator abandoned cannot be re-derived from the event log,
	// because nothing in the events ended it. Reapply that decision.
	if r.Status == StatusFinished && r.StoredResult == cricket.ResultAbandoned && !g.Complete() {
		if err := g.Abandon(); err != nil {
			return nil, err
		}
	}
	return g, nil
}

// CreateCricketGame records a new cricket game, its teams and their orders.
func (s *Store) CreateCricketGame(ctx context.Context, cfg cricket.Config, teamA, teamB cricket.Team) (int64, error) {
	var gameID int64
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
INSERT INTO games (mode, status, created_at, wicket_limit, first_batting)
VALUES (?, ?, ?, ?, ?)`,
			ModeCricket, StatusInProgress, nowString(), cfg.WicketLimit, cfg.FirstBatting.String())
		if err != nil {
			return fmt.Errorf("create game: %w", err)
		}
		if gameID, err = res.LastInsertId(); err != nil {
			return err
		}

		sides := map[int64]string{}
		var ordered []int64
		for _, team := range []cricket.Team{teamA, teamB} {
			teamRes, err := tx.ExecContext(ctx,
				`INSERT INTO teams (game_id, side, name) VALUES (?, ?, ?)`,
				gameID, team.Side.String(), team.Name)
			if err != nil {
				return fmt.Errorf("create team %s: %w", team.Side, err)
			}
			teamID, err := teamRes.LastInsertId()
			if err != nil {
				return err
			}
			for i, playerID := range team.Players {
				if _, err := tx.ExecContext(ctx,
					`INSERT INTO team_players (team_id, player_id, position) VALUES (?, ?, ?)`,
					teamID, playerID, i); err != nil {
					return fmt.Errorf("add player %d to team %s: %w", playerID, team.Side, err)
				}
				sides[playerID] = team.Side.String()
				ordered = append(ordered, playerID)
			}
		}
		return insertParticipants(ctx, tx, gameID, ordered, sides)
	})
	return gameID, err
}

// AppendCricketEvent stores one visit and, for a bowling visit, the individual
// darts that made it up.
func (s *Store) AppendCricketEvent(ctx context.Context, gameID int64, e cricket.Event) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
INSERT INTO cricket_events
    (game_id, seq, innings, kind, side, player_id, raw_points, runs, darts, wickets)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			gameID, e.Seq, e.Innings, e.Kind.String(), e.Side.String(), e.PlayerID,
			e.RawPoints, e.Runs, e.Darts, e.Wickets)
		if err != nil {
			return fmt.Errorf("record cricket visit: %w", err)
		}
		if e.Kind != cricket.Bowling || len(e.Bowls) == 0 {
			return nil
		}
		eventID, err := res.LastInsertId()
		if err != nil {
			return err
		}
		stmt, err := tx.PrepareContext(ctx,
			`INSERT INTO bowling_darts (event_id, idx, result) VALUES (?, ?, ?)`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for i, b := range e.Bowls {
			if _, err := stmt.ExecContext(ctx, eventID, i, b.String()); err != nil {
				return fmt.Errorf("record bowling dart %d: %w", i+1, err)
			}
		}
		return nil
	})
}

// DeleteLastCricketEvent removes the most recent visit. The bowling darts go
// with it through the ON DELETE CASCADE.
func (s *Store) DeleteLastCricketEvent(ctx context.Context, gameID int64) error {
	_, err := s.db.ExecContext(ctx, `
DELETE FROM cricket_events
WHERE game_id = ?
  AND seq = (SELECT MAX(seq) FROM cricket_events WHERE game_id = ?)`, gameID, gameID)
	if err != nil {
		return fmt.Errorf("undo cricket visit: %w", err)
	}
	return nil
}

// LoadCricketGame reads a cricket game, its teams and its events.
func (s *Store) LoadCricketGame(ctx context.Context, gameID int64) (*CricketGameRecord, error) {
	rec := &CricketGameRecord{ID: gameID}

	var (
		mode         string
		created      string
		finishedAt   sql.NullString
		wicketLimit  sql.NullInt64
		firstBatting sql.NullString
		outcome      sql.NullString
	)
	err := s.db.QueryRowContext(ctx, `
SELECT mode, status, created_at, finished_at, wicket_limit, first_batting, outcome
FROM games WHERE id = ?`, gameID).
		Scan(&mode, &rec.Status, &created, &finishedAt, &wicketLimit, &firstBatting, &outcome)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: game %d", ErrNotFound, gameID)
	}
	if err != nil {
		return nil, fmt.Errorf("load game: %w", err)
	}
	if mode != ModeCricket {
		return nil, fmt.Errorf("%w: game %d is %s", ErrWrongMode, gameID, mode)
	}

	if rec.CreatedAt, err = parseTime(created); err != nil {
		return nil, fmt.Errorf("game %d has an unreadable created date: %w", gameID, err)
	}
	rec.FinishedAt = parseNullTime(finishedAt)
	rec.StoredResult = cricket.ParseResult(outcome.String)

	side, err := cricket.ParseSide(firstBatting.String)
	if err != nil {
		return nil, fmt.Errorf("game %d: %w", gameID, err)
	}
	rec.Config = cricket.Config{WicketLimit: int(wicketLimit.Int64), FirstBatting: side}

	if rec.TeamA, rec.TeamB, err = s.cricketTeams(ctx, gameID); err != nil {
		return nil, err
	}
	if rec.Events, err = s.cricketEvents(ctx, gameID); err != nil {
		return nil, err
	}
	return rec, nil
}

func (s *Store) cricketTeams(ctx context.Context, gameID int64) (cricket.Team, cricket.Team, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT t.side, t.name, tp.player_id
FROM teams t
LEFT JOIN team_players tp ON tp.team_id = t.id
WHERE t.game_id = ?
ORDER BY t.side ASC, tp.position ASC`, gameID)
	if err != nil {
		return cricket.Team{}, cricket.Team{}, fmt.Errorf("load teams: %w", err)
	}
	defer rows.Close()

	teams := map[cricket.Side]*cricket.Team{
		cricket.TeamA: {Side: cricket.TeamA},
		cricket.TeamB: {Side: cricket.TeamB},
	}
	for rows.Next() {
		var (
			sideStr  string
			name     string
			playerID sql.NullInt64
		)
		if err := rows.Scan(&sideStr, &name, &playerID); err != nil {
			return cricket.Team{}, cricket.Team{}, err
		}
		side, err := cricket.ParseSide(sideStr)
		if err != nil {
			return cricket.Team{}, cricket.Team{}, err
		}
		teams[side].Name = name
		if playerID.Valid {
			teams[side].Players = append(teams[side].Players, playerID.Int64)
		}
	}
	if err := rows.Err(); err != nil {
		return cricket.Team{}, cricket.Team{}, err
	}
	return *teams[cricket.TeamA], *teams[cricket.TeamB], nil
}

func (s *Store) cricketEvents(ctx context.Context, gameID int64) ([]cricket.Event, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, seq, innings, kind, side, player_id, raw_points, runs, darts, wickets
FROM cricket_events WHERE game_id = ? ORDER BY seq ASC`, gameID)
	if err != nil {
		return nil, fmt.Errorf("load cricket visits: %w", err)
	}
	defer rows.Close()

	var (
		out      []cricket.Event
		eventIDs []int64
		byID     = map[int64]int{}
	)
	for rows.Next() {
		var (
			id      int64
			e       cricket.Event
			kindStr string
			sideStr string
		)
		if err := rows.Scan(&id, &e.Seq, &e.Innings, &kindStr, &sideStr, &e.PlayerID,
			&e.RawPoints, &e.Runs, &e.Darts, &e.Wickets); err != nil {
			return nil, err
		}
		if e.Kind, err = cricket.ParseKind(kindStr); err != nil {
			return nil, err
		}
		if e.Side, err = cricket.ParseSide(sideStr); err != nil {
			return nil, err
		}
		byID[id] = len(out)
		eventIDs = append(eventIDs, id)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(eventIDs) == 0 {
		return out, nil
	}

	// Attach the individual bowling darts.
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(eventIDs)), ",")
	args := make([]any, 0, len(eventIDs))
	for _, id := range eventIDs {
		args = append(args, id)
	}
	dartRows, err := s.db.QueryContext(ctx, `
SELECT event_id, result FROM bowling_darts
WHERE event_id IN (`+placeholders+`) ORDER BY event_id ASC, idx ASC`, args...)
	if err != nil {
		return nil, fmt.Errorf("load bowling darts: %w", err)
	}
	defer dartRows.Close()

	for dartRows.Next() {
		var (
			eventID int64
			result  string
		)
		if err := dartRows.Scan(&eventID, &result); err != nil {
			return nil, err
		}
		b, err := cricket.ParseBowl([]rune(result)[0])
		if err != nil {
			return nil, err
		}
		idx, ok := byID[eventID]
		if !ok {
			continue
		}
		out[idx].Bowls = append(out[idx].Bowls, b)
	}
	return out, dartRows.Err()
}

// CricketResultEntry is one player's stored outcome in a cricket game.
type CricketResultEntry struct {
	PlayerID int64
	TeamSide string
	Won      bool
	Darts    int
	Visits   int
	Points   int
}

// FinishCricketGame closes a cricket game with its result and per-player rows.
func (s *Store) FinishCricketGame(ctx context.Context, gameID int64, result cricket.Result, entries []CricketResultEntry) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
UPDATE games SET status = ?, finished_at = ?, outcome = ? WHERE id = ?`,
			StatusFinished, nowString(), result.String(), gameID); err != nil {
			return fmt.Errorf("finish game: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM game_results WHERE game_id = ?`, gameID); err != nil {
			return err
		}
		stmt, err := tx.PrepareContext(ctx, `
INSERT INTO game_results
    (game_id, player_id, team_side, completed, won, darts, visits, points)
VALUES (?, ?, ?, 1, ?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, e := range entries {
			if _, err := stmt.ExecContext(ctx, gameID, e.PlayerID, e.TeamSide,
				boolToInt(e.Won), e.Darts, e.Visits, e.Points); err != nil {
				return fmt.Errorf("record result for player %d: %w", e.PlayerID, err)
			}
		}
		return nil
	})
}

// ------------------------------------------------------------------- listing

func participantIDs(ctx context.Context, q queryer, gameID int64) ([]int64, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT player_id FROM game_participants WHERE game_id = ? ORDER BY position ASC`, gameID)
	if err != nil {
		return nil, fmt.Errorf("load participants: %w", err)
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

type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// ListInProgressGames returns resumable games, most recent first.
func (s *Store) ListInProgressGames(ctx context.Context) ([]GameSummary, error) {
	return s.listGames(ctx, `WHERE g.status = 'in_progress'`, `ORDER BY g.created_at DESC`, nil)
}

// ListRecentGames returns the most recently finished games.
func (s *Store) ListRecentGames(ctx context.Context, limit int) ([]GameSummary, error) {
	return s.listGames(ctx,
		`WHERE g.status = 'finished'`,
		fmt.Sprintf(`ORDER BY g.finished_at DESC LIMIT %d`, clampLimit(limit)), nil)
}

// ListRecentGamesForPlayer returns a player's most recent finished games, for
// their profile screen.
func (s *Store) ListRecentGamesForPlayer(ctx context.Context, playerID int64, limit int) ([]GameSummary, error) {
	return s.listGames(ctx,
		`WHERE g.status = 'finished'
   AND EXISTS (SELECT 1 FROM game_participants gp WHERE gp.game_id = g.id AND gp.player_id = ?)`,
		fmt.Sprintf(`ORDER BY g.finished_at DESC LIMIT %d`, clampLimit(limit)),
		[]any{playerID})
}

func clampLimit(limit int) int {
	if limit <= 0 {
		return 10
	}
	if limit > 500 {
		return 500
	}
	return limit
}

func (s *Store) listGames(ctx context.Context, where, order string, args []any) ([]GameSummary, error) {
	query := `
SELECT g.id, g.mode, g.status, g.created_at, g.finished_at,
       g.start_score, g.double_out, g.wicket_limit, g.first_batting,
       g.winner_player_id, g.outcome,
       (SELECT COUNT(*) FROM x01_visits v WHERE v.game_id = g.id)
     + (SELECT COUNT(*) FROM cricket_events e WHERE e.game_id = g.id)
     + (SELECT COUNT(*) FROM atw_visits a WHERE a.game_id = g.id)
FROM games g
` + where + `
` + order

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list games: %w", err)
	}
	defer rows.Close()

	var out []GameSummary
	for rows.Next() {
		var (
			g            GameSummary
			created      string
			finishedAt   sql.NullString
			startScore   sql.NullInt64
			doubleOut    sql.NullInt64
			wicketLimit  sql.NullInt64
			firstBatting sql.NullString
			winner       sql.NullInt64
			outcome      sql.NullString
		)
		if err := rows.Scan(&g.ID, &g.Mode, &g.Status, &created, &finishedAt,
			&startScore, &doubleOut, &wicketLimit, &firstBatting,
			&winner, &outcome, &g.VisitCount); err != nil {
			return nil, err
		}
		if g.CreatedAt, err = parseTime(created); err != nil {
			return nil, fmt.Errorf("game %d has an unreadable created date: %w", g.ID, err)
		}
		g.FinishedAt = parseNullTime(finishedAt)
		g.StartScore = int(startScore.Int64)
		g.DoubleOut = doubleOut.Int64 != 0
		g.WicketLimit = int(wicketLimit.Int64)
		g.FirstBatting = firstBatting.String
		if winner.Valid {
			w := winner.Int64
			g.WinnerPlayerID = &w
		}
		g.Outcome = outcome.String
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range out {
		ids, err := participantIDs(ctx, s.db, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].ParticipantIDs = ids
	}
	return out, nil
}

// DeleteGame removes a game and everything recorded under it. It exists for
// discarding an abandoned in-progress game; the UI confirms first, because this
// is the one irreversible operation in the application.
func (s *Store) DeleteGame(ctx context.Context, gameID int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM games WHERE id = ?`, gameID)
	if err != nil {
		return fmt.Errorf("delete game: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: game %d", ErrNotFound, gameID)
	}
	return nil
}

// ResultRow is one participant's stored outcome, used to describe a finished
// game without replaying it.
type ResultRow struct {
	GameID    int64
	PlayerID  int64
	TeamSide  string
	Completed bool
	Won       bool
	Remaining *int
	Darts     int
	Visits    int
	Points    int
	Average   float64
}

// GameResults returns the stored outcomes for one game.
func (s *Store) GameResults(ctx context.Context, gameID int64) ([]ResultRow, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT game_id, player_id, team_side, completed, won, remaining, darts, visits, points, average
FROM game_results WHERE game_id = ?`, gameID)
	if err != nil {
		return nil, fmt.Errorf("load game results: %w", err)
	}
	defer rows.Close()

	var out []ResultRow
	for rows.Next() {
		var (
			r              ResultRow
			side           sql.NullString
			completed, won int
			remaining      sql.NullInt64
		)
		if err := rows.Scan(&r.GameID, &r.PlayerID, &side, &completed, &won,
			&remaining, &r.Darts, &r.Visits, &r.Points, &r.Average); err != nil {
			return nil, err
		}
		r.TeamSide = side.String
		r.Completed = completed != 0
		r.Won = won != 0
		if remaining.Valid {
			v := int(remaining.Int64)
			r.Remaining = &v
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetAutoSwap records whether a game moves to the next player in sequence after
// each visit. It applies to any mode, and can change at any point in the game.
func (s *Store) SetAutoSwap(ctx context.Context, gameID int64, on bool) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE games SET auto_swap = ? WHERE id = ?`, boolToInt(on), gameID)
	if err != nil {
		return fmt.Errorf("set auto swap: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("%w: game %d", ErrNotFound, gameID)
	}
	return nil
}

// ReopenGame returns a finished game to in-progress and clears its outcome.
//
// This exists for undo: if the visit that ended a cricket game is taken back,
// the stored game must stop claiming a result it no longer has.
func (s *Store) ReopenGame(ctx context.Context, gameID int64) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
UPDATE games
SET status = ?, finished_at = NULL, outcome = NULL, winner_player_id = NULL
WHERE id = ?`, StatusInProgress, gameID); err != nil {
			return fmt.Errorf("reopen game: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM game_results WHERE game_id = ?`, gameID); err != nil {
			return fmt.Errorf("clear results: %w", err)
		}
		return nil
	})
}
