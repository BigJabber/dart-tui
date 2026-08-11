package atw

import (
	"errors"
	"fmt"

	"github.com/BigJabber/dart-tui/internal/domain/darts"
)

// Call is what the operator keyed for a single dart.
//
// The operator records the dart against the target, not the number that was
// physically hit: a player needing 8 who lands single 12 has simply missed, and
// which wrong bed they found is not worth a keystroke at the board. Only a hit
// on the required target needs to say how it landed, because that is what
// decides how far the player moves.
type Call int

// The four things the operator can key for a dart.
const (
	// CallMiss is a dart that did not hit the required target.
	CallMiss Call = 0
	// CallSingle is a hit that advances one target. On the outer bull it is the
	// outer bull itself.
	CallSingle Call = 1
	// CallDouble is a hit that advances two targets. On the outer bull it is the
	// bullseye, which is worth the same two steps.
	CallDouble Call = 2
	// CallTreble is a hit that advances three targets. There is no treble bull,
	// so it is not accepted on either bull target.
	CallTreble Call = 3
)

// MaxDartsPerVisit is the number of darts in a full visit.
const MaxDartsPerVisit = darts.MaxDartsPerVisit

// Errors returned when reading or applying a dart. They are all recoverable
// input problems: the screen reports them and the operator retypes the visit.
var (
	ErrBadCall      = errors.New("a dart must be 0, 1, 2 or 3")
	ErrNoTrebleBull = errors.New("there is no treble bull: use 1 or 2")
	ErrRunComplete  = errors.New("that player has already finished their run")
	ErrBadDart      = errors.New("that dart could not have been thrown at that target")
)

// ParseCall reads one keystroke.
func ParseCall(r rune) (Call, error) {
	switch r {
	case '0':
		return CallMiss, nil
	case '1':
		return CallSingle, nil
	case '2':
		return CallDouble, nil
	case '3':
		return CallTreble, nil
	default:
		return 0, fmt.Errorf("%w: %q", ErrBadCall, string(r))
	}
}

// ParseCalls reads a whole visit typed as one string, such as "031".
//
// Spaces are ignored so "0 3 1" works as well, and an empty visit is refused:
// a visit that recorded nothing would still occupy a place in the undo history
// while changing nothing on the board.
func ParseCalls(text string) ([]Call, error) {
	out := make([]Call, 0, MaxDartsPerVisit)
	for _, r := range text {
		if r == ' ' || r == '\t' || r == ',' {
			continue
		}
		c, err := ParseCall(r)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: no darts", ErrBadCall)
	}
	if len(out) > MaxDartsPerVisit {
		return nil, fmt.Errorf("a visit is at most %d darts", MaxDartsPerVisit)
	}
	return out, nil
}

// Key is the character the call is typed and displayed as.
func (c Call) Key() string {
	switch c {
	case CallMiss:
		return "0"
	case CallSingle:
		return "1"
	case CallDouble:
		return "2"
	case CallTreble:
		return "3"
	default:
		return "?"
	}
}

// Dart is one thrown dart as it is recorded and stored.
//
// It holds the board segment rather than the keystroke. "Double" means nothing
// on its own once the target has moved on — but "D14" is a fact about the board
// that stays true, can be replayed against the target it was thrown at, and can
// be counted later for statistics nobody has asked for yet.
type Dart struct {
	// Hit reports whether the required target was struck.
	Hit bool
	// Segment is where the dart landed. It is meaningful only when Hit is set:
	// a miss deliberately records nothing, because the operator is not asked
	// which wrong number was hit.
	Segment darts.Segment
}

// Miss is a dart that missed the required target.
func Miss() Dart { return Dart{} }

// HitOn is a dart that struck a segment.
func HitOn(s darts.Segment) Dart { return Dart{Hit: true, Segment: s} }

// missNotation is how a miss is written down. It cannot collide with a segment,
// which is always a number or a letter followed by one.
const missNotation = "-"

