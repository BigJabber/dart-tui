// Package cricket implements the office's custom darts cricket variant.
//
// This is NOT standard numbers-based darts cricket. There are no 15-20 marks and
// no closing of numbers. The game borrows its shape from cricket the bat-and-ball
// sport: two teams, batting and bowling innings, runs, wickets and a chase.
//
// The rules, all of which are deliberately confined to this package:
//
//   - Exactly two teams, each with one or more players in a defined order.
//   - Unlike X01, turn order IS enforced. The batter always throws first, then a
//     bowler, alternating. Each team cycles its own player list independently, so
//     the teams need not be the same size.
//   - Batting: the operator enters the raw visit score. Runs are derived, not
//     entered — 40 or below scores nothing, and every point above 40 is one run.
//   - Bowling: darts are aimed at the bull. An outer bull takes one wicket, an
//     inner bull takes two, anything else takes none.
//   - An innings ends when the batting side has lost the configured number of
//     wickets.
//
// A match is played over TWO LEGS. Each team bats once per leg, so there are
// four innings in all and the winner is decided on the combined total:
//
//	Leg 1   Innings 1  the side chosen to bat first
//	        Innings 2  the other side
//	Leg 2   Innings 3  whichever side is BEHIND on aggregate
//	        Innings 4  the side that is ahead — only if it still needs to bat
//
// Leg 1 is played out in full: neither innings ends early for passing the other
// side's score, because the match continues into leg 2 regardless. Only the very
// last innings is a chase, ending the moment the batting side's aggregate
// exceeds its opponent's.
//
// Because the trailing side bats first in leg 2, the leading side may never need
// to bat again: if the trailing side is bowled out with a combined total still
// short of its opponent's, the match is already decided.
//
// As in the X01 engine, state is derived by replaying an ordered event log, so
// undo unwinds wickets, innings transitions, leg changes and the result together.
package cricket

import (
	"errors"
	"fmt"

	"github.com/pourmoi/dart-tui/internal/domain/darts"
)

// RunsThreshold is the raw visit score at or below which a batting visit scores
// no runs. Every point above it is worth one run.
//
// This is an office house rule; it is a single named constant so it can be
// changed without hunting through the scoring logic.
const RunsThreshold = 40

// Side identifies one of the two teams.
type Side int

// The two teams.
const (
	TeamA Side = iota
	TeamB
)

// String renders the side as the single letter used throughout the UI and the
// database.
func (s Side) String() string {
	if s == TeamB {
		return "B"
	}
	return "A"
}

// Other returns the opposing side.
func (s Side) Other() Side {
	if s == TeamA {
		return TeamB
	}
	return TeamA
}

// ParseSide converts a stored "A"/"B" back into a Side.
func ParseSide(s string) (Side, error) {
	switch s {
	case "A", "a":
		return TeamA, nil
	case "B", "b":
		return TeamB, nil
	}
	return TeamA, fmt.Errorf("%w: %q", ErrBadSide, s)
}

// Team is one side's name and its ordered list of players.
type Team struct {
	Side Side
	Name string
	// Players is the throwing order. It cycles independently of the other team.
	Players []int64
}

// Config is the setup chosen before the first dart is thrown.
type Config struct {
	// WicketLimit is how many wickets end an innings.
	WicketLimit int
	// FirstBatting is the side that bats in the first innings.
	FirstBatting Side
}

// MinWickets and MaxWickets bound the configurable wicket limit. The upper
// bound simply stops a typo producing an innings nobody can finish.
const (
	MinWickets = 1
	MaxWickets = 20
)

// The shape of a match: two legs, each team batting once per leg.
const (
	// LegsPerGame is how many times each team bats.
	LegsPerGame = 2
	// InningsPerGame is the most innings a match can contain. It may end in
	// three if the leading side never needs to bat again.
	InningsPerGame = 4
	// FinalInnings is the only innings that is a chase: it ends the moment the
	// batting side's aggregate passes its opponent's.
	FinalInnings = 4
)

// Bowl is the outcome of a single bowling dart.
type Bowl int

