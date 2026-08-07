package x01

import (
	"errors"
	"strings"
	"testing"

	"github.com/BigJabber/dart-tui/internal/domain/darts"
)

// bogeyNumbers are the scores that cannot be finished in three darts even though
// higher scores can. They are the published list; the solver derives them
// independently, and TestBogeyNumbersAreExactlyTheKnownList checks the two agree.
var bogeyNumbers = []int{159, 162, 163, 165, 166, 168, 169}

func routeStrings(routes []CheckoutRoute) []string {
	out := make([]string, 0, len(routes))
	for _, r := range routes {
		out = append(out, r.String())
	}
	return out
}

func containsRoute(routes []CheckoutRoute, want string) bool {
	for _, r := range routes {
		if r.String() == want {
			return true
		}
	}
	return false
}

// ------------------------------------------------------------------ segments

func TestSegmentValues(t *testing.T) {
	cases := []struct {
		seg  darts.Segment
		want int
		text string
	}{
		{darts.S(20), 20, "20"},
		{darts.S(1), 1, "1"},
		{darts.D(20), 40, "D20"},
		{darts.D(16), 32, "D16"},
		{darts.D(1), 2, "D1"},
		{darts.T(20), 60, "T20"},
		{darts.T(19), 57, "T19"},
		{darts.Outer(), 25, "25"},
		{darts.Bull(), 50, "Bull"},
	}
	for _, tc := range cases {
		if got := tc.seg.Value(); got != tc.want {
			t.Errorf("%s value = %d, want %d", tc.text, got, tc.want)
		}
		if got := tc.seg.String(); got != tc.text {
			t.Errorf("segment rendered as %q, want %q", got, tc.text)
		}
	}
}

// Only doubles and the inner bull can finish a leg. The outer bull scores 25 but
// is not a double.
func TestOnlyDoublesAndBullCanFinish(t *testing.T) {
	if !darts.D(20).IsFinish() {
		t.Error("D20 should be able to finish")
	}
	if !darts.Bull().IsFinish() {
		t.Error("the inner bull should be able to finish")
	}
	if darts.Outer().IsFinish() {
		t.Error("the outer bull is not a double and must not finish a leg")
	}
	if darts.S(20).IsFinish() || darts.T(20).IsFinish() {
		t.Error("singles and trebles must not finish a leg")
	}

	if len(darts.FinishSegments) != 21 {
		t.Errorf("got %d finishing segments, want 21 (D1-D20 and the bull)", len(darts.FinishSegments))
	}
	if _, ok := darts.FinishByValue[25]; ok {
		t.Error("25 must not be reachable as a finish")
	}
	if seg, ok := darts.FinishByValue[50]; !ok || seg != darts.Bull() {
		t.Errorf("50 should finish on the bull, got %v %v", seg, ok)
	}
}

func TestSegmentValidity(t *testing.T) {
	for _, bad := range []darts.Segment{
		{Kind: darts.Single, Number: 0},
		{Kind: darts.Single, Number: 21},
		{Kind: darts.Double, Number: 25},
		{Kind: darts.Treble, Number: -1},
		{Kind: darts.InnerBull, Number: 5},
		{},
	} {
		if bad.Valid() {
			t.Errorf("%+v should not be a valid segment", bad)
		}
	}
	for _, s := range darts.AllSegments {
		if !s.Valid() {
			t.Errorf("%+v from AllSegments should be valid", s)
		}
	}
	if len(darts.AllSegments) != 62 {
		t.Errorf("got %d segments, want 62 (20 each of S/D/T plus two bulls)", len(darts.AllSegments))
	}
}

func TestParseSegment(t *testing.T) {
	cases := map[string]darts.Segment{
		"T20":  darts.T(20),
		"t20":  darts.T(20),
		"D16":  darts.D(16),
		"S8":   darts.S(8),
		"8":    darts.S(8),
		"Bull": darts.Bull(),
		"bull": darts.Bull(),
		"50":   darts.Bull(),
		"25":   darts.Outer(),
	}
	for text, want := range cases {
		got, err := darts.ParseSegment(text)
		if err != nil {
			t.Errorf("ParseSegment(%q): %v", text, err)
			continue
		}
		if got != want {
			t.Errorf("ParseSegment(%q) = %v, want %v", text, got, want)
		}
	}
	for _, bad := range []string{"", "T21", "D0", "X5", "treble", "T", "99"} {
		if _, err := darts.ParseSegment(bad); !errors.Is(err, darts.ErrBadSegment) {
			t.Errorf("ParseSegment(%q) should have failed, got %v", bad, err)
		}
	}
}

