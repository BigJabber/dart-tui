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

// atwPhase is what the Around the World screen is currently doing.
type atwPhase int

const (
	atwIdle atwPhase = iota
	atwEntering
	atwConfirmLeave
)

// ATWGame is the Around the World play screen.
//
// A visit is three keystrokes and enter: "031" is a miss, a treble and a
// single. That is the whole interaction, because this game is many short visits
// rather than a few long ones, and anything that needs confirming between darts
// would be felt every single turn.
//
// Like the X01 screen it never advances to the next player by itself. The
// operator picks whoever has walked up to the board.
type ATWGame struct {
	ctx     Context
	session *app.ATWSession

	phase  atwPhase
	cursor components.Cursor

	// entry is the visit being typed, in 0-3 notation.
	entry    string
	entryErr string

	// lastVisit describes the previous entry, so the operator can see their
	// correction target before pressing undo.
	lastVisit string
}

// NewATWGame builds the play screen for a live or resumed session.
func NewATWGame(ctx Context, session *app.ATWSession) *ATWGame {
	s := &ATWGame{ctx: ctx, session: session}
	s.cursor.SetCount(len(session.Order()))
	s.selectFirstActive()
	return s
}

// Init does nothing; the session is already loaded.
func (s *ATWGame) Init() tea.Cmd { return nil }

// Title names the screen. A solo game is labelled training throughout.
func (s *ATWGame) Title() string {
	if s.session.Game().Training() {
		return fmt.Sprintf("Around the World — training %d", s.session.GameID())
	}
	return fmt.Sprintf("Around the World — game %d", s.session.GameID())
}

// Hints lists the keys available in the current phase.
func (s *ATWGame) Hints() []components.KeyHint {
	switch s.phase {
	case atwEntering:
		return []components.KeyHint{
			{Keys: "0-3", Desc: "one key per dart"},
			{Keys: "enter", Desc: "record visit"},
			{Keys: "backspace", Desc: "rub out a dart"},
			{Keys: "esc", Desc: "cancel"},
		}
	case atwConfirmLeave:
		return []components.KeyHint{
			{Keys: "y", Desc: "leave (the game is saved)"},
			{Keys: "n/esc", Desc: "stay"},
		}
	default:
		return []components.KeyHint{
			{Keys: "↑/↓", Desc: "choose who threw"},
			{Keys: "0-3", Desc: "enter their darts"},
			{Keys: "u", Desc: "undo last visit"},
			{Keys: "f", Desc: "finish game"},
			{Keys: "esc", Desc: "leave"},
		}
	}
}

// Update routes keys according to the current phase.
func (s *ATWGame) Update(msg tea.Msg) (Screen, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}

	switch s.phase {
	case atwEntering:
		return s.updateEntering(key)
	case atwConfirmLeave:
		return s.updateLeaveConfirm(key)
	default:
		return s.updateIdle(key)
	}
}

// isDartKey reports whether a keystroke is one of the four dart results.
func isDartKey(str string) bool {
	return len(str) == 1 && str[0] >= '0' && str[0] <= '3'
}

func (s *ATWGame) updateIdle(key tea.KeyMsg) (Screen, tea.Cmd) {
	str := key.String()

	// Typing a dart goes straight into the visit: the common case takes no
	// setup keystrokes at all.
	if isDartKey(str) {
		if _, ok := s.selectedPlayer(); !ok {
			return s, Warn("Everyone has finished their run. Press f to finish the game.")
		}
		s.phase = atwEntering
		s.entry = ""
		s.entryErr = ""
		return s.appendDart(str)
	}

	switch str {
	case "up", "k":
		s.moveCursor(-1)
	case "down", "j":
		s.moveCursor(1)
	case "u":
		return s, s.undo()
	case "f":
		return s, Push(NewATWFinish(s.ctx, s.session))
	case "enter", " ":
		if _, ok := s.selectedPlayer(); ok {
			s.phase = atwEntering
			s.entry = ""
			s.entryErr = ""
		}
	case "esc":
		s.phase = atwConfirmLeave
	}
	return s, nil
}