// Notation renders the dart for storage and display: "D14", "25", "Bull", or
// "-" for a miss.
func (d Dart) Notation() string {
	if !d.Hit {
		return missNotation
	}
	return d.Segment.String()
}

// ParseDart reads back what Notation wrote.
func ParseDart(text string) (Dart, error) {
	if text == missNotation {
		return Miss(), nil
	}
	seg, err := darts.ParseSegment(text)
	if err != nil {
		return Dart{}, err
	}
	return HitOn(seg), nil
}

// Label describes the dart in words, for the entry preview.
func (d Dart) Label() string {
	if !d.Hit {
		return "miss"
	}
	switch d.Segment.Kind {
	case darts.Double:
		return "double " + d.Segment.String()[1:]
	case darts.Treble:
		return "treble " + d.Segment.String()[1:]
	case darts.OuterBull:
		return "outer bull"
	case darts.InnerBull:
		return "bullseye"
	default:
		return "single " + d.Segment.String()
	}
}

// Resolve turns a keystroke into the dart it means, given what the player is
// currently aiming at.
//
// This is where the two bull targets stop being special cases and become
// ordinary ones. On a number, 1, 2 and 3 are the single, double and treble of
// that number. On the outer bull there is no treble to hit, so 2 means the
// player found the bullseye instead — which is physically two steps along the
// sequence and is recorded as the bullseye, not as "a double of something".
func Resolve(t Target, c Call) (Dart, error) {
	if t.Done() {
		return Dart{}, ErrRunComplete
	}
	if !t.Valid() {
		return Dart{}, fmt.Errorf("%w: %v", ErrBadDart, t)
	}
	if c == CallMiss {
		return Miss(), nil
	}

	switch t.Kind {
	case Number:
		switch c {
		case CallSingle:
			return HitOn(darts.S(t.Number)), nil
		case CallDouble:
			return HitOn(darts.D(t.Number)), nil
		case CallTreble:
			return HitOn(darts.T(t.Number)), nil
		}
	case OuterBull:
		switch c {
		case CallSingle:
			return HitOn(darts.Outer()), nil
		case CallDouble:
			// Hitting the bullseye while the outer bull is required carries the
			// player through both, exactly as a double carries them through two
			// numbers.
			return HitOn(darts.Bull()), nil
		case CallTreble:
			return Dart{}, ErrNoTrebleBull
		}
	case Bullseye:
		switch c {
		case CallSingle, CallDouble:
			// 2 is the natural key for the bullseye, having just meant it on the
			// outer bull, and 1 is the natural key for "hit what I needed". Both
			// are accepted, and both record the bullseye, because that is the
			// segment the dart actually struck.
			return HitOn(darts.Bull()), nil
		case CallTreble:
			return Dart{}, ErrNoTrebleBull
		}
	}
	return Dart{}, fmt.Errorf("%w: %d", ErrBadCall, c)
}

// Advance is how many targets a recorded dart moves the player along.
//
// It is derived from the segment and the target rather than stored, so a
// replayed log is scored by the same rule as a live visit and the two cannot
// drift apart.
func Advance(t Target, d Dart) (int, error) {
	if t.Done() {
		return 0, ErrRunComplete
	}
	if !d.Hit {
		return 0, nil
	}

	switch t.Kind {
	case Number:
		if d.Segment.Number != t.Number {
			return 0, fmt.Errorf("%w: %s at target %s", ErrBadDart, d.Segment, t)
		}
		switch d.Segment.Kind {
		case darts.Single:
			return 1, nil
		case darts.Double:
			return 2, nil
		case darts.Treble:
			return 3, nil
		}
	case OuterBull:
		switch d.Segment.Kind {
		case darts.OuterBull:
			return 1, nil
		case darts.InnerBull:
			return 2, nil
		}
	case Bullseye:
		if d.Segment.Kind == darts.InnerBull {
			return 1, nil
		}
	}
	return 0, fmt.Errorf("%w: %s at target %s", ErrBadDart, d.Segment, t)
}
