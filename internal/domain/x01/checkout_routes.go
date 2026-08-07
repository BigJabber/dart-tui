package x01

import "fmt"

// Curated checkout routes: the finishes players actually use.
//
// The solver knows every legal way to finish a score, but most of them are
// combinations nobody would throw. This table records the conventional routes,
// following the published checkout charts (Darts501's chart is the usual
// reference), so the first suggestion is one a player would recognise.
//
// Three things matter about how this is stored:
//
//   - It is data in source, not a download. The application never contacts a
//     website, and works entirely offline.
//   - It is NOT the source of truth for legality. Every entry is checked by the
//     solver, and a typo here fails a test rather than reaching a player. An
//     entry that somehow failed to parse is dropped rather than crashing the
//     application, so a bad edit can never show an illegal route at the board.
//   - Scores that can be finished with a single dart are deliberately absent.
//     Their route is forced — the double itself, or the bull for 50 — so there
//     is nothing to prefer.
//
// Where a score has more than one genuinely popular finish, the alternatives
// follow the preferred route in order. Routes are written the way they would be
// called out.
var curatedCheckouts = map[int][]string{
	// Three-dart finishes above 110.
	170: {"T20 T20 Bull"},
	167: {"T20 T19 Bull"},
	164: {"T20 T18 Bull"},
	161: {"T20 T17 Bull"},
	160: {"T20 T20 D20"},
	158: {"T20 T20 D19"},
	157: {"T20 T19 D20"},
	156: {"T20 T20 D18"},
	155: {"T20 T19 D19"},
	154: {"T20 T18 D20"},
	153: {"T20 T19 D18"},
	152: {"T20 T20 D16"},
	151: {"T20 T17 D20"},
	150: {"T20 T18 D18"},
	149: {"T20 T19 D16"},
	148: {"T20 T16 D20"},
	147: {"T20 T17 D18"},
	146: {"T20 T18 D16"},
	145: {"T20 T15 D20"},
	144: {"T20 T20 D12"},
	143: {"T20 T17 D16"},
	142: {"T20 T14 D20"},
	141: {"T20 T19 D12"},
	140: {"T20 T20 D10"},
	139: {"T20 T13 D20", "T19 T14 D20"},
	138: {"T20 T18 D12"},
	137: {"T20 T19 D10"},
	136: {"T20 T20 D8"},
	135: {"T20 T17 D12"},
	134: {"T20 T14 D16"},
	133: {"T20 T19 D8"},
	132: {"T20 T16 D12"},
	131: {"T20 T13 D16"},
	130: {"T20 T18 D8"},
	129: {"T19 T16 D12"},
	128: {"T18 T14 D16"},
	127: {"T20 T17 D8"},
	126: {"T19 T19 D6"},
	125: {"T20 T19 D4", "T20 25 D20"},
	124: {"T20 T16 D8"},
	123: {"T19 T16 D9"},
	122: {"T20 T18 D4"},
	121: {"T17 T10 D20"},
	120: {"T20 20 D20"},
	119: {"T19 T10 D16"},
	118: {"T20 18 D20"},
	117: {"T20 17 D20"},
	116: {"T20 16 D20"},
	115: {"T20 15 D20"},
	114: {"T20 14 D20"},
	113: {"T20 13 D20"},
	112: {"T20 12 D20"},
	111: {"T20 11 D20"},

	// 110 is a two-dart finish, and one of the few where the bull is the route
	// rather than something to avoid.
	110: {"T20 Bull"},

	// Three-dart finishes below 110.
	109: {"T20 9 D20"},
	108: {"T20 16 D16"},
	107: {"T19 10 D20"},
	106: {"T20 10 D18"},
	105: {"T20 13 D16"},
	104: {"T18 18 D16"},
	103: {"T19 10 D18"},
	102: {"T20 10 D16"},
	101: {"T17 10 D20", "T20 9 D16"},
	99:  {"T19 10 D16"},

	// The two-dart region: a scoring dart to leave a double.
	100: {"T20 D20"},
	98:  {"T20 D19"},
	97:  {"T19 D20"},
	96:  {"T20 D18"},
	95:  {"T19 D19"},
	94:  {"T18 D20"},
	93:  {"T19 D18"},
	92:  {"T20 D16"},
	91:  {"T17 D20"},
	90:  {"T20 D15", "T18 D18"},
	89:  {"T19 D16"},
	88:  {"T20 D14", "T16 D20"},
	87:  {"T17 D18"},
	86:  {"T18 D16"},
	85:  {"T15 D20"},
	84:  {"T20 D12"},
	83:  {"T17 D16"},
	82:  {"T14 D20", "Bull D16"},
	81:  {"T19 D12", "T15 D18"},
	80:  {"T20 D10", "T16 D16", "D20 D20"},
	79:  {"T19 D11", "T13 D20"},
	78:  {"T18 D12"},
	77:  {"T19 D10"},
	76:  {"T20 D8"},
	75:  {"T17 D12"},
	74:  {"T14 D16"},
	73:  {"T19 D8"},
	72:  {"T16 D12"},
	71:  {"T13 D16"},
	70:  {"T18 D8", "T10 D20"},
	69:  {"T19 D6"},
	68:  {"T20 D4"},
	67:  {"T17 D8"},
	66:  {"T10 D18", "T16 D9"},
	65:  {"T19 D4", "25 D20"},
	64:  {"T16 D8"},
	63:  {"T13 D12", "T17 D6"},
	62:  {"T10 D16"},
	61:  {"T15 D8", "25 D18"},
	60:  {"20 D20"},
	59:  {"19 D20"},
	58:  {"18 D20"},
	57:  {"17 D20"},
	56:  {"16 D20"},
	55:  {"15 D20"},
	54:  {"14 D20"},
	53:  {"13 D20"},
	52:  {"12 D20"},
	51:  {"11 D20"},
	49:  {"9 D20", "17 D16"},
	48:  {"16 D16", "8 D20"},
	47:  {"15 D16", "7 D20"},
	46:  {"6 D20", "14 D16"},
	45:  {"13 D16", "5 D20"},
	44:  {"12 D16", "4 D20"},
	43:  {"11 D16", "3 D20"},
	42:  {"10 D16", "6 D18"},
	41:  {"9 D16", "1 D20"},

	// The odd scores under 40 still need two darts: a single to leave a double.
	39: {"7 D16", "19 D10"},
	37: {"5 D16", "1 D18"},
	35: {"3 D16", "19 D8"},
	33: {"1 D16", "17 D8"},
	31: {"15 D8", "7 D12"},
	29: {"13 D8", "5 D12"},
	27: {"11 D8", "3 D12"},
	25: {"9 D8", "17 D4"},
	23: {"7 D8", "3 D10"},
	21: {"5 D8", "1 D10"},
	19: {"3 D8", "11 D4"},
	17: {"1 D8", "9 D4"},
	15: {"7 D4", "3 D6"},
	13: {"5 D4", "1 D6"},
	11: {"3 D4", "7 D2"},
	9:  {"1 D4", "5 D2"},
	7:  {"3 D2", "1 D3"},
	5:  {"1 D2", "3 D1"},
	3:  {"1 D1"},
}

