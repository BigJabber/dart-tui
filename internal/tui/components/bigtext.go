package components

import (
	"strings"
	"unicode"
)

// bigtext draws numbers large enough to read from the other side of the room.
//
// The scoreboard is normally wall-mounted and watched from the oche, where the
// tables are a grey blur. The remaining score and the runs needed are the two
// numbers the room actually wants, so they get drawn out of blocks instead.
//
// The font is a bitmap here rather than a dependency: the application ships as
// one static executable that needs no build tools, and pulling in a figlet
// library for one panel would not be a fair trade. Nothing in this file knows
// anything about darts.

// Glyph dimensions. 5x7 is the classic LED-matrix cell — the smallest grid on
// which 6 and 8, 3 and 9, and 1 and 7 are all unmistakable.
const (
	glyphCols = 5
	glyphRows = 7
)

// glyphs is the font, one bitmap per rune, top row first. Everything else in
// this file is derived from it: the sizes differ only in how many characters
// each pixel becomes and whether pairs of rows are packed into half blocks.
// Editing a digit here changes it at every size.
var glyphs = map[rune][glyphRows]string{
	'0': {" ### ", "#   #", "#   #", "#   #", "#   #", "#   #", " ### "},
	'1': {"  #  ", " ##  ", "  #  ", "  #  ", "  #  ", "  #  ", " ### "},
	'2': {" ### ", "#   #", "    #", "   # ", "  #  ", " #   ", "#####"},
	'3': {" ### ", "#   #", "    #", "  ## ", "    #", "#   #", " ### "},
	'4': {"   # ", "  ## ", " # # ", "#  # ", "#####", "   # ", "   # "},
	'5': {"#####", "#    ", "#### ", "    #", "    #", "#   #", " ### "},
	'6': {"  ## ", " #   ", "#    ", "#### ", "#   #", "#   #", " ### "},
	'7': {"#####", "    #", "   # ", "  #  ", " #   ", " #   ", " #   "},
	'8': {" ### ", "#   #", "#   #", " ### ", "#   #", "#   #", " ### "},
	'9': {" ### ", "#   #", "#   #", " ####", "    #", "   # ", " ##  "},
	'-': {"     ", "     ", "     ", "#####", "     ", "     ", "     "},
	'.': {"     ", "     ", "     ", "     ", "     ", " ##  ", " ##  "},
	'/': {"    #", "    #", "   # ", "  #  ", " #   ", "#    ", "#    "},
	'?': {" ### ", "#   #", "    #", "   # ", "  #  ", "     ", "  #  "},

	// Letters, for the thrower's name. Only the capitals exist: a lower-case
	// rune is looked up as its upper-case form, so names come out in capitals
	// however they were typed.
	' ':  {"     ", "     ", "     ", "     ", "     ", "     ", "     "},
	'\'': {"  #  ", "  #  ", "     ", "     ", "     ", "     ", "     "},
	'A':  {" ### ", "#   #", "#   #", "#####", "#   #", "#   #", "#   #"},
	'B':  {"#### ", "#   #", "#   #", "#### ", "#   #", "#   #", "#### "},
	'C':  {" ### ", "#   #", "#    ", "#    ", "#    ", "#   #", " ### "},
	'D':  {"#### ", "#   #", "#   #", "#   #", "#   #", "#   #", "#### "},
	'E':  {"#####", "#    ", "#    ", "#### ", "#    ", "#    ", "#####"},
	'F':  {"#####", "#    ", "#    ", "#### ", "#    ", "#    ", "#    "},
	'G':  {" ### ", "#   #", "#    ", "#  ##", "#   #", "#   #", " ####"},
	'H':  {"#   #", "#   #", "#   #", "#####", "#   #", "#   #", "#   #"},
	'I':  {" ### ", "  #  ", "  #  ", "  #  ", "  #  ", "  #  ", " ### "},
	'J':  {"  ###", "   # ", "   # ", "   # ", "   # ", "#  # ", " ##  "},
	'K':  {"#   #", "#  # ", "# #  ", "##   ", "# #  ", "#  # ", "#   #"},
	'L':  {"#    ", "#    ", "#    ", "#    ", "#    ", "#    ", "#####"},
	'M':  {"#   #", "## ##", "# # #", "#   #", "#   #", "#   #", "#   #"},
	'N':  {"#   #", "##  #", "# # #", "#  ##", "#   #", "#   #", "#   #"},
	'O':  {" ### ", "#   #", "#   #", "#   #", "#   #", "#   #", " ### "},
	'P':  {"#### ", "#   #", "#   #", "#### ", "#    ", "#    ", "#    "},
	'Q':  {" ### ", "#   #", "#   #", "#   #", "# # #", "#  # ", " ## #"},
	'R':  {"#### ", "#   #", "#   #", "#### ", "# #  ", "#  # ", "#   #"},
	'S':  {" ####", "#    ", "#    ", " ### ", "    #", "    #", "#### "},
	'T':  {"#####", "  #  ", "  #  ", "  #  ", "  #  ", "  #  ", "  #  "},
	'U':  {"#   #", "#   #", "#   #", "#   #", "#   #", "#   #", " ### "},
	'V':  {"#   #", "#   #", "#   #", "#   #", "#   #", " # # ", "  #  "},
	'W':  {"#   #", "#   #", "#   #", "#   #", "# # #", "## ##", "#   #"},
	'X':  {"#   #", "#   #", " # # ", "  #  ", " # # ", "#   #", "#   #"},
	'Y':  {"#   #", "#   #", " # # ", "  #  ", "  #  ", "  #  ", "  #  "},
	'Z':  {"#####", "    #", "   # ", "  #  ", " #   ", "#    ", "#####"},
}

