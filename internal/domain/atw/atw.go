// Package atw implements Around the World as a pure, terminal-free state
// machine.
//
// Each player works along an ordered sequence of targets — 1 to 20, the outer
// bull, the bullseye — and their run is complete when they hit the last one.
// How far a dart carries them depends on how it landed: a single completes the
// target, a double skips the next as well, a treble skips two. Every dart is
// evaluated against the target as it stands at that moment, so a visit's second
// dart is scored against whatever the first left behind.
//
// Three properties drive the design, and two of them are shared with x01:
//
//   - Turn order is never enforced. The caller names the thrower on every visit
//     and the engine never advances to "the next player".
//
//   - State is derived from an ordered log, never mutated in place. Undo pops
//     the last visit and replays what remains, so targets, dart counts and
//     completed runs all unwind together.
//
//   - Darts are recorded as board segments, not as keystrokes. "D14" stays true
//     after the target has moved on; "double" does not. How far a dart advanced
//     a player is recomputed on replay from the segment and the target it was
//     thrown at, which is why a resumed game cannot disagree with a live one.
//
// The rules here are deliberately independent of x01 and cricket. Nothing in
// this package knows what a bust or a wicket is, and nothing in those packages
// knows what a target is.
package atw

import (
	"errors"
	"fmt"
	"sort"

	"github.com/BigJabber/dart-tui/internal/domain/darts"
)

// MinPlayersForWinner is how many participants a game needs before winning it
// means anything. A player working round the board alone is practising.
//
// This deliberately restates x01's rule rather than sharing it. The two games
// happen to agree, but they are separate games: one could change its mind about
// solo play without dragging the other with it.
const MinPlayersForWinner = 2

// IsTraining reports whether a game with this many participants is a solo
// practice session rather than a contest.
func IsTraining(participants int) bool { return participants < MinPlayersForWinner }

// VisitInput is what the operator records for one turn at the board.
type VisitInput struct {
	// PlayerID names the thrower. Any unfinished participant is valid at any
	// time; the engine imposes no ordering.
	PlayerID int64
	// Calls are the keyed results of the darts thrown, in order, at most three.
	// Anything keyed after the run is complete is dropped rather than counted.
	Calls []Call
}

// Visit is a recorded turn together with everything the engine derived from it.
type Visit struct {
	Seq      int
	PlayerID int64
	// Darts are the darts that actually counted, in order. A visit that
	// completes a run on its second dart holds two darts, never three.
	Darts []Dart

	// TargetBefore and TargetAfter bracket the visit. They are derived on
	// replay and stored only so the log can be read without one.
	TargetBefore Target
	TargetAfter  Target
	// Completed reports that this visit finished the player's run.
	Completed bool
}

// DartCount is how many darts the visit counted.
func (v Visit) DartCount() int { return len(v.Darts) }

// Advanced is how many targets the visit moved the player along.
func (v Visit) Advanced() int {
	return v.TargetAfter.Index() - v.TargetBefore.Index()
}

// PlayerState is one participant's live position in the game.
type PlayerState struct {
	PlayerID int64
	// Position is the order the players were selected in. It is display
	// metadata only and never restricts who may throw.
	Position int

	// Target is what they must hit next.
	Target Target
	// Darts is every dart they have actually thrown. A run completed on the
	// second dart of a visit adds two, not three: unused darts are not counted
	// because they were never thrown.
	Darts  int
	Visits int

	// Finished is set once the run is complete.
	Finished bool
	// FinishDarts and FinishVisits are the totals at the moment the run was
	// completed, and are what a result is ranked by.
	FinishDarts  int
	FinishVisits int
	// FinishSeq is the visit that completed the run. It exists for describing
	// the game, never for ranking: who happened to finish first is meaningless
	// when players throw in any order.
	FinishSeq int

	// The dart-by-dart tallies. They are derived from the log on every rebuild
	// and are kept so progress can be judged by more than a dart count.
	Hits    int
	Misses  int
	Singles int
	Doubles int
	Trebles int
	// Advanced is how many targets have been completed.
	Advanced int
}

