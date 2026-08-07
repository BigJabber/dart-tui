package darts

import (
	"math"
	"testing"
)

func TestReachableCommonScores(t *testing.T) {
	cases := []struct {
		points, darts int
		want          bool
	}{
		{0, 3, true},    // three misses
		{0, 1, true},    // one miss
		{180, 3, true},  // three treble twenties
		{181, 3, false}, // above the maximum
		{179, 3, false}, // classic impossible total
		{178, 3, false},
		{176, 3, false},
		{175, 3, false},
		{173, 3, false},
		{172, 3, false},
		{169, 3, false},
		{166, 3, false},
		{163, 3, false},
		{177, 3, true}, // T20 T20 T19
		{174, 3, true}, // T20 T20 T18
		{171, 3, true}, // T19 T19 T19
		{170, 3, true}, // T20 T20 bull
		{167, 3, true}, // T20 T19 bull
		{164, 3, true},
		{60, 1, true},   // treble twenty
		{61, 1, false},  // nothing scores 61 with one dart
		{50, 1, true},   // inner bull
		{25, 1, true},   // outer bull
		{23, 1, false},  // no single segment scores 23
		{120, 2, true},  // two trebles
		{121, 2, false}, // above the two-dart maximum
		{100, 2, true},  // bull + bull
		{-1, 3, false},
	}
	for _, tc := range cases {
		if got := Reachable(tc.points, tc.darts); got != tc.want {
			t.Errorf("Reachable(%d, %d) = %v, want %v", tc.points, tc.darts, got, tc.want)
		}
	}
}

func TestReachableRejectsBadDartCounts(t *testing.T) {
	for _, n := range []int{-1, 0, 4, 99} {
		if Reachable(60, n) {
			t.Errorf("Reachable(60, %d) should be false", n)
		}
	}
}

// Anything reachable with n darts is reachable with n+1, because a dart may miss.
func TestReachabilityIsMonotonic(t *testing.T) {
	for points := 0; points <= 120; points++ {
		if Reachable(points, 2) && !Reachable(points, 3) {
			t.Errorf("%d reachable with 2 darts but not 3", points)
		}
	}
	for points := 0; points <= 60; points++ {
		if Reachable(points, 1) && !Reachable(points, 2) {
			t.Errorf("%d reachable with 1 dart but not 2", points)
		}
	}
}

func TestValidDartCount(t *testing.T) {
	for _, n := range []int{1, 2, 3} {
		if !ValidDartCount(n) {
			t.Errorf("%d should be a valid dart count", n)
		}
	}
	for _, n := range []int{-1, 0, 4} {
		if ValidDartCount(n) {
			t.Errorf("%d should not be a valid dart count", n)
		}
	}
}

func TestMaxScore(t *testing.T) {
	cases := map[int]int{0: 0, 1: 60, 2: 120, 3: 180, 9: 180, -1: 0}
	for darts, want := range cases {
		if got := MaxScore(darts); got != want {
			t.Errorf("MaxScore(%d) = %d, want %d", darts, got, want)
		}
	}
}

func TestThreeDartAverage(t *testing.T) {
	cases := []struct {
		points, darts int
		want          float64
	}{
		{501, 28, 501.0 / 28 * 3},
		{501, 30, 50.1},
		{0, 3, 0},
		{100, 0, 0}, // no darts thrown: no average, not a divide by zero
		{60, 3, 60},
	}
	for _, tc := range cases {
		if got := ThreeDartAverage(tc.points, tc.darts); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("ThreeDartAverage(%d, %d) = %v, want %v", tc.points, tc.darts, got, tc.want)
		}
	}
}

func TestSingleDartScoresAreSortedAndSane(t *testing.T) {
	if SingleDartScores[0] != 0 {
		t.Errorf("first single-dart score = %d, want 0 (a miss)", SingleDartScores[0])
	}
	last := SingleDartScores[len(SingleDartScores)-1]
	if last != 60 {
		t.Errorf("highest single-dart score = %d, want 60", last)
	}
	for i := 1; i < len(SingleDartScores); i++ {
		if SingleDartScores[i] <= SingleDartScores[i-1] {
			t.Fatalf("single dart scores are not sorted at index %d", i)
		}
	}
}