// glyphFor looks a rune up, falling back to its upper-case form so a name does
// not have to be upper-cased before it is drawn.
func glyphFor(r rune) ([glyphRows]string, bool) {
	if g, ok := glyphs[r]; ok {
		return g, true
	}
	g, ok := glyphs[unicode.ToUpper(r)]
	return g, ok
}

// CanRender reports whether every rune of s is in the font.
//
// Numbers always are. Names may not be — an accented letter has no glyph — and
// a name pocked with blank cells is worse than one drawn in ordinary text, so
// callers check first and fall back.
func CanRender(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if _, ok := glyphFor(r); !ok {
			return false
		}
	}
	return true
}

// blankGlyph stands in for anything the font does not have, including a space.
// An unknown rune therefore costs a cell of the usual width rather than a
// panic or a column that silently goes missing, so a caller's width arithmetic
// stays true whatever it is handed.
var blankGlyph = [glyphRows]string{
	"     ", "     ", "     ", "     ", "     ", "     ", "     ",
}

// BigSize is how large a number is drawn.
//
// The sizes exist because the same panel has to survive on a 14-row terminal
// and fill a wall-mounted 1080p screen. Each one repeats a pixel a different
// number of times across and down, chosen so the pixel comes out roughly square
// given that a terminal cell is about twice as tall as it is wide. A single
// "scale" number could not do that: it would stretch the digits at every size
// but one.
type BigSize int

// The sizes, smallest first.
const (
	// BigNone means there is no room for blocks at all and the caller should
	// fall back to ordinary text.
	BigNone BigSize = iota
	// BigShort packs two pixel rows into each terminal row with half blocks.
	BigShort
	// BigTall gives each pixel a row of its own.
	BigTall
	// BigHuge is the wall-display size.
	BigHuge
	// BigGiant is only reachable when the panel has a column to itself, where
	// there is roughly twice the height a stacked panel ever gets.
	BigGiant
)

// biggestFirst is every drawable size, largest first. Both the fitting helper
// and the panel step down it, so a size added here is picked up by both.
var biggestFirst = []BigSize{BigGiant, BigHuge, BigTall, BigShort}

// scaleOf returns how many characters wide and how many rows tall one pixel is
// at a size, and whether rows are packed in pairs.
//
// Only BigShort uses half blocks. A few terminal fonts draw them with a hairline
// gap between the rows, and keeping them to the size used when space is tight
// means that risk never lands on the wall-display path.
func scaleOf(size BigSize) (sx, sy int, half bool) {
	switch size {
	case BigShort:
		return 1, 1, true
	case BigTall:
		return 2, 1, false
	case BigHuge:
		return 4, 2, false
	case BigGiant:
		return 6, 3, false
	default:
		return 0, 0, false
	}
}