// -------------------------------------------------------------------- solver

// The maximum checkout, and the fact it needs all three darts.
func TestMaximumCheckout(t *testing.T) {
	routes := FindCheckouts(170, 3)
	if !containsRoute(routes, "T20 T20 Bull") {
		t.Errorf("170 with 3 darts should include T20 T20 Bull, got %v", routeStrings(routes))
	}
	// It is the only way to take 170.
	if len(routes) != 1 {
		t.Errorf("170 has %d routes (%v), want exactly 1", len(routes), routeStrings(routes))
	}
	if got := FindCheckouts(170, 2); len(got) != 0 {
		t.Errorf("170 with 2 darts should have no checkout, got %v", routeStrings(got))
	}
	if got := FindCheckouts(170, 1); len(got) != 0 {
		t.Errorf("170 with 1 dart should have no checkout, got %v", routeStrings(got))
	}
	if CanCheckout(170, 2) {
		t.Error("CanCheckout(170, 2) should be false")
	}
	if !CanCheckout(170, 3) {
		t.Error("CanCheckout(170, 3) should be true")
	}
}

// Nothing above 170 can be finished in a visit.
func TestAboveMaximumHasNoCheckout(t *testing.T) {
	for _, score := range []int{171, 172, 180, 200, 501} {
		if got := FindCheckouts(score, 3); len(got) != 0 {
			t.Errorf("%d should have no checkout, got %v", score, routeStrings(got))
		}
		if CanCheckout(score, 3) {
			t.Errorf("CanCheckout(%d, 3) should be false", score)
		}
	}
}

// The solver must derive the published bogey numbers rather than be told them.
func TestBogeyNumbersAreExactlyTheKnownList(t *testing.T) {
	expected := make(map[int]bool, len(bogeyNumbers))
	for _, b := range bogeyNumbers {
		expected[b] = true
		if got := FindCheckouts(b, 3); len(got) != 0 {
			t.Errorf("%d is a bogey number but produced %v", b, routeStrings(got))
		}
		if !IsBogeyNumber(b) {
			t.Errorf("IsBogeyNumber(%d) should be true", b)
		}
	}

	// And no other score in range is unfinishable.
	for score := 2; score <= MaxCheckout; score++ {
		checkable := len(FindCheckouts(score, 3)) > 0
		if expected[score] == checkable {
			t.Errorf("%d: checkable=%v but bogey=%v", score, checkable, expected[score])
		}
	}
}

// One remaining can never be finished: there is no double worth 1.
func TestOneCannotBeFinished(t *testing.T) {
	for _, dartsLeft := range []int{1, 2, 3} {
		if got := FindCheckouts(1, dartsLeft); len(got) != 0 {
			t.Errorf("1 with %d darts should have no checkout, got %v", dartsLeft, routeStrings(got))
		}
	}
	if MinimumDarts(1) != 0 {
		t.Errorf("MinimumDarts(1) = %d, want 0", MinimumDarts(1))
	}
}

func TestOneDartFinishes(t *testing.T) {
	cases := map[int]string{
		40: "D20",
		38: "D19",
		32: "D16",
		20: "D10",
		2:  "D1",
		50: "Bull",
	}
	for score, want := range cases {
		routes := FindCheckouts(score, 1)
		if len(routes) != 1 {
			t.Errorf("%d with 1 dart: got %v, want exactly [%s]", score, routeStrings(routes), want)
			continue
		}
		if routes[0].String() != want {
			t.Errorf("%d with 1 dart = %s, want %s", score, routes[0], want)
		}
		if MinimumDarts(score) != 1 {
			t.Errorf("MinimumDarts(%d) = %d, want 1", score, MinimumDarts(score))
		}
	}
}

