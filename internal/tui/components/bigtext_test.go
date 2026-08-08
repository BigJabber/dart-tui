package components

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// blockSizes are every size that actually draws something.
var blockSizes = []BigSize{BigShort, BigTall, BigHuge, BigGiant}

func sizeName(size BigSize) string {
	switch size {
	case BigShort:
		return "short"
	case BigTall:
		return "tall"
	case BigHuge:
		return "huge"
	case BigGiant:
		return "giant"
	default:
		return "none"
	}
}

// biggestFirst is what everything steps down, so it must hold every drawable
// size exactly once and be in order.
func TestBiggestFirstIsCompleteAndOrdered(t *testing.T) {
	if len(biggestFirst) != len(blockSizes) {
		t.Fatalf("biggestFirst has %d sizes, the tests know about %d",
			len(biggestFirst), len(blockSizes))
	}
	for i, size := range biggestFirst {
		if i > 0 && BigRows(size) >= BigRows(biggestFirst[i-1]) {
			t.Errorf("%s is not smaller than %s", sizeName(size), sizeName(biggestFirst[i-1]))
		}
	}
}

// A rendered number is a rectangle: the row count and every row's printable
// width have to agree with what BigRows and BigWidth promised, because the
// panel budgets its layout from those two numbers before it draws anything.
func TestBigIsARectangleOfThePromisedSize(t *testing.T) {
	for _, size := range blockSizes {
		for _, s := range []string{"0", "61", "170", "501", "12.5", "-1"} {
			t.Run(sizeName(size)+"/"+s, func(t *testing.T) {
				rows := Big(s, size)
				if len(rows) != BigRows(size) {
					t.Fatalf("got %d rows, want %d", len(rows), BigRows(size))
				}
				want := BigWidth(s, size)
				for i, row := range rows {
					// lipgloss.Width, not len: the half blocks are multi-byte
					// but a single column, and that is the thing being asserted.
					if got := lipgloss.Width(row); got != want {
						t.Errorf("row %d is %d columns, want %d", i, got, want)
					}
				}
			})
		}
	}
}

// The font has to fail softly. A rune it does not know costs a blank cell of
// the usual width, so a caller that measures before it draws is never lied to.
func TestBigHandlesRunesTheFontDoesNotHave(t *testing.T) {
	for _, size := range blockSizes {
		rows := Big("1x1", size)
		if len(rows) != BigRows(size) {
			t.Fatalf("%s: got %d rows, want %d", sizeName(size), len(rows), BigRows(size))
		}
		if got, want := lipgloss.Width(rows[0]), BigWidth("1x1", size); got != want {
			t.Errorf("%s: %d columns, want %d", sizeName(size), got, want)
		}
		// The middle cell is the unknown one, and it must be empty.
		if strings.Count(rows[0], "█")+strings.Count(rows[0], "▀")+
			strings.Count(rows[0], "▄") == 0 {
			t.Errorf("%s: nothing was drawn at all", sizeName(size))
		}
	}
}

func TestBigOfNothingDrawsNothing(t *testing.T) {
	for _, size := range append(blockSizes, BigNone) {
		if rows := Big("", size); rows != nil {
			t.Errorf("%s: empty string drew %d rows", sizeName(size), len(rows))
		}
	}
	if rows := Big("501", BigNone); rows != nil {
		t.Errorf("BigNone drew %d rows", len(rows))
	}
	if got := BigWidth("501", BigNone); got != 0 {
		t.Errorf("BigNone width = %d, want 0", got)
	}
}

// Every character must be distinguishable from the others it can appear beside,
// at every size. This is cheap, and it catches the likeliest bug in the file: a
// copy-and-paste in the glyph table.
//
// Digits and letters are checked separately because the two never share a field
// — the number is always numeric and the name is always letters — and 0 and O
// are deliberately the same shape. Giving the zero a slash to tell them apart
// would only make the score look odd for a distinction nothing depends on.
func TestEveryCharacterLooksDifferent(t *testing.T) {
	var digits, letters []string
	for d := 0; d <= 9; d++ {
		digits = append(digits, strconv.Itoa(d))
	}
	for c := 'A'; c <= 'Z'; c++ {
		letters = append(letters, string(c))
	}

	for _, set := range [][]string{digits, letters} {
		for _, size := range blockSizes {
			seen := make(map[string]string, len(set))
			for _, c := range set {
				art := strings.Join(Big(c, size), "\n")
				if other, clash := seen[art]; clash {
					t.Errorf("%s: %s and %s draw identically", sizeName(size), other, c)
				}
				seen[art] = c
			}
		}
	}
}

// Names arrive in whatever case they were typed and come out in capitals.
func TestBigDrawsLowerCaseAsCapitals(t *testing.T) {
	for _, size := range blockSizes {
		lower := strings.Join(Big("alice", size), "\n")
		upper := strings.Join(Big("ALICE", size), "\n")
		if lower != upper {
			t.Errorf("%s: %q and %q draw differently", sizeName(size), "alice", "ALICE")
		}
	}
}

func TestCanRender(t *testing.T) {
	cases := map[string]bool{
		"":         false, // nothing to draw
		"170":      true,
		"Alice":    true,
		"ANN LEE":  true,
		"O'Rourke": true,
		"12.5":     true,
		"Zoë":      false, // no glyph for the accent
		"Ali©e":    false,
	}
	for in, want := range cases {
		if got := CanRender(in); got != want {
			t.Errorf("CanRender(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestBigFitPicksTheLargestThatFits(t *testing.T) {
	const s = "170"
	cases := []struct {
		name       string
		rows, cols int
		want       BigSize
	}{
		{"a wall display", 40, 200, BigGiant},
		{"exactly giant", BigRows(BigGiant), BigWidth(s, BigGiant), BigGiant},
		{"one row short of giant", BigRows(BigGiant) - 1, 200, BigHuge},
		{"one column short of giant", 40, BigWidth(s, BigGiant) - 1, BigHuge},
		// The case nobody thinks of: plenty of height, no width.
		{"tall but narrow", 40, BigWidth(s, BigShort), BigShort},
		{"too narrow for anything", 40, BigWidth(s, BigShort) - 1, BigNone},
		{"too short for anything", BigRows(BigShort) - 1, 200, BigNone},
		{"nothing at all", 0, 0, BigNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := BigFit(s, tc.rows, tc.cols); got != tc.want {
				t.Errorf("BigFit(%q, %d, %d) = %s, want %s",
					s, tc.rows, tc.cols, sizeName(got), sizeName(tc.want))
			}
		})
	}
}

func TestBigFitOfNothingIsNone(t *testing.T) {
	if got := BigFit("", 100, 100); got != BigNone {
		t.Errorf("BigFit(\"\") = %s, want none", sizeName(got))
	}
}

func TestLetterSpace(t *testing.T) {
	cases := map[string]string{
		"":      "",
		"a":     "A",
		"Alice": "A L I C E",
		"bob":   "B O B",
		// Names are not all ASCII, and spacing has to work on runes.
		"Zoë":     "Z O Ë",
		"Ann Lee": "A N N   L E E",
	}
	for in, want := range cases {
		if got := LetterSpace(in); got != want {
			t.Errorf("LetterSpace(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLineCount(t *testing.T) {
	cases := map[string]int{
		"":       0,
		"a":      1,
		"a\nb":   2,
		"a\nb\n": 3, // a trailing newline is a blank last row, and it takes space
	}
	for in, want := range cases {
		if got := LineCount(in); got != want {
			t.Errorf("LineCount(%q) = %d, want %d", in, got, want)
		}
	}
}