// BigRows is how many terminal rows a size occupies, or 0 for BigNone.
func BigRows(size BigSize) int {
	switch size {
	case BigShort:
		// Seven pixel rows in pairs, the last one on its own.
		return (glyphRows + 1) / 2
	case BigTall:
		return glyphRows
	case BigHuge:
		return glyphRows * 2
	case BigGiant:
		return glyphRows * 3
	default:
		return 0
	}
}

// BigWidth is how many columns rendering s at size will take.
func BigWidth(s string, size BigSize) int {
	sx, _, _ := scaleOf(size)
	n := len([]rune(s))
	if sx == 0 || n == 0 {
		return 0
	}
	// Each character is a glyph plus a gutter, except the last, which has no
	// gutter after it.
	return ((glyphCols+1)*n - 1) * sx
}

// BigFit returns the largest size that draws s inside rows by cols, or BigNone
// when even the smallest will not fit.
//
// All the layout arithmetic lives here so callers do not each reinvent it, and
// so the width case — a tall but narrow window — is handled as carefully as the
// height case that everyone thinks of first.
func BigFit(s string, rows, cols int) BigSize {
	if s == "" {
		return BigNone
	}
	for _, size := range biggestFirst {
		if BigRows(size) <= rows && BigWidth(s, size) <= cols {
			return size
		}
	}
	return BigNone
}

// Big renders s at the given size, one string per terminal row. Every row has
// the same printable width. It returns nil for BigNone or an empty string.
func Big(s string, size BigSize) []string {
	sx, sy, half := scaleOf(size)
	if sx == 0 || s == "" {
		return nil
	}

	px := pixelRows(s, sx)
	if half {
		return packHalfBlocks(px)
	}

	out := make([]string, 0, glyphRows*sy)
	for _, row := range px {
		line := strings.Map(func(r rune) rune {
			if r == '#' {
				return '█'
			}
			return ' '
		}, row)
		for i := 0; i < sy; i++ {
			out = append(out, line)
		}
	}
	return out
}

// pixelRows expands s into one string per pixel row, with a one-pixel gutter
// between characters, each pixel repeated sx times across.
func pixelRows(s string, sx int) [glyphRows]string {
	runes := []rune(s)
	gutter := strings.Repeat(" ", sx)

	var b [glyphRows]strings.Builder
	for i, r := range runes {
		g, ok := glyphFor(r)
		if !ok {
			g = blankGlyph
		}
		for row := 0; row < glyphRows; row++ {
			for _, c := range g[row] {
				b[row].WriteString(strings.Repeat(string(c), sx))
			}
			if i < len(runes)-1 {
				b[row].WriteString(gutter)
			}
		}
	}

	var out [glyphRows]string
	for row := range b {
		out[row] = b[row].String()
	}
	return out
}

// packHalfBlocks folds pairs of pixel rows into single terminal rows, which is
// what makes the small size half the height for the same shape. The seventh row
// has no partner and pairs with blank.
func packHalfBlocks(px [glyphRows]string) []string {
	out := make([]string, 0, (glyphRows+1)/2)
	for row := 0; row < glyphRows; row += 2 {
		top := []rune(px[row])
		var bottom []rune
		if row+1 < glyphRows {
			bottom = []rune(px[row+1])
		}

		var b strings.Builder
		for i, t := range top {
			var lower rune = ' '
			if i < len(bottom) {
				lower = bottom[i]
			}
			switch {
			case t == '#' && lower == '#':
				b.WriteRune('█')
			case t == '#':
				b.WriteRune('▀')
			case lower == '#':
				b.WriteRune('▄')
			default:
				b.WriteRune(' ')
			}
		}
		out = append(out, b.String())
	}
	return out
}

// LetterSpace upper-cases a name and puts a space between every letter, so it
// reads as a heading from a distance: "Alice" becomes "A L I C E".
//
// This is what the large panel does instead of block letters. Block letters
// would eat the width the number needs and are harder to read than spaced
// capitals anyway; the emphasis comes from the spacing and the colour.
func LetterSpace(s string) string {
	runes := []rune(strings.ToUpper(s))
	if len(runes) == 0 {
		return ""
	}
	parts := make([]string, 0, len(runes))
	for _, r := range runes {
		parts = append(parts, string(r))
	}
	return strings.Join(parts, " ")
}

// LineCount is how many terminal rows a rendered block occupies. An empty
// string occupies none.
//
// It is called LineCount rather than Rows because Row already means a table row
// in this package.
func LineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}