// Bowling dart outcomes. The operator enters these as M, O and B.
const (
	// Miss is anything other than a bull.
	Miss Bowl = iota
	// Outer is the outer bull: one wicket.
	Outer
	// Inner is the inner bull (bullseye): two wickets.
	Inner
)

// Wickets is how many wickets this dart takes.
func (b Bowl) Wickets() int {
	switch b {
	case Outer:
		return 1
	case Inner:
		return 2
	default:
		return 0
	}
}

// String renders the dart in the storage notation used by the database.
//
// This is deliberately separate from Key: the letters are what is written to
// disk and must stay stable, while the keys the operator presses are a UI
// choice that can change without a migration.
func (b Bowl) String() string {
	switch b {
	case Outer:
		return "O"
	case Inner:
		return "B"
	default:
		return "M"
	}
}

// Key is the digit the operator presses to record this outcome.
//
// 1, 2 and 3 sit together on the numeric keypad, next to the digits used for
// batting scores and the keypad's own Enter, so a whole match can be scored
// without the hand leaving the keypad.
func (b Bowl) Key() string {
	switch b {
	case Outer:
		return "2"
	case Inner:
		return "3"
	default:
		return "1"
	}
}

// Label is a human-readable description for the UI.
func (b Bowl) Label() string {
	switch b {
	case Outer:
		return "outer bull"
	case Inner:
		return "inner bull"
	default:
		return "no wicket"
	}
}

// ParseBowl converts one character into a dart outcome.
//
// It accepts both the keypad digits the operator types and the letters stored in
// the database, so reading back an existing game needs no translation step.
func ParseBowl(c rune) (Bowl, error) {
	switch c {
	case '1', 'm', 'M':
		return Miss, nil
	case '2', 'o', 'O':
		return Outer, nil
	case '3', 'b', 'B':
		return Inner, nil
	}
	return Miss, fmt.Errorf("%w: %q (use 1, 2 or 3)", ErrBadBowlNotation, string(c))
}

