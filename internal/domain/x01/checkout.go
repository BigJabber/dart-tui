package x01

import (
	"fmt"
	"sort"
	"strings"

	"github.com/BigJabber/dart-tui/internal/domain/darts"
)

// A checkout is the sequence of darts that finishes a leg.
//
// Checkouts live in the X01 package rather than alongside the board because
// finishing on a double is an X01 rule, not a fact about dartboards. The board
// itself — which segments exist, what they score — comes from the darts package,
// so Cricket and X01 share one description of the board and neither owns the
// other's rules.

// CheckoutRoute is a way of finishing a leg: one to three darts whose values sum
// to the remaining score, ending on a double or the bull.
type CheckoutRoute struct {
	Darts []darts.Segment
}

// NewCheckoutRoute builds a route from segments.
func NewCheckoutRoute(segments ...darts.Segment) CheckoutRoute {
	return CheckoutRoute{Darts: append([]darts.Segment(nil), segments...)}
}

// Total is the score the route checks out.
func (r CheckoutRoute) Total() int {
	total := 0
	for _, d := range r.Darts {
		total += d.Value()
	}
	return total
}

// DartCount is how many darts the route needs.
func (r CheckoutRoute) DartCount() int { return len(r.Darts) }

// Finish is the last dart, the one that must be a double or the bull.
func (r CheckoutRoute) Finish() (darts.Segment, bool) {
	if len(r.Darts) == 0 {
		return darts.Segment{}, false
	}
	return r.Darts[len(r.Darts)-1], true
}

// Setup returns every dart before the finish.
func (r CheckoutRoute) Setup() []darts.Segment {
	if len(r.Darts) == 0 {
		return nil
	}
	return r.Darts[:len(r.Darts)-1]
}

// String renders the route the way it would be called out: "T20 T20 Bull".
func (r CheckoutRoute) String() string {
	parts := make([]string, 0, len(r.Darts))
	for _, d := range r.Darts {
		parts = append(parts, d.String())
	}
	return strings.Join(parts, " ")
}

// Key is a canonical identity for the route, used to spot duplicates.
//
// The finishing dart is kept in place but the setup darts are sorted, so
// "T20 T19 Bull" and "T19 T20 Bull" are recognised as the same route. Throwing
// order between setup darts is not a meaningful alternative.
func (r CheckoutRoute) Key() string {
	setup := make([]string, 0, len(r.Darts))
	for _, d := range r.Setup() {
		setup = append(setup, d.String())
	}
	sort.Strings(setup)
	finish := ""
	if f, ok := r.Finish(); ok {
		finish = f.String()
	}
	return strings.Join(setup, "+") + ">" + finish
}

// Errors describing why a route is not a legal checkout.
var (
	// ErrRouteEmpty means the route has no darts.
	ErrRouteEmpty = fmt.Errorf("a checkout needs at least one dart")
	// ErrRouteTooLong means the route needs more than a visit's worth of darts.
	ErrRouteTooLong = fmt.Errorf("a checkout is at most %d darts", darts.MaxDartsPerVisit)
	// ErrRouteBadSegment means the route uses a segment that is not on a board.
	ErrRouteBadSegment = fmt.Errorf("that segment is not on a dartboard")
	// ErrRouteNotOnDouble means the route does not finish on a double or bull.
	ErrRouteNotOnDouble = fmt.Errorf("a checkout must finish on a double or the bull")
	// ErrRouteWrongTotal means the route does not add up to the score.
	ErrRouteWrongTotal = fmt.Errorf("the route does not add up to the score")
	// ErrRouteOvershoots means an intermediate dart would take the score past zero.
	ErrRouteOvershoots = fmt.Errorf("the route passes zero before the final dart")
)

// Validate checks a route really is a legal way to check out the given score.
//
// This is the authority that curated routes are held to: a typo in the hand
// written checkout table fails here rather than reaching a player at the board.
func (r CheckoutRoute) Validate(score int) error {
	if len(r.Darts) == 0 {
		return ErrRouteEmpty
	}
	if len(r.Darts) > darts.MaxDartsPerVisit {
		return fmt.Errorf("%w: %s uses %d", ErrRouteTooLong, r, len(r.Darts))
	}

	remaining := score
	for i, d := range r.Darts {
		if !d.Valid() {
			return fmt.Errorf("%w: %q", ErrRouteBadSegment, d)
		}
		remaining -= d.Value()
		// Only the final dart may bring the score to zero, and nothing may take
		// it below.
		if remaining < 0 {
			return fmt.Errorf("%w: %s", ErrRouteOvershoots, r)
		}
		if remaining == 0 && i != len(r.Darts)-1 {
			return fmt.Errorf("%w: %s", ErrRouteOvershoots, r)
		}
	}
	if remaining != 0 {
		return fmt.Errorf("%w: %s makes %d, not %d", ErrRouteWrongTotal, r, r.Total(), score)
	}

	finish, _ := r.Finish()
	if !finish.IsFinish() {
		return fmt.Errorf("%w: %s ends on %s", ErrRouteNotOnDouble, r, finish)
	}
	return nil
}

// ParseCheckoutRoute reads a route written the way a player would say it, such
// as "T20 T20 Bull".
func ParseCheckoutRoute(text string) (CheckoutRoute, error) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return CheckoutRoute{}, ErrRouteEmpty
	}
	route := CheckoutRoute{Darts: make([]darts.Segment, 0, len(fields))}
	for _, f := range fields {
		seg, err := darts.ParseSegment(f)
		if err != nil {
			return CheckoutRoute{}, fmt.Errorf("%q: %w", text, err)
		}
		route.Darts = append(route.Darts, seg)
	}
	return route, nil
}
