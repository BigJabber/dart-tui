package components

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// ansiPattern strips styling so assertions can be made on the text itself, the
// same way the screen tests do it.
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func testSpotlight(rows int) Spotlight {
	return Spotlight{
		Banner:  "NOW THROWING",
		Name:    "Alice",
		Colour:  "#5FD7FF",
		Number:  "170",
		Caption: "LEFT",
		Sub:     "CHECKOUT  T20  T20  Bull",
		Rows:    rows,
	}
}

func plain(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

// The panel must never be taller than it was told it could be, because a view
// that overflows loses its top — the header and the scoreboard — rather than
// its bottom.
func TestSpotlightNeverExceedsTheRowsItWasGiven(t *testing.T) {
	for rows := 0; rows <= 30; rows++ {
		out := testSpotlight(rows).View(120)
		if got := LineCount(out); got > rows {
			t.Errorf("given %d rows the panel drew %d", rows, got)
		}
	}
}

// The sizes step down in order as the space runs out, and the panel disappears
// rather than being clipped.
func TestSpotlightStepsDownThroughTheSizes(t *testing.T) {
	cases := []struct {
		rows int
		want int
	}{
		{30, 24}, // capped at the largest size: a 14-row number under a 4-row name
		{24, 24},
		{23, 14}, // no room for both, so the number drops and the name is text
		{14, 14},
		{13, 11},
		{11, 11},
		{10, 4}, // the compact two-liner
		{4, 4},
		{3, 0}, // nothing at all
		{0, 0},
	}
	for _, tc := range cases {
		if got := LineCount(testSpotlight(tc.rows).View(120)); got != tc.want {
			t.Errorf("with %d rows available the panel drew %d rows, want %d",
				tc.rows, got, tc.want)
		}
	}
}

// At a block size the number exists only as glyphs, so the plain form has to
// live somewhere else on the screen. Callers depend on this.
func TestSpotlightDrawsTheNumberInBlocksWhenItFits(t *testing.T) {
	out := plain(testSpotlight(24).View(120))
	if strings.Contains(out, "170") {
		t.Error("the number appears as ordinary text at a block size")
	}
	for _, want := range []string{"NOW THROWING", "LEFT", "CHECKOUT  T20  T20  Bull"} {
		if !strings.Contains(out, want) {
			t.Errorf("panel does not contain %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "█") {
		t.Error("no blocks were drawn")
	}
}

// With the rows for it the name is drawn in blocks too, and at the size
// NameSizes asks for.
func TestSpotlightDrawsTheNameInBlocksAtTheLargestSize(t *testing.T) {
	full := testSpotlight(24).View(120)
	if strings.Contains(plain(full), "A L I C E") {
		t.Error("the name is still ordinary text where there was room for blocks")
	}
	// The name occupies exactly what NameSizes asked for, so the number keeps
	// the rest.
	wantRows := spotlightFixedRows + BigRows(BigHuge) + BigRows(NameSizes[BigHuge])
	if got := LineCount(full); got != wantRows {
		t.Errorf("panel drew %d rows, want %d", got, wantRows)
	}

	// One size down there is no room, so it goes back to ordinary text.
	if !strings.Contains(plain(testSpotlight(14).View(120)), "A L I C E") {
		t.Error("the name is not ordinary text at the smaller size")
	}
}

// A name the font cannot draw falls back to ordinary text rather than coming
// out pocked with blank cells.
func TestSpotlightFallsBackForANameTheFontCannotDraw(t *testing.T) {
	s := testSpotlight(24)
	s.Name = "Zoë"
	out := plain(s.View(120))
	if !strings.Contains(out, "Z O Ë") {
		t.Errorf("panel does not fall back to text for an accented name:\n%s", out)
	}
}

// A name too wide for blocks at this width falls back the same way, rather than
// bursting the panel.
func TestSpotlightFallsBackForANameTooWideForBlocks(t *testing.T) {
	s := testSpotlight(24)
	s.Name = "Bartholomew Fotheringay"
	out := s.View(120)
	if !strings.Contains(plain(out), "B A R T") {
		t.Errorf("panel does not fall back to text for a long name:\n%s", plain(out))
	}
	if got := lipgloss.Width(out); got > 120 {
		t.Errorf("panel is %d columns wide", got)
	}
}

// The compact form loses the size but never the meaning.
func TestSpotlightCompactFormKeepsEverythingAsText(t *testing.T) {
	out := plain(testSpotlight(4).View(120))
	for _, want := range []string{"NOW THROWING", "A L I C E", "170", "LEFT", "CHECKOUT"} {
		if !strings.Contains(out, want) {
			t.Errorf("compact panel does not contain %q:\n%s", want, out)
		}
	}
}

// A narrow terminal is as much of a constraint as a short one.
func TestSpotlightShrinksWithTheWidth(t *testing.T) {
	for _, width := range []int{200, 120, 100, 80, 60, 52, 30, 24, 20, 10} {
		out := testSpotlight(30).View(width)
		if got := lipgloss.Width(out); got > width {
			t.Errorf("at width %d the panel drew %d columns:\n%s", width, got, out)
		}
	}
	if out := testSpotlight(30).View(20); out != "" {
		t.Errorf("the panel drew something at width 20:\n%s", out)
	}
}

// A long name is cut to fit rather than bursting the border. Truncate leaves
// styled text alone, so this only works if the name is cut before it is spaced
// and coloured.
func TestSpotlightTruncatesALongName(t *testing.T) {
	s := testSpotlight(21)
	s.Name = strings.Repeat("Bartholomew", 8)
	out := s.View(120)
	if got := lipgloss.Width(out); got > 120 {
		t.Errorf("a long name widened the panel to %d columns:\n%s", got, out)
	}
	for _, line := range strings.Split(plain(out), "\n") {
		if lipgloss.Width(line) > 120 {
			t.Errorf("line is %d columns:\n%s", lipgloss.Width(line), line)
		}
	}
}

// A caller may leave the supporting line and the caption out entirely.
func TestSpotlightWithoutACaptionOrSubLine(t *testing.T) {
	s := Spotlight{Banner: "▶ BOWLING", Name: "Bob", Number: "40", Rows: 24}
	out := plain(s.View(120))
	if !strings.Contains(out, "▶ BOWLING") {
		t.Errorf("panel is missing its banner:\n%s", out)
	}
	if got, want := LineCount(s.View(120)), 24; got != want {
		t.Errorf("panel drew %d rows, want %d", got, want)
	}
}
