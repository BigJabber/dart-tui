// Package x01 implements the X01 family of games (501, 301, and any other
// starting score) as a pure, terminal-free state machine.
//
// Two properties drive the design:
//
//   - Turn order is never enforced. The office does not play strict alternating
//     turns: one player may throw an entire leg from 501 to checkout before
//     another has thrown a single dart. The engine therefore never advances to
//     "the next player" — the caller names the thrower on every visit.
//
//   - State is derived from an ordered log of visits, never mutated in place.
//     Undo pops the last visit and replays the log, so scores, checkouts,
//     finish positions and averages all unwind together with no per-field
//     reversal logic to get wrong.
package x01

import (
	"errors"
	"fmt"
	"sort"

	"github.com/pourmoi/dart-tui/internal/domain/darts"
)

// Config describes the variant being played.
type Config struct {
	// StartScore is the score every player begins on, e.g. 501 or 301.
	StartScore int
	// DoubleOut requires the leg to be finished on a double. With it enabled a
	// visit that would leave exactly 1 is a bust, and a visit that reaches
	// exactly 0 must be confirmed by the operator as a genuine double finish.
	//
	// This is the office default. It is configurable so straight-out or
	// double-in variants can be added later without touching the engine.
	DoubleOut bool
}

// DefaultConfig returns the standard configuration for a starting score.
func DefaultConfig(startScore int) Config {
	return Config{StartScore: startScore, DoubleOut: true}
}

// Common starting scores.
const (
	Start301 = 301
	Start501 = 501
)

// VisitInput is what the operator records for one turn at the board.
type VisitInput struct {
	// PlayerID names the thrower. Any active, unfinished participant is valid
	// at any time; the engine imposes no ordering.
	PlayerID int64
	// Points is the raw total scored across the darts thrown.
	Points int
	// Darts is how many darts were actually thrown, 1 to 3. Recording this
	// exactly is what lets a 28-dart leg be told apart from a 30-dart leg.
	Darts int
	// ConfirmCheckout records the operator's confirmation that a visit landing
	// exactly on zero was a valid double finish. Because only visit totals are
	// captured, the engine cannot tell a legal checkout from an illegal one and
	// must ask. Ignored when DoubleOut is off.
	ConfirmCheckout bool
}

// Visit is a recorded turn together with everything the engine derived from it.
// The derived fields are persisted so the database is directly queryable for
// statistics, but they are always recomputed on replay and never trusted as the
// source of truth.
type Visit struct {
	Seq             int
	PlayerID        int64
	Points          int
	Darts           int
	ConfirmCheckout bool

	// Bust means the visit scored nothing and the score was restored.
	Bust bool
	// Checkout means the visit completed the player's leg.
	Checkout bool
	// Credited is the score actually deducted: Points normally, 0 on a bust.
	Credited    int
	ScoreBefore int
	ScoreAfter  int
}

// PlayerState is one participant's live position in the game.
type PlayerState struct {
	PlayerID int64
	// Position is the order the players were selected in. It is display
	// metadata only and never restricts who may throw.
	Position int

	Score  int
	Darts  int
	Visits int
	// Credited is the total score subtracted over the leg. It excludes busted
	// visits, which is what makes the three-dart average correct.
	Credited int
	Busts    int

	// Finished is set when the player has checked out.
	Finished bool
	// FinishDarts and FinishVisits are the totals at the moment of checkout.
	FinishDarts  int
	FinishVisits int
	// FinishSeq is the visit sequence number of the checkout, used only to
	// break ties between players who finished in the same number of darts.
	FinishSeq int
}

// Average is the player's three-dart average for this game so far.
func (s PlayerState) Average() float64 {
	return darts.ThreeDartAverage(s.Credited, s.Darts)
}

