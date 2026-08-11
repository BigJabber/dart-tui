package atw

import "testing"

func number(n int) Target { return Target{Kind: Number, Number: n} }

var (
	outer = Target{Kind: OuterBull}
	bull  = Target{Kind: Bullseye}
)

func TestSequenceIsTheWholeBoardInOrder(t *testing.T) {
	if TargetCount != 22 {
		t.Fatalf("TargetCount = %d, want 22 (1-20, outer bull, bullseye)", TargetCount)
	}
	for n := 1; n <= 20; n++ {
		if got := Sequence[n-1]; got != number(n) {
			t.Errorf("Sequence[%d] = %v, want %d", n-1, got, n)
		}
	}
	if Sequence[20] != outer {
		t.Errorf("Sequence[20] = %v, want the outer bull", Sequence[20])
	}
	if Sequence[21] != bull {
		t.Errorf("Sequence[21] = %v, want the bullseye", Sequence[21])
	}
	if First() != number(1) {
		t.Errorf("First() = %v, want 1", First())
	}
}

func TestIndexAndAtRoundTrip(t *testing.T) {
	for i, want := range Sequence {
		if got := At(i); got != want {
			t.Errorf("At(%d) = %v, want %v", i, got, want)
		}
		if got := want.Index(); got != i {
			t.Errorf("%v.Index() = %d, want %d", want, got, i)
		}
	}
	// Anything past the end is a completed run rather than a target.
	if got := At(TargetCount); !got.Done() {
		t.Errorf("At(%d) = %v, want finished", TargetCount, got)
	}
	if got := At(TargetCount + 5); !got.Done() {
		t.Errorf("At(%d) = %v, want finished", TargetCount+5, got)
	}
	if got := Finished.Index(); got != TargetCount {
		t.Errorf("Finished.Index() = %d, want %d", got, TargetCount)
	}
}

// The specified progression, target by target.
func TestAdvanceThroughTheNumbers(t *testing.T) {
	cases := []struct {
		from  Target
		steps int
		want  Target
	}{
		{number(1), 0, number(1)},
		{number(1), 1, number(2)},
		{number(1), 2, number(3)},
		{number(1), 3, number(4)},
		{number(4), 0, number(4)},
		{number(7), 1, number(8)},
		{number(7), 2, number(9)},
		{number(7), 3, number(10)},
	}
	for _, c := range cases {
		if got := c.from.Advance(c.steps); got != c.want {
			t.Errorf("%v advanced %d = %v, want %v", c.from, c.steps, got, c.want)
		}
	}
}

// The join between the numbers and the bulls is where the ordered sequence
// earns its keep.
func TestAdvanceOffTheEndOfTheNumbers(t *testing.T) {
	cases := []struct {
		from  Target
		steps int
		want  Target
	}{
		{number(19), 1, number(20)},
		{number(19), 2, outer},
		{number(19), 3, bull},
		{number(20), 1, outer},
		{number(20), 2, bull},
		{number(20), 3, Finished},
		{outer, 1, bull},
		{outer, 2, Finished},
		{bull, 1, Finished},
	}
	for _, c := range cases {
		got := c.from.Advance(c.steps)
		if got != c.want {
			t.Errorf("%v advanced %d = %v, want %v", c.from, c.steps, got, c.want)
		}
	}
}

func TestRemainingCountsTargetsLeft(t *testing.T) {
	if got := number(1).Remaining(); got != 22 {
		t.Errorf("1 remaining = %d, want 22", got)
	}
	if got := number(20).Remaining(); got != 3 {
		t.Errorf("20 remaining = %d, want 3 (20, outer, bull)", got)
	}
	if got := bull.Remaining(); got != 1 {
		t.Errorf("bullseye remaining = %d, want 1", got)
	}
	if got := Finished.Remaining(); got != 0 {
		t.Errorf("finished remaining = %d, want 0", got)
	}
}

// The bull targets must never show their position in the sequence.
func TestTargetNamesAreFriendly(t *testing.T) {
	cases := map[Target]string{
		number(1):  "1",
		number(20): "20",
		outer:      "Outer Bull",
		bull:       "Bullseye",
		Finished:   "Finished",
	}
	for target, want := range cases {
		if got := target.String(); got != want {
			t.Errorf("%v.String() = %q, want %q", target.Kind, got, want)
		}
	}
}

func TestTargetValuesAreBoardScores(t *testing.T) {
	if got := number(12).Value(); got != 12 {
		t.Errorf("12 value = %d, want 12", got)
	}
	if got := outer.Value(); got != 25 {
		t.Errorf("outer bull value = %d, want 25", got)
	}
	if got := bull.Value(); got != 50 {
		t.Errorf("bullseye value = %d, want 50", got)
	}
}

func TestTargetValidity(t *testing.T) {
	valid := []Target{number(1), number(20), outer, bull, Finished}
	for _, target := range valid {
		if !target.Valid() {
			t.Errorf("%v should be valid", target)
		}
	}
	invalid := []Target{{Kind: Number, Number: 0}, {Kind: Number, Number: 21}, {}}
	for _, target := range invalid {
		if target.Valid() {
			t.Errorf("%v should not be valid", target)
		}
	}
}