func (s *ATWGame) updateEntering(key tea.KeyMsg) (Screen, tea.Cmd) {
	str := key.String()

	if isDartKey(str) {
		return s.appendDart(str)
	}

	switch str {
	case "backspace":
		if len(s.entry) > 0 {
			s.entry = s.entry[:len(s.entry)-1]
		}
		s.entryErr = ""
	case "esc":
		s.phase = atwIdle
		s.entry = ""
		s.entryErr = ""
	case "enter":
		return s, s.record()
	case "u":
		if s.entry == "" {
			s.phase = atwIdle
			return s, s.undo()
		}
	}
	return s, nil
}

// appendDart adds one keyed dart to the visit, refusing anything the engine
// would not accept at the target that dart is actually thrown at.
//
// Validating here rather than on enter means a treble keyed at the bull is
// rejected on the keystroke that caused it, while the operator still knows
// which dart they meant.
func (s *ATWGame) appendDart(str string) (Screen, tea.Cmd) {
	if len(s.entry) >= atw.MaxDartsPerVisit {
		s.entryErr = fmt.Sprintf("A visit is at most %d darts.", atw.MaxDartsPerVisit)
		return s, nil
	}
	// A run completed part-way through a visit ends it: the remaining darts
	// were never thrown and must not be recorded.
	if target, ok := s.entryTarget(); ok && target.Done() {
		s.entryErr = "The run is complete — press enter to record it."
		return s, nil
	}

	candidate := s.entry + str
	if _, err := s.previewEntry(candidate); err != nil {
		s.entryErr = capitalise(err.Error())
		return s, nil
	}
	s.entry = candidate
	s.entryErr = ""
	return s, nil
}

// previewEntry asks the engine what the typed visit would do, without recording
// it. Every rule stays in the engine; the screen only displays the answer.
func (s *ATWGame) previewEntry(notation string) (atw.Visit, error) {
	id, ok := s.selectedPlayer()
	if !ok {
		return atw.Visit{}, atw.ErrPlayerFinished
	}
	calls, err := atw.ParseCalls(notation)
	if err != nil {
		return atw.Visit{}, err
	}
	return s.session.Game().Preview(atw.VisitInput{PlayerID: id, Calls: calls})
}

// entryTarget is what the next dart of the visit will be thrown at: the
// player's target once the darts typed so far have been applied.
func (s *ATWGame) entryTarget() (atw.Target, bool) {
	id, ok := s.selectedPlayer()
	if !ok {
		return atw.Target{}, false
	}
	if s.entry == "" {
		st, ok := s.session.Game().State(id)
		if !ok {
			return atw.Target{}, false
		}
		return st.Target, true
	}
	v, err := s.previewEntry(s.entry)
	if err != nil {
		return atw.Target{}, false
	}
	return v.TargetAfter, true
}

func (s *ATWGame) record() tea.Cmd {
	id, ok := s.selectedPlayer()
	if !ok {
		s.phase = atwIdle
		return Warn("Choose a player who still has a run to finish.")
	}
	if s.entry == "" {
		s.entryErr = "Type one key per dart thrown, then press enter."
		return nil
	}

	calls, err := atw.ParseCalls(s.entry)
	if err != nil {
		s.entryErr = capitalise(err.Error())
		return nil
	}

	v, err := s.session.Record(s.ctx.Ctx, atw.VisitInput{PlayerID: id, Calls: calls})
	if err != nil {
		s.entryErr = capitalise(err.Error())
		return nil
	}

	name := s.session.Player(id).Name
	s.lastVisit = describeATWVisit(name, v)
	s.entry = ""
	s.entryErr = ""
	s.phase = atwIdle

	if v.Completed {
		darts := s.finishDarts(id)
		// The cursor moves off the finished player, because they cannot throw
		// again. The scoreboard keeps their row and their dart count.
		s.selectFirstActive()
		return Success(fmt.Sprintf("%s is round the board in %d darts.", name, darts))
	}
	return nil
}