// Errors returned by the engine. They are all recoverable input problems: the
// TUI reports them and lets the operator correct the entry immediately.
var (
	ErrNoPlayers        = errors.New("a game needs at least one player")
	ErrDuplicatePlayer  = errors.New("a player cannot be selected twice")
	ErrBadStartScore    = errors.New("starting score must be positive")
	ErrUnknownPlayer    = errors.New("that player is not in this game")
	ErrPlayerFinished   = errors.New("that player has already checked out")
	ErrGameFinished     = errors.New("this game has already been finished")
	ErrNothingToUndo    = errors.New("there is nothing to undo")
	ErrBadDartCount     = errors.New("darts thrown must be 1, 2 or 3")
	ErrImpossibleScore  = errors.New("that score cannot be thrown with that many darts")
	ErrWinnerNotPlaying = errors.New("the winner must be one of the players in this game")
)

// Game is an X01 game in progress or finished.
type Game struct {
	cfg      Config
	order    []int64
	states   map[int64]*PlayerState
	visits   []Visit
	finished bool
	winner   *int64
}

// New creates a game for the given players, in selection order.
func New(cfg Config, playerIDs []int64) (*Game, error) {
	if cfg.StartScore <= 0 {
		return nil, ErrBadStartScore
	}
	if len(playerIDs) == 0 {
		return nil, ErrNoPlayers
	}
	seen := make(map[int64]bool, len(playerIDs))
	for _, id := range playerIDs {
		if seen[id] {
			return nil, ErrDuplicatePlayer
		}
		seen[id] = true
	}
	g := &Game{
		cfg:   cfg,
		order: append([]int64(nil), playerIDs...),
	}
	g.reset()
	return g, nil
}

// Config returns the variant being played.
func (g *Game) Config() Config { return g.cfg }

// Order returns the player IDs in selection order.
func (g *Game) Order() []int64 { return append([]int64(nil), g.order...) }

// Visits returns the recorded visit log, oldest first.
func (g *Game) Visits() []Visit { return append([]Visit(nil), g.visits...) }

// VisitCount returns how many visits have been recorded.
func (g *Game) VisitCount() int { return len(g.visits) }

// Finished reports whether the operator has finished the game.
func (g *Game) Finished() bool { return g.finished }

// Winner returns the recorded winner, if one was chosen.
func (g *Game) Winner() *int64 {
	if g.winner == nil {
		return nil
	}
	w := *g.winner
	return &w
}

// State returns a copy of one player's current position.
func (g *Game) State(playerID int64) (PlayerState, bool) {
	s, ok := g.states[playerID]
	if !ok {
		return PlayerState{}, false
	}
	return *s, true
}

// States returns every player's position, in selection order.
func (g *Game) States() []PlayerState {
	out := make([]PlayerState, 0, len(g.order))
	for _, id := range g.order {
		out = append(out, *g.states[id])
	}
	return out
}

// ActivePlayers returns the players who may still throw: everyone who has not
// yet checked out. Any of them is a legal choice for the next visit.
func (g *Game) ActivePlayers() []int64 {
	out := make([]int64, 0, len(g.order))
	for _, id := range g.order {
		if !g.states[id].Finished {
			out = append(out, id)
		}
	}
	return out
}

// AllFinished reports whether every participant has checked out. Even then the
// game does not end by itself — only the operator finishes a game.
func (g *Game) AllFinished() bool {
	for _, id := range g.order {
		if !g.states[id].Finished {
			return false
		}
	}
	return true
}

func (g *Game) reset() {
	g.states = make(map[int64]*PlayerState, len(g.order))
	for i, id := range g.order {
		g.states[id] = &PlayerState{
			PlayerID: id,
			Position: i,
			Score:    g.cfg.StartScore,
		}
	}
}

// Preview evaluates a visit without recording it, so the UI can tell the
// operator what will happen — in particular whether the entry leaves zero and
// therefore needs checkout confirmation, or whether it will bust.
func (g *Game) Preview(in VisitInput) (Visit, error) {
	st, err := g.validate(in)
	if err != nil {
		return Visit{}, err
	}
	return evaluate(g.cfg, st, in, len(g.visits)+1), nil
}

