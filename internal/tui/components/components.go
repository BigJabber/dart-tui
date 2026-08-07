// Package components holds the small reusable widgets the screens are built
// from: a cursor list, a confirmation dialog, a numeric entry field and a table
// renderer. They are deliberately plain — no darts knowledge lives here.
package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/pourmoi/dart-tui/internal/tui/styles"
)

// ---------------------------------------------------------------- cursor list

// Cursor tracks a selection within a list of n items and keeps it in range as
// the list changes underneath it.
type Cursor struct {
	index int
	count int
}

// SetCount updates the number of items, clamping the selection.
func (c *Cursor) SetCount(n int) {
	c.count = n
	c.clamp()
}

// Count returns the number of items.
func (c *Cursor) Count() int { return c.count }

// Index returns the selected position, or -1 when the list is empty.
func (c *Cursor) Index() int {
	if c.count == 0 {
		return -1
	}
	return c.index
}

// Set moves the selection to a specific index.
func (c *Cursor) Set(i int) {
	c.index = i
	c.clamp()
}

// Next moves down, wrapping to the top.
func (c *Cursor) Next() {
	if c.count == 0 {
		return
	}
	c.index = (c.index + 1) % c.count
}

// Prev moves up, wrapping to the bottom.
func (c *Cursor) Prev() {
	if c.count == 0 {
		return
	}
	c.index = (c.index - 1 + c.count) % c.count
}

// First jumps to the top of the list.
func (c *Cursor) First() { c.index = 0 }

// Last jumps to the bottom of the list.
func (c *Cursor) Last() {
	if c.count > 0 {
		c.index = c.count - 1
	}
}

func (c *Cursor) clamp() {
	if c.count == 0 {
		c.index = 0
		return
	}
	if c.index >= c.count {
		c.index = c.count - 1
	}
	if c.index < 0 {
		c.index = 0
	}
}

// Window returns the slice bounds to display so the selection stays visible in
// a viewport of the given height.
func (c *Cursor) Window(height int) (start, end int) {
	if height <= 0 || c.count == 0 {
		return 0, c.count
	}
	if c.count <= height {
		return 0, c.count
	}
	start = c.index - height/2
	if start < 0 {
		start = 0
	}
	if start+height > c.count {
		start = c.count - height
	}
	return start, start + height
}

// ------------------------------------------------------------------- confirm

// Confirm is a yes/no dialog used before anything destructive or irreversible.
type Confirm struct {
	Title   string
	Body    string
	Confirm string
	Cancel  string
	// Dangerous marks an action that cannot be undone, so the dialog says so.
	Dangerous bool
}

// View renders the dialog.
func (c Confirm) View() string {
	var b strings.Builder
	b.WriteString(styles.Bold.Render(c.Title))
	if c.Body != "" {
		b.WriteString("\n\n")
		b.WriteString(c.Body)
	}
	if c.Dangerous {
		b.WriteString("\n\n")
		b.WriteString(styles.ErrorText.Render("This cannot be undone."))
	}

	confirmLabel := c.Confirm
	if confirmLabel == "" {
		confirmLabel = "Yes"
	}
	cancelLabel := c.Cancel
	if cancelLabel == "" {
		cancelLabel = "No"
	}
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("%s %s    %s %s",
		styles.Key.Render("y"), confirmLabel,
		styles.Key.Render("n/esc"), cancelLabel))

	return styles.Dialog.Render(b.String())
}

// --------------------------------------------------------------------- table

// Column describes one table column.
type Column struct {
	Title string
	Width int
	// Right aligns the column's content to the right, for numbers.
	Right bool
}

// Table renders fixed-width columns. It is intentionally simpler than the
// Bubbles table: rows here contain pre-styled, coloured cells, and a general
// table widget would strip or mismeasure the escape sequences.
type Table struct {
	Columns []Column
	// Rows holds already-rendered cell content, which may include styling.
	Rows []Row
}

// Row is one table row.
type Row struct {
	Cells []string
	// Selected draws the row highlighted, with a marker in the first column.
	Selected bool
	// Style, when set, wraps the whole rendered row.
	Style *lipgloss.Style
}

// View renders the table including its heading row.
func (t Table) View() string {
	var b strings.Builder

	var head []string
	for _, col := range t.Columns {
		head = append(head, pad(col.Title, col.Width, col.Right))
	}
	b.WriteString(styles.TableHeader.Render(strings.Join(head, " ")))

	for _, row := range t.Rows {
		b.WriteString("\n")
		var cells []string
		for i, col := range t.Columns {
			cell := ""
			if i < len(row.Cells) {
				cell = row.Cells[i]
			}
			cells = append(cells, pad(cell, col.Width, col.Right))
		}
		line := strings.Join(cells, " ")
		switch {
		case row.Style != nil:
			line = row.Style.Render(line)
		case row.Selected:
			line = styles.SelectedRow.Render(line)
		}
		b.WriteString(line)
	}
	return b.String()
}

// pad sizes a cell to a column width, measuring printable width so styled text
// with escape sequences still lines up.
func pad(s string, width int, right bool) string {
	if width <= 0 {
		return s
	}
	w := lipgloss.Width(s)
	if w > width {
		return truncate(s, width)
	}
	gap := strings.Repeat(" ", width-w)
	if right {
		return gap + s
	}
	return s + gap
}

// truncate shortens plain text to width, ending in an ellipsis. Styled text is
// returned unchanged rather than risk cutting through an escape sequence.
func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if strings.ContainsRune(s, '\x1b') {
		return s
	}
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}

// Truncate shortens a plain string to fit a width.
func Truncate(s string, width int) string { return truncate(s, width) }

// ------------------------------------------------------------------ helpers

// KeyHint is one entry in the contextual footer.
type KeyHint struct {
	Keys string
	Desc string
}

// RenderHints formats key hints for the footer, wrapping to the given width.
func RenderHints(hints []KeyHint, width int) string {
	if len(hints) == 0 {
		return ""
	}
	var (
		lines   []string
		current string
	)
	for _, h := range hints {
		part := styles.Key.Render(h.Keys) + " " + h.Desc
		candidate := part
		if current != "" {
			candidate = current + styles.Faint.Render("  ·  ") + part
		}
		if width > 0 && lipgloss.Width(candidate) > width && current != "" {
			lines = append(lines, current)
			current = part
			continue
		}
		current = candidate
	}
	if current != "" {
		lines = append(lines, current)
	}
	return strings.Join(lines, "\n")
}

// Bar renders a simple horizontal rule.
func Bar(width int) string {
	if width <= 0 {
		return ""
	}
	return styles.Faint.Render(strings.Repeat("─", width))
}

// Blank returns n newlines, for vertical spacing.
func Blank(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat("\n", n)
}
