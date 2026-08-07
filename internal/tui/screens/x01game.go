package screens

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/BigJabber/dart-tui/internal/app"
	"github.com/BigJabber/dart-tui/internal/domain/darts"
	"github.com/BigJabber/dart-tui/internal/domain/x01"
	"github.com/BigJabber/dart-tui/internal/tui/components"
	"github.com/BigJabber/dart-tui/internal/tui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

// x01Phase is what the game screen is waiting for.
type x01Phase int

const (
	// x01Idle is the resting state: pick who threw, then start typing a score.
	x01Idle x01Phase = iota
	// x01Entering is score entry for the selected player.
	x01Entering
	// x01ConfirmCheckout asks whether a visit that reached zero finished on a
	// double. The application records visit totals, not individual darts, so it
	// cannot know — and under double-out rules the answer decides whether the
	// visit is a checkout or a bust.
	x01ConfirmCheckout
	// x01ConfirmLeave guards against accidentally leaving a game in progress.
	x01ConfirmLeave
	// x01Calculating adds up the individual darts. Operators know what each
	// dart scored but adding three numbers in their head at the board is where
	// mistakes creep in, so the total is worked out and then confirmed.
	x01Calculating
)

// defaultDarts is the number of darts a visit is assumed to use. Almost every
// visit is three, so the entry field defaults to it and the operator only
// intervenes on a short final visit.
const defaultDarts = darts.MaxDartsPerVisit

// dartsAtVisitStart is how many darts a checkout suggestion assumes are in hand.
//
// Scores are recorded a visit at a time rather than dart by dart, so the
// application never learns mid-visit how many darts are left. The suggestion is
// therefore the one that applies at the start of the visit, with all three darts
// available. The advisor takes the number as a parameter, so a future
// dart-by-dart mode can ask for fresh advice after every dart without any change
// here.
const dartsAtVisitStart = darts.MaxDartsPerVisit

// X01Game is the in-play screen for 301 and 501.
type X01Game struct {
	ctx     Context
	session *app.X01Session

	phase  x01Phase
	cursor components.Cursor

	// entry is the score being typed, and entryDarts how many darts it took.
	entry      string
	entryDarts int
	entryErr   string
	// entryFrom records the sum a calculated score came from, so the operator
	// can check the arithmetic before committing to it.
	entryFrom string

	calc    components.Calculator
	calcErr string

	// checkouts suggests ways of finishing. It is held rather than called
	// through a package function so a future version could give a player their
	// own preferred doubles without touching this screen.
	checkouts *x01.CheckoutAdvisor

	// lastVisit describes what the previous entry did, so the operator can see
	// their correction target before pressing undo.
	lastVisit string
}

// NewX01Game builds the play screen for a live or resumed session.
func NewX01Game(ctx Context, session *app.X01Session) *X01Game {
	s := &X01Game{
		ctx:        ctx,
		session:    session,
		entryDarts: defaultDarts,
		checkouts:  x01.DefaultCheckoutAdvisor(),
	}
	s.cursor.SetCount(len(session.Order()))
	s.selectFirstActive()
	return s
}

// Init does nothing; the session is already loaded.
func (s *X01Game) Init() tea.Cmd { return nil }

// Title names the screen.
func (s *X01Game) Title() string {
	return fmt.Sprintf("%d — game %d", s.session.Game().Config().StartScore, s.session.GameID())
}

