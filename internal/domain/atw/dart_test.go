package atw

import (
	"errors"
	"testing"

	"github.com/BigJabber/dart-tui/internal/domain/darts"
)

func TestParseCall(t *testing.T) {
	cases := map[rune]Call{'0': CallMiss, '1': CallSingle, '2': CallDouble, '3': CallTreble}
	for r, want := range cases {
		got, err := ParseCall(r)
		if err != nil {
			t.Errorf("ParseCall(%q): %v", string(r), err)
			continue
		}
		if got != want {
			t.Errorf("ParseCall(%q) = %d, want %d", string(r), got, want)
		}
		if got.Key() != string(r) {
			t.Errorf("%d.Key() = %q, want %q", got, got.Key(), string(r))
		}
	}
	for _, r := range []rune{'4', '9', 'x', '-', ' '} {
		if _, err := ParseCall(r); !errors.Is(err, ErrBadCall) {
			t.Errorf("ParseCall(%q) error = %v, want ErrBadCall", string(r), err)
		}
	}
}

func TestParseCallsReadsAWholeVisit(t *testing.T) {
	got, err := ParseCalls("031")
	if err != nil {
		t.Fatal(err)
	}
	want := []Call{CallMiss, CallTreble, CallSingle}
	if len(got) != len(want) {
		t.Fatalf("ParseCalls(\"031\") = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("dart %d = %d, want %d", i+1, got[i], want[i])
		}
	}

	// Spaces are for the operator's benefit and mean nothing.
	spaced, err := ParseCalls("0 3 1")
	if err != nil {
		t.Fatal(err)
	}
	if len(spaced) != 3 {
		t.Errorf("ParseCalls(\"0 3 1\") = %v, want three darts", spaced)
	}
}

func TestParseCallsRejectsBadInput(t *testing.T) {
	for _, text := range []string{"", "  ", "0314", "1119", "abc", "4"} {
		if _, err := ParseCalls(text); err == nil {
			t.Errorf("ParseCalls(%q) should have been refused", text)
		}
	}
}

// A keystroke means different things on different targets, which is the whole
// reason the domain resolves it rather than the screen.
func TestResolveOnANumber(t *testing.T) {
	cases := map[Call]darts.Segment{
		CallSingle: darts.S(7),
		CallDouble: darts.D(7),
		CallTreble: darts.T(7),
	}
	for call, want := range cases {
		got, err := Resolve(number(7), call)
		if err != nil {
			t.Fatalf("Resolve(7, %d): %v", call, err)
		}
		if !got.Hit || got.Segment != want {
			t.Errorf("Resolve(7, %d) = %+v, want %v", call, got, want)
		}
	}

	miss, err := Resolve(number(7), CallMiss)
	if err != nil {
		t.Fatal(err)
	}
	if miss.Hit {
		t.Errorf("a miss should not be a hit: %+v", miss)
	}
	if miss.Notation() != "-" {
		t.Errorf("miss notation = %q, want %q", miss.Notation(), "-")
	}
}

// The physical board has no treble bull, and a hit on either bull is recorded
// as the ring that was actually struck rather than as a multiplier.
func TestResolveOnTheBulls(t *testing.T) {
	outerHit, err := Resolve(outer, CallSingle)
	if err != nil {
		t.Fatal(err)
	}
	if outerHit.Segment != darts.Outer() {
		t.Errorf("outer bull, 1 = %v, want the outer bull", outerHit.Segment)
	}

	// 2 on the outer bull means the player found the middle instead.
	inner, err := Resolve(outer, CallDouble)
	if err != nil {
		t.Fatal(err)
	}
	if inner.Segment != darts.Bull() {
		t.Errorf("outer bull, 2 = %v, want the bullseye", inner.Segment)
	}

	// Both keys are accepted on the bullseye, and both record the bullseye.
	for _, call := range []Call{CallSingle, CallDouble} {
		d, err := Resolve(bull, call)
		if err != nil {
			t.Fatalf("Resolve(bullseye, %d): %v", call, err)
		}
		if d.Segment != darts.Bull() {
			t.Errorf("bullseye, %d = %v, want the bullseye", call, d.Segment)
		}
	}
}