// Odd scores, 25 and anything above 50 cannot go in a single dart.
func TestScoresWithNoOneDartFinish(t *testing.T) {
	for _, score := range []int{1, 3, 25, 41, 39, 51, 60, 100} {
		if got := FindCheckouts(score, 1); len(got) != 0 {
			t.Errorf("%d with 1 dart should have no checkout, got %v", score, routeStrings(got))
		}
		if CanCheckout(score, 1) {
			t.Errorf("CanCheckout(%d, 1) should be false", score)
		}
	}
	// 25 is the outer bull, which is not a double, but it goes in two darts.
	if !CanCheckout(25, 2) {
		t.Error("25 should be checkable with two darts")
	}
}

func TestTwoDartFinishes(t *testing.T) {
	cases := map[int]string{
		100: "T20 D20",
		98:  "T20 D19",
		97:  "T19 D20",
		96:  "T20 D18",
		90:  "T20 D15",
		110: "T20 Bull",
	}
	for score, want := range cases {
		routes := FindCheckouts(score, 2)
		if !containsRoute(routes, want) {
			t.Errorf("%d with 2 darts should include %s, got %v", score, want, routeStrings(routes))
		}
		if MinimumDarts(score) != 2 {
			t.Errorf("MinimumDarts(%d) = %d, want 2", score, MinimumDarts(score))
		}
	}
	// 50 goes in one dart, so the bull is a one-dart finish, not a two.
	if MinimumDarts(50) != 1 {
		t.Errorf("MinimumDarts(50) = %d, want 1", MinimumDarts(50))
	}
	if !containsRoute(FindCheckouts(50, 2), "Bull") {
		t.Error("50 with two darts available should still offer the single-dart Bull")
	}
}

func TestThreeDartFinishes(t *testing.T) {
	cases := map[int]string{
		170: "T20 T20 Bull",
		167: "T20 T19 Bull",
		164: "T20 T18 Bull",
		161: "T20 T17 Bull",
		160: "T20 T20 D20",
		158: "T20 T20 D19",
		157: "T20 T19 D20",
	}
	for score, want := range cases {
		routes := FindCheckouts(score, 3)
		if !containsRoute(routes, want) {
			t.Errorf("%d should include %s, got %v", score, want, routeStrings(routes))
		}
	}
}

// A route needing three darts must never be offered when only two are left.
func TestDartLimitIsRespected(t *testing.T) {
	for score := 2; score <= MaxCheckout; score++ {
		for _, available := range []int{1, 2, 3} {
			for _, r := range FindCheckouts(score, available) {
				if r.DartCount() > available {
					t.Fatalf("%d with %d darts offered %s, which needs %d",
						score, available, r, r.DartCount())
				}
			}
		}
	}
}

// A player with three darts may finish in fewer; shorter routes must still be
// offered.
func TestShorterRoutesAreOfferedWhenMoreDartsAreAvailable(t *testing.T) {
	for _, score := range []int{40, 50, 32, 100, 98} {
		withMax := FindCheckouts(score, 3)
		needed := MinimumDarts(score)
		found := false
		for _, r := range withMax {
			if r.DartCount() == needed {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%d with 3 darts never offered a %d-dart route: %v",
				score, needed, routeStrings(withMax))
		}
	}
}

// Every generated route must be a legal checkout for the score requested.
func TestEveryGeneratedRouteIsLegal(t *testing.T) {
	for score := 1; score <= MaxCheckout; score++ {
		for _, r := range FindCheckouts(score, 3) {
			if err := r.Validate(score); err != nil {
				t.Errorf("%d: %s is not legal: %v", score, r, err)
			}
			if r.Total() != score {
				t.Errorf("%d: %s totals %d", score, r, r.Total())
			}
			finish, ok := r.Finish()
			if !ok || !finish.IsFinish() {
				t.Errorf("%d: %s does not end on a double or bull", score, r)
			}
			if r.DartCount() < 1 || r.DartCount() > darts.MaxDartsPerVisit {
				t.Errorf("%d: %s uses %d darts", score, r, r.DartCount())
			}
		}
	}
}