// LandsOnZero reports whether the given entry would leave the player on exactly
// zero. Under double-out rules such a visit must be confirmed by the operator as
// a genuine double finish before it counts as a checkout rather than a bust.
func (g *Game) LandsOnZero(playerID int64, points int) bool {
	st, ok := g.states[playerID]
	if !ok || st.Finished {
		return false
	}
	return st.Score-points == 0
}

// Record validates and applies a visit, returning what the engine made of it.
func (g *Game) Record(in VisitInput) (Visit, error) {
	st, err := g.validate(in)
	if err != nil {
		return Visit{}, err
	}
	v := evaluate(g.cfg, st, in, len(g.visits)+1)
	g.visits = append(g.visits, v)
	apply(st, v)
	return v, nil
}

func (g *Game) validate(in VisitInput) (*PlayerState, error) {
	if g.finished {
		return nil, ErrGameFinished
	}
	st, ok := g.states[in.PlayerID]
	if !ok {
		return nil, ErrUnknownPlayer
	}
	if st.Finished {
		return nil, ErrPlayerFinished
	}
	if !darts.ValidDartCount(in.Darts) {
		return nil, ErrBadDartCount
	}
	if in.Points < 0 {
		return nil, fmt.Errorf("%w: score cannot be negative", ErrImpossibleScore)
	}
	if !darts.Reachable(in.Points, in.Darts) {
		return nil, fmt.Errorf("%w: %d with %d dart(s)", ErrImpossibleScore, in.Points, in.Darts)
	}
	return st, nil
}

// evaluate is the whole of the X01 scoring rule. It is a pure function of the
// configuration, the player's position before the visit, and the input, which
// is what makes replay-based undo safe.
func evaluate(cfg Config, st *PlayerState, in VisitInput, seq int) Visit {
	v := Visit{
		Seq:             seq,
		PlayerID:        in.PlayerID,
		Points:          in.Points,
		Darts:           in.Darts,
		ConfirmCheckout: in.ConfirmCheckout,
		ScoreBefore:     st.Score,
	}

	remaining := st.Score - in.Points
	switch {
	case remaining < 0:
		// Scored more than was left.
		v.Bust = true
	case remaining == 0:
		// Landed exactly on zero. Under double-out this only counts if the
		// operator confirms the final dart was a double; otherwise the throw
		// busts, exactly as it would at the board.
		if cfg.DoubleOut && !in.ConfirmCheckout {
			v.Bust = true
		} else {
			v.Checkout = true
		}
	case remaining == 1 && cfg.DoubleOut:
		// Cannot finish from 1 on a double.
		v.Bust = true
	}

	if v.Bust {
		// A bust credits nothing and restores the score, but the darts thrown
		// still happened and still count towards averages and dart totals.
		v.Credited = 0
		v.ScoreAfter = st.Score
		return v
	}
	v.Credited = in.Points
	v.ScoreAfter = remaining
	return v
}

// apply folds an evaluated visit into a player's running state.
func apply(st *PlayerState, v Visit) {
	st.Darts += v.Darts
	st.Visits++
	st.Credited += v.Credited
	st.Score = v.ScoreAfter
	if v.Bust {
		st.Busts++
	}
	if v.Checkout {
		st.Finished = true
		st.FinishDarts = st.Darts
		st.FinishVisits = st.Visits
		st.FinishSeq = v.Seq
	}
}

// Undo removes the most recent visit and rebuilds every player's state by
// replaying what remains. A checkout, and the finished position that came with
// it, unwinds like any other visit.
func (g *Game) Undo() (Visit, error) {
	if g.finished {
		return Visit{}, ErrGameFinished
	}
	if len(g.visits) == 0 {
		return Visit{}, ErrNothingToUndo
	}
	removed := g.visits[len(g.visits)-1]
	g.visits = g.visits[:len(g.visits)-1]
	g.Rebuild()
	return removed, nil
}