// Errors returned by the engine. Like the dart errors, they are all recoverable
// input problems rather than failures.
var (
	ErrNoPlayers        = errors.New("a game needs at least one player")
	ErrDuplicatePlayer  = errors.New("a player cannot be selected twice")
	ErrUnknownPlayer    = errors.New("that player is not in this game")
	ErrPlayerFinished   = errors.New("that player has already completed their run")
	ErrGameFinished     = errors.New("this game has already been finished")
	ErrNothingToUndo    = errors.New("there is nothing to undo")
	ErrNoDarts          = errors.New("a visit needs at least one dart")
	ErrTooManyDarts     = fmt.Errorf("a visit is at most %d darts", MaxDartsPerVisit)
	ErrWinnerNotPlaying = errors.New("a winner must be one of the players in this game")
	ErrTrainingNoWinner = errors.New("a training game has no winner to record")
	ErrCorruptLog       = errors.New("the recorded darts do not fit the targets they were thrown at")
)

// Game is an Around the World game in progress or finished.
type Game struct {
	order    []int64
	states   map[int64]*PlayerState
	visits   []Visit
	finished bool
	winners  []int64
}

// New creates a game for the given players, in selection order. Everybody
// starts on 1.
func New(playerIDs []int64) (*Game, error) {
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
	g := &Game{order: append([]int64(nil), playerIDs...)}
	g.reset()
	return g, nil
}

func (g *Game) reset() {
	g.states = make(map[int64]*PlayerState, len(g.order))
	for i, id := range g.order {
		g.states[id] = &PlayerState{PlayerID: id, Position: i, Target: First()}
	}
}

// Order returns the player IDs in selection order.
func (g *Game) Order() []int64 { return append([]int64(nil), g.order...) }

// Training reports whether this is a solo practice session. A training game is
// scored, saved and counted like any other, but cannot be won.
func (g *Game) Training() bool { return IsTraining(len(g.order)) }

// Visits returns the recorded visit log, oldest first.
func (g *Game) Visits() []Visit { return append([]Visit(nil), g.visits...) }

// VisitCount returns how many visits have been recorded.
func (g *Game) VisitCount() int { return len(g.visits) }

// Finished reports whether the operator has finished the game.
func (g *Game) Finished() bool { return g.finished }

// Winners returns the recorded winners. There may be more than one: two players
// completing the run in the same number of darts is a genuine tie, not
// something to be broken by who threw first.
func (g *Game) Winners() []int64 { return append([]int64(nil), g.winners...) }

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

// ActivePlayers returns everybody who has not yet completed their run. Any of
// them is a legal choice for the next visit.
func (g *Game) ActivePlayers() []int64 {
	out := make([]int64, 0, len(g.order))
	for _, id := range g.order {
		if !g.states[id].Finished {
			out = append(out, id)
		}
	}
	return out
}

// AllFinished reports whether every participant has completed their run. Even
// then the game does not end by itself — only the operator finishes a game.
func (g *Game) AllFinished() bool {
	for _, id := range g.order {
		if !g.states[id].Finished {
			return false
		}
	}
	return true
}

// Preview evaluates a visit without recording it, so the screen can show what
// it will do before the operator commits to it.
func (g *Game) Preview(in VisitInput) (Visit, error) {
	st, err := g.validate(in)
	if err != nil {
		return Visit{}, err
	}
	return evaluate(st.Target, in, len(g.visits)+1)
}

// Record validates and applies a visit.
func (g *Game) Record(in VisitInput) (Visit, error) {
	st, err := g.validate(in)
	if err != nil {
		return Visit{}, err
	}
	v, err := evaluate(st.Target, in, len(g.visits)+1)
	if err != nil {
		return Visit{}, err
	}
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
	if len(in.Calls) == 0 {
		return nil, ErrNoDarts
	}
	if len(in.Calls) > MaxDartsPerVisit {
		return nil, ErrTooManyDarts
	}
	return st, nil
}