// finishDarts is the dart count a player completed their run in.
func (s *ATWGame) finishDarts(id int64) int {
	if st, ok := s.session.Game().State(id); ok {
		return st.FinishDarts
	}
	return 0
}

// describeATWVisit puts a recorded visit into one line of plain English, which
// is what the operator reads before deciding whether to undo it.
func describeATWVisit(name string, v atw.Visit) string {
	parts := make([]string, 0, len(v.Darts))
	for _, d := range v.Darts {
		parts = append(parts, d.Label())
	}
	summary := fmt.Sprintf("%s threw %s", name, strings.Join(parts, ", "))
	if v.Completed {
		return summary + " — run complete"
	}
	return fmt.Sprintf("%s — next target %s", summary, v.TargetAfter)
}

func (s *ATWGame) undo() tea.Cmd {
	v, err := s.session.Undo(s.ctx.Ctx)
	if err != nil {
		return Fail(err)
	}
	name := s.session.Player(v.PlayerID).Name
	s.lastVisit = ""
	s.selectPlayer(v.PlayerID)
	return Success(fmt.Sprintf("Undid %s's visit — back on %s.", name, v.TargetBefore))
}

func (s *ATWGame) updateLeaveConfirm(key tea.KeyMsg) (Screen, tea.Cmd) {
	switch key.String() {
	case "y", "Y":
		return s, tea.Batch(PopToRoot(), Success("Game saved — resume it from the main menu."))
	case "n", "N", "esc":
		s.phase = atwIdle
	}
	return s, nil
}

// ---------------------------------------------------------- player selection

// moveCursor steps to the next selectable player, skipping anybody who has
// finished. It never wraps past everybody being finished.
func (s *ATWGame) moveCursor(delta int) {
	order := s.session.Order()
	if len(order) == 0 {
		return
	}
	game := s.session.Game()
	for i := 1; i <= len(order); i++ {
		next := (s.cursor.Index() + delta*i + len(order)*len(order)) % len(order)
		if st, ok := game.State(order[next]); ok && !st.Finished {
			s.cursor.Set(next)
			return
		}
	}
}

func (s *ATWGame) playerAt(index int) (int64, bool) {
	order := s.session.Order()
	if index < 0 || index >= len(order) {
		return 0, false
	}
	return order[index], true
}

// selectedPlayer is who the next visit will be recorded for, or false when
// everybody has finished their run.
func (s *ATWGame) selectedPlayer() (int64, bool) {
	id, ok := s.playerAt(s.cursor.Index())
	if !ok {
		return 0, false
	}
	st, found := s.session.Game().State(id)
	if !found || st.Finished {
		return 0, false
	}
	return id, true
}

func (s *ATWGame) selectPlayer(id int64) {
	for i, pid := range s.session.Order() {
		if pid == id {
			s.cursor.Set(i)
			return
		}
	}
}

func (s *ATWGame) selectFirstActive() {
	if _, ok := s.selectedPlayer(); ok {
		return
	}
	for i, id := range s.session.Order() {
		if st, ok := s.session.Game().State(id); ok && !st.Finished {
			s.cursor.Set(i)
			return
		}
	}
}

// -------------------------------------------------------------------- view

// View renders the scoreboard, the large panel and whichever prompt is active.
func (s *ATWGame) View(width, height int) string {
	layout := components.NewLayout(width, height)

	board := s.scoreboard(layout.Body)
	lower := s.lowerPanel(layout.Body)

	rows := layout.PanelRows
	if !layout.SideBySide {
		rows = height - components.LineCount(board) - components.LineCount(lower) - 2
	}
	return layout.Join(board, s.spotlight(layout.Panel, rows), lower)
}

func (s *ATWGame) lowerPanel(width int) string {
	switch s.phase {
	case atwEntering:
		return s.entryPanel(width)
	case atwConfirmLeave:
		return components.Confirm{
			Title:   "Leave this game?",
			Body:    "It is saved automatically and will be waiting under Resume game.",
			Confirm: "leave",
			Cancel:  "stay",
		}.View()
	default:
		return s.idlePanel(width)
	}
}

