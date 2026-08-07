package components

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/pourmoi/dart-tui/internal/tui/styles"
)

// MaxCalculatorLength caps the typed expression. A visit is three darts, so
// anything longer than this is a mistake rather than a sum.
const MaxCalculatorLength = 24

// Errors returned when an expression cannot be totalled.
var (
	// ErrCalculatorEmpty means nothing has been entered yet.
	ErrCalculatorEmpty = errors.New("enter the score for each dart, separated by +")
	// ErrCalculatorBadTerm means one of the numbers could not be read.
	ErrCalculatorBadTerm = errors.New("that is not a number")
)

// Calculator adds up the scores of individual darts.
//
// Operators usually know what each dart scored — 60, 30, 21 — but adding them
// up in their head while someone waits at the board is where manual scoring goes
// wrong. This turns "60+30+21" into a single number to confirm.
//
// Terms are not required to be single darts: "120+50+21" is a perfectly good way
// to enter a visit where two darts have already been added together in the
// operator's head.
type Calculator struct {
	expr string
}

// Reset clears the calculator.
func (c *Calculator) Reset() { c.expr = "" }

// SetExpression seeds the calculator, ignoring anything it could not accept.
// It is used to carry across digits the operator had already typed.
func (c *Calculator) SetExpression(expr string) {
	c.expr = ""
	for _, r := range expr {
		c.Key(string(r))
	}
}

// Expression returns the raw text entered.
func (c *Calculator) Expression() string { return c.expr }

// Empty reports whether anything has been typed.
func (c *Calculator) Empty() bool { return c.expr == "" }

// Key feeds one keystroke to the calculator and reports whether it was used.
//
// Space is accepted as a separator as well as +, so "60 30 21" works for anyone
// who types it that way.
func (c *Calculator) Key(key string) bool {
	switch {
	case len(key) == 1 && key[0] >= '0' && key[0] <= '9':
		if len(c.expr) >= MaxCalculatorLength {
			return true
		}
		c.expr += key
		return true

	case key == "+" || key == " " || key == "space":
		c.plus()
		return true

	case key == "backspace":
		if len(c.expr) > 0 {
			c.expr = c.expr[:len(c.expr)-1]
		}
		return true
	}
	return false
}

// plus adds a separator, refusing a leading or doubled one so the expression
// always stays readable.
func (c *Calculator) plus() {
	if c.expr == "" || strings.HasSuffix(c.expr, "+") {
		return
	}
	if len(c.expr) >= MaxCalculatorLength {
		return
	}
	c.expr += "+"
}

// Terms returns the numbers entered so far, ignoring a trailing separator.
func (c *Calculator) Terms() ([]int, error) {
	if strings.Trim(c.expr, "+ ") == "" {
		return nil, ErrCalculatorEmpty
	}
	var out []int
	for _, part := range strings.Split(c.expr, "+") {
		part = strings.TrimSpace(part)
		if part == "" {
			// A trailing "+" while the operator is still typing.
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("%w: %q", ErrCalculatorBadTerm, part)
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil, ErrCalculatorEmpty
	}
	return out, nil
}

// Total adds up the expression.
func (c *Calculator) Total() (int, error) {
	terms, err := c.Terms()
	if err != nil {
		return 0, err
	}
	total := 0
	for _, n := range terms {
		total += n
	}
	return total, nil
}

// CalculatorHint is the reminder shown beside a score box that the darts can be
// added up rather than totalled in the operator's head.
//
// It is worded once, here, so both game screens say the same thing — a feature
// nobody knows about is no feature at all.
func CalculatorHint() string {
	return styles.Faint.Render("Not sure of the total?  Press = or + to add the darts up.")
}

// Display renders the expression with spaces around the separators.
func (c *Calculator) Display() string {
	if c.expr == "" {
		return ""
	}
	return strings.ReplaceAll(c.expr, "+", " + ")
}

// View renders the expression and its running total.
func (c *Calculator) View() string {
	var b strings.Builder

	b.WriteString("Sum    ")
	if c.expr == "" {
		b.WriteString(styles.Faint.Render("e.g. 60+30+21"))
	} else {
		b.WriteString(styles.Bold.Render(c.Display()))
	}
	b.WriteString("\n")

	b.WriteString("Total  ")
	total, err := c.Total()
	if err != nil {
		b.WriteString(styles.Faint.Render("—"))
		return b.String()
	}
	b.WriteString(styles.SuccessText.Render(strconv.Itoa(total)))

	// Spell out the arithmetic so the operator can check it at a glance.
	if terms, err := c.Terms(); err == nil && len(terms) > 1 {
		parts := make([]string, 0, len(terms))
		for _, n := range terms {
			parts = append(parts, strconv.Itoa(n))
		}
		b.WriteString(styles.Faint.Render(
			fmt.Sprintf("   (%s)", strings.Join(parts, " + "))))
	}
	return b.String()
}
