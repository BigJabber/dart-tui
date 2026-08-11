package atw

import "strconv"

// Kind is what sort of thing a player is currently aiming at.
type Kind int

// The kinds of target, in the order they are played.
const (
	// Number is one of the numbers 1 to 20.
	Number Kind = iota + 1
	// OuterBull is the green ring, worth 25.
	OuterBull
	// Bullseye is the centre, worth 50. It is the last target in the run.
	Bullseye
	// Complete is not a target at all: it is where a player ends up after
	// hitting the bullseye, and the only state in which they have finished.
	Complete
)

// Target is one step of the run.
//
// It is a small ordered model rather than a bare integer on purpose. Around the
// World ends with two targets that are not numbers, and the interesting rules
// are exactly at that join: a treble 20 has to carry a player through the outer
// bull and the bullseye and out the other side. Arithmetic on "21 means outer
// bull" gets that right only by accident, and gets it wrong the first time
// somebody changes the sequence. Advancing along an explicit list cannot.
type Target struct {
	Kind Kind
	// Number is 1-20 when Kind is Number, and zero otherwise.
	Number int
}

// Sequence is the run, in order: 1 to 20, then the outer bull, then the
// bullseye. Completion is what lies past the end of it, not a member of it.
var Sequence = func() []Target {
	out := make([]Target, 0, HighestNumber+2)
	for n := 1; n <= HighestNumber; n++ {
		out = append(out, Target{Kind: Number, Number: n})
	}
	return append(out, Target{Kind: OuterBull}, Target{Kind: Bullseye})
}()

// HighestNumber is the last numbered target before the bulls.
const HighestNumber = 20

// TargetCount is how many targets make up a full run.
var TargetCount = len(Sequence)

// Finished is the state of a player who has completed their run.
var Finished = Target{Kind: Complete}

// First is where every player starts.
func First() Target { return Sequence[0] }

// Index is a target's position in the sequence, counting from zero.
// A completed run reports TargetCount, one past the end.
func (t Target) Index() int {
	switch t.Kind {
	case Number:
		return t.Number - 1
	case OuterBull:
		return HighestNumber
	case Bullseye:
		return HighestNumber + 1
	default:
		return TargetCount
	}
}

// At returns the target at a position in the sequence. Anything at or past the
// end of the sequence is a completed run, which is what makes advancing off the
// end of the board safe rather than a special case.
func At(index int) Target {
	if index < 0 {
		return First()
	}
	if index >= TargetCount {
		return Finished
	}
	return Sequence[index]
}

// Advance moves n targets along the sequence. Advancing past the bullseye
// completes the run: a treble 20 progresses through 20, the outer bull and the
// bullseye, and finishes.
func (t Target) Advance(n int) Target {
	if n <= 0 {
		return t
	}
	return At(t.Index() + n)
}

// Done reports whether the run is complete.
func (t Target) Done() bool { return t.Kind == Complete }

// Valid reports whether the target is one the sequence actually contains, or a
// completed run.
func (t Target) Valid() bool {
	switch t.Kind {
	case Number:
		return t.Number >= 1 && t.Number <= HighestNumber
	case OuterBull, Bullseye, Complete:
		return t.Number == 0
	default:
		return false
	}
}

// Remaining is how many targets are still to be hit, zero once finished. It is
// what makes an unfinished player comparable to another unfinished player
// without pretending either of them has a dart count worth ranking.
func (t Target) Remaining() int {
	if r := TargetCount - t.Index(); r > 0 {
		return r
	}
	return 0
}

// String is the name to show the operator: "12", "Outer Bull", "Bullseye".
//
// The internal position in the sequence is never displayed. A player aiming at
// the bull wants to read the word, not the number 21.
func (t Target) String() string {
	switch t.Kind {
	case Number:
		return strconv.Itoa(t.Number)
	case OuterBull:
		return "Outer Bull"
	case Bullseye:
		return "Bullseye"
	case Complete:
		return "Finished"
	default:
		return "?"
	}
}

// Value is what the target is worth on a dartboard when hit with a single dart:
// its face value, 25 for the outer bull, 50 for the bullseye.
func (t Target) Value() int {
	switch t.Kind {
	case Number:
		return t.Number
	case OuterBull:
		return 25
	case Bullseye:
		return 50
	default:
		return 0
	}
}