// Hints lists the keys available in the current phase.
func (s *X01Game) Hints() []components.KeyHint {
	switch s.phase {
	case x01Entering:
		return []components.KeyHint{
			{Keys: "0-9", Desc: "score"},
			{Keys: "tab/d", Desc: "darts thrown"},
			{Keys: "enter", Desc: "record"},
			{Keys: "esc", Desc: "cancel"},
		}
	case x01ConfirmCheckout:
		return []components.KeyHint{
			{Keys: "y", Desc: "yes, finished on a double"},
			{Keys: "n", Desc: "no, record as a bust"},
			{Keys: "esc", Desc: "back to the score"},
		}
	case x01ConfirmLeave:
		return []components.KeyHint{
			{Keys: "y", Desc: "leave (the game is saved)"},
			{Keys: "n/esc", Desc: "stay"},
		}
	case x01Calculating:
		return []components.KeyHint{
			{Keys: "0-9", Desc: "a dart's score"},
			{Keys: "+", Desc: "add another"},
			{Keys: "enter", Desc: "use the total"},
			{Keys: "esc", Desc: "cancel"},
		}
	default:
		return []components.KeyHint{
			{Keys: "↑/↓", Desc: "choose who threw"},
			{Keys: "0-9", Desc: "enter their score"},
			{Keys: "=", Desc: "add darts up"},
			{Keys: "u", Desc: "undo last visit"},
			{Keys: "f", Desc: "finish game"},
			{Keys: "esc", Desc: "leave"},
		}
	}
}

// Update routes keys according to the current phase.
func (s *X01Game) Update(msg tea.Msg) (Screen, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}

	switch s.phase {
	case x01Entering:
		return s.updateEntering(key)
	case x01ConfirmCheckout:
		return s.updateCheckoutConfirm(key)
	case x01ConfirmLeave:
		return s.updateLeaveConfirm(key)
	case x01Calculating:
		return s.updateCalculating(key)
	default:
		return s.updateIdle(key)
	}
}

// openCalculator starts a sum, carrying across any digits already typed so
// "60" then "+" continues naturally into "60+".
func (s *X01Game) openCalculator(seed string, withPlus bool) tea.Cmd {
	if _, ok := s.selectedPlayer(); !ok {
		return Warn("Choose a player who still has a score to throw at.")
	}
	s.calc.Reset()
	s.calc.SetExpression(seed)
	if withPlus {
		s.calc.Key("+")
	}
	s.calcErr = ""
	s.phase = x01Calculating
	return nil
}

// updateCalculating handles the sum, then hands the total to the score field
// for the operator to confirm.
func (s *X01Game) updateCalculating(key tea.KeyMsg) (Screen, tea.Cmd) {
	switch key.String() {
	case "esc":
		s.phase = x01Entering
		s.calcErr = ""
		return s, nil
	case "enter":
		return s, s.applyCalculation()
	}

	if s.calc.Key(key.String()) {
		s.calcErr = ""
	}
	return s, nil
}

// applyCalculation validates the total through the engine and, if it is a score
// that could actually be thrown, puts it in the score field.
//
// Validating here rather than after the calculator closes means a mistyped sum
// is corrected where it was made.
func (s *X01Game) applyCalculation() tea.Cmd {
	total, err := s.calc.Total()
	if err != nil {
		s.calcErr = capitalise(err.Error())
		return nil
	}
	player, ok := s.selectedPlayer()
	if !ok {
		s.phase = x01Idle
		return Warn("That player can no longer throw.")
	}

	// The same check the score field itself would run, so a sum and a typed
	// score are held to identical standards.
	if _, err := s.session.Game().Preview(x01.VisitInput{
		PlayerID: player, Points: total, Darts: s.entryDarts,
	}); err != nil {
		s.calcErr = capitalise(err.Error())
		return nil
	}

	s.entry = strconv.Itoa(total)
	s.entryFrom = s.calc.Display()
	s.entryErr = ""
	s.phase = x01Entering
	return nil
}

