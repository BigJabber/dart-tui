package components

import (
	"strings"

	"github.com/BigJabber/dart-tui/internal/tui/styles"
	"github.com/charmbracelet/lipgloss"
)

// Spotlight is the large panel that sits between the scoreboard and the entry
// box: who is throwing, and the one number that matters.
//
// It is sized to be read from the oche rather than from the keyboard, so it
// carries very little — a role in words, a name, a number and one supporting
// line. Everything in it also appears as ordinary text elsewhere on the screen,
// because the panel shrinks and eventually disappears on a small terminal and
// nothing may depend on it being there.
//
// Both game screens use it so the two look identical on the wall. It knows
// nothing about darts: the caller decides what the number means.
type Spotlight struct {
	// Banner is the role, in words: "NOW THROWING", "▶ BATTING", "▶ BOWLING".
	// A word, never a colour on its own.
	Banner string
	// BannerStyle renders the banner. Nil renders it bold. It is a pointer for
	// the same reason Row.Style is: a Lip Gloss style has no usable zero value
	// to test against.
	BannerStyle *lipgloss.Style

	// Name is the thrower. It is upper-cased and letter-spaced here.
	Name string
	// Colour is the player's colour. Empty renders the name in bold instead.
	Colour string

	// Number is drawn out of blocks, already formatted by the caller.
	Number string
	// Caption sits beside the number: "LEFT", "TO WIN". It may span two lines.
	Caption string
	// Sub is one supporting line under the number, or empty. Plain text: it is
	// truncated here, which styled text would silently escape.
	Sub string

	// Rows is how many terminal rows the panel may occupy. The panel picks the
	// largest size that fits and renders nothing at all when even the smallest
	// will not.
	Rows int
}

// NameSizes decides how big the thrower's name is drawn, given the size of the
// number underneath it.
//
// ---- This is the dial. Change it to taste. ----
//
// The name is deliberately the smaller of the two: it is read once, when the
// thrower changes, while the number is read continuously all through the visit.
// A quarter of the number's height is about right, which is why the largest
// number gets a four-row name against its fourteen.
//
// BigNone means "not in blocks at all" — the name falls back to ordinary text,
// upper-cased and letter-spaced, on one row. That is also what happens
// automatically when the name is too wide for the panel at the size asked for,
// or contains a letter the font does not have.
//
// The sizes available are BigNone (1 row), BigShort (4), BigTall (7) and
// BigHuge (14). Making a name bigger costs the number the same rows, so a name
// at BigTall will often push the number down a size on anything but a very tall
// window.
var NameSizes = map[BigSize]BigSize{
	BigGiant: BigTall,  // 7 rows against 21
	BigHuge:  BigShort, // 4 rows against 14
	BigTall:  BigNone,  // a 4-row name against a 7-row number is too close
	BigShort: BigNone,
}

// Panel geometry.
const (
	// spotlightFixedRows is every row of a full-size panel that is neither the
	// number nor the name: the two borders, the banner, the blank above the
	// number, and the blank and the sub-line below it.
	spotlightFixedRows = 6

	// spotlightCompactRows is the fallback: two borders and two lines.
	spotlightCompactRows = 4

	// captionGap separates the number from its caption.
	captionGap = 3
)

// View renders the panel, or "" when there is no room for even the compact form.
func (s Spotlight) View(width int) string {
	// The panel fills whatever width it is given: how much that is belongs to
	// Layout, which is the only thing that knows whether it has a column to
	// itself. The two is the border, which Width does not count.
	panelWidth := width - 2
	// Below this there is nothing worth drawing a border around.
	if s.Rows < spotlightCompactRows || panelWidth < 24 {
		return ""
	}
	// The panel's own left and right padding, from styles.Panel.
	inner := panelWidth - 2

	captionWidth := 0
	if s.Caption != "" {
		captionWidth = lipgloss.Width(s.Caption) + captionGap
	}

	// The number and the name come out of one budget, so they are chosen
	// together: the largest number whose name also fits above it.
	for _, size := range biggestFirst {
		if BigWidth(s.Number, size) > inner-captionWidth {
			continue
		}
		nameSize := s.nameSizeFor(size, inner)
		if spotlightFixedRows+BigRows(size)+s.nameRows(nameSize) <= s.Rows {
			return s.panel(panelWidth).Render(s.fullBody(inner, size, nameSize))
		}
	}
	return s.panel(panelWidth).Render(s.compactBody(inner))
}

// panel frames the content. The parts inside have already been centred, each as
// a block, by centreBlock.
func (s Spotlight) panel(width int) lipgloss.Style {
	return styles.Panel.Width(width)
}

