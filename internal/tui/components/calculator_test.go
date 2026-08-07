package components

import (
	"errors"
	"strings"
	"testing"
)

// typeInto feeds a string to the calculator one keystroke at a time.
func typeInto(c *Calculator, keys string) {
	for _, r := range keys {
		c.Key(string(r))
	}
}

func TestCalculatorAddsUpDarts(t *testing.T) {
	cases := []struct {
		typed string
		want  int
	}{
		{"60+30+21", 111},
		{"120+50+21", 191}, // terms need not be single darts
		{"60", 60},
		{"0", 0},
		{"180", 180},
		{"20+20+20", 60},
		{"1+1+1", 3},
	}
	for _, tc := range cases {
		var c Calculator
		typeInto(&c, tc.typed)
		got, err := c.Total()
		if err != nil {
			t.Errorf("%q: %v", tc.typed, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%q = %d, want %d", tc.typed, got, tc.want)
		}
	}
}

// A space is accepted as a separator for anyone who types it that way.
func TestCalculatorAcceptsSpacesAsSeparators(t *testing.T) {
	var c Calculator
	typeInto(&c, "60 30 21")
	got, err := c.Total()
	if err != nil {
		t.Fatal(err)
	}
	if got != 111 {
		t.Errorf("total = %d, want 111", got)
	}
	if c.Expression() != "60+30+21" {
		t.Errorf("expression = %q, want the normalised form", c.Expression())
	}
}

// A running total is available while the operator is still typing, including
// straight after a separator.
func TestCalculatorTotalsWhileTyping(t *testing.T) {
	var c Calculator
	steps := []struct {
		key   string
		total int
	}{
		{"6", 6},
		{"0", 60},
		{"+", 60}, // a trailing separator does not break the total
		{"3", 63},
		{"0", 90},
		{"+", 90},
		{"2", 92},
		{"1", 111},
	}
	for _, s := range steps {
		c.Key(s.key)
		got, err := c.Total()
		if err != nil {
			t.Fatalf("after %q: %v", c.Expression(), err)
		}
		if got != s.total {
			t.Errorf("after %q: total = %d, want %d", c.Expression(), got, s.total)
		}
	}
}

func TestCalculatorRejectsLeadingAndDoubledSeparators(t *testing.T) {
	var c Calculator
	c.Key("+")
	if c.Expression() != "" {
		t.Errorf("a leading + should be ignored, got %q", c.Expression())
	}
	typeInto(&c, "60++")
	if c.Expression() != "60+" {
		t.Errorf("a doubled + should be ignored, got %q", c.Expression())
	}
}

func TestCalculatorEmptyIsAnError(t *testing.T) {
	var c Calculator
	if !c.Empty() {
		t.Error("a new calculator should be empty")
	}
	if _, err := c.Total(); !errors.Is(err, ErrCalculatorEmpty) {
		t.Errorf("got %v, want ErrCalculatorEmpty", err)
	}
	// Separators alone are not a sum.
	typeInto(&c, "+ +")
	if _, err := c.Total(); !errors.Is(err, ErrCalculatorEmpty) {
		t.Errorf("separators only: got %v, want ErrCalculatorEmpty", err)
	}
}

func TestCalculatorBackspace(t *testing.T) {
	var c Calculator
	typeInto(&c, "60+30")
	c.Key("backspace")
	if c.Expression() != "60+3" {
		t.Errorf("expression = %q, want %q", c.Expression(), "60+3")
	}
	c.Key("backspace")
	c.Key("backspace")
	if c.Expression() != "60" {
		t.Errorf("expression = %q, want %q", c.Expression(), "60")
	}
	// Backspacing past the start is harmless.
	for i := 0; i < 5; i++ {
		c.Key("backspace")
	}
	if !c.Empty() {
		t.Errorf("expression = %q, want empty", c.Expression())
	}
}

func TestCalculatorIgnoresUnrelatedKeys(t *testing.T) {
	var c Calculator
	for _, key := range []string{"a", "esc", "enter", "up", "ctrl+c", "-", "*"} {
		if c.Key(key) {
			t.Errorf("Key(%q) should not have been consumed", key)
		}
	}
	if !c.Empty() {
		t.Errorf("expression = %q, want empty", c.Expression())
	}
}

func TestCalculatorLengthIsCapped(t *testing.T) {
	var c Calculator
	typeInto(&c, strings.Repeat("9", MaxCalculatorLength*2))
	if len(c.Expression()) > MaxCalculatorLength {
		t.Errorf("expression grew to %d characters, want at most %d",
			len(c.Expression()), MaxCalculatorLength)
	}
}

func TestCalculatorTerms(t *testing.T) {
	var c Calculator
	typeInto(&c, "120+50+21")
	terms, err := c.Terms()
	if err != nil {
		t.Fatal(err)
	}
	want := []int{120, 50, 21}
	if len(terms) != len(want) {
		t.Fatalf("got %d terms, want %d", len(terms), len(want))
	}
	for i := range want {
		if terms[i] != want[i] {
			t.Errorf("term %d = %d, want %d", i, terms[i], want[i])
		}
	}
}

func TestCalculatorResetAndSeed(t *testing.T) {
	var c Calculator
	typeInto(&c, "60+30")
	c.Reset()
	if !c.Empty() {
		t.Errorf("Reset left %q", c.Expression())
	}

	// Seeding carries across digits the operator had already typed.
	c.SetExpression("140")
	if c.Expression() != "140" {
		t.Errorf("expression = %q, want %q", c.Expression(), "140")
	}
	total, err := c.Total()
	if err != nil || total != 140 {
		t.Errorf("total = %d (%v), want 140", total, err)
	}

	// Seeding with something unusable does not leave junk behind.
	c.SetExpression("oops")
	if !c.Empty() {
		t.Errorf("expression = %q, want empty", c.Expression())
	}
}

func TestCalculatorDisplayAndView(t *testing.T) {
	var c Calculator
	if c.Display() != "" {
		t.Errorf("empty display = %q", c.Display())
	}
	typeInto(&c, "60+30+21")
	if got := c.Display(); got != "60 + 30 + 21" {
		t.Errorf("display = %q, want %q", got, "60 + 30 + 21")
	}
	view := c.View()
	for _, want := range []string{"Sum", "Total", "111"} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing %q:\n%s", want, view)
		}
	}
	// An empty calculator still renders, with a hint rather than a total.
	var empty Calculator
	if v := empty.View(); !strings.Contains(v, "60+30+21") {
		t.Errorf("empty view should show an example:\n%s", v)
	}
}