// No score should offer the same route twice, in any ordering.
func TestNoDuplicateRoutes(t *testing.T) {
	for score := 2; score <= MaxCheckout; score++ {
		seen := make(map[string]string)
		for _, r := range FindCheckouts(score, 3) {
			if prev, ok := seen[r.Key()]; ok {
				t.Errorf("%d: %s duplicates %s", score, r, prev)
			}
			seen[r.Key()] = r.String()
		}
	}
}

// Reordering the setup darts is the same route, not an alternative.
func TestSetupOrderIsNotAnAlternative(t *testing.T) {
	a := NewCheckoutRoute(darts.T(20), darts.T(19), darts.Bull())
	b := NewCheckoutRoute(darts.T(19), darts.T(20), darts.Bull())
	if a.Key() != b.Key() {
		t.Errorf("%s and %s should share a key, got %q and %q", a, b, a.Key(), b.Key())
	}
	// A different finish is a genuinely different route.
	c := NewCheckoutRoute(darts.T(20), darts.Bull(), darts.T(19))
	if a.Key() == c.Key() {
		t.Error("routes finishing on different segments must not share a key")
	}
}

func TestMinimumDartsOutOfRange(t *testing.T) {
	for _, score := range []int{-5, 0, 171, 999} {
		if MinimumDarts(score) != 0 {
			t.Errorf("MinimumDarts(%d) = %d, want 0", score, MinimumDarts(score))
		}
		if IsBogeyNumber(score) {
			t.Errorf("IsBogeyNumber(%d) should be false — it is out of range", score)
		}
	}
	if FindCheckouts(100, 0) != nil {
		t.Error("no darts means no checkout")
	}
}

// ------------------------------------------------------------------ curated

// A typo in the hand-maintained chart must fail here rather than reach a player.
func TestCuratedRoutesAreAllLegal(t *testing.T) {
	if errs := CuratedCheckoutErrors(); len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("curated checkout rejected: %v", err)
		}
	}
	for _, score := range CuratedCheckoutScores() {
		for _, r := range curatedRoutes[score] {
			if err := r.Validate(score); err != nil {
				t.Errorf("%d: curated %s is not legal: %v", score, r, err)
			}
			// The solver must independently agree the route exists.
			if !containsRoute(FindCheckouts(score, r.DartCount()), r.String()) {
				t.Errorf("%d: curated %s was not produced by the solver", score, r)
			}
		}
	}
}

// Curated entries must not duplicate a route within one score.
func TestCuratedRoutesAreDistinct(t *testing.T) {
	for _, score := range CuratedCheckoutScores() {
		seen := make(map[string]bool)
		for _, r := range curatedRoutes[score] {
			if seen[r.Key()] {
				t.Errorf("%d lists %s twice", score, r)
			}
			seen[r.Key()] = true
		}
	}
}

// Scores that go in a single dart have no curated entry: the route is forced.
func TestSingleDartScoresAreNotCurated(t *testing.T) {
	for _, score := range CuratedCheckoutScores() {
		if MinimumDarts(score) == 1 {
			t.Errorf("%d goes in one dart and should not be curated", score)
		}
	}
}

// ---------------------------------------------------------- recommendations

// The conventional route must be suggested first.
func TestPreferredRouteComesFirst(t *testing.T) {
	cases := map[int]string{
		170: "T20 T20 Bull",
		167: "T20 T19 Bull",
		164: "T20 T18 Bull",
		161: "T20 T17 Bull",
		160: "T20 T20 D20",
		158: "T20 T20 D19",
		157: "T20 T19 D20",
		100: "T20 D20",
		98:  "T20 D19",
		97:  "T19 D20",
		96:  "T20 D18",
		90:  "T20 D15",
		80:  "T20 D10",
		60:  "20 D20",
		40:  "D20",
		50:  "Bull",
		32:  "D16",
		2:   "D1",
	}
	for score, want := range cases {
		got := RecommendCheckouts(score, 3, MaxCheckoutRecommendations)
		if len(got) == 0 {
			t.Errorf("%d: no recommendation, want %s", score, want)
			continue
		}
		if got[0].String() != want {
			t.Errorf("%d: first recommendation %s, want %s (all: %v)",
				score, got[0], want, routeStrings(got))
		}
	}
}

