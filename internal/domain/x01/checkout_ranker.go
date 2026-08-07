package x01

import (
	"sort"

	"github.com/BigJabber/dart-tui/internal/domain/darts"
)

// MaxCheckoutRecommendations is the most routes worth showing at the board.
const MaxCheckoutRecommendations = 3

// CheckoutAdvisor turns the solver's legal routes into a short, ordered list of
// recommendations.
//
// The solver decides what is possible; the advisor decides what is worth
// suggesting. Keeping them apart means the legality rules can never be bent by a
// preference, and preferences can be changed freely without risking correctness.
//
// The advisor holds its preferences as data rather than logic, so a future
// version could give a player their own favourite doubles by building an advisor
// with a different table — no change to the solver, and no restructuring here.
type CheckoutAdvisor struct {
	// preferred maps a score to the conventional routes for it, best first.
	preferred map[int][]CheckoutRoute
	// finishRank orders the finishing segments, lowest being most desirable.
	finishRank map[darts.Segment]int
}

// preferredFinishes lists the finishing segments from most to least desirable.
//
// The halving doubles come first — D20, D16, D8, D4, D2, D1 — because missing
// into the single leaves another double from the same family. The remaining even
// doubles follow, then the odd ones, which leave an awkward odd number when
// missed. The bull is last: it finishes a leg but is a small target, and the
// scores where it is genuinely the right route are curated explicitly.
var preferredFinishes = []darts.Segment{
	darts.D(20), darts.D(16), darts.D(8), darts.D(4), darts.D(2), darts.D(1),
	darts.D(10), darts.D(12), darts.D(18), darts.D(6), darts.D(14),
	darts.D(5), darts.D(3), darts.D(7), darts.D(9), darts.D(11),
	darts.D(13), darts.D(15), darts.D(17), darts.D(19),
	darts.Bull(),
}

// NewCheckoutAdvisor builds an advisor from the curated routes.
func NewCheckoutAdvisor() *CheckoutAdvisor {
	rank := make(map[darts.Segment]int, len(preferredFinishes))
	for i, seg := range preferredFinishes {
		rank[seg] = i
	}
	return &CheckoutAdvisor{preferred: curatedRoutes, finishRank: rank}
}

// defaultAdvisor is the advisor the application uses.
var defaultAdvisor = NewCheckoutAdvisor()

// DefaultCheckoutAdvisor returns the shared advisor.
func DefaultCheckoutAdvisor() *CheckoutAdvisor { return defaultAdvisor }

// Recommend returns up to max routes for finishing score with the darts
// available, best first. It returns nothing when the score cannot be finished.
//
// Where a score has curated routes those are the whole answer, in chart order.
// They are the finishes players recognise, and a score with one sensible finish
// should show one suggestion rather than have the list padded out with worse
// ways of getting there. The solver still has the final say on legality: a
// curated route is only offered if it really is a legal finish with these darts.
//
// A curated route may use more darts than strictly necessary, and that is
// deliberate. 101 goes in two darts as T17 Bull, but the conventional route is
// T17 10 D20 because leaving D20 beats throwing at the bull. When the player has
// only two darts left, that three-dart route drops out and the solver's answer
// is used instead.
func (a *CheckoutAdvisor) Recommend(score, dartsAvailable, max int) []CheckoutRoute {
	if max <= 0 {
		return nil
	}
	if !CanCheckout(score, dartsAvailable) {
		return nil
	}

	var (
		out  []CheckoutRoute
		seen = make(map[string]bool, max)
	)
	take := func(r CheckoutRoute) bool {
		if seen[r.Key()] {
			return false
		}
		seen[r.Key()] = true
		out = append(out, r)
		return len(out) >= max
	}

	// Each curated route is checked against the same rules the solver applies,
	// so the table can never introduce an illegal suggestion. Validating the
	// three routes in hand is far cheaper than enumerating every legal finish,
	// which matters because this runs on every redraw.
	for _, r := range a.preferred[score] {
		if r.DartCount() > dartsAvailable || r.Validate(score) != nil {
			continue
		}
		if take(r) {
			return out
		}
	}
	if len(out) > 0 {
		return out
	}

	// No curated route applies, so fall back to the solver. Working up from one
	// dart stops at the fewest that can finish the score: if it goes in two there
	// is no reason to suggest a three-dart way of doing it, and no reason to
	// enumerate those either.
	var rest []CheckoutRoute
	for n := 1; n <= dartsAvailable; n++ {
		if rest = FindCheckouts(score, n); len(rest) > 0 {
			break
		}
	}
	sort.Slice(rest, func(i, j int) bool {
		ri, rj := a.score(rest[i]), a.score(rest[j])
		if ri != rj {
			return ri < rj
		}
		// A stable tie-break keeps the suggestions identical from one render to
		// the next, which matters when they are being read off a screen.
		return rest[i].String() < rest[j].String()
	})
	for _, r := range rest {
		if take(r) {
			break
		}
	}
	return out
}

// score rates a route, lower being better. It is deliberately a small, readable
// set of weights rather than a model of professional darts strategy.
func (a *CheckoutAdvisor) score(r CheckoutRoute) int {
	finish, ok := r.Finish()
	if !ok {
		return 1 << 30
	}

	// The finishing double dominates everything else: which double a route
	// leaves matters far more than how it gets there.
	rank, known := a.finishRank[finish]
	if !known {
		rank = len(preferredFinishes)
	}
	total := rank * 1000

	for _, d := range r.Setup() {
		total += setupPenalty(d)
	}

	// A route whose first dart is a treble is worth a little more if missing
	// into the single still leaves a finish: the visit is not wasted.
	if setup := r.Setup(); len(setup) > 0 && setup[0].Kind == darts.Treble {
		if CanCheckout(r.Total()-setup[0].Number, len(r.Darts)-1) {
			total -= 5
		}
	}
	return total
}

// setupPenalty rates a non-finishing dart, lower being better.
//
// Trebles are what players aim at to set up a finish, and the high trebles are
// where they are already aiming. Singles are ordinary. Doubles and the bull are
// unusual choices for a setup dart and are marked down accordingly.
func setupPenalty(s darts.Segment) int {
	switch s.Kind {
	case darts.Treble:
		return darts.HighestNumber - s.Number
	case darts.Single:
		return 25 + (darts.HighestNumber - s.Number)
	case darts.Double:
		return 45 + (darts.HighestNumber - s.Number)
	case darts.OuterBull:
		return 60
	case darts.InnerBull:
		return 70
	default:
		return 100
	}
}

// RecommendCheckouts is the shared advisor's Recommend, for callers that do not
// need their own preferences.
func RecommendCheckouts(score, dartsAvailable, max int) []CheckoutRoute {
	return defaultAdvisor.Recommend(score, dartsAvailable, max)
}