// evaluate is the whole of the Around the World scoring rule.
//
// It walks the darts in order, re-reading the target before each one, because
// the target the visit started on stops being the right one the moment a dart
// lands. It is a pure function of the starting target and the input, which is
// what makes replay-based undo safe.
//
// A dart that completes the run ends the visit. Anything keyed after it is
// dropped rather than recorded: those darts were never thrown, and counting
// them would inflate the one number the whole game is judged by.
func evaluate(from Target, in VisitInput, seq int) (Visit, error) {
	v := Visit{
		Seq:          seq,
		PlayerID:     in.PlayerID,
		TargetBefore: from,
		Darts:        make([]Dart, 0, len(in.Calls)),
	}

	target := from
	for _, c := range in.Calls {
		dart, err := Resolve(target, c)
		if err != nil {
			return Visit{}, err
		}
		steps, err := Advance(target, dart)
		if err != nil {
			return Visit{}, err
		}
		v.Darts = append(v.Darts, dart)
		target = target.Advance(steps)
		if target.Done() {
			break
		}
	}

	v.TargetAfter = target
	v.Completed = target.Done()
	return v, nil
}

// replay re-evaluates a stored visit against the target the player is on,
// recomputing what it did from the segments rather than trusting what was
// written down beside them.
func replay(from Target, v Visit) (Visit, error) {
	out := Visit{
		Seq:          v.Seq,
		PlayerID:     v.PlayerID,
		TargetBefore: from,
		Darts:        make([]Dart, 0, len(v.Darts)),
	}

	target := from
	for _, dart := range v.Darts {
		if target.Done() {
			// Storage holds a dart thrown after the run was complete. It cannot
			// have happened, so it is dropped rather than counted.
			break
		}
		steps, err := Advance(target, dart)
		if err != nil {
			return Visit{}, fmt.Errorf("%w: visit %d: %v", ErrCorruptLog, v.Seq, err)
		}
		out.Darts = append(out.Darts, dart)
		target = target.Advance(steps)
	}

	out.TargetAfter = target
	out.Completed = target.Done()
	return out, nil
}

// apply folds an evaluated visit into a player's running state.
func apply(st *PlayerState, v Visit) {
	st.Visits++
	st.Darts += len(v.Darts)
	st.Advanced += v.Advanced()

	for _, d := range v.Darts {
		if !d.Hit {
			st.Misses++
			continue
		}
		st.Hits++
		switch d.Segment.Kind {
		case darts.Double:
			st.Doubles++
		case darts.Treble:
			st.Trebles++
		default:
			// Singles, the outer bull and the bullseye all advance one target
			// and are counted together as single hits.
			st.Singles++
		}
	}

	st.Target = v.TargetAfter
	if v.Completed {
		st.Finished = true
		st.FinishDarts = st.Darts
		st.FinishVisits = st.Visits
		st.FinishSeq = v.Seq
	}
}

// Undo removes the most recent visit and rebuilds every player's state by
// replaying what remains. A completed run, and the dart count that came with
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
	// Replaying a prefix of a log that was already accepted cannot fail: every
	// remaining visit still meets the target it met before. The error is
	// returned rather than dropped so a future change that breaks that
	// assumption is noticed.
	if err := g.Rebuild(); err != nil {
		return Visit{}, err
	}
	return removed, nil
}

// Rebuild recomputes all derived state from the visit log. It is called after
// undo and when a game is loaded from storage, so a resumed game and a live one
// are guaranteed to agree.
func (g *Game) Rebuild() error {
	g.reset()
	for i := range g.visits {
		v := &g.visits[i]
		st, ok := g.states[v.PlayerID]
		if !ok {
			// A visit by somebody who is not in the game cannot be scored. It is
			// skipped rather than guessed at.
			continue
		}
		if st.Finished {
			continue
		}
		v.Seq = i + 1
		replayed, err := replay(st.Target, *v)
		if err != nil {
			return err
		}
		*v = replayed
		apply(st, *v)
	}
	return nil
}

// Restore rebuilds a game from persisted data. The stored darts are replayed
// through the same rules used live, so a resumed game cannot drift from the one
// that was saved.
func Restore(playerIDs []int64, visits []Visit, finished bool, winners []int64) (*Game, error) {
	g, err := New(playerIDs)
	if err != nil {
		return nil, err
	}
	g.visits = append([]Visit(nil), visits...)
	if err := g.Rebuild(); err != nil {
		return nil, err
	}
	g.finished = finished
	g.winners = append([]int64(nil), winners...)
	return g, nil
}