// ParseBowls converts a whole notation string such as "MOB" into darts.
func ParseBowls(s string) ([]Bowl, error) {
	runes := []rune(s)
	if len(runes) == 0 {
		return nil, ErrNoDarts
	}
	if len(runes) > darts.MaxDartsPerVisit {
		return nil, fmt.Errorf("%w: %d darts entered", ErrTooManyDarts, len(runes))
	}
	out := make([]Bowl, 0, len(runes))
	for _, c := range runes {
		b, err := ParseBowl(c)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

// Kind distinguishes the two sorts of visit.
type Kind int

// Visit kinds.
const (
	Batting Kind = iota + 1
	Bowling
)

// String renders the kind as stored in the database.
func (k Kind) String() string {
	if k == Bowling {
		return "bowl"
	}
	return "bat"
}

// ParseKind converts a stored kind back into a Kind.
func ParseKind(s string) (Kind, error) {
	switch s {
	case "bat":
		return Batting, nil
	case "bowl":
		return Bowling, nil
	}
	return 0, fmt.Errorf("%w: %q", ErrBadKind, s)
}

// Event is one recorded visit. Batting and bowling share the type so the log
// stays a single ordered sequence, which is what makes replay simple.
type Event struct {
	Seq     int
	Innings int
	Kind    Kind
	// Side is the team that took the visit, and PlayerID the individual.
	Side     Side
	PlayerID int64

	// Batting fields.
	RawPoints int
	Runs      int

	// Bowling fields. Bowls holds only the darts actually thrown, which may be
	// fewer than three if the wicket limit was reached mid-visit.
	Bowls   []Bowl
	Wickets int

	// Darts is the number of darts thrown, for both kinds of visit.
	Darts int
}

// OuterBulls counts the outer bulls in a bowling visit.
func (e Event) OuterBulls() int { return e.countBowls(Outer) }

// InnerBulls counts the inner bulls in a bowling visit.
func (e Event) InnerBulls() int { return e.countBowls(Inner) }

func (e Event) countBowls(want Bowl) int {
	n := 0
	for _, b := range e.Bowls {
		if b == want {
			n++
		}
	}
	return n
}

// Innings is the derived state of one innings.
type Innings struct {
	Number      int
	BattingSide Side
	Runs        int
	Wickets     int
	Complete    bool
}

// Leg is which of the two legs this innings belongs to: innings 1 and 2 are
// leg 1, innings 3 and 4 are leg 2.
func (i Innings) Leg() int { return (i.Number + 1) / 2 }

// Result is the outcome of a completed game.
type Result int

// Game outcomes.
const (
	ResultNone Result = iota
	ResultTeamA
	ResultTeamB
	ResultDraw
	// ResultAbandoned records a game the office stopped part-way through.
	ResultAbandoned
)

// String renders the result as stored in the database.
func (r Result) String() string {
	switch r {
	case ResultTeamA:
		return "team_a"
	case ResultTeamB:
		return "team_b"
	case ResultDraw:
		return "draw"
	case ResultAbandoned:
		return "abandoned"
	default:
		return "none"
	}
}

// ParseResult converts a stored result back into a Result.
func ParseResult(s string) Result {
	switch s {
	case "team_a":
		return ResultTeamA
	case "team_b":
		return ResultTeamB
	case "draw":
		return ResultDraw
	case "abandoned":
		return ResultAbandoned
	default:
		return ResultNone
	}
}

// Turn describes whose visit is next and which sort of visit it must be.
type Turn struct {
	Innings  int
	Kind     Kind
	Side     Side
	PlayerID int64
}

// Errors returned by the engine.
var (
	ErrBadWicketLimit  = fmt.Errorf("wicket limit must be between %d and %d", MinWickets, MaxWickets)
	ErrEmptyTeam       = errors.New("each team needs at least one player")
	ErrDuplicatePlayer = errors.New("a player cannot appear twice, or on both teams")
	ErrBadSide         = errors.New("invalid team side")
	ErrBadKind         = errors.New("invalid visit kind")
	ErrBadBowlNotation = errors.New("invalid bowling notation")
	ErrNoDarts         = errors.New("at least one dart must be recorded")
	ErrTooManyDarts    = fmt.Errorf("a visit is at most %d darts", darts.MaxDartsPerVisit)
	ErrGameComplete    = errors.New("this game has already finished")
	ErrNotBatting      = errors.New("a bowling visit is expected, not a batting visit")
	ErrNotBowling      = errors.New("a batting visit is expected, not a bowling visit")
	ErrWrongPlayer     = errors.New("it is not that player's turn")
	ErrImpossibleScore = errors.New("that score cannot be thrown with that many darts")
	ErrBadDartCount    = errors.New("darts thrown must be 1, 2 or 3")
	ErrNothingToUndo   = errors.New("there is nothing to undo")
)

// Game is a cricket match in progress or finished.
type Game struct {
	cfg    Config
	teams  [2]Team
	events []Event

	// Derived state, rebuilt from events.
	innings  []Innings
	result   Result
	complete bool
	// batTaken and bowlTaken count the visits each role has taken in the
	// current innings, which is what drives the independent player cycles.
	batTaken  int
	bowlTaken int
}

// New creates a cricket game.
func New(cfg Config, teamA, teamB Team) (*Game, error) {
	if cfg.WicketLimit < MinWickets || cfg.WicketLimit > MaxWickets {
		return nil, ErrBadWicketLimit
	}
	teamA.Side, teamB.Side = TeamA, TeamB
	if len(teamA.Players) == 0 || len(teamB.Players) == 0 {
		return nil, ErrEmptyTeam
	}
	seen := map[int64]bool{}
	for _, t := range []Team{teamA, teamB} {
		for _, id := range t.Players {
			if seen[id] {
				return nil, ErrDuplicatePlayer
			}
			seen[id] = true
		}
	}
	if teamA.Name == "" {
		teamA.Name = "Team A"
	}
	if teamB.Name == "" {
		teamB.Name = "Team B"
	}
	teamA.Players = append([]int64(nil), teamA.Players...)
	teamB.Players = append([]int64(nil), teamB.Players...)

	g := &Game{cfg: cfg, teams: [2]Team{teamA, teamB}}
	g.rebuild()
	return g, nil
}

// Config returns the game's configuration.
func (g *Game) Config() Config { return g.cfg }

// Team returns one side's team.
func (g *Game) Team(s Side) Team { return g.teams[s] }

// Teams returns both teams, A first.
func (g *Game) Teams() [2]Team { return g.teams }

// Events returns the recorded event log, oldest first.
func (g *Game) Events() []Event { return append([]Event(nil), g.events...) }

// EventCount returns how many visits have been recorded.
func (g *Game) EventCount() int { return len(g.events) }

// Complete reports whether the game has ended.
func (g *Game) Complete() bool { return g.complete }

// Result returns the outcome, which is meaningful only once complete.
func (g *Game) Result() Result { return g.result }

// Innings returns the innings played so far, first innings first.
func (g *Game) Innings() []Innings { return append([]Innings(nil), g.innings...) }

// CurrentInnings returns the innings in progress.
func (g *Game) CurrentInnings() Innings { return g.innings[len(g.innings)-1] }

// InningsNumber returns the number of the innings in progress, 1 to 4.
func (g *Game) InningsNumber() int { return g.CurrentInnings().Number }

// Leg returns which leg is in progress, 1 or 2.
func (g *Game) Leg() int { return g.CurrentInnings().Leg() }

// Aggregate is a side's combined runs across every innings it has batted. This,
// not any single innings, is what decides the match.
func (g *Game) Aggregate(s Side) int {
	total := 0
	for _, inn := range g.innings {
		if inn.BattingSide == s {
			total += inn.Runs
		}
	}
	return total
}

// Lead returns the side that is ahead on aggregate and by how much. The second
// return value is false when the aggregates are level.
func (g *Game) Lead() (Side, int, bool) {
	a, b := g.Aggregate(TeamA), g.Aggregate(TeamB)
	switch {
	case a > b:
		return TeamA, a - b, true
	case b > a:
		return TeamB, b - a, true
	default:
		return TeamA, 0, false
	}
}

// Target is the aggregate the batting side must EXCEED to win.
//
// Only the final innings is a chase — leg 1 is played out in full and the
// trailing side's leg 2 innings is played out too, because the leading side
// still has an innings in hand. The second return value reports whether a target
// applies right now.
func (g *Game) Target() (int, bool) {
	inn := g.CurrentInnings()
	if inn.Number != FinalInnings {
		return 0, false
	}
	return g.Aggregate(inn.BattingSide.Other()), true
}

// RunsNeeded is how many more runs the side batting the final innings needs to
// win, given that the target must be exceeded rather than matched.
func (g *Game) RunsNeeded() (int, bool) {
	target, ok := g.Target()
	if !ok {
		return 0, false
	}
	needed := target - g.Aggregate(g.CurrentInnings().BattingSide) + 1
	if needed < 0 {
		needed = 0
	}
	return needed, true
}

// RunsFor returns the raw score achieved by a raw batting visit total.
//
// The office rule: 40 or below is nothing, and every point above 40 is one run.
// So 26 and 40 both score nothing, 41 scores one, 57 scores seventeen and 100
// scores sixty.
func RunsFor(rawPoints int) int {
	if rawPoints <= RunsThreshold {
		return 0
	}
	return rawPoints - RunsThreshold
}

// CurrentTurn returns whose visit is next. The second return value is false once
// the game is complete.
func (g *Game) CurrentTurn() (Turn, bool) {
	if g.complete {
		return Turn{}, false
	}
	inn := g.CurrentInnings()
	battingTeam := g.teams[inn.BattingSide]
	bowlingTeam := g.teams[inn.BattingSide.Other()]

	// The batter always throws first, then the sides alternate. So a bowling
	// visit is due exactly when the batters are one visit ahead.
	if g.batTaken > g.bowlTaken {
		return Turn{
			Innings:  inn.Number,
			Kind:     Bowling,
			Side:     bowlingTeam.Side,
			PlayerID: bowlingTeam.Players[g.bowlTaken%len(bowlingTeam.Players)],
		}, true
	}
	return Turn{
		Innings:  inn.Number,
		Kind:     Batting,
		Side:     battingTeam.Side,
		PlayerID: battingTeam.Players[g.batTaken%len(battingTeam.Players)],
	}, true
}

// ValidateBattingScore checks that a raw visit score could actually have been
// thrown with the given number of darts.
//
// It is exported so the interface can check a score before committing to it —
// when adding up individual darts, for instance — without restating the rule.
func ValidateBattingScore(rawPoints, dartCount int) error {
	if !darts.ValidDartCount(dartCount) {
		return ErrBadDartCount
	}
	if rawPoints < 0 {
		return fmt.Errorf("%w: score cannot be negative", ErrImpossibleScore)
	}
	if !darts.Reachable(rawPoints, dartCount) {
		return fmt.Errorf("%w: %d with %d dart(s)", ErrImpossibleScore, rawPoints, dartCount)
	}
	return nil
}

// RecordBatting records a batting visit. The operator enters the raw score for
// the darts thrown; the runs are derived here, never typed in.
func (g *Game) RecordBatting(playerID int64, rawPoints, dartCount int) (Event, error) {
	turn, err := g.expect(Batting, playerID)
	if err != nil {
		return Event{}, err
	}
	if err := ValidateBattingScore(rawPoints, dartCount); err != nil {
		return Event{}, err
	}

	e := Event{
		Seq:       len(g.events) + 1,
		Innings:   turn.Innings,
		Kind:      Batting,
		Side:      turn.Side,
		PlayerID:  playerID,
		RawPoints: rawPoints,
		Runs:      RunsFor(rawPoints),
		Darts:     dartCount,
	}
	g.events = append(g.events, e)
	g.step(&g.events[len(g.events)-1])
	return g.events[len(g.events)-1], nil
}

// RecordBowling records a bowling visit from up to three darts of notation.
//
// If the wicket limit is reached part-way through the visit, the remaining darts
// are dropped: they were never thrown, because the innings was already over. The
// returned event holds only the darts that actually counted.
func (g *Game) RecordBowling(playerID int64, bowls []Bowl) (Event, error) {
	turn, err := g.expect(Bowling, playerID)
	if err != nil {
		return Event{}, err
	}
	if len(bowls) == 0 {
		return Event{}, ErrNoDarts
	}
	if len(bowls) > darts.MaxDartsPerVisit {
		return Event{}, fmt.Errorf("%w: %d darts entered", ErrTooManyDarts, len(bowls))
	}

	thrown, wickets := g.truncateAtWicketLimit(bowls)
	e := Event{
		Seq:      len(g.events) + 1,
		Innings:  turn.Innings,
		Kind:     Bowling,
		Side:     turn.Side,
		PlayerID: playerID,
		Bowls:    thrown,
		Wickets:  wickets,
		Darts:    len(thrown),
	}
	g.events = append(g.events, e)
	g.step(&g.events[len(g.events)-1])
	return g.events[len(g.events)-1], nil
}

// truncateAtWicketLimit keeps only the darts thrown before the innings ended.
func (g *Game) truncateAtWicketLimit(bowls []Bowl) ([]Bowl, int) {
	standing := g.CurrentInnings().Wickets
	thrown := make([]Bowl, 0, len(bowls))
	taken := 0
	for _, b := range bowls {
		thrown = append(thrown, b)
		taken += b.Wickets()
		if standing+taken >= g.cfg.WicketLimit {
			break
		}
	}
	return thrown, taken
}

// expect checks that the requested visit is the one the game is waiting for.
// Cricket, unlike X01, enforces order strictly.
func (g *Game) expect(kind Kind, playerID int64) (Turn, error) {
	turn, ok := g.CurrentTurn()
	if !ok {
		return Turn{}, ErrGameComplete
	}
	if turn.Kind != kind {
		if turn.Kind == Bowling {
			return Turn{}, ErrNotBatting
		}
		return Turn{}, ErrNotBowling
	}
	if turn.PlayerID != playerID {
		return Turn{}, fmt.Errorf("%w: expecting player %d", ErrWrongPlayer, turn.PlayerID)
	}
	return turn, nil
}

// step folds one event into the derived state, applying every rule that can end
// a visit, an innings or the game.
func (g *Game) step(e *Event) {
	inn := &g.innings[len(g.innings)-1]

	switch e.Kind {
	case Batting:
		inn.Runs += e.Runs
		g.batTaken++
		// Only the final innings is a chase. Leg 1 is played out in full, and so
		// is the trailing side's leg 2 innings, because the leading side still
		// has an innings in hand either way.
		if inn.Number == FinalInnings {
			if g.Aggregate(inn.BattingSide) > g.Aggregate(inn.BattingSide.Other()) {
				g.finishWith(sideResult(inn.BattingSide))
				return
			}
		}
	case Bowling:
		inn.Wickets += e.Wickets
		g.bowlTaken++
		if inn.Wickets >= g.cfg.WicketLimit {
			inn.Complete = true
			g.endInnings(*inn)
		}
	}
}

// endInnings advances the match when a side is bowled out.
func (g *Game) endInnings(finished Innings) {
	switch finished.Number {
	case 1:
		// The other side bats to complete leg 1.
		g.startInnings(2, finished.BattingSide.Other())

	case 2:
		// Leg 1 is over. The side BEHIND on aggregate bats first in leg 2, which
		// leaves the leading side batting last — and possibly not at all.
		g.startInnings(3, g.trailingSide())

	case 3:
		leader := finished.BattingSide.Other()
		if g.Aggregate(leader) > g.Aggregate(finished.BattingSide) {
			// The trailing side was bowled out with a combined total still short
			// of its opponent's. There is nothing left to play for, so the
			// leading side wins without batting a second time.
			g.finishWith(sideResult(leader))
			return
		}
		// Otherwise the match is level or has been overtaken, and the side that
		// was leading must bat to settle it.
		g.startInnings(FinalInnings, leader)

	default:
		// The final innings has been bowled out. It never passed the target,
		// because doing so would have ended the match during a batting visit,
		// so the aggregates decide.
		a, b := g.Aggregate(TeamA), g.Aggregate(TeamB)
		switch {
		case a > b:
			g.finishWith(ResultTeamA)
		case b > a:
			g.finishWith(ResultTeamB)
		default:
			// Level on aggregate with nobody left to bat.
			g.finishWith(ResultDraw)
		}
	}
}

// startInnings opens an innings and restarts both teams' player rotations at the
// top of their order, which keeps each innings predictable for the operator.
func (g *Game) startInnings(number int, batting Side) {
	g.innings = append(g.innings, Innings{Number: number, BattingSide: batting})
	g.batTaken, g.bowlTaken = 0, 0
}

// trailingSide is the side behind on aggregate at the end of leg 1, which bats
// first in leg 2. Level aggregates keep the leg 1 order, so the choice stays
// deterministic rather than depending on who happened to bat first.
func (g *Game) trailingSide() Side {
	if leader, _, ok := g.Lead(); ok {
		return leader.Other()
	}
	return g.cfg.FirstBatting
}

func sideResult(s Side) Result {
	if s == TeamB {
		return ResultTeamB
	}
	return ResultTeamA
}

func (g *Game) finishWith(r Result) {
	g.complete = true
	g.result = r
	if n := len(g.innings); n > 0 {
		g.innings[n-1].Complete = true
	}
}

// Undo removes the most recent visit and replays the log. Wickets, innings
// transitions and the recorded result all unwind together, so undoing the dart
// that ended the first innings genuinely returns the game to the first innings.
func (g *Game) Undo() (Event, error) {
	if len(g.events) == 0 {
		return Event{}, ErrNothingToUndo
	}
	removed := g.events[len(g.events)-1]
	g.events = g.events[:len(g.events)-1]
	g.rebuild()
	return removed, nil
}

// rebuild recomputes every derived field from the event log.
func (g *Game) rebuild() {
	g.innings = []Innings{{Number: 1, BattingSide: g.cfg.FirstBatting}}
	g.result = ResultNone
	g.complete = false
	g.batTaken, g.bowlTaken = 0, 0

	for i := range g.events {
		if g.complete {
			// The log decided the game before its last entry. Stop rather than
			// fold stray events into a finished match.
			g.events = g.events[:i]
			break
		}
		e := &g.events[i]
		e.Seq = i + 1
		e.Innings = g.CurrentInnings().Number
		if e.Kind == Batting {
			// Runs are always re-derived so a change to the house rule applies
			// to replayed games rather than being frozen into old rows.
			e.Runs = RunsFor(e.RawPoints)
		} else {
			e.Darts = len(e.Bowls)
			e.Wickets = 0
			for _, b := range e.Bowls {
				e.Wickets += b.Wickets()
			}
		}
		g.step(e)
	}
}

// Abandon ends a game that the office stopped part-way through, without
// inventing a winner.
func (g *Game) Abandon() error {
	if g.complete {
		return ErrGameComplete
	}
	g.finishWith(ResultAbandoned)
	return nil
}

// Restore rebuilds a game from persisted events.
func Restore(cfg Config, teamA, teamB Team, events []Event) (*Game, error) {
	g, err := New(cfg, teamA, teamB)
	if err != nil {
		return nil, err
	}
	g.events = append([]Event(nil), events...)
	g.rebuild()
	return g, nil
}

// PlayerStats is one player's contribution within this game, derived from the
// event log. Batting and bowling are kept apart because they measure different
// skills and averaging them together would be meaningless.
type PlayerStats struct {
	PlayerID int64

	BattingVisits int
	BattingDarts  int
	RawPoints     int
	Runs          int
	HighestRaw    int
	HighestRuns   int
	BowlingVisits int
	BowlingDarts  int
	Wickets       int
	OuterBulls    int
	InnerBulls    int
}

// AverageRawPerVisit is the mean raw score per batting visit.
func (s PlayerStats) AverageRawPerVisit() float64 {
	if s.BattingVisits == 0 {
		return 0
	}
	return float64(s.RawPoints) / float64(s.BattingVisits)
}

// AverageRunsPerVisit is the mean runs per batting visit.
func (s PlayerStats) AverageRunsPerVisit() float64 {
	if s.BattingVisits == 0 {
		return 0
	}
	return float64(s.Runs) / float64(s.BattingVisits)
}

// WicketsPerVisit is the mean wickets per bowling visit.
func (s PlayerStats) WicketsPerVisit() float64 {
	if s.BowlingVisits == 0 {
		return 0
	}
	return float64(s.Wickets) / float64(s.BowlingVisits)
}

// StatsFor returns one player's figures for this game.
func (g *Game) StatsFor(playerID int64) PlayerStats {
	s := PlayerStats{PlayerID: playerID}
	for _, e := range g.events {
		if e.PlayerID != playerID {
			continue
		}
		switch e.Kind {
		case Batting:
			s.BattingVisits++
			s.BattingDarts += e.Darts
			s.RawPoints += e.RawPoints
			s.Runs += e.Runs
			if e.RawPoints > s.HighestRaw {
				s.HighestRaw = e.RawPoints
			}
			if e.Runs > s.HighestRuns {
				s.HighestRuns = e.Runs
			}
		case Bowling:
			s.BowlingVisits++
			s.BowlingDarts += e.Darts
			s.Wickets += e.Wickets
			s.OuterBulls += e.OuterBulls()
			s.InnerBulls += e.InnerBulls()
		}
	}
	return s
}

// WinningSide returns the side that won, if the game ended with a winner.
func (g *Game) WinningSide() (Side, bool) {
	switch g.result {
	case ResultTeamA:
		return TeamA, true
	case ResultTeamB:
		return TeamB, true
	default:
		return TeamA, false
	}
}

// SideOf returns the team a player belongs to.
func (g *Game) SideOf(playerID int64) (Side, bool) {
	for _, t := range g.teams {
		for _, id := range t.Players {
			if id == playerID {
				return t.Side, true
			}
		}
	}
	return TeamA, false
}

// Players returns every player in the game, Team A first, each in order.
func (g *Game) Players() []int64 {
	out := make([]int64, 0, len(g.teams[0].Players)+len(g.teams[1].Players))
	out = append(out, g.teams[0].Players...)
	out = append(out, g.teams[1].Players...)
	return out
}