// spotlight is the large panel: who is throwing and what they must hit, drawn
// big enough to read from the oche rather than from the keyboard.
func (s *ATWGame) spotlight(width, rows int) string {
	if s.phase == atwConfirmLeave {
		return ""
	}
	id, ok := s.selectedPlayer()
	if !ok {
		return ""
	}
	p := s.session.Player(id)
	st, ok := s.session.Game().State(id)
	if !ok {
		return ""
	}

	// The number shown is the player's recorded target, not what the half-typed
	// visit would leave them on. A visit is keyed in after the darts have been
	// thrown, so a panel that changed on every keystroke would be reporting the
	// past while looking like an instruction — and it would disagree with the
	// scoreboard row directly above it. The prompt below shows the live result.
	number, caption := atwSpotlightTarget(st.Target)

	sub := fmt.Sprintf("%d OF %d TARGETS  ·  %d DARTS", st.Advanced, atw.TargetCount, st.Darts)
	if st.Darts == 0 {
		sub = "NO DARTS YET"
	}

	return components.Spotlight{
		Banner:  "NOW THROWING",
		Name:    p.Name,
		Colour:  p.Colour,
		Number:  number,
		Caption: caption,
		Sub:     sub,
		Rows:    rows,
	}.View(width)
}

// atwSpotlightTarget splits a target into the big number and the words beside
// it.
//
// The bulls show their board value in blocks with their name spelled out
// alongside, so what is read from across the room is "25 OUTER BULL" rather
// than a bare number that could be mistaken for the twenty-five bed, and never
// a position in the sequence.
func atwSpotlightTarget(t atw.Target) (number, caption string) {
	switch t.Kind {
	case atw.OuterBull:
		return "25", "OUTER\nBULL"
	case atw.Bullseye:
		return "50", "BULLSEYE"
	case atw.Complete:
		return "0", "TO GO"
	default:
		return strconv.Itoa(t.Number), "TO HIT"
	}
}

func (s *ATWGame) scoreboard(width int) string {
	game := s.session.Game()

	nameWidth := maxInt(minInt(width-46, 20), 8)
	table := components.Table{
		Columns: []components.Column{
			{Title: "", Width: 2},
			{Title: "Player", Width: nameWidth},
			{Title: "Next target", Width: 12},
			{Title: "Darts", Width: 6, Right: true},
			{Title: "Visits", Width: 6, Right: true},
			{Title: "Status", Width: 14},
		},
	}

	for i, id := range s.session.Order() {
		st, ok := game.State(id)
		if !ok {
			continue
		}
		p := s.session.Player(id)
		current := i == s.cursor.Index() && !st.Finished

		// The status is a word in every case: a finished run must be readable
		// without noticing that a row has stopped being highlighted.
		var status, target string
		switch {
		case st.Finished:
			status = styles.SuccessText.Render("Finished")
			target = styles.SuccessText.Render("—")
		case current:
			status = styles.Bold.Render("← throwing")
			target = styles.Bold.Render(st.Target.String())
		case st.Visits == 0:
			status = styles.Faint.Render("yet to throw")
			target = st.Target.String()
		default:
			status = "playing"
			target = st.Target.String()
		}

		table.Rows = append(table.Rows, components.Row{
			Selected: current,
			Cells: []string{
				styles.Marker(current),
				styles.PlayerName(p.Colour, current || st.Finished).Render(
					components.Truncate(p.Name, nameWidth)),
				target,
				strconv.Itoa(st.Darts),
				strconv.Itoa(st.Visits),
				status,
			},
		})
	}

	out := table.View()
	if game.AllFinished() {
		if game.Training() {
			out += "\n" + styles.SuccessText.Render(
				"Round the board — press f to save the training session.")
		} else {
			out += "\n" + styles.SuccessText.Render(
				"Everyone is round — press f to finish and confirm the winner.")
		}
	}
	return out
}