// ParticipantResult is one player's outcome when the game is finished.
type ParticipantResult struct {
	PlayerID int64
	Position int
	// Completed reports whether the player finished the run.
	Completed bool
	// Darts is how many darts they threw. For a completed run it is the result;
	// for an unfinished one it is simply how far they got, and is never
	// comparable with a completed run's total.
	Darts  int
	Visits int
	// Remaining is how many targets they still had to hit, zero when completed.
	Remaining int
	// Target is where they had reached.
	Target Target
	// Advanced is how many targets they completed.
	Advanced int
	Hits     int
	Misses   int
	Singles  int
	Doubles  int
	Trebles  int
	Won      bool
}

// Results returns every participant's outcome, ordered best first: players who
// completed the run come before those who did not, fewest darts wins among
// them, and unfinished players are ranked by how far round the board they got.
//
// Players who never threw are included with zeroed figures rather than being
// dropped or given a fabricated result.
func (g *Game) Results() []ParticipantResult {
	won := make(map[int64]bool, len(g.winners))
	for _, id := range g.winners {
		won[id] = true
	}

	out := make([]ParticipantResult, 0, len(g.order))
	for _, id := range g.order {
		st := g.states[id]
		out = append(out, ParticipantResult{
			PlayerID:  id,
			Position:  st.Position,
			Completed: st.Finished,
			Darts:     st.Darts,
			Visits:    st.Visits,
			Remaining: st.Target.Remaining(),
			Target:    st.Target,
			Advanced:  st.Advanced,
			Hits:      st.Hits,
			Misses:    st.Misses,
			Singles:   st.Singles,
			Doubles:   st.Doubles,
			Trebles:   st.Trebles,
			Won:       won[id],
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return better(out[i], out[j]) })
	return out
}

// better ranks two participants.
//
// Completing the run beats not completing it, and among those who completed it
// the fewest darts is the better result. Nothing here consults the order the
// runs were finished in: players throw in whatever order the office likes, so
// finishing first says only that somebody got to the board more often.
func better(a, b ParticipantResult) bool {
	if a.Completed != b.Completed {
		return a.Completed
	}
	if a.Completed {
		if a.Darts != b.Darts {
			return a.Darts < b.Darts
		}
		// A genuine tie. Selection order is the tie-break for display only; the
		// suggested winner treats them as equals.
		return a.Position < b.Position
	}
	if a.Remaining != b.Remaining {
		return a.Remaining < b.Remaining
	}
	if a.Darts != b.Darts {
		return a.Darts < b.Darts
	}
	return a.Position < b.Position
}

// SuggestWinners proposes the winner from the recorded results: whoever
// completed the run in the fewest darts.
//
// It returns every player tied on that count, because two players finishing in
// 29 darts have drawn and picking one of them would be inventing a result. It
// suggests nobody when nobody completed a run, and nobody in a training game.
// The operator always confirms or overrides the suggestion.
func (g *Game) SuggestWinners() []int64 {
	if g.Training() {
		return nil
	}
	best := 0
	var winners []int64
	for _, id := range g.order {
		st := g.states[id]
		if !st.Finished {
			continue
		}
		switch {
		case len(winners) == 0 || st.FinishDarts < best:
			best, winners = st.FinishDarts, []int64{id}
		case st.FinishDarts == best:
			winners = append(winners, id)
		}
	}
	return winners
}

// Finish closes the game with the winner or winners the operator confirmed.
// Passing none records a game with no winner, which is the honest outcome for a
// run the office abandoned part-way through.
//
// The game does not finish by itself when a player completes their run — others
// may still want to complete their own, and only the operator decides when to
// stop.
func (g *Game) Finish(winners []int64) error {
	if g.finished {
		return ErrGameFinished
	}
	if len(winners) > 0 {
		if g.Training() {
			return ErrTrainingNoWinner
		}
		seen := make(map[int64]bool, len(winners))
		for _, id := range winners {
			if _, ok := g.states[id]; !ok {
				return ErrWinnerNotPlaying
			}
			if seen[id] {
				return ErrDuplicatePlayer
			}
			seen[id] = true
		}
		g.winners = append([]int64(nil), winners...)
	}
	g.finished = true
	return nil
}