// centreBlock centres a block of lines inside a width, shifting every line by
// the same amount.
//
// The parts are centred here rather than by giving the panel style a centre
// alignment, because that would place each line on its own. The rows of a
// number are not independent — a digit's stroke has to stay in the same column
// from its top row to its bottom — so the block is measured once by its widest
// line and every line moves together.
func centreBlock(s string, width int) string {
	if s == "" {
		return s
	}
	lines := strings.Split(s, "\n")

	widest := 0
	for _, line := range lines {
		if w := lipgloss.Width(line); w > widest {
			widest = w
		}
	}
	pad := (width - widest) / 2
	if pad <= 0 {
		return s
	}

	prefix := strings.Repeat(" ", pad)
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

// nameSizeFor picks the size of the name from NameSizes, stepping down and then
// out to ordinary text if the panel is not wide enough for it.
func (s Spotlight) nameSizeFor(number BigSize, inner int) BigSize {
	if !CanRender(s.Name) {
		return BigNone
	}
	for size := NameSizes[number]; size > BigNone; size-- {
		if BigWidth(s.Name, size) <= inner {
			return size
		}
	}
	return BigNone
}

// nameRows is how many rows the name occupies. Ordinary text is one row.
func (s Spotlight) nameRows(size BigSize) int {
	return maxInt(BigRows(size), 1)
}

// fullBody is the wall-display layout: banner, name, the number in blocks
// beside its caption, then the supporting line.
func (s Spotlight) fullBody(inner int, size, nameSize BigSize) string {
	var b strings.Builder

	b.WriteString(centreBlock(s.bannerText(), inner))
	b.WriteString("\n")
	b.WriteString(centreBlock(s.nameBlock(inner, nameSize), inner))
	b.WriteString("\n\n")
	b.WriteString(centreBlock(s.numberBlock(size), inner))
	b.WriteString("\n\n")
	b.WriteString(centreBlock(s.subText(inner), inner))
	return b.String()
}

// compactBody is what is left when the blocks will not fit: the same
// information on two ordinary lines, so a short terminal loses the size but
// never the meaning.
func (s Spotlight) compactBody(inner int) string {
	var b strings.Builder

	first := s.bannerText()
	if s.Name != "" {
		first += "   " + s.nameText(inner-lipgloss.Width(s.Banner)-3)
	}
	b.WriteString(centreBlock(first, inner))
	b.WriteString("\n")

	parts := make([]string, 0, 2)
	headline := styles.Bold.Render(s.Number)
	if s.Caption != "" {
		// A two-line caption reads as one phrase on a single line here.
		headline += " " + styles.Bold.Render(strings.ReplaceAll(s.Caption, "\n", " "))
	}
	parts = append(parts, headline)
	if s.Sub != "" {
		parts = append(parts, truncate(s.Sub, maxInt(inner-lipgloss.Width(headline)-7, 8)))
	}
	b.WriteString(centreBlock(strings.Join(parts, styles.Faint.Render("   ·   ")), inner))
	return b.String()
}

func (s Spotlight) bannerText() string {
	if s.BannerStyle != nil {
		return s.BannerStyle.Render(s.Banner)
	}
	return styles.Bold.Render(s.Banner)
}

// nameBlock draws the name, in blocks when it has been given the rows for them
// and in ordinary text otherwise.
func (s Spotlight) nameBlock(inner int, size BigSize) string {
	if size == BigNone {
		return s.nameText(inner)
	}
	rows := Big(s.Name, size)
	style := s.nameStyle()
	for i, row := range rows {
		rows[i] = style.Render(row)
	}
	return strings.Join(rows, "\n")
}

// nameText spaces the name out and colours it. The name is truncated before it
// is spaced and before it is styled: Truncate returns styled text unchanged, so
// doing it the other way round would quietly overflow the panel.
func (s Spotlight) nameText(budget int) string {
	if s.Name == "" {
		return ""
	}
	// Spacing doubles the width bar the last character, so half the budget is
	// what actually fits.
	name := truncate(s.Name, maxInt((budget+1)/2, 3))
	return s.nameStyle().Render(LetterSpace(name))
}

// nameStyle is the player's colour, which is what carries the identity here.
// The number below stays in the ordinary foreground so it is legible whatever
// the palette; the name is the one thing in the panel that is coloured.
func (s Spotlight) nameStyle() lipgloss.Style {
	if s.Colour == "" {
		return styles.Bold
	}
	return styles.PlayerName(s.Colour, true)
}

// numberBlock draws the number beside its caption.
//
// This is the one place in the application that uses lipgloss to lay out a
// block, rather than joining strings by hand. The number is several rows of
// styled text that has to line up against a caption of a different height, and
// measuring that by counting characters is exactly the mistake the escape
// sequences make easy — the rest of the screen stays plain strings.
func (s Spotlight) numberBlock(size BigSize) string {
	rows := Big(s.Number, size)

	// The number stays in the default foreground rather than the player's
	// colour, for the same reason the checkout routes do: it has to be legible
	// from across the room whatever the palette and whatever the terminal theme.
	for i, row := range rows {
		rows[i] = styles.Bold.Render(row)
	}
	block := strings.Join(rows, "\n")

	if s.Caption == "" {
		return block
	}
	caption := lipgloss.NewStyle().
		PaddingLeft(captionGap).
		Render(styles.Bold.Render(s.Caption))
	return lipgloss.JoinHorizontal(lipgloss.Center, block, caption)
}

func (s Spotlight) subText(inner int) string {
	if s.Sub == "" {
		return ""
	}
	// Left in the default foreground, not dimmed: this line carries the
	// checkout route, which is the other thing worth reading from a distance.
	return truncate(s.Sub, inner)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