func (s *ATWGame) idlePanel(width int) string {
	var b strings.Builder

	if s.lastVisit != "" {
		b.WriteString(styles.Faint.Render("Last: " + components.Truncate(s.lastVisit, maxInt(width-8, 20))))
		b.WriteString("\n\n")
	}

	id, ok := s.selectedPlayer()
	if !ok {
		b.WriteString(styles.Faint.Render("Nobody left to throw. Press f to finish the game."))
		return styles.Panel.Render(b.String())
	}

	p := s.session.Player(id)
	st, _ := s.session.Game().State(id)

	b.WriteString("Ready for ")
	b.WriteString(styles.PlayerName(p.Colour, true).Render(p.Name))
	b.WriteString(" on ")
	b.WriteString(styles.Bold.Render(st.Target.String()))
	if s.session.Game().Training() {
		b.WriteString(styles.Faint.Render(" — type their darts."))
	} else {
		b.WriteString(styles.Faint.Render(" — type their darts, or ↑/↓ to pick someone else."))
	}
	b.WriteString("\n\n")
	b.WriteString(atwLegend(st.Target))
	return styles.Panel.Render(b.String())
}

func (s *ATWGame) entryPanel(width int) string {
	id, ok := s.selectedPlayer()
	if !ok {
		return styles.ErrorText.Render("That player can no longer throw. Press esc and choose another.")
	}
	p := s.session.Player(id)
	st, _ := s.session.Game().State(id)

	var b strings.Builder
	b.WriteString(styles.PlayerName(p.Colour, true).Render(p.Name))
	b.WriteString(styles.Faint.Render(fmt.Sprintf(
		"   started this visit on %s   ·   %d darts so far", st.Target, st.Darts)))
	b.WriteString("\n\n")

	b.WriteString("Darts  ")
	if s.entry == "" {
		b.WriteString(styles.Faint.Render(strings.Repeat("_ ", atw.MaxDartsPerVisit)))
	} else {
		b.WriteString(styles.Bold.Render(strings.Join(strings.Split(s.entry, ""), " ")))
		b.WriteString(strings.Repeat("  ", atw.MaxDartsPerVisit-len(s.entry)))
	}

	// What the typed darts did, dart by dart, so a mistyped visit is caught
	// before it is recorded rather than undone afterwards.
	if v, err := s.previewEntry(s.entry); err == nil {
		described := make([]string, 0, len(v.Darts))
		for _, d := range v.Darts {
			described = append(described, d.Label())
		}
		b.WriteString(styles.Faint.Render("   " + strings.Join(described, ", ")))
		b.WriteString("\n\n")
		if v.Completed {
			b.WriteString(styles.SuccessText.Render(fmt.Sprintf(
				"→ round the board in %d darts — press enter to record it",
				st.Darts+v.DartCount())))
		} else {
			b.WriteString(styles.SuccessText.Render("→ next target " + v.TargetAfter.String()))
		}
	}
	// The legend follows the target the next dart faces, which is not
	// necessarily the one the visit started on. Once the run is complete there
	// is no next dart and no legend — and no blank line held open for one.
	target := st.Target
	if t, ok := s.entryTarget(); ok {
		target = t
	}
	if !target.Done() {
		b.WriteString("\n\n")
		b.WriteString(atwLegend(target))
	}

	if s.entryErr != "" {
		b.WriteString("\n\n")
		b.WriteString(styles.ErrorText.Render(s.entryErr))
	}

	panel := styles.FocusPanel
	if width > 4 {
		panel = panel.Width(minInt(width-2, 78))
	}
	return panel.Render(b.String())
}

// atwLegend spells out what each key means at this target. It changes on the
// bulls, where there is no treble to hit and 2 means the bullseye.
func atwLegend(t atw.Target) string {
	switch t.Kind {
	case atw.OuterBull:
		return styles.Faint.Render(
			"0 = miss    1 = outer bull    2 = bullseye (finishes)    no treble bull")
	case atw.Bullseye:
		return styles.Faint.Render(
			"0 = miss    2 = bullseye (finishes)    no treble bull")
	default:
		return styles.Faint.Render(
			"0 = miss    1 = single    2 = double (skip 1)    3 = treble (skip 2)")
	}
}
