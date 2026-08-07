package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/pourmoi/dart-tui/internal/domain/player"
)

// ErrDuplicateName is returned when a player name is already taken.
var ErrDuplicateName = errors.New("a player with that name already exists")

const playerColumns = `id, name, colour, created_at, archived_at`

// CreatePlayer registers a new player, assigning a display colour automatically.
//
// The colour is picked from the palette preferring one nobody is using, which is
// why the selection happens here where the in-use colours are known.
func (s *Store) CreatePlayer(ctx context.Context, name string) (player.Player, error) {
	cleanName, err := player.ValidateName(name)
	if err != nil {
		return player.Player{}, err
	}

	var created player.Player
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		used, err := usedColours(ctx, tx)
		if err != nil {
			return err
		}
		colour := player.PickColour(used)
		now := nowString()

		res, err := tx.ExecContext(ctx,
			`INSERT INTO players (name, colour, created_at) VALUES (?, ?, ?)`,
			cleanName, colour, now,
		)
		if err != nil {
			if isUniqueViolation(err) {
				return fmt.Errorf("%w: %s", ErrDuplicateName, cleanName)
			}
			return fmt.Errorf("insert player: %w", err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		t, err := parseTime(now)
		if err != nil {
			return err
		}
		created = player.Player{ID: id, Name: cleanName, Colour: colour, CreatedAt: t}
		return nil
	})
	return created, err
}

// usedColours lists the colours currently assigned, including archived players
// so an archived player's colour is not immediately reused by someone else.
func usedColours(ctx context.Context, tx *sql.Tx) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT colour FROM players`)
	if err != nil {
		return nil, fmt.Errorf("read colours in use: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetPlayer loads a single player.
func (s *Store) GetPlayer(ctx context.Context, id int64) (player.Player, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+playerColumns+` FROM players WHERE id = ?`, id)
	p, err := scanPlayer(row)
	if errors.Is(err, sql.ErrNoRows) {
		return player.Player{}, fmt.Errorf("%w: player %d", ErrNotFound, id)
	}
	return p, err
}

// ListPlayers returns players ordered by name. Archived players are excluded
// unless asked for: they keep their history but should not clutter game setup.
func (s *Store) ListPlayers(ctx context.Context, includeArchived bool) ([]player.Player, error) {
	query := `SELECT ` + playerColumns + ` FROM players`
	if !includeArchived {
		query += ` WHERE archived_at IS NULL`
	}
	query += ` ORDER BY name COLLATE NOCASE ASC`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list players: %w", err)
	}
	defer rows.Close()

	var out []player.Player
	for rows.Next() {
		p, err := scanPlayer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PlayersByID loads the given players into a lookup map, which the UI uses to
// colour names without a query per row.
func (s *Store) PlayersByID(ctx context.Context, ids []int64) (map[int64]player.Player, error) {
	out := make(map[int64]player.Player, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+playerColumns+` FROM players WHERE id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("load players: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		p, err := scanPlayer(rows)
		if err != nil {
			return nil, err
		}
		out[p.ID] = p
	}
	return out, rows.Err()
}

// RenamePlayer changes a player's display name, keeping all their history.
func (s *Store) RenamePlayer(ctx context.Context, id int64, name string) error {
	cleanName, err := player.ValidateName(name)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE players SET name = ? WHERE id = ?`, cleanName, id)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: %s", ErrDuplicateName, cleanName)
		}
		return fmt.Errorf("rename player: %w", err)
	}
	return requireOneRow(res, id)
}

// SetPlayerColour changes a player's display colour.
func (s *Store) SetPlayerColour(ctx context.Context, id int64, colour string) error {
	clean, err := player.ValidateColour(colour)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE players SET colour = ? WHERE id = ?`, clean, id)
	if err != nil {
		return fmt.Errorf("set player colour: %w", err)
	}
	return requireOneRow(res, id)
}

// SetPlayerArchived archives or restores a player.
//
// Archiving is the only form of removal the application offers. Historic visits,
// results and every statistic derived from them are untouched, so an archived
// player still appears correctly in the games they played.
func (s *Store) SetPlayerArchived(ctx context.Context, id int64, archived bool) error {
	var res sql.Result
	var err error
	if archived {
		res, err = s.db.ExecContext(ctx,
			`UPDATE players SET archived_at = ? WHERE id = ?`, nowString(), id)
	} else {
		res, err = s.db.ExecContext(ctx,
			`UPDATE players SET archived_at = NULL WHERE id = ?`, id)
	}
	if err != nil {
		return fmt.Errorf("archive player: %w", err)
	}
	return requireOneRow(res, id)
}

// rowScanner covers both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanPlayer(sc rowScanner) (player.Player, error) {
	var (
		p          player.Player
		created    string
		archivedAt sql.NullString
	)
	if err := sc.Scan(&p.ID, &p.Name, &p.Colour, &created, &archivedAt); err != nil {
		return player.Player{}, err
	}
	t, err := parseTime(created)
	if err != nil {
		return player.Player{}, fmt.Errorf("player %d has an unreadable created date: %w", p.ID, err)
	}
	p.CreatedAt = t
	p.ArchivedAt = parseNullTime(archivedAt)
	// A stored colour that cannot be parsed must not make the player invisible.
	p.Colour = player.SafeColour(p.Colour)
	return p, nil
}

func requireOneRow(res sql.Result, id int64) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: player %d", ErrNotFound, id)
	}
	return nil
}

// isUniqueViolation detects a UNIQUE constraint failure without depending on
// driver-specific error types.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToUpper(err.Error())
	return strings.Contains(msg, "UNIQUE CONSTRAINT FAILED") || strings.Contains(msg, "CONSTRAINT FAILED: UNIQUE")
}
