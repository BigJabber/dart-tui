package app

import (
	"context"
	"fmt"
	"time"

	"github.com/BigJabber/dart-tui/internal/domain/cricket"
	"github.com/BigJabber/dart-tui/internal/domain/player"
	"github.com/BigJabber/dart-tui/internal/domain/x01"
	"github.com/BigJabber/dart-tui/internal/persistence/sqlite"
)

// GameService starts, resumes and lists games.
type GameService struct {
	store *sqlite.Store
}

// ResumableGame describes an in-progress game on the resume screen.
type ResumableGame struct {
	ID        int64
	Mode      string
	Label     string
	StartedAt time.Time
	Players   []player.Player
	// VisitCount shows how far the game had got.
	VisitCount int
}

// Resumable lists the games that can be picked up again.
func (s *GameService) Resumable(ctx context.Context) ([]ResumableGame, error) {
	summaries, err := s.store.ListInProgressGames(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ResumableGame, 0, len(summaries))
	for _, g := range summaries {
		byID, err := s.store.PlayersByID(ctx, g.ParticipantIDs)
		if err != nil {
			return nil, err
		}
		rg := ResumableGame{
			ID:         g.ID,
			Mode:       g.Mode,
			Label:      GameLabel(g),
			StartedAt:  g.CreatedAt,
			VisitCount: g.VisitCount,
		}
		for _, id := range g.ParticipantIDs {
			if p, ok := byID[id]; ok {
				rg.Players = append(rg.Players, p)
			}
		}
		out = append(out, rg)
	}
	return out, nil
}

// Discard permanently removes an in-progress game. The UI confirms first.
func (s *GameService) Discard(ctx context.Context, gameID int64) error {
	return s.store.DeleteGame(ctx, gameID)
}

// ------------------------------------------------------------- X01 sessions

// X01Session is a live X01 game: the engine, the row it is stored in, and the
// players' display details.
//
// Every accepted visit is written to the database immediately, so closing the
// terminal mid-leg costs nothing.
type X01Session struct {
	store  *sqlite.Store
	gameID int64

	game    *x01.Game
	players map[int64]player.Player
	order   []int64
}

// StartX01 creates a new 301 or 501 game for the selected players.
func (s *GameService) StartX01(ctx context.Context, cfg x01.Config, playerIDs []int64) (*X01Session, error) {
	// Build the engine first: if the selection is invalid, nothing is written.
	game, err := x01.New(cfg, playerIDs)
	if err != nil {
		return nil, err
	}
	gameID, err := s.store.CreateX01Game(ctx, cfg, playerIDs)
	if err != nil {
		return nil, err
	}
	players, err := s.store.PlayersByID(ctx, playerIDs)
	if err != nil {
		return nil, err
	}
	return &X01Session{
		store:   s.store,
		gameID:  gameID,
		game:    game,
		players: players,
		order:   append([]int64(nil), playerIDs...),
	}, nil
}

// ResumeX01 reloads a stored game and replays it through the engine.
func (s *GameService) ResumeX01(ctx context.Context, gameID int64) (*X01Session, error) {
	rec, err := s.store.LoadX01Game(ctx, gameID)
	if err != nil {
		return nil, err
	}
	game, err := rec.Restore()
	if err != nil {
		return nil, fmt.Errorf("replay game %d: %w", gameID, err)
	}
	players, err := s.store.PlayersByID(ctx, rec.PlayerIDs)
	if err != nil {
		return nil, err
	}
	return &X01Session{
		store:   s.store,
		gameID:  gameID,
		game:    game,
		players: players,
		order:   rec.PlayerIDs,
	}, nil
}

// GameID is the database identifier of this game.
func (s *X01Session) GameID() int64 { return s.gameID }

// Game exposes the engine for read-only queries by the screens.
func (s *X01Session) Game() *x01.Game { return s.game }

// Order returns the players in selection order. It is display metadata: any
// unfinished player may throw at any time.
func (s *X01Session) Order() []int64 { return s.order }

