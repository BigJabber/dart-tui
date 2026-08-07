package darts

import (
	"errors"
	"testing"
)

func TestSegmentValueForEveryKind(t *testing.T) {
	cases := []struct {
		seg  Segment
		want int
	}{
		{S(1), 1}, {S(20), 20},
		{D(1), 2}, {D(20), 40},
		{T(1), 3}, {T(20), 60},
		{Outer(), 25},
		{Bull(), 50},
		{Segment{}, 0}, // an unset segment scores nothing rather than panicking
	}
	for _, tc := range cases {
		if got := tc.seg.Value(); got != tc.want {
			t.Errorf("%v value = %d, want %d", tc.seg, got, tc.want)
		}
	}
}

func TestAllSegmentsCoversTheBoard(t *testing.T) {
	if len(AllSegments) != 62 {
		t.Fatalf("got %d segments, want 62", len(AllSegments))
	}
	counts := map[SegmentKind]int{}
	seen := map[Segment]bool{}
	for _, s := range AllSegments {
		if !s.Valid() {
			t.Errorf("%v is not valid", s)
		}
		if seen[s] {
			t.Errorf("%v appears twice", s)
		}
		seen[s] = true
		counts[s.Kind]++
	}
	for _, kind := range []SegmentKind{Single, Double, Treble} {
		if counts[kind] != HighestNumber {
			t.Errorf("kind %d has %d segments, want %d", kind, counts[kind], HighestNumber)
		}
	}
	if counts[OuterBull] != 1 || counts[InnerBull] != 1 {
		t.Errorf("bulls = %d outer, %d inner; want one of each", counts[OuterBull], counts[InnerBull])
	}
}

// The highest a single dart can score is the treble twenty.
func TestHighestSegmentIsTrebleTwenty(t *testing.T) {
	best := 0
	for _, s := range AllSegments {
		if v := s.Value(); v > best {
			best = v
		}
	}
	if best != 60 {
		t.Errorf("highest segment = %d, want 60", best)
	}
}

func TestFinishSegments(t *testing.T) {
	if len(FinishSegments) != HighestNumber+1 {
		t.Errorf("got %d finishes, want %d", len(FinishSegments), HighestNumber+1)
	}
	for _, s := range FinishSegments {
		if !s.IsFinish() {
			t.Errorf("%v is in FinishSegments but is not a finish", s)
		}
		if s.Kind != Double && s.Kind != InnerBull {
			t.Errorf("%v should not be able to finish a leg", s)
		}
	}
	// The outer bull scores but cannot finish.
	if Outer().IsFinish() {
		t.Error("the outer bull must not finish a leg")
	}
	if _, ok := FinishByValue[25]; ok {
		t.Error("25 must not appear as a finishing value")
	}
}

// Every finishing value maps to exactly one segment, which is what lets the
// solver look a finish up by score.
func TestFinishByValueIsUnambiguous(t *testing.T) {
	if len(FinishByValue) != len(FinishSegments) {
		t.Errorf("FinishByValue has %d entries for %d segments — a value collides",
			len(FinishByValue), len(FinishSegments))
	}
	for value, seg := range FinishByValue {
		if seg.Value() != value {
			t.Errorf("FinishByValue[%d] = %v, which scores %d", value, seg, seg.Value())
		}
	}
	for v := 2; v <= 40; v += 2 {
		if _, ok := FinishByValue[v]; !ok {
			t.Errorf("%d should be finishable on a double", v)
		}
	}
	for _, v := range []int{1, 3, 41, 42, 49, 51} {
		if _, ok := FinishByValue[v]; ok {
			t.Errorf("%d should not be a finishing value", v)
		}
	}
}

func TestSegmentString(t *testing.T) {
	cases := map[string]Segment{
		"20":   S(20),
		"D16":  D(16),
		"T20":  T(20),
		"25":   Outer(),
		"Bull": Bull(),
		"?":    Segment{},
	}
	for want, seg := range cases {
		if got := seg.String(); got != want {
			t.Errorf("%v rendered as %q, want %q", seg, got, want)
		}
	}
}

func TestParseSegmentRoundTrip(t *testing.T) {
	for _, s := range AllSegments {
		got, err := ParseSegment(s.String())
		if err != nil {
			t.Errorf("ParseSegment(%q): %v", s, err)
			continue
		}
		if got != s {
			t.Errorf("ParseSegment(%q) = %v, want %v", s, got, s)
		}
	}
}

func TestParseSegmentAlternativeSpellings(t *testing.T) {
	cases := map[string]Segment{
		"s20": S(20), "S20": S(20), " 20 ": S(20),
		"d16": D(16), "t9": T(9),
		"bull": Bull(), "B": Bull(), "50": Bull(), "db": Bull(),
		"ob": Outer(), "SB": Outer(),
	}
	for text, want := range cases {
		got, err := ParseSegment(text)
		if err != nil {
			t.Errorf("ParseSegment(%q): %v", text, err)
			continue
		}
		if got != want {
			t.Errorf("ParseSegment(%q) = %v, want %v", text, got, want)
		}
	}
}

func TestParseSegmentRejectsNonsense(t *testing.T) {
	for _, bad := range []string{"", "  ", "T21", "D21", "S0", "T0", "X1", "26", "-5", "T", "D", "twenty"} {
		if got, err := ParseSegment(bad); !errors.Is(err, ErrBadSegment) {
			t.Errorf("ParseSegment(%q) = %v, %v; want ErrBadSegment", bad, got, err)
		}
	}
}

// The set of single-dart scores is derived from the segments, so the two must
// agree.
func TestSingleDartScoresMatchTheSegments(t *testing.T) {
	fromSegments := map[int]bool{0: true}
	for _, s := range AllSegments {
		fromSegments[s.Value()] = true
	}
	if len(SingleDartScores) != len(fromSegments) {
		t.Errorf("got %d single-dart scores, want %d", len(SingleDartScores), len(fromSegments))
	}
	for _, v := range SingleDartScores {
		if !fromSegments[v] {
			t.Errorf("%d is a listed score but no segment produces it", v)
		}
	}
}