func (s *X01Game) updateIdle(key tea.KeyMsg) (Screen, tea.Cmd) {
	str := key.String()

	// Typing a digit goes straight into score entry: the common case takes no
	// setup keystrokes at all.
	if len(str) == 1 && str[0] >= '0' && str[0] <= '9' {
		if _, ok := s.selectedPlayer(); !ok {
			return s, Warn("Everyone has checked out. Press f to finish the game.")
		}
		s.phase = x01Entering
		s.entry = str
		s.entryDarts = defaultDarts
		s.entryErr = ""
		s.entryFrom = ""
		return s, nil
	}

	switch str {
	case "up", "k":
		s.moveCursor(-1)
	case "down", "j":
		s.moveCursor(1)
	// "+" is not on the main keyboard next to "=", but it is on the numeric
	// keypad the scoreboard is usually driven from, so both open the sum.
	case "=", "+":
		s.entryDarts = defaultDarts
		s.entryFrom = ""
		return s, s.openCalculator("", false)
	case "u":
		return s, s.undo()
	case "f":
		return s, Push(NewX01Finish(s.ctx, s.session))
	case "enter", " ":
		if _, ok := s.selectedPlayer(); ok {
			s.phase = x01Entering
			s.entry = ""
			s.entryDarts = defaultDarts
			s.entryErr = ""
		}
	case "esc":
		s.phase = x01ConfirmLeave
	}
	return s, nil
}

func (s *X01Game) updateEntering(key tea.KeyMsg) (Screen, tea.Cmd) {
	str := key.String()

	if len(str) == 1 && str[0] >= '0' && str[0] <= '9' {
		// Three digits is enough for any visit total.
		if len(s.entry) < 3 {
			s.entry += str
			s.entryErr = ""
			s.entryFrom = ""
		}
		return s, nil
	}

	switch str {
	// Pressing "+" part-way through typing turns what is already there into the
	// first term of a sum, which is how an operator naturally reaches for it.
	case "=", "+":
		return s, s.openCalculator(s.entry, str == "+")
	case "backspace":
		if len(s.entry) > 0 {
			s.entry = s.entry[:len(s.entry)-1]
		}
		s.entryErr = ""
		s.entryFrom = ""
	case "tab", "d":
		// Cycle 3 → 2 → 1 → 3. The digits are taken by score entry, so the
		// dart count gets its own key.
		s.entryDarts--
		if s.entryDarts < 1 {
			s.entryDarts = darts.MaxDartsPerVisit
		}
		s.entryErr = ""
	case "shift+tab", "D":
		s.entryDarts++
		if s.entryDarts > darts.MaxDartsPerVisit {
			s.entryDarts = 1
		}
		s.entryErr = ""
	case "up", "k":
		s.moveCursor(-1)
	case "down", "j":
		s.moveCursor(1)
	case "esc":
		s.phase = x01Idle
		s.entry = ""
		s.entryErr = ""
		s.entryFrom = ""
	case "enter":
		return s, s.submit()
	}
	return s, nil
}

func (s *X01Game) updateCheckoutConfirm(key tea.KeyMsg) (Screen, tea.Cmd) {
	switch key.String() {
	case "y", "Y":
		return s, s.record(true)
	case "n", "N":
		// Not a double: at the board this throw would have busted.
		return s, s.record(false)
	case "esc":
		s.phase = x01Entering
	}
	return s, nil
}

func (s *X01Game) updateLeaveConfirm(key tea.KeyMsg) (Screen, tea.Cmd) {
	switch key.String() {
	case "y", "Y":
		return s, tea.Batch(PopToRoot(), Status("Game saved — resume it from the main menu."))
	case "n", "N", "esc":
		s.phase = x01Idle
	}
	return s, nil
}

// submit validates the typed entry and either records it or asks about the
// checkout first.
func (s *X01Game) submit() tea.Cmd {
	p, ok := s.selectedPlayer()
	if !ok {
		return Warn("Choose a player who still has a score to throw at.")
	}
	if s.entry == "" {
		s.entryErr = "Enter the total scored with these darts."
		return nil
	}
	points, err := strconv.Atoi(s.entry)
	if err != nil {
		s.entryErr = "That is not a number."
		return nil
	}

	// Validate through the engine so the same rules apply here as anywhere.
	in := x01.VisitInput{PlayerID: p, Points: points, Darts: s.entryDarts}
	if _, err := s.session.Game().Preview(in); err != nil {
		s.entryErr = capitalise(err.Error())
		return nil
	}

	if s.session.Game().Config().DoubleOut && s.session.Game().LandsOnZero(p, points) {
		s.phase = x01ConfirmCheckout
		return nil
	}
	return s.record(false)
}