// Player returns a participant's display details.
func (s *X01Session) Player(id int64) player.Player {
	if p, ok := s.players[id]; ok {
		return p
	}
	return player.Player{ID: id, Name: fmt.Sprintf("Player %d", id), Colour: player.DefaultColour}
}

// Record applies a visit and stores it.
//
// If the write fails, the session reloads from the database so the on-screen
// state can never claim a visit that was not saved.
func (s *X01Session) Record(ctx context.Context, in x01.VisitInput) (x01.Visit, error) {
	v, err := s.game.Record(in)
	if err != nil {
		return x01.Visit{}, err
	}
	if err := s.store.AppendX01Visit(ctx, s.gameID, v); err != nil {
		s.resync(ctx)
		return x01.Visit{}, err
	}
	return v, nil
}

// Undo removes the most recent visit from both the engine and the database.
func (s *X01Session) Undo(ctx context.Context) (x01.Visit, error) {
	v, err := s.game.Undo()
	if err != nil {
		return x01.Visit{}, err
	}
	if err := s.store.DeleteLastX01Visit(ctx, s.gameID); err != nil {
		s.resync(ctx)
		return x01.Visit{}, err
	}
	return v, nil
}

// Finish closes the game with the winner the operator confirmed. Passing nil
// records a game that ended without one.
func (s *X01Session) Finish(ctx context.Context, winner *int64) error {
	if err := s.game.Finish(winner); err != nil {
		return err
	}
	if err := s.store.FinishX01Game(ctx, s.gameID, winner, s.game.Results()); err != nil {
		s.resync(ctx)
		return err
	}
	return nil
}

// resync reloads the engine from storage after a failed write, so the screen
// shows what was actually saved rather than what was attempted.
func (s *X01Session) resync(ctx context.Context) {
	rec, err := s.store.LoadX01Game(ctx, s.gameID)
	if err != nil {
		return
	}
	if game, err := rec.Restore(); err == nil {
		s.game = game
	}
}

// --------------------------------------------------------- cricket sessions

// CricketSession is a live cricket game.
//
// Cricket ends itself: when the chase succeeds or a side is bowled out the
// engine completes the game, and the session records the result at that moment.
type CricketSession struct {
	store  *sqlite.Store
	gameID int64

	game    *cricket.Game
	players map[int64]player.Player
	// finished guards against writing the result twice.
	finished bool
}

// StartCricket creates a cricket game from the configured teams.
func (s *GameService) StartCricket(ctx context.Context, cfg cricket.Config, teamA, teamB cricket.Team) (*CricketSession, error) {
	teamA.Side, teamB.Side = cricket.TeamA, cricket.TeamB
	game, err := cricket.New(cfg, teamA, teamB)
	if err != nil {
		return nil, err
	}
	gameID, err := s.store.CreateCricketGame(ctx, cfg, game.Team(cricket.TeamA), game.Team(cricket.TeamB))
	if err != nil {
		return nil, err
	}
	players, err := s.store.PlayersByID(ctx, game.Players())
	if err != nil {
		return nil, err
	}
	return &CricketSession{store: s.store, gameID: gameID, game: game, players: players}, nil
}

// ResumeCricket reloads a stored cricket game and replays its events.
func (s *GameService) ResumeCricket(ctx context.Context, gameID int64) (*CricketSession, error) {
	rec, err := s.store.LoadCricketGame(ctx, gameID)
	if err != nil {
		return nil, err
	}
	game, err := rec.Restore()
	if err != nil {
		return nil, fmt.Errorf("replay game %d: %w", gameID, err)
	}
	players, err := s.store.PlayersByID(ctx, game.Players())
	if err != nil {
		return nil, err
	}
	return &CricketSession{
		store:    s.store,
		gameID:   gameID,
		game:     game,
		players:  players,
		finished: rec.Status == sqlite.StatusFinished,
	}, nil
}

// GameID is the database identifier of this game.
func (s *CricketSession) GameID() int64 { return s.gameID }

// Game exposes the engine for read-only queries by the screens.
func (s *CricketSession) Game() *cricket.Game { return s.game }

