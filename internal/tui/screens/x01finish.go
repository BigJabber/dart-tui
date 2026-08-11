package screens

import (
	"fmt"
	"strings"

	"github.com/BigJabber/dart-tui/internal/app"
	"github.com/BigJabber/dart-tui/internal/domain/x01"
	"github.com/BigJabber/dart-tui/internal/tui/components"
	"github.com/BigJabber/dart-tui/internal/tui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

// X01Finish confirms the winner and closes the game.
//
// The application suggests a winner from the recorded results — completed legs
// first, fewest darts among them — but never decides. Office games stop for all
// sorts of reasons, and the operator has the final say.
//
// A training game has no winner to confirm, so the screen offers no choice at
// all and simply saves the session.
type X01Finish struct {
	ctx     Context
	session *app.X01Session

	results []x01.ParticipantResult
	// options are the selectable winners, plus a "no winner" entry at the end.
	// A training game has none.
	options       []int64
	cursor        components.Cursor
	suggested     int64
	hasSuggestion bool
	training      bool
}

// NewX01Finish builds the finish screen.
func NewX01Finish(ctx Context, session *app.X01Session) *X01Finish {
	s := &X01Finish{ctx: ctx, session: session}
	s.reload()
	return s
}

// Init refreshes the results.
func (s *X01Finish) Init() tea.Cmd {
	s.reload()
	return nil
}

func (s *X01Finish) reload() {
	game := s.session.Game()
	s.results = game.Results()
	s.training = game.Training()

	s.options = s.options[:0]
	if s.training {
		s.cursor.SetCount(0)
		s.suggested, s.hasSuggestion = 0, false
		return
	}
	for _, r := range s.results {
		s.options = append(s.options, r.PlayerID)
	}
	// A trailing zero means "finish with no winner recorded", which is the
	// honest outcome for a game the office simply stopped.
	s.options = append(s.options, 0)
	s.cursor.SetCount(len(s.options))

	s.suggested, s.hasSuggestion = game.SuggestWinner()
	if s.hasSuggestion {
		for i, id := range s.options {
			if id == s.suggested {
				s.cursor.Set(i)
				break
			}
		}
	} else {
		// Nothing to suggest: start on "no winner" rather than on whoever
		// happens to be listed first.
		s.cursor.Set(len(s.options) - 1)
	}
}

// Title names the screen.
func (s *X01Finish) Title() string {
	if s.training {
		return "Finish training session"
	}
	return "Finish game"
}

// Hints lists the available keys.
func (s *X01Finish) Hints() []components.KeyHint {
	if s.training {
		return []components.KeyHint{
			{Keys: "enter", Desc: "save and close"},
			{Keys: "esc", Desc: "back to the game"},
		}
	}
	return []components.KeyHint{
		{Keys: "↑/↓", Desc: "choose winner"},
		{Keys: "enter", Desc: "confirm and save"},
		{Keys: "esc", Desc: "back to the game"},
	}
}

// Update handles winner selection.
func (s *X01Finish) Update(msg tea.Msg) (Screen, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	switch key.String() {
	case "up", "k":
		s.cursor.Prev()
	case "down", "j":
		s.cursor.Next()
	case "esc":
		return s, Pop()
	case "enter":
		return s, s.finish()
	}
	return s, nil
}

func (s *X01Finish) finish() tea.Cmd {
	var winner *int64
	if !s.training {
		i := s.cursor.Index()
		if i < 0 || i >= len(s.options) {
			return nil
		}
		if id := s.options[i]; id != 0 {
			w := id
			winner = &w
		}
	}
	if err := s.session.Finish(s.ctx.Ctx, winner); err != nil {
		return Fail(err)
	}

	if s.training {
		return tea.Batch(PopToRoot(), Success("Training session saved — statistics recorded, no win awarded."))
	}
	if winner == nil {
		return tea.Batch(PopToRoot(), Success("Game saved with no winner recorded."))
	}
	name := s.session.Player(*winner).Name
	return tea.Batch(PopToRoot(), Success(name+" wins — result saved."))
}

// View renders the results table and the winner choice.
func (s *X01Finish) View(width, height int) string {
	var b strings.Builder

	b.WriteString(styles.Subtitle.Render(
		"Results so far. Fewer darts is the stronger leg."))
	b.WriteString("\n\n")
	b.WriteString(s.resultsTable(width))
	b.WriteString("\n\n")

	if s.training {
		b.WriteString(styles.Bold.Render("Training session"))
		b.WriteString("\n")
		b.WriteString(styles.Faint.Render(
			"Only one player took part, so there is nobody to beat and no win is\n" +
				"recorded. Every dart, average, checkout and personal best from this\n" +
				"session still counts towards their statistics.\n"))
		b.WriteString("\n")
		b.WriteString(styles.SuccessText.Render("Press enter to save and close."))
		return b.String()
	}

	b.WriteString(styles.Bold.Render("Who won?"))
	b.WriteString("\n")
	if s.hasSuggestion {
		b.WriteString(styles.Faint.Render("Suggested from the recorded legs — change it if the office disagrees."))
	} else {
		b.WriteString(styles.Faint.Render("Nobody completed their leg, so there is nothing to suggest."))
	}
	b.WriteString("\n\n")

	for i, id := range s.options {
		selected := i == s.cursor.Index()
		var label string
		if id == 0 {
			label = "No winner — the game was stopped"
		} else {
			p := s.session.Player(id)
			label = styles.PlayerName(p.Colour, selected).Render(p.Name)
			if s.hasSuggestion && id == s.suggested {
				label += "  " + styles.SuccessText.Render("(suggested)")
			}
		}
		line := styles.Marker(selected) + label
		if selected {
			line = styles.SelectedRow.Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

func (s *X01Finish) resultsTable(width int) string {
	nameWidth := maxInt(minInt(width-46, 20), 8)
	table := components.Table{
		Columns: []components.Column{
			{Title: "Player", Width: nameWidth},
			{Title: "Darts", Width: 6, Right: true},
			{Title: "Visits", Width: 6, Right: true},
			{Title: "Avg", Width: 7, Right: true},
			{Title: "Result", Width: 20},
		},
	}

	for _, r := range s.results {
		p := s.session.Player(r.PlayerID)

		var result string
		switch {
		case r.Completed:
			result = styles.SuccessText.Render("checked out")
		case r.Visits == 0:
			// Never threw: recorded honestly rather than given a fake finish.
			result = styles.Faint.Render("did not throw")
		default:
			result = fmt.Sprintf("%d left", r.Remaining)
		}

		average := "—"
		if r.Darts > 0 {
			average = fmt.Sprintf("%.1f", r.Average)
		}

		table.Rows = append(table.Rows, components.Row{
			Cells: []string{
				styles.PlayerName(p.Colour, r.Completed).Render(
					components.Truncate(p.Name, nameWidth)),
				fmt.Sprintf("%d", r.Darts),
				fmt.Sprintf("%d", r.Visits),
				average,
				result,
			},
		})
	}
	return table.View()
}
