package components

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// The panel only moves into a column of its own when that column is at least as
// wide as it would have had stacked. A column buys height but costs width, and
// the number is usually limited by width first.
func TestLayoutOnlyMovesToAColumnWhenItGainsWidth(t *testing.T) {
	cases := []struct {
		width int
		want  bool
	}{
		{240, true}, // a fullscreen 1080p terminal
		{182, true}, // the narrowest that gains anything
		{181, false},
		{160, false}, // wide, but a column here would cost the number a size
		{100, false},
		{52, false},
	}
	for _, tc := range cases {
		got := NewLayout(tc.width, 50)
		if got.SideBySide != tc.want {
			t.Errorf("at width %d SideBySide = %v, want %v", tc.width, got.SideBySide, tc.want)
		}
		if got.SideBySide && got.Panel < stackedPanelWidth {
			t.Errorf("at width %d the column is %d wide, narrower than stacking would give (%d)",
				tc.width, got.Panel, stackedPanelWidth)
		}
	}
}

// Neither arrangement may ask for more columns than the terminal has.
func TestLayoutFitsTheWidth(t *testing.T) {
	for width := 52; width <= 400; width++ {
		l := NewLayout(width, 50)
		if l.SideBySide {
			if total := l.Body + columnGutter + l.Panel; total != width {
				t.Errorf("at width %d the columns total %d", width, total)
			}
		} else if l.Body != width || l.Panel > width {
			t.Errorf("at width %d stacked gives body %d panel %d", width, l.Body, l.Panel)
		}
	}
}

// Join is what actually assembles the screen, so it has to agree with the
// arrangement Layout chose.
func TestLayoutJoin(t *testing.T) {
	const board, panel, prompt = "BOARD", "PANEL", "PROMPT"

	stacked := NewLayout(100, 50)
	if got, want := stacked.Join(board, panel, prompt), "BOARD\nPANEL\nPROMPT"; got != want {
		t.Errorf("stacked join = %q, want %q", got, want)
	}

	// An absent panel leaves no gap behind it, in either arrangement.
	if got, want := stacked.Join(board, "", prompt), "BOARD\nPROMPT"; got != want {
		t.Errorf("stacked join without a panel = %q, want %q", got, want)
	}
	side := NewLayout(240, 50)
	if got, want := side.Join(board, "", prompt), "BOARD\nPROMPT"; got != want {
		t.Errorf("side-by-side join without a panel = %q, want %q", got, want)
	}

	// Side by side the panel shares its rows with the other two rather than
	// following them.
	joined := side.Join(board, panel, prompt)
	if LineCount(joined) != 2 {
		t.Errorf("side-by-side join is %d rows, want 2:\n%s", LineCount(joined), joined)
	}
	if !strings.Contains(strings.Split(joined, "\n")[0], "PANEL") {
		t.Errorf("the panel is not on the first row:\n%s", joined)
	}
}

// The whole point of the column is that the panel can be bigger in it. If that
// ever stops being true the arrangement is not worth its complexity.
func TestLayoutColumnLetsThePanelGrow(t *testing.T) {
	const width, height = 240, 55

	side := NewLayout(width, height)
	stacked := NewLayout(stackedPanelWidth, height)

	spot := func(l Layout, rows int) string {
		s := testSpotlight(rows)
		return s.View(l.Panel)
	}
	// Stacked, the panel shares the height with a table and a prompt; in a
	// column it has all of it.
	inColumn := LineCount(spot(side, side.PanelRows))
	onTopOfEachOther := LineCount(spot(stacked, 24))
	if inColumn <= onTopOfEachOther {
		t.Errorf("the panel is %d rows in a column and %d stacked — no gain",
			inColumn, onTopOfEachOther)
	}
}

// centreBlock has to shift every line of a block by the same amount. Centring
// the lines individually would shear a number: a digit's stroke would move
// between its top row and its bottom.
func TestCentreBlockMovesEveryLineTogether(t *testing.T) {
	const block = "###\n#\n#####"
	got := centreBlock(block, 21)

	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}
	// The widest line is 5, so the block sits 8 columns in, all of it.
	for i, line := range lines {
		lead := lipgloss.Width(line) - lipgloss.Width(strings.TrimLeft(line, " "))
		if lead != 8 {
			t.Errorf("line %d starts at column %d, want 8: %q", i, lead, line)
		}
	}

	// A block that already fills the width is left alone, and so is nothing.
	if got := centreBlock(block, 5); got != block {
		t.Errorf("a block that fills the width was moved: %q", got)
	}
	if got := centreBlock("", 40); got != "" {
		t.Errorf("centreBlock(\"\") = %q", got)
	}
}

// The same thing, checked through the real panel: every row of the number keeps
// the offset it had, so the strokes line up down the whole digit.
func TestSpotlightCentringKeepsTheGlyphRowsAligned(t *testing.T) {
	const width, rows = 160, 40
	body := panelBody(t, testSpotlight(rows).View(width))

	// Whatever size the panel settled on, its rows must all land together.
	size := BigFit("170", rows, width)
	want := Big("170", size)
	if len(want) == 0 {
		t.Fatal("no size was chosen")
	}

	offset, found := -1, 0
	for _, line := range body {
		trimmed := strings.TrimRight(line, " ")
		for _, raw := range want {
			rawTrimmed := strings.TrimRight(raw, " ")
			if rawTrimmed == "" || !strings.HasSuffix(trimmed, rawTrimmed) {
				continue
			}
			at := lipgloss.Width(trimmed) - lipgloss.Width(rawTrimmed)
			if offset >= 0 && at != offset {
				t.Errorf("a glyph row starts at column %d, the others at %d", at, offset)
			}
			offset, found = at, found+1
			break
		}
	}
	// Rows the caption sits beside do not end with the raw glyph row, so not
	// every row will match; most must.
	if found < len(want)-2 {
		t.Fatalf("matched %d of the %d glyph rows at size %s",
			found, len(want), sizeName(size))
	}
}

// panelBody strips the styling and the border, leaving the content lines.
func panelBody(t *testing.T, panel string) []string {
	t.Helper()
	var out []string
	for _, line := range strings.Split(plain(panel), "\n") {
		if !strings.HasPrefix(line, "│") {
			continue
		}
		out = append(out, strings.TrimSuffix(strings.TrimPrefix(line, "│"), "│"))
	}
	if len(out) == 0 {
		t.Fatalf("no bordered content in:\n%s", panel)
	}
	return out
}
