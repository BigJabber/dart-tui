package screens

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/BigJabber/dart-tui/internal/app"
	"github.com/BigJabber/dart-tui/internal/domain/atw"
	"github.com/BigJabber/dart-tui/internal/tui/components"
	"github.com/BigJabber/dart-tui/internal/tui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

// ATWFinish confirms the winner and closes the game.
//
// The suggestion is whoever went round in the fewest darts, never whoever
// happened to finish first: players throw in any order here, so finishing first
// says only that somebody got to the board more often.
//
// Two players tied on the same dart count are offered as a single choice that
// records both. Picking one of them would be inventing a result.
type ATWFinish struct {
	ctx     Context
	session *app.ATWSession

	results []atw.ParticipantResult
	options []winnerOption
	cursor  components.Cursor

	suggested []int64
	training  bool
}

// winnerOption is one selectable outcome. Ids holds everybody it awards the win
// to, which is empty for "no winner" and more than one for a tie.
type winnerOption struct {
	label     string
	ids       []int64
	suggested bool
}

// NewATWFinish builds the finish screen.
func NewATWFinish(ctx Context, session *app.ATWSession) *ATWFinish {
	s := &ATWFinish{ctx: ctx, session: session}
	s.reload()
	return s
}

// Init refreshes the results.
func (s *ATWFinish) Init() tea.Cmd {
	s.reload()
	return nil
}

func (s *ATWFinish) reload() {
	game := s.session.Game()
	s.results = game.Results()
	s.training = game.Training()
	s.suggested = game.SuggestWinners()

	s.options = s.options[:0]
	if s.training {
		s.cursor.SetCount(0)
		return
	}

	// A tie leads, as one choice awarding the win to everybody in it.
	if len(s.suggested) > 1 {
		names := make([]string, 0, len(s.suggested))
		for _, id := range s.suggested {
			names = append(names, s.session.Player(id).Name)
		}
		s.options = append(s.options, winnerOption{
			label:     strings.Join(names, " and ") + " — tied",
			ids:       append([]int64(nil), s.suggested...),
			suggested: true,
		})
	}

	suggested := make(map[int64]bool, len(s.suggested))
	for _, id := range s.suggested {
		suggested[id] = true
	}
	for _, r := range s.results {
		s.options = append(s.options, winnerOption{
			label: s.session.Player(r.PlayerID).Name,
			ids:   []int64{r.PlayerID},
			// A lone suggestion is marked; one of a tied pair is not, because
			// choosing them alone would be overriding the tie rather than
			// accepting the suggestion.
			suggested: len(s.suggested) == 1 && suggested[r.PlayerID],
		})
	}
	// A trailing entry for a game the office simply stopped.
	s.options = append(s.options, winnerOption{label: "No winner — the game was stopped"})

	s.cursor.SetCount(len(s.options))
	for i, opt := range s.options {
		if opt.suggested {
			s.cursor.Set(i)
			return
		}
	}
	// Nothing to suggest: start on "no winner" rather than on whoever happens
	// to be listed first.
	s.cursor.Set(len(s.options) - 1)
}

// Title names the screen.
func (s *ATWFinish) Title() string {
	if s.training {
		return "Finish training session"
	}
	return "Finish game"
}

// Hints lists the available keys.
func (s *ATWFinish) Hints() []components.KeyHint {
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
func (s *ATWFinish) Update(msg tea.Msg) (Screen, tea.Cmd) {
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

func (s *ATWFinish) finish() tea.Cmd {
	var winners []int64
	if !s.training {
		i := s.cursor.Index()
		if i < 0 || i >= len(s.options) {
			return nil
		}
		winners = s.options[i].ids
	}

	if err := s.session.Finish(s.ctx.Ctx, winners); err != nil {
		return Fail(err)
	}

	switch {
	case s.training:
		return tea.Batch(PopToRoot(),
			Success("Training session saved — statistics recorded, no win awarded."))
	case len(winners) == 0:
		return tea.Batch(PopToRoot(), Success("Game saved with no winner recorded."))
	case len(winners) == 1:
		return tea.Batch(PopToRoot(),
			Success(s.session.Player(winners[0]).Name+" wins — result saved."))
	default:
		names := make([]string, 0, len(winners))
		for _, id := range winners {
			names = append(names, s.session.Player(id).Name)
		}
		return tea.Batch(PopToRoot(),
			Success(strings.Join(names, " and ")+" tie — result saved."))
	}
}

// View renders the results table and the winner choice.
func (s *ATWFinish) View(width, height int) string {
	var b strings.Builder

	b.WriteString(styles.Subtitle.Render(
		"Fewest darts to go round wins. An unfinished run has no result to compare."))
	b.WriteString("\n\n")
	b.WriteString(s.resultsTable(width))
	b.WriteString("\n\n")

	if s.training {
		b.WriteString(styles.Bold.Render("Training session"))
		b.WriteString("\n")
		b.WriteString(styles.Faint.Render(
			"Only one player took part, so there is nobody to beat and no win is\n" +
				"recorded. Every dart thrown still counts towards their statistics,\n" +
				"including a completed run and a fewest-darts record.\n"))
		b.WriteString("\n")
		b.WriteString(styles.SuccessText.Render("Press enter to save and close."))
		return b.String()
	}

	b.WriteString(styles.Bold.Render("Who won?"))
	b.WriteString("\n")
	switch {
	case len(s.suggested) > 1:
		b.WriteString(styles.Faint.Render(
			"Two runs finished in the same number of darts, so the suggestion is a tie."))
	case len(s.suggested) == 1:
		b.WriteString(styles.Faint.Render(
			"Suggested from the fewest darts — change it if the office disagrees."))
	default:
		b.WriteString(styles.Faint.Render(
			"Nobody completed a run, so there is nothing to suggest."))
	}
	b.WriteString("\n\n")

	for i, opt := range s.options {
		selected := i == s.cursor.Index()
		label := opt.label
		if len(opt.ids) == 1 {
			p := s.session.Player(opt.ids[0])
			label = styles.PlayerName(p.Colour, selected).Render(p.Name)
		}
		if opt.suggested {
			label += "  " + styles.SuccessText.Render("(suggested)")
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

func (s *ATWFinish) resultsTable(width int) string {
	nameWidth := maxInt(minInt(width-46, 20), 8)
	table := components.Table{
		Columns: []components.Column{
			{Title: "Player", Width: nameWidth},
			{Title: "Darts", Width: 6, Right: true},
			{Title: "Visits", Width: 6, Right: true},
			{Title: "Result", Width: 22},
		},
	}

	for _, r := range s.results {
		p := s.session.Player(r.PlayerID)

		// An unfinished run is never described by its dart count alone: how far
		// round somebody got is the only honest thing to say about it.
		var result string
		switch {
		case r.Completed:
			result = styles.SuccessText.Render(fmt.Sprintf("round in %d darts", r.Darts))
		case r.Visits == 0:
			result = styles.Faint.Render("did not throw")
		default:
			result = fmt.Sprintf("on %s, %d to go", r.Target, r.Remaining)
		}

		table.Rows = append(table.Rows, components.Row{
			Cells: []string{
				styles.PlayerName(p.Colour, r.Completed).Render(
					components.Truncate(p.Name, nameWidth)),
				strconv.Itoa(r.Darts),
				strconv.Itoa(r.Visits),
				result,
			},
		})
	}
	return table.View()
}