func TestTrebleBullIsRejected(t *testing.T) {
	for _, target := range []Target{outer, bull} {
		if _, err := Resolve(target, CallTreble); !errors.Is(err, ErrNoTrebleBull) {
			t.Errorf("Resolve(%v, 3) error = %v, want ErrNoTrebleBull", target, err)
		}
	}
}

func TestResolveRefusesAFinishedRun(t *testing.T) {
	if _, err := Resolve(Finished, CallSingle); !errors.Is(err, ErrRunComplete) {
		t.Errorf("error = %v, want ErrRunComplete", err)
	}
}

// Advance is what replay uses, so it has to agree with Resolve on every case.
func TestAdvanceMatchesResolve(t *testing.T) {
	cases := []struct {
		target Target
		call   Call
		steps  int
	}{
		{number(7), CallMiss, 0},
		{number(7), CallSingle, 1},
		{number(7), CallDouble, 2},
		{number(7), CallTreble, 3},
		{outer, CallMiss, 0},
		{outer, CallSingle, 1},
		{outer, CallDouble, 2},
		{bull, CallMiss, 0},
		{bull, CallSingle, 1},
		{bull, CallDouble, 1},
	}
	for _, c := range cases {
		dart, err := Resolve(c.target, c.call)
		if err != nil {
			t.Fatalf("Resolve(%v, %d): %v", c.target, c.call, err)
		}
		steps, err := Advance(c.target, dart)
		if err != nil {
			t.Fatalf("Advance(%v, %v): %v", c.target, dart, err)
		}
		if steps != c.steps {
			t.Errorf("%v with %d advanced %d, want %d", c.target, c.call, steps, c.steps)
		}
	}
}

// A dart that could not have been thrown at the target is refused, which is
// what protects a replay from a hand-edited database.
func TestAdvanceRejectsADartFromAnotherTarget(t *testing.T) {
	cases := []struct {
		target Target
		dart   Dart
	}{
		{number(7), HitOn(darts.S(8))},
		{number(7), HitOn(darts.Bull())},
		{outer, HitOn(darts.S(20))},
		{bull, HitOn(darts.Outer())},
	}
	for _, c := range cases {
		if _, err := Advance(c.target, c.dart); !errors.Is(err, ErrBadDart) {
			t.Errorf("Advance(%v, %v) error = %v, want ErrBadDart", c.target, c.dart, err)
		}
	}
}

func TestDartNotationRoundTrips(t *testing.T) {
	all := []Dart{
		Miss(),
		HitOn(darts.S(1)),
		HitOn(darts.D(14)),
		HitOn(darts.T(20)),
		HitOn(darts.Outer()),
		HitOn(darts.Bull()),
	}
	for _, d := range all {
		back, err := ParseDart(d.Notation())
		if err != nil {
			t.Fatalf("ParseDart(%q): %v", d.Notation(), err)
		}
		if back != d {
			t.Errorf("%q round-tripped to %+v, want %+v", d.Notation(), back, d)
		}
	}
	if _, err := ParseDart("nonsense"); err == nil {
		t.Error("ParseDart should refuse nonsense")
	}
}

func TestDartLabels(t *testing.T) {
	cases := map[string]Dart{
		"miss":       Miss(),
		"single 7":   HitOn(darts.S(7)),
		"double 14":  HitOn(darts.D(14)),
		"treble 20":  HitOn(darts.T(20)),
		"outer bull": HitOn(darts.Outer()),
		"bullseye":   HitOn(darts.Bull()),
	}
	for want, d := range cases {
		if got := d.Label(); got != want {
			t.Errorf("label = %q, want %q", got, want)
		}
	}
}