// record commits the entry to the engine and the database.
func (s *X01Game) record(confirmCheckout bool) tea.Cmd {
	p, ok := s.selectedPlayer()
	if !ok {
		s.phase = x01Idle
		return Warn("That player can no longer throw.")
	}
	points, err := strconv.Atoi(s.entry)
	if err != nil {
		s.phase = x01Entering
		s.entryErr = "That is not a number."
		return nil
	}

	visit, err := s.session.Record(s.ctx.Ctx, x01.VisitInput{
		PlayerID:        p,
		Points:          points,
		Darts:           s.entryDarts,
		ConfirmCheckout: confirmCheckout,
	})
	if err != nil {
		s.phase = x01Entering
		s.entryErr = capitalise(err.Error())
		return nil
	}

	s.phase = x01Idle
	s.entry = ""
	s.entryDarts = defaultDarts
	s.entryErr = ""
	s.entryFrom = ""

	name := s.session.Player(p).Name
	dartWord := "darts"
	if visit.Darts == 1 {
		dartWord = "dart"
	}

	switch {
	case visit.Checkout:
		s.lastVisit = fmt.Sprintf("%s checked out with %d in %d %s",
			name, visit.Points, visit.Darts, dartWord)
		// The cursor stays put deliberately: the game does not end here, and
		// the operator chooses who throws next.
		s.selectFirstActive()
		return Success(s.lastVisit + " — press f to finish the game when you are ready")
	case visit.Bust:
		s.lastVisit = fmt.Sprintf("%s BUST on %d (%d %s) — back to %d",
			name, visit.Points, visit.Darts, dartWord, visit.ScoreAfter)
		return Warn(s.lastVisit)
	default:
		s.lastVisit = fmt.Sprintf("%s scored %d with %d %s — %d left",
			name, visit.Points, visit.Darts, dartWord, visit.ScoreAfter)
		return Status(s.lastVisit)
	}
}

func (s *X01Game) undo() tea.Cmd {
	visit, err := s.session.Undo(s.ctx.Ctx)
	if err != nil {
		return Fail(err)
	}
	// Put the cursor back on whoever threw the removed visit, which is almost
	// always who the operator is about to re-enter.
	s.selectPlayer(visit.PlayerID)
	name := s.session.Player(visit.PlayerID).Name
	s.lastVisit = ""
	return Success(fmt.Sprintf("Undid %s's visit of %d — re-enter it now", name, visit.Points))
}

// ------------------------------------------------------------------ cursor

func (s *X01Game) moveCursor(delta int) {
	order := s.session.Order()
	if len(order) == 0 {
		return
	}
	// Step over players who have already checked out: they cannot throw again.
	for i := 0; i < len(order); i++ {
		if delta > 0 {
			s.cursor.Next()
		} else {
			s.cursor.Prev()
		}
		if id, ok := s.playerAt(s.cursor.Index()); ok {
			if st, found := s.session.Game().State(id); found && !st.Finished {
				return
			}
		}
	}
}

func (s *X01Game) playerAt(index int) (int64, bool) {
	order := s.session.Order()
	if index < 0 || index >= len(order) {
		return 0, false
	}
	return order[index], true
}