// Rebuild recomputes all derived state from the visit log. It is called after
// undo and when a game is loaded from storage, so a resumed game and a live one
// are guaranteed to agree.
func (g *Game) Rebuild() {
	g.reset()
	for i := range g.visits {
		v := &g.visits[i]
		st, ok := g.states[v.PlayerID]
		if !ok {
			continue
		}
		v.Seq = i + 1
		*v = evaluate(g.cfg, st, VisitInput{
			PlayerID:        v.PlayerID,
			Points:          v.Points,
			Darts:           v.Darts,
			ConfirmCheckout: v.ConfirmCheckout,
		}, v.Seq)
		apply(st, *v)
	}
}

// Restore rebuilds a game from persisted data. The stored visits are replayed
// through the same rules used live, so a resumed game cannot drift from the one
// that was saved.
func Restore(cfg Config, playerIDs []int64, visits []Visit, finished bool, winner *int64) (*Game, error) {
	g, err := New(cfg, playerIDs)
	if err != nil {
		return nil, err
	}
	g.visits = append([]Visit(nil), visits...)
	g.Rebuild()
	g.finished = finished
	if winner != nil {
		w := *winner
		g.winner = &w
	}
	return g, nil
}

// ParticipantResult is one player's outcome when the game is finished.
type ParticipantResult struct {
	PlayerID  int64
	Position  int
	Completed bool
	// Remaining is the score left on the board. Zero for a completed leg.
	Remaining int
	Darts     int
	Visits    int
	Credited  int
	Average   float64
	Won       bool
}

// Results returns every participant's outcome, ordered best first: players who
// completed their leg come before those who did not, fewest darts wins among
// finishers, and unfinished players are ranked by the score they have left.
//
// Players who never threw are included with zeroed figures rather than being
// dropped or given a fabricated finish.
func (g *Game) Results() []ParticipantResult {
	out := make([]ParticipantResult, 0, len(g.order))
	for _, id := range g.order {
		st := g.states[id]
		r := ParticipantResult{
			PlayerID:  id,
			Position:  st.Position,
			Completed: st.Finished,
			Remaining: st.Score,
			Darts:     st.Darts,
			Visits:    st.Visits,
			Credited:  st.Credited,
			Average:   st.Average(),
		}
		if g.winner != nil && *g.winner == id {
			r.Won = true
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return g.better(out[i], out[j])
	})
	return out
}

// better ranks two participants. Completing the leg beats not completing it;
// among finishers fewer darts is the stronger leg, which is precisely why the
// exact dart count is recorded — a 28-dart 501 beats a 30-dart 501.
func (g *Game) better(a, b ParticipantResult) bool {
	if a.Completed != b.Completed {
		return a.Completed
	}
	if a.Completed {
		if a.Darts != b.Darts {
			return a.Darts < b.Darts
		}
		// Same darts: whoever got there first is listed first.
		return g.states[a.PlayerID].FinishSeq < g.states[b.PlayerID].FinishSeq
	}
	if a.Remaining != b.Remaining {
		return a.Remaining < b.Remaining
	}
	return a.Position < b.Position
}

// SuggestWinner proposes a winner from the recorded results. It never guesses
// from throwing order: only a completed leg can suggest a winner, and the
// fewest darts wins. The operator always confirms or overrides the suggestion.
func (g *Game) SuggestWinner() (int64, bool) {
	results := g.Results()
	if len(results) == 0 || !results[0].Completed {
		return 0, false
	}
	return results[0].PlayerID, true
}

// Finish closes the game with the winner the operator confirmed. Passing nil
// records a game with no winner, which is the honest outcome for a leg the
// office abandoned part-way through.
//
// The game does not finish by itself when a player reaches zero — other players
// may still complete their own legs, and only the operator decides when to stop.
func (g *Game) Finish(winner *int64) error {
	if g.finished {
		return ErrGameFinished
	}
	if winner != nil {
		if _, ok := g.states[*winner]; !ok {
			return ErrWinnerNotPlaying
		}
		w := *winner
		g.winner = &w
	}
	g.finished = true
	return nil
}