// A score with one sensible finish shows one suggestion, not padding.
func TestSingleSensibleRouteIsNotPadded(t *testing.T) {
	for _, score := range []int{170, 167, 164, 161, 160, 100, 40, 50, 32} {
		got := RecommendCheckouts(score, 3, MaxCheckoutRecommendations)
		if len(got) != 1 {
			t.Errorf("%d should offer one route, got %v", score, routeStrings(got))
		}
	}
}

// Where several finishes are genuinely used, alternatives are offered.
func TestAlternativesAreOfferedWhereSensible(t *testing.T) {
	got := RecommendCheckouts(80, 3, MaxCheckoutRecommendations)
	if len(got) < 2 {
		t.Fatalf("80 should offer alternatives, got %v", routeStrings(got))
	}
	if got[0].String() != "T20 D10" || got[1].String() != "T16 D16" {
		t.Errorf("80 = %v, want T20 D10 then T16 D16", routeStrings(got))
	}
}

func TestRecommendationsRespectTheLimit(t *testing.T) {
	for score := 2; score <= MaxCheckout; score++ {
		for _, max := range []int{1, 2, 3} {
			got := RecommendCheckouts(score, 3, max)
			if len(got) > max {
				t.Errorf("%d: got %d recommendations, want at most %d", score, len(got), max)
			}
		}
	}
	if got := RecommendCheckouts(100, 3, 0); got != nil {
		t.Errorf("a limit of zero should return nothing, got %v", routeStrings(got))
	}
}

// Every recommendation must be legal, unique, and within the darts available.
func TestRecommendationsAreAlwaysValid(t *testing.T) {
	for score := 1; score <= 200; score++ {
		for _, available := range []int{1, 2, 3} {
			got := RecommendCheckouts(score, available, MaxCheckoutRecommendations)

			if !CanCheckout(score, available) {
				if len(got) != 0 {
					t.Errorf("%d with %d darts cannot check out but got %v",
						score, available, routeStrings(got))
				}
				continue
			}
			if len(got) == 0 {
				t.Errorf("%d with %d darts is checkable but got no recommendation", score, available)
				continue
			}

			seen := make(map[string]bool)
			for _, r := range got {
				if err := r.Validate(score); err != nil {
					t.Errorf("%d with %d darts: %s is illegal: %v", score, available, r, err)
				}
				if r.DartCount() > available {
					t.Errorf("%d with %d darts: %s needs %d", score, available, r, r.DartCount())
				}
				if seen[r.Key()] {
					t.Errorf("%d with %d darts: %s recommended twice", score, available, r)
				}
				seen[r.Key()] = true
			}
		}
	}
}

// With fewer darts the conventional route may not fit, and the solver's answer
// is used instead. 101 is the clearest case: T17 10 D20 normally, T17 Bull when
// only two darts remain.
func TestFewerDartsFallsBackToTheSolver(t *testing.T) {
	withThree := RecommendCheckouts(101, 3, 3)
	if len(withThree) == 0 || withThree[0].String() != "T17 10 D20" {
		t.Errorf("101 with 3 darts = %v, want T17 10 D20 first", routeStrings(withThree))
	}
	withTwo := RecommendCheckouts(101, 2, 3)
	if len(withTwo) != 1 || withTwo[0].String() != "T17 Bull" {
		t.Errorf("101 with 2 darts = %v, want [T17 Bull]", routeStrings(withTwo))
	}
	if got := RecommendCheckouts(101, 1, 3); len(got) != 0 {
		t.Errorf("101 with 1 dart should have no checkout, got %v", routeStrings(got))
	}
}

// Recommendations must not change between identical calls: they are read off a
// screen while somebody is standing at the board.
func TestRecommendationsAreDeterministic(t *testing.T) {
	for score := 2; score <= MaxCheckout; score++ {
		first := routeStrings(RecommendCheckouts(score, 3, 3))
		for i := 0; i < 5; i++ {
			again := routeStrings(RecommendCheckouts(score, 3, 3))
			if strings.Join(first, "|") != strings.Join(again, "|") {
				t.Fatalf("%d changed between calls: %v then %v", score, first, again)
			}
		}
	}
}

