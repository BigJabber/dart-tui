package darts

import (
	"fmt"
	"strconv"
	"strings"
)

// SegmentKind is the ring of the board a dart landed in.
type SegmentKind int

// The scoring areas of a dartboard.
const (
	// Single is the plain area of a number, worth its face value.
	Single SegmentKind = iota + 1
	// Double is the outer ring, worth twice the number.
	Double
	// Treble is the inner ring, worth three times the number.
	Treble
	// OuterBull is the green ring around the centre, worth 25. It is NOT a
	// double, so it cannot finish a double-out leg.
	OuterBull
	// InnerBull is the centre, worth 50. It counts as a double for finishing.
	InnerBull
)

// HighestNumber is the highest number on the board.
const HighestNumber = 20

// Segment is one scoring area: a specific ring on a specific number.
//
// Modelling the board this way, rather than passing integers around, means a
// checkout can name the actual targets a player should aim at — and makes
// impossible combinations unrepresentable rather than merely unlikely.
type Segment struct {
	Kind SegmentKind
	// Number is 1-20 for singles, doubles and trebles, and zero for both bulls.
	Number int
}

// S returns the single of a number.
func S(number int) Segment { return Segment{Kind: Single, Number: number} }

// D returns the double of a number.
func D(number int) Segment { return Segment{Kind: Double, Number: number} }

// T returns the treble of a number.
func T(number int) Segment { return Segment{Kind: Treble, Number: number} }

// Bull returns the inner bull, worth 50.
func Bull() Segment { return Segment{Kind: InnerBull} }

// Outer returns the outer bull, worth 25.
func Outer() Segment { return Segment{Kind: OuterBull} }

// Value is what the segment scores.
func (s Segment) Value() int {
	switch s.Kind {
	case Single:
		return s.Number
	case Double:
		return 2 * s.Number
	case Treble:
		return 3 * s.Number
	case OuterBull:
		return 25
	case InnerBull:
		return 50
	default:
		return 0
	}
}

// Valid reports whether the segment actually exists on a board.
func (s Segment) Valid() bool {
	switch s.Kind {
	case Single, Double, Treble:
		return s.Number >= 1 && s.Number <= HighestNumber
	case OuterBull, InnerBull:
		return s.Number == 0
	default:
		return false
	}
}

// IsFinish reports whether a leg played to double-out may end on this segment.
//
// Only the doubles and the inner bull qualify. The outer bull scores 25 but is
// not a double, so it can never finish a leg.
func (s Segment) IsFinish() bool {
	return s.Valid() && (s.Kind == Double || s.Kind == InnerBull)
}

// String renders the segment in the usual darts shorthand: T20, D16, 8, Bull.
func (s Segment) String() string {
	switch s.Kind {
	case Single:
		return strconv.Itoa(s.Number)
	case Double:
		return "D" + strconv.Itoa(s.Number)
	case Treble:
		return "T" + strconv.Itoa(s.Number)
	case OuterBull:
		return "25"
	case InnerBull:
		return "Bull"
	default:
		return "?"
	}
}

// AllSegments is every scoring segment on the board, in a fixed order so that
// anything enumerating them is deterministic.
var AllSegments = func() []Segment {
	out := make([]Segment, 0, 3*HighestNumber+2)
	for _, kind := range []SegmentKind{Single, Double, Treble} {
		for n := 1; n <= HighestNumber; n++ {
			out = append(out, Segment{Kind: kind, Number: n})
		}
	}
	return append(out, Outer(), Bull())
}()

// FinishSegments is every segment a double-out leg may finish on: D1 to D20 and
// the inner bull.
var FinishSegments = func() []Segment {
	out := make([]Segment, 0, HighestNumber+1)
	for _, s := range AllSegments {
		if s.IsFinish() {
			out = append(out, s)
		}
	}
	return out
}()

// FinishByValue maps a score to the single segment that can finish on it.
//
// The mapping is unambiguous: the doubles cover the even numbers 2 to 40 and
// the bull covers 50, with no overlap.
var FinishByValue = func() map[int]Segment {
	out := make(map[int]Segment, len(FinishSegments))
	for _, s := range FinishSegments {
		out[s.Value()] = s
	}
	return out
}()

// ErrBadSegment is returned when a segment cannot be read.
var ErrBadSegment = fmt.Errorf("not a dartboard segment")

// ParseSegment reads the usual shorthand: T20, D16, S8, 8, Bull, 25.
//
// It exists so curated checkout routes can be written the way a player would
// say them, and still be turned into real segments that the solver can check.
func ParseSegment(text string) (Segment, error) {
	t := strings.TrimSpace(text)
	if t == "" {
		return Segment{}, fmt.Errorf("%w: empty", ErrBadSegment)
	}

	switch strings.ToUpper(t) {
	case "BULL", "B", "50", "DB":
		return Bull(), nil
	case "25", "OB", "SB":
		return Outer(), nil
	}

	kind := Single
	digits := t
	switch strings.ToUpper(t)[0] {
	case 'S':
		digits = t[1:]
	case 'D':
		kind, digits = Double, t[1:]
	case 'T':
		kind, digits = Treble, t[1:]
	}

	number, err := strconv.Atoi(strings.TrimSpace(digits))
	if err != nil {
		return Segment{}, fmt.Errorf("%w: %q", ErrBadSegment, text)
	}
	seg := Segment{Kind: kind, Number: number}
	if !seg.Valid() {
		return Segment{}, fmt.Errorf("%w: %q", ErrBadSegment, text)
	}
	return seg, nil
}