// curatedRoutes is the parsed and validated form of the table above.
var curatedRoutes, curatedLoadErrors = loadCuratedCheckouts()

// loadCuratedCheckouts parses the curated table and checks every route against
// the solver's rules, dropping anything that does not survive.
//
// Dropping rather than panicking keeps a bad edit out of a player's face: the
// worst case is that a score falls back to the ranked solver output. The
// accompanying test asserts that nothing was dropped, so the mistake still gets
// caught before it ships.
func loadCuratedCheckouts() (map[int][]CheckoutRoute, []error) {
	routes := make(map[int][]CheckoutRoute, len(curatedCheckouts))
	var errs []error

	for score, written := range curatedCheckouts {
		for _, text := range written {
			route, err := ParseCheckoutRoute(text)
			if err != nil {
				errs = append(errs, fmt.Errorf("checkout %d: %w", score, err))
				continue
			}
			if err := route.Validate(score); err != nil {
				errs = append(errs, fmt.Errorf("checkout %d: %w", score, err))
				continue
			}
			routes[score] = append(routes[score], route)
		}
	}
	return routes, errs
}

// CuratedCheckoutErrors reports any curated route that failed validation. It is
// empty in a correct build; the test suite asserts as much.
func CuratedCheckoutErrors() []error {
	return append([]error(nil), curatedLoadErrors...)
}

// CuratedCheckoutScores returns every score with a curated preference.
func CuratedCheckoutScores() []int {
	out := make([]int, 0, len(curatedRoutes))
	for score := range curatedRoutes {
		out = append(out, score)
	}
	return out
}