// selectedPlayer returns the player under the cursor if they can still throw.
func (s *X01Game) selectedPlayer() (int64, bool) {
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

func (s *X01Game) selectPlayer(id int64) {
	for i, pid := range s.session.Order() {
		if pid == id {
			s.cursor.Set(i)
			return
		}
	}
}

func (s *X01Game) selectFirstActive() {
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
func (s *X01Game) View(width, height int) string {
	board := s.scoreboard(width)
	lower := s.lowerPanel(width)

	var b strings.Builder
	b.WriteString(board)
	b.WriteString("\n")

	// Whatever height is left over once the table and the prompt have had
	// theirs belongs to the large panel. The prompt is what the operator types
	// into, so it is never the thing that gets squeezed; the large panel is a
	// reminder, so it shrinks and then goes away. The two is the newline either
	// side of the panel, which are rows as well.
	spare := height - components.LineCount(board) - components.LineCount(lower) - 2
	if spot := s.spotlight(width, spare); spot != "" {
		b.WriteString(spot)
		b.WriteString("\n")
	}

	b.WriteString(lower)
	return b.String()
}

// lowerPanel renders the prompt for the current phase.
func (s *X01Game) lowerPanel(width int) string {
	switch s.phase {
	case x01Entering:
		return s.entryPanel(width)
	case x01ConfirmCheckout:
		return s.checkoutPanel()
	case x01ConfirmLeave:
		return components.Confirm{
			Title:   "Leave this game?",
			Body:    "It is saved automatically and will be waiting under Resume game.",
			Confirm: "leave",
			Cancel:  "stay",
		}.View()
	case x01Calculating:
		return s.calculatorPanel(width)
	default:
		return s.idlePanel(width)
	}
}

// spotlight is the large panel: who is throwing and what they have left, drawn
// big enough to read from the oche rather than from the keyboard.
//
// It is shown while the game is live and the number it displays is current, and
// hidden whenever a dialog owns the screen. During the checkout question in
// particular it would show the score from *before* the visit that is being asked
// about, which is worse than showing nothing.
func (s *X01Game) spotlight(width, rows int) string {
	switch s.phase {
	case x01ConfirmCheckout, x01ConfirmLeave:
		return ""
	}

	id, ok := s.selectedPlayer()
	if !ok {
		// Everyone has checked out. The scoreboard already says so across the
		// full width, and repeating it in a box would only add another thing to
		// read at the moment one key needs pressing.
		return ""
	}
	p := s.session.Player(id)
	st, ok := s.session.Game().State(id)
	if !ok {
		return ""
	}

	// A way to finish is always more use than an average, and there is room for
	// one supporting line, so the average is the fallback rather than a second
	// number that could be mistaken for the big one.
	sub := ""
	switch route := s.bestCheckout(st.Score); {
	case route != "":
		sub = "CHECKOUT  " + route
	case st.Darts > 0:
		sub = fmt.Sprintf("%.1f AVG OVER %d DARTS", st.Average(), st.Darts)
	default:
		sub = "NO DARTS YET"
	}

	return components.Spotlight{
		Banner:  "NOW THROWING",
		Name:    p.Name,
		Colour:  p.Colour,
		Number:  strconv.Itoa(st.Score),
		Caption: "LEFT",
		Sub:     sub,
		Rows:    rows,
	}.View(width)
}

// bestCheckout is the single route worth putting in the large panel. The full
// list of alternatives stays in the prompt below, where it is read from the
// keyboard rather than from across the room — a numbered list is noise at that
// distance.
//
// It goes quiet on exactly the scores checkoutAdvice does, so a score with no
// finish shows nothing here either.
func (s *X01Game) bestCheckout(score int) string {
	routes := s.checkouts.Recommend(score, dartsAtVisitStart, 1)
	if len(routes) == 0 {
		return ""
	}
	return renderRoute(routes[0])
}

func (s *X01Game) scoreboard(width int) string {
	game := s.session.Game()

	nameWidth := maxInt(minInt(width-52, 20), 8)
	table := components.Table{
		Columns: []components.Column{
			{Title: "", Width: 2},
			{Title: "Player", Width: nameWidth},
			{Title: "Score", Width: 6, Right: true},
			{Title: "Darts", Width: 6, Right: true},
			{Title: "Visits", Width: 6, Right: true},
			{Title: "Avg", Width: 7, Right: true},
			{Title: "Status", Width: 18},
		},
	}

	for i, st := range game.States() {
		p := s.session.Player(st.PlayerID)
		current := i == s.cursor.Index() && !st.Finished

		status := ""
		switch {
		case st.Finished:
			// Spelled out, not just coloured: the dart count is the result.
			status = styles.SuccessText.Render(fmt.Sprintf("OUT · %d darts", st.FinishDarts))
		case current:
			status = styles.Bold.Render("← throwing")
		case st.Visits == 0:
			status = styles.Faint.Render("yet to throw")
		}

		average := "—"
		if st.Darts > 0 {
			average = fmt.Sprintf("%.1f", st.Average())
		}

		table.Rows = append(table.Rows, components.Row{
			Selected: current,
			Cells: []string{
				styles.Marker(current),
				styles.PlayerName(p.Colour, current || st.Finished).Render(
					components.Truncate(p.Name, nameWidth)),
				fmt.Sprintf("%d", st.Score),
				fmt.Sprintf("%d", st.Darts),
				fmt.Sprintf("%d", st.Visits),
				average,
				status,
			},
		})
	}

	out := table.View()
	if game.AllFinished() {
		out += "\n" + styles.SuccessText.Render(
			"Everyone has checked out — press f to finish and confirm the winner.")
	}
	return out
}

func (s *X01Game) idlePanel(width int) string {
	var b strings.Builder

	if s.lastVisit != "" {
		b.WriteString(styles.Faint.Render("Last: " + components.Truncate(s.lastVisit, maxInt(width-8, 20))))
		b.WriteString("\n\n")
	}

	if id, ok := s.selectedPlayer(); ok {
		p := s.session.Player(id)
		b.WriteString("Ready for ")
		b.WriteString(styles.PlayerName(p.Colour, true).Render(p.Name))
		b.WriteString(styles.Faint.Render(" — type their score, or ↑/↓ to pick someone else."))
		if st, ok := s.session.Game().State(id); ok {
			if advice := s.checkoutAdvice(p.Colour, st.Score); advice != "" {
				b.WriteString("\n\n")
				b.WriteString(advice)
			}
		}
		b.WriteString("\n\n")
		b.WriteString(components.CalculatorHint())
	} else {
		b.WriteString(styles.Faint.Render("Nobody left to throw. Press f to finish the game."))
	}
	return styles.Panel.Render(b.String())
}

func (s *X01Game) entryPanel(width int) string {
	id, ok := s.selectedPlayer()
	if !ok {
		return styles.ErrorText.Render("That player can no longer throw. Press esc and choose another.")
	}
	p := s.session.Player(id)
	st, _ := s.session.Game().State(id)

	var b strings.Builder
	b.WriteString(styles.PlayerName(p.Colour, true).Render(p.Name))
	b.WriteString(styles.Faint.Render(fmt.Sprintf("  ·  %d left", st.Score)))

	// Explicitly this game's average, never the player's lifetime figure.
	if st.Darts > 0 {
		b.WriteString(styles.Faint.Render(fmt.Sprintf("  ·  this game: %.2f avg over %d darts",
			st.Average(), st.Darts)))
	} else {
		b.WriteString(styles.Faint.Render("  ·  this game: no darts yet"))
	}
	if advice := s.checkoutAdvice(p.Colour, st.Score); advice != "" {
		b.WriteString("\n\n")
		b.WriteString(advice)
	}
	b.WriteString("\n\n")

	entry := s.entry
	if entry == "" {
		entry = styles.Faint.Render("___")
	} else {
		entry = styles.Bold.Render(entry)
	}
	b.WriteString("Scored  " + entry)
	// Show the sum a calculated score came from, so it can be checked before it
	// is committed.
	if s.entryFrom != "" {
		b.WriteString(styles.Faint.Render("   = " + s.entryFrom))
	}
	b.WriteString("\n")

	b.WriteString("Darts   ")
	for d := 1; d <= darts.MaxDartsPerVisit; d++ {
		label := fmt.Sprintf(" %d ", d)
		if d == s.entryDarts {
			label = styles.SelectedRow.Render("[" + fmt.Sprintf("%d", d) + "]")
		} else {
			label = styles.Faint.Render(label)
		}
		b.WriteString(label)
	}
	b.WriteString(styles.Faint.Render("  (tab to change — 3 is the default)"))
	b.WriteString("\n\n")
	b.WriteString(components.CalculatorHint())

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

// calculatorPanel renders the running sum of the individual darts.
func (s *X01Game) calculatorPanel(width int) string {
	id, ok := s.selectedPlayer()
	if !ok {
		return styles.ErrorText.Render("That player can no longer throw. Press esc and choose another.")
	}
	p := s.session.Player(id)
	st, _ := s.session.Game().State(id)

	var b strings.Builder
	b.WriteString(styles.Bold.Render("Add up the darts"))
	b.WriteString(styles.Faint.Render("  for "))
	b.WriteString(styles.PlayerName(p.Colour, true).Render(p.Name))
	b.WriteString(styles.Faint.Render(fmt.Sprintf("  ·  %d left", st.Score)))
	b.WriteString("\n\n")

	b.WriteString(s.calc.View())
	b.WriteString("\n\n")

	if s.calcErr != "" {
		b.WriteString(styles.ErrorText.Render(s.calcErr))
	} else {
		b.WriteString(styles.Faint.Render(
			"Enter puts the total in the score box; press enter again to record it."))
	}

	panel := styles.FocusPanel
	if width > 4 {
		panel = panel.Width(minInt(width-2, 78))
	}
	return panel.Render(b.String())
}

// checkoutAdvice renders the recommended ways of finishing a score.
//
// It returns nothing at all when the score cannot be finished with the darts in
// hand — the bogey numbers, anything over 170, and 1 — because a suggestion that
// cannot be thrown is worse than no suggestion. Setup shots are deliberately not
// offered here: these are checkouts or nothing.
func (s *X01Game) checkoutAdvice(colour string, score int) string {
	routes := s.checkouts.Recommend(score, dartsAtVisitStart, x01.MaxCheckoutRecommendations)
	if len(routes) == 0 {
		return ""
	}

	// The heading carries the player's colour; the routes themselves stay in the
	// default foreground so they are legible from across the room whatever the
	// palette and whatever the terminal theme.
	const label = "Checkout"
	const gap = "   "
	heading := styles.PlayerName(colour, false).Render(label)

	if len(routes) == 1 {
		return heading + gap + styles.Bold.Render(renderRoute(routes[0]))
	}

	// Alternatives line up underneath the first, so the column of routes reads
	// as one block rather than a ragged list.
	indent := strings.Repeat(" ", len(label)+len(gap))

	var b strings.Builder
	b.WriteString(heading + gap)
	for i, r := range routes {
		if i > 0 {
			b.WriteString("\n" + indent)
		}
		b.WriteString(styles.Faint.Render(fmt.Sprintf("%d. ", i+1)))
		b.WriteString(styles.Bold.Render(renderRoute(r)))
	}
	return b.String()
}

// renderRoute spaces the darts out so a route can be read at a glance.
func renderRoute(r x01.CheckoutRoute) string {
	parts := make([]string, 0, r.DartCount())
	for _, d := range r.Darts {
		parts = append(parts, d.String())
	}
	return strings.Join(parts, "  ")
}

func (s *X01Game) checkoutPanel() string {
	id, ok := s.selectedPlayer()
	if !ok {
		return ""
	}
	p := s.session.Player(id)
	st, _ := s.session.Game().State(id)

	body := fmt.Sprintf(
		"%s has %d left and you entered %s.\n\n"+
			"Only visit totals are recorded, so the application cannot tell whether\n"+
			"the last dart was a double. Under double-out rules this decides whether\n"+
			"the leg is complete or the visit busts.",
		p.Name, st.Score, s.entry)

	return components.Confirm{
		Title:   "Did they finish on a double?",
		Body:    body,
		Confirm: "yes — checkout in " + fmt.Sprintf("%d dart(s)", s.entryDarts),
		Cancel:  "no — record a bust",
	}.View()
}

// capitalise upper-cases the first letter of an error for display.
func capitalise(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	if r[0] >= 'a' && r[0] <= 'z' {
		r[0] = r[0] - ('a' - 'A')
	}
	return string(r)
}