// Player returns a participant's display details.
func (s *CricketSession) Player(id int64) player.Player {
	if p, ok := s.players[id]; ok {
		return p
	}
	return player.Player{ID: id, Name: fmt.Sprintf("Player %d", id), Colour: player.DefaultColour}
}

// RecordBatting records a batting visit from its raw dartboard score.
func (s *CricketSession) RecordBatting(ctx context.Context, playerID int64, rawPoints, dartCount int) (cricket.Event, error) {
	e, err := s.game.RecordBatting(playerID, rawPoints, dartCount)
	if err != nil {
		return cricket.Event{}, err
	}
	return e, s.persist(ctx, e)
}

// RecordBowling records a bowling visit from its darts.
func (s *CricketSession) RecordBowling(ctx context.Context, playerID int64, bowls []cricket.Bowl) (cricket.Event, error) {
	e, err := s.game.RecordBowling(playerID, bowls)
	if err != nil {
		return cricket.Event{}, err
	}
	return e, s.persist(ctx, e)
}

// persist writes an event and, if that event ended the game, its result.
func (s *CricketSession) persist(ctx context.Context, e cricket.Event) error {
	if err := s.store.AppendCricketEvent(ctx, s.gameID, e); err != nil {
		s.resync(ctx)
		return err
	}
	return s.recordResultIfComplete(ctx)
}

// Undo removes the most recent visit. If it had ended the game, the game is
// reopened in the database too.
func (s *CricketSession) Undo(ctx context.Context) (cricket.Event, error) {
	e, err := s.game.Undo()
	if err != nil {
		return cricket.Event{}, err
	}
	if err := s.store.DeleteLastCricketEvent(ctx, s.gameID); err != nil {
		s.resync(ctx)
		return cricket.Event{}, err
	}
	if s.finished && !s.game.Complete() {
		if err := s.store.ReopenGame(ctx, s.gameID); err != nil {
			s.resync(ctx)
			return cricket.Event{}, err
		}
		s.finished = false
	}
	return e, nil
}

// Abandon ends a game the office stopped part-way through, with no winner.
func (s *CricketSession) Abandon(ctx context.Context) error {
	if err := s.game.Abandon(); err != nil {
		return err
	}
	return s.recordResultIfComplete(ctx)
}

// recordResultIfComplete stores the outcome once the engine says the game is
// over, whether that came from the rules or from the operator abandoning it.
func (s *CricketSession) recordResultIfComplete(ctx context.Context) error {
	if !s.game.Complete() || s.finished {
		return nil
	}
	winningSide, hasWinner := s.game.WinningSide()

	var entries []sqlite.CricketResultEntry
	for _, side := range []cricket.Side{cricket.TeamA, cricket.TeamB} {
		for _, id := range s.game.Team(side).Players {
			st := s.game.StatsFor(id)
			entries = append(entries, sqlite.CricketResultEntry{
				PlayerID: id,
				TeamSide: side.String(),
				Won:      hasWinner && side == winningSide,
				Darts:    st.BattingDarts + st.BowlingDarts,
				Visits:   st.BattingVisits + st.BowlingVisits,
				Points:   st.RawPoints,
			})
		}
	}
	if err := s.store.FinishCricketGame(ctx, s.gameID, s.game.Result(), entries); err != nil {
		s.resync(ctx)
		return err
	}
	s.finished = true
	return nil
}

func (s *CricketSession) resync(ctx context.Context) {
	rec, err := s.store.LoadCricketGame(ctx, s.gameID)
	if err != nil {
		return
	}
	if game, err := rec.Restore(); err == nil {
		s.game = game
		s.finished = rec.Status == sqlite.StatusFinished
	}
}

// TeamPlayers returns one side's players in throwing order.
func (s *CricketSession) TeamPlayers(side cricket.Side) []player.Player {
	team := s.game.Team(side)
	out := make([]player.Player, 0, len(team.Players))
	for _, id := range team.Players {
		out = append(out, s.Player(id))
	}
	return out
}