// An advisor with different preferences changes the suggestions without
// touching the solver, which is what future per-player favourites would need.
func TestAdvisorPreferencesAreReplaceable(t *testing.T) {
	custom := NewCheckoutAdvisor()
	custom.preferred = map[int][]CheckoutRoute{
		80: {NewCheckoutRoute(darts.D(20), darts.D(20))},
	}
	got := custom.Recommend(80, 3, 3)
	if len(got) != 1 || got[0].String() != "D20 D20" {
		t.Errorf("custom advisor gave %v, want [D20 D20]", routeStrings(got))
	}
	// The shared advisor is unaffected.
	if shared := RecommendCheckouts(80, 3, 1); shared[0].String() != "T20 D10" {
		t.Errorf("the default advisor changed: %v", routeStrings(shared))
	}
}

// -------------------------------------------------------------- route basics

func TestRouteValidationRejectsBadRoutes(t *testing.T) {
	cases := []struct {
		name  string
		route CheckoutRoute
		score int
		want  error
	}{
		{"empty", CheckoutRoute{}, 40, ErrRouteEmpty},
		{
			"too many darts",
			NewCheckoutRoute(darts.S(1), darts.S(1), darts.S(1), darts.D(1)),
			5, ErrRouteTooLong,
		},
		{
			"not a double",
			NewCheckoutRoute(darts.T(20), darts.S(20)),
			80, ErrRouteNotOnDouble,
		},
		{
			"outer bull cannot finish",
			NewCheckoutRoute(darts.T(20), darts.Outer()),
			85, ErrRouteNotOnDouble,
		},
		{
			// Falls short rather than overshooting: 120 - 60 - 40 leaves 20.
			"wrong total",
			NewCheckoutRoute(darts.T(20), darts.D(20)),
			120, ErrRouteWrongTotal,
		},
		{
			"passes zero early",
			NewCheckoutRoute(darts.T(20), darts.D(20)),
			40, ErrRouteOvershoots,
		},
		{
			"reaches zero before the last dart",
			NewCheckoutRoute(darts.D(20), darts.D(20)),
			40, ErrRouteOvershoots,
		},
		{
			"impossible segment",
			NewCheckoutRoute(darts.Segment{Kind: darts.Treble, Number: 25}),
			75, ErrRouteBadSegment,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.route.Validate(tc.score); !errors.Is(err, tc.want) {
				t.Errorf("got %v, want %v", err, tc.want)
			}
		})
	}

	valid := NewCheckoutRoute(darts.T(20), darts.T(20), darts.Bull())
	if err := valid.Validate(170); err != nil {
		t.Errorf("T20 T20 Bull should be a valid 170: %v", err)
	}
}

func TestParseCheckoutRoute(t *testing.T) {
	route, err := ParseCheckoutRoute("T20 T20 Bull")
	if err != nil {
		t.Fatal(err)
	}
	if route.Total() != 170 {
		t.Errorf("total = %d, want 170", route.Total())
	}
	if route.DartCount() != 3 {
		t.Errorf("darts = %d, want 3", route.DartCount())
	}
	if route.String() != "T20 T20 Bull" {
		t.Errorf("rendered as %q", route.String())
	}
	if finish, ok := route.Finish(); !ok || finish != darts.Bull() {
		t.Errorf("finish = %v, want Bull", finish)
	}
	if len(route.Setup()) != 2 {
		t.Errorf("setup = %v, want two darts", route.Setup())
	}

	if _, err := ParseCheckoutRoute(""); !errors.Is(err, ErrRouteEmpty) {
		t.Errorf("empty route: got %v", err)
	}
	if _, err := ParseCheckoutRoute("T20 nonsense"); !errors.Is(err, darts.ErrBadSegment) {
		t.Errorf("bad segment: got %v", err)
	}
}

func TestEmptyRouteAccessors(t *testing.T) {
	var r CheckoutRoute
	if _, ok := r.Finish(); ok {
		t.Error("an empty route has no finish")
	}
	if r.Setup() != nil {
		t.Error("an empty route has no setup darts")
	}
	if r.Total() != 0 || r.DartCount() != 0 || r.String() != "" {
		t.Errorf("empty route = %q total %d", r.String(), r.Total())
	}
}
