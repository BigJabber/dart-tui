package x01

import (
	"sort"

	"github.com/BigJabber/dart-tui/internal/domain/darts"
)

// MaxCheckout is the highest score that can be finished in one visit:
// T20, T20, Bull.
const MaxCheckout = 170

// FindCheckouts returns every legal way to finish score using at most
// dartsAvailable darts.
//
// This is the authority on what is possible. It enumerates real board segments
// rather than consulting a table, so it is correct for any score including the
// ones no published chart bothers to list, and a mistake in the curated
// preferences cannot produce an illegal suggestion.
//
// The rules applied are exactly the ones a double-out leg imposes:
//
//   - the darts must total the score exactly;
//   - the last dart must be a double or the inner bull;
//   - no dart may take the score below zero, and only the last may reach it;
//   - every dart must be a segment that exists on a board.
//
// Routes needing fewer darts than are available are included: a player with
// three darts in hand is not obliged to use all three.
func FindCheckouts(score, dartsAvailable int) []CheckoutRoute {
	if dartsAvailable < 1 || score < 1 || score > MaxCheckout {
		return nil
	}
	if dartsAvailable > darts.MaxDartsPerVisit {
		dartsAvailable = darts.MaxDartsPerVisit
	}

	var routes []CheckoutRoute

	// One dart: the score itself must be a finishing segment.
	if finish, ok := darts.FinishByValue[score]; ok {
		routes = append(routes, NewCheckoutRoute(finish))
	}
	if dartsAvailable == 1 {
		return routes
	}

	// Two darts: one setup dart, then whatever is left must be a finish.
	for _, first := range darts.AllSegments {
		remaining := score - first.Value()
		if remaining <= 0 {
			continue
		}
		if finish, ok := darts.FinishByValue[remaining]; ok {
			routes = append(routes, NewCheckoutRoute(first, finish))
		}
	}
	if dartsAvailable == 2 {
		return sortRoutes(routes)
	}

	// Three darts: two setup darts, then the finish.
	//
	// The setup darts are enumerated over a value-ordered list, taking only pairs
	// from the current position onwards. That generates each unordered pair
	// exactly once and leaves the bigger scorer first, which is both the throwing
	// convention and the reason the reverse ordering is not offered as a separate
	// alternative.
	for i, first := range setupOrder {
		afterFirst := score - first.Value()
		if afterFirst <= 0 {
			continue
		}
		for _, second := range setupOrder[i:] {
			remaining := afterFirst - second.Value()
			if remaining <= 0 {
				continue
			}
			if finish, ok := darts.FinishByValue[remaining]; ok {
				routes = append(routes, NewCheckoutRoute(first, second, finish))
			}
		}
	}
	return sortRoutes(routes)
}

// setupOrder is every board segment sorted by value, highest first.
//
// The three-dart search relies on this ordering to enumerate each pair of setup
// darts exactly once. darts.AllSegments is grouped by ring rather than by value,
// so it cannot be used for that directly.
var setupOrder = func() []darts.Segment {
	out := append([]darts.Segment(nil), darts.AllSegments...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Value() != out[j].Value() {
			return out[i].Value() > out[j].Value()
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Number < out[j].Number
	})
	return out
}()

// sortRoutes puts routes in a stable, predictable order: fewest darts first,
// then by their written form. Callers that care about quality rank them
// afterwards; this only makes the solver's output deterministic.
func sortRoutes(routes []CheckoutRoute) []CheckoutRoute {
	sort.SliceStable(routes, func(i, j int) bool {
		if len(routes[i].Darts) != len(routes[j].Darts) {
			return len(routes[i].Darts) < len(routes[j].Darts)
		}
		return routes[i].String() < routes[j].String()
	})
	return routes
}

// minDartsToCheckout[score] is the fewest darts that can finish score, or zero
// when it cannot be finished in a single visit.
//
// It is computed once from the solver so that asking "can this be checked out?"
// during a game is a table lookup rather than a search.
var minDartsToCheckout = func() [MaxCheckout + 1]int {
	var table [MaxCheckout + 1]int
	for score := 1; score <= MaxCheckout; score++ {
		for n := 1; n <= darts.MaxDartsPerVisit; n++ {
			if len(FindCheckouts(score, n)) > 0 {
				table[score] = n
				break
			}
		}
	}
	return table
}()

// MinimumDarts is the fewest darts that can check out a score, or zero if it
// cannot be done in a single visit.
//
// The scores returning zero include the well-known bogey numbers — 169, 168,
// 166, 165, 163, 162 and 159 — which cannot be finished in three darts even
// though 170 can.
func MinimumDarts(score int) int {
	if score < 1 || score > MaxCheckout {
		return 0
	}
	return minDartsToCheckout[score]
}

// CanCheckout reports whether a score can be finished with the darts available.
func CanCheckout(score, dartsAvailable int) bool {
	needed := MinimumDarts(score)
	return needed > 0 && needed <= dartsAvailable
}

// IsBogeyNumber reports whether a score cannot be checked out with a full visit
// of three darts, despite being within range.
func IsBogeyNumber(score int) bool {
	return score >= 1 && score <= MaxCheckout && MinimumDarts(score) == 0
}
