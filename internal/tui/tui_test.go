package tui_test

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pourmoi/dart-tui/internal/app"
	"github.com/pourmoi/dart-tui/internal/persistence/sqlite"
	"github.com/pourmoi/dart-tui/internal/tui"
)

// harness drives the root model the way Bubble Tea does: it feeds messages in
// and runs whatever commands come back, so navigation, persistence and
// rendering are all exercised together without a terminal.
type harness struct {
	t     *testing.T
	model tui.Model
	svc   *app.Services
	ctx   context.Context
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	svc := app.New(store)
	ctx := context.Background()
	h := &harness{
		t:     t,
		model: tui.New(ctx, svc, "/tmp/test/darts.db"),
		svc:   svc,
		ctx:   ctx,
	}
	h.run(h.model.Init())
	h.send(tea.WindowSizeMsg{Width: 100, Height: 30})
	return h
}

func (h *harness) send(msg tea.Msg) {
	h.t.Helper()
	next, cmd := h.model.Update(msg)
	m, ok := next.(tui.Model)
	if !ok {
		h.t.Fatalf("Update returned %T, want tui.Model", next)
	}
	h.model = m
	h.run(cmd)
}

// run executes a command and feeds its message back, mirroring the event loop.
func (h *harness) run(cmd tea.Cmd) {
	h.t.Helper()
	if cmd == nil {
		return
	}
	msg := cmd()
	switch m := msg.(type) {
	case nil:
		return
	case tea.BatchMsg:
		for _, c := range m {
			h.run(c)
		}
	case tea.QuitMsg:
		return
	default:
		h.send(msg)
	}
}

// press sends a key by name. Single characters are sent as runes.
func (h *harness) press(keys ...string) {
	h.t.Helper()
	for _, k := range keys {
		h.send(keyMsg(k))
	}
}

// typeText sends each character of a string as a separate keystroke.
func (h *harness) typeText(s string) {
	h.t.Helper()
	for _, r := range s {
		h.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func keyMsg(k string) tea.KeyMsg {
	switch k {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
	}
}

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// view returns the rendered screen with styling stripped, for assertions.
func (h *harness) view() string {
	h.t.Helper()
	return ansi.ReplaceAllString(h.model.View(), "")
}

func (h *harness) assertContains(substrings ...string) {
	h.t.Helper()
	v := h.view()
	for _, want := range substrings {
		if !strings.Contains(v, want) {
			h.t.Errorf("screen does not contain %q.\n---\n%s\n---", want, v)
		}
	}
}

func (h *harness) assertNotContains(substr string) {
	h.t.Helper()
	if v := h.view(); strings.Contains(v, substr) {
		h.t.Errorf("screen unexpectedly contains %q.\n---\n%s\n---", substr, v)
	}
}

// addPlayer creates a player through the roster screen, as an operator would.
func (h *harness) addPlayer(name string) {
	h.t.Helper()
	h.press("p") // Players
	h.press("n") // New player
	h.typeText(name)
	h.press("enter")
	h.press("esc") // back to the menu
}

// ------------------------------------------------------------------- basics

func TestMenuRenders(t *testing.T) {
	h := newHarness(t)
	h.assertContains("Darts", "Start game", "Resume game", "Players", "Statistics", "Exit")
	// The data location is shown so nobody has to hunt for the database.
	h.assertContains("darts.db")
}

func TestTinyTerminalIsReportedNotCorrupted(t *testing.T) {
	h := newHarness(t)
	h.send(tea.WindowSizeMsg{Width: 20, Height: 5})
	h.assertContains("Terminal too small")

	// And it recovers when the window is resized back.
	h.send(tea.WindowSizeMsg{Width: 100, Height: 30})
	h.assertContains("Start game")
}

func TestResizeIsHandledGracefully(t *testing.T) {
	h := newHarness(t)
	for _, size := range []tea.WindowSizeMsg{
		{Width: 52, Height: 14}, // the documented minimum
		{Width: 80, Height: 24}, // the classic default
		{Width: 120, Height: 40},
		{Width: 200, Height: 60},
	} {
		h.send(size)
		if v := h.view(); v == "" {
			t.Errorf("no output at %dx%d", size.Width, size.Height)
		}
	}
}

// ------------------------------------------------------------------ players

func TestAddAndArchivePlayer(t *testing.T) {
	h := newHarness(t)

	h.press("p")
	h.assertContains("No players yet")

	h.press("n")
	h.typeText("Alice")
	h.press("enter")
	h.assertContains("Alice")

	// Duplicate names are rejected in place rather than crashing.
	h.press("n")
	h.typeText("Alice")
	h.press("enter")
	h.assertContains("already exists")
	h.press("esc")

	// Open the profile and archive her.
	h.press("enter")
	h.assertContains("Lifetime summary", "Cricket — batting", "Cricket — bowling", "Recent games")

	h.press("a") // archive
	h.assertContains("Archive Alice?", "Nothing is deleted")
	h.press("y")
	h.assertContains("[archived]")

	// She disappears from the active roster but is still there under the archive.
	h.press("esc")
	h.assertNotContains("Alice")
	h.press("a")
	h.assertContains("Alice", "[archived]")
}

func TestChangePlayerColour(t *testing.T) {
	h := newHarness(t)
	h.addPlayer("Alice")

	h.press("p", "enter") // roster → profile
	h.press("c")          // change colour
	h.assertContains("Choose a display colour")
	h.press("right", "enter")
	h.assertContains("Colour updated")
}

func TestRenamePlayerKeepsHistory(t *testing.T) {
	h := newHarness(t)
	h.addPlayer("Alice")

	h.press("p", "enter", "e")
	h.assertContains("Edit name", "keeps every recorded game")
	// Clear "Alice" and type a new name.
	for i := 0; i < 10; i++ {
		h.press("backspace")
	}
	h.typeText("Alicia")
	h.press("enter")
	h.assertContains("Alicia")
}

// --------------------------------------------------------------------- X01

// A full 501 game driven entirely through the interface.
func TestX01GameFlow(t *testing.T) {
	h := newHarness(t)
	h.addPlayer("Alice")
	h.addPlayer("Bob")

	h.press("s") // Start game
	h.assertContains("Which game?", "501", "301", "Cricket")

	h.press("5") // 501
	h.assertContains("choose players", "Alice", "Bob")

	h.press("space")         // select Alice
	h.press("down", "space") // select Bob
	h.assertContains("2 players selected")
	h.press("enter")

	// The scoreboard starts everyone on 501.
	h.assertContains("Player", "Score", "Darts", "Visits", "Avg", "Status")
	h.assertContains("501")

	// Typing a digit goes straight into score entry.
	h.typeText("140")
	h.assertContains("Scored", "140", "Darts")
	h.press("enter")
	h.assertContains("361") // 501 - 140

	// Bob has not thrown at all; the game never advanced to him by itself.
	h.assertContains("yet to throw")

	// Impossible scores are refused with a readable message, not a crash.
	h.typeText("179")
	h.press("enter")
	h.assertContains("cannot be thrown")
	h.press("esc")

	// Undo takes the visit back.
	h.press("u")
	h.assertContains("Undid")
	h.assertContains("501")
}

// One player completing an entire leg before the other throws is the office's
// normal way of playing and must work end to end.
func TestX01OnePlayerFinishesEntireLegFirst(t *testing.T) {
	h := newHarness(t)
	h.addPlayer("Alice")
	h.addPlayer("Bob")

	h.press("s", "5")
	h.press("space", "down", "space", "enter")

	// Nine visits of 55 leaves Alice on 6.
	for i := 0; i < 9; i++ {
		h.typeText("55")
		h.press("enter")
	}
	h.assertContains("6")

	// Check out with a single dart: type the score, switch to one dart, record.
	h.typeText("6")
	h.press("tab", "tab") // 3 → 2 → 1
	h.press("enter")

	// Landing on zero asks whether it was a double.
	h.assertContains("Did they finish on a double?")
	h.press("y")
	h.assertContains("checked out")

	// The game has NOT ended: Bob can still play his leg.
	h.assertContains("OUT · 28 darts")
	h.assertNotContains("Finish game —")

	// Bob throws once, then the operator finishes manually.
	h.typeText("100")
	h.press("enter")

	h.press("f")
	h.assertContains("Finish game", "Who won?", "suggested")
	h.press("enter")

	// Back at the menu with the result saved.
	h.assertContains("wins", "Start game")
}

// The 28 versus 30 dart distinction has to survive all the way to the profile.
func TestX01FewestDartsRecordedInProfile(t *testing.T) {
	h := newHarness(t)
	h.addPlayer("Alice")

	h.press("s", "5")
	h.press("space", "enter")

	for i := 0; i < 9; i++ {
		h.typeText("55")
		h.press("enter")
	}
	h.typeText("6")
	h.press("tab", "tab") // one dart
	h.press("enter")
	h.press("y") // confirm the double
	h.press("f")
	h.press("enter") // accept the suggested winner

	// Her profile shows the 28-dart leg and a matching average.
	h.press("p", "enter")
	h.assertContains("Fewest darts to finish", "28")
	h.assertContains("Legs completed", "1")
	h.assertContains("Games won", "1")
}

func TestX01BustIsReportedAndScoreRestored(t *testing.T) {
	h := newHarness(t)
	h.addPlayer("Alice")

	h.press("s", "3") // 301
	h.press("space", "enter")

	h.typeText("180")
	h.press("enter")
	h.assertContains("121")

	// 140 from 121 overshoots.
	h.typeText("140")
	h.press("enter")
	h.assertContains("BUST")
	h.assertContains("121") // the score is restored
}

func TestX01LeaveAndResume(t *testing.T) {
	h := newHarness(t)
	h.addPlayer("Alice")

	h.press("s", "5")
	h.press("space", "enter")
	h.typeText("100")
	h.press("enter")

	// Leaving is confirmed, then the game waits under Resume.
	h.press("esc")
	h.assertContains("Leave this game?")
	h.press("y")
	h.assertContains("Start game", "1 game waiting")

	h.press("r")
	h.assertContains("501", "Alice", "1 visits recorded")
	h.press("enter")
	h.assertContains("401") // exactly where it was left
}

// The calculator adds up individual darts, which is where mental arithmetic at
// the board goes wrong.
func TestX01CalculatorAddsDartsUp(t *testing.T) {
	h := newHarness(t)
	h.addPlayer("Alice")

	h.press("s", "5")
	h.press("space", "enter")

	// "=" opens the sum.
	h.press("=")
	h.assertContains("Add up the darts", "Alice", "Sum", "Total")

	h.typeText("60")
	h.press("+")
	h.typeText("30")
	h.press("+")
	h.typeText("21")
	// The running total is visible while typing.
	h.assertContains("60 + 30 + 21", "111")

	// Enter puts the total into the score box rather than recording it.
	h.press("enter")
	h.assertContains("Scored", "111")
	h.assertContains("= 60 + 30 + 21")
	h.assertContains("501") // nothing recorded yet

	// A second Enter records it.
	h.press("enter")
	h.assertContains("390") // 501 - 111
}

// The keypad has a "+" but no "=", so both open the calculator.
func TestX01CalculatorOpensFromThePlusKey(t *testing.T) {
	h := newHarness(t)
	h.addPlayer("Alice")

	h.press("s", "5")
	h.press("space", "enter")

	h.press("+")
	h.assertContains("Add up the darts")
	h.typeText("60")
	h.press("+")
	h.typeText("60")
	h.press("enter", "enter")
	h.assertContains("381") // 501 - 120
}

// Pressing "+" part-way through typing a score continues it as a sum.
func TestX01CalculatorContinuesATypedScore(t *testing.T) {
	h := newHarness(t)
	h.addPlayer("Alice")

	h.press("s", "5")
	h.press("space", "enter")

	h.typeText("140")
	h.press("+")
	h.assertContains("Add up the darts", "140")
	h.typeText("41")
	h.assertContains("181")

	// 181 cannot be thrown, and the sum is corrected where it was made.
	h.press("enter")
	h.assertContains("cannot be thrown")
	h.assertContains("Add up the darts")

	// Fix it and carry on: drop the "41" but keep the "+".
	h.press("backspace", "backspace")
	h.typeText("40")
	h.assertContains("180")
	h.press("enter")
	h.assertContains("Scored", "180")
	h.press("enter")
	h.assertContains("321") // 501 - 180
}

func TestX01CalculatorCanBeCancelled(t *testing.T) {
	h := newHarness(t)
	h.addPlayer("Alice")

	h.press("s", "5")
	h.press("space", "enter")

	h.press("=")
	h.typeText("60")
	h.press("esc")
	h.assertNotContains("Add up the darts")
	// Back at the score box with nothing recorded.
	h.assertContains("501")
}

// A sum that lands on zero still goes through the double-out question.
func TestX01CalculatorCheckoutStillAsksAboutTheDouble(t *testing.T) {
	h := newHarness(t)
	h.addPlayer("Alice")

	h.press("s", "3") // 301
	h.press("space", "enter")

	// Get down to 40.
	h.typeText("180")
	h.press("enter")
	h.typeText("81")
	h.press("enter")
	h.assertContains("40")

	// Check out with a calculated 20 + 20.
	h.press("=")
	h.typeText("20")
	h.press("+")
	h.typeText("20")
	h.press("enter")
	h.assertContains("Scored", "40")
	h.press("enter")
	h.assertContains("Did they finish on a double?")
	h.press("y")
	h.assertContains("checked out")
}

// ----------------------------------------------------------------- cricket

func TestCricketGameFlow(t *testing.T) {
	h := newHarness(t)
	h.addPlayer("Alice")
	h.addPlayer("Bob")
	h.addPlayer("Carol")

	h.press("s", "c")
	h.assertContains("Team A", "throwing order")

	// Team A: Alice.  Team B: Bob then Carol, in that order.
	h.press("space", "enter")
	h.assertContains("Team B")
	h.press("down", "space")
	h.press("down", "space")
	h.press("enter")

	h.assertContains("bats first")
	h.press("enter")

	h.assertContains("wickets", "Wickets")
	h.press("backspace")
	h.typeText("2")
	h.press("enter")

	// The batter always throws first, and the screen says so in words.
	h.assertContains("Leg 1 of 2", "innings 1", "BATTING", "Alice", "Raw score")
	// Each team bats twice, so leg 1 has no target to chase.
	h.assertNotContains("chasing")

	// 100 raw scores 60 runs under the house rule.
	h.typeText("100")
	h.assertContains("60 runs")
	h.press("enter")

	// Now it wants a bowling result, and offers the keypad digits.
	h.assertContains("BOWLING", "Bob", "no wicket", "outer bull", "inner bull")
	h.assertContains("1 = no wicket")

	// 3 is the inner bull: two wickets, which ends a two-wicket innings.
	h.press("3")
	h.assertContains("2 wickets")
	h.press("enter")

	// Leg 1 continues with Team B batting; it does NOT end the match.
	h.assertContains("Leg 1 of 2", "innings 2", "BATTING", "Bob")
	h.assertNotContains("Game over")

	h.typeText("60") // 20 runs
	h.press("enter")
	h.assertContains("BOWLING", "Alice")
	h.press("3", "enter")

	// Leg 1 is over. Team B trail 20 to 60, so they bat first in leg 2.
	h.assertContains("Leg 2 of 2", "innings 3", "BATTING", "Bob")
	h.assertContains("lead by 40")

	// Bowled out again short of Team A's total: Team A win without batting.
	h.typeText("60")
	h.press("enter")
	h.assertContains("BOWLING", "Alice")
	h.press("3", "enter")

	h.assertContains("Game over", "Team A win")
	h.assertContains("did not need to bat again")
}

// A match that goes the full four innings, decided by a chase on aggregate.
func TestCricketChaseWins(t *testing.T) {
	h := newHarness(t)
	h.addPlayer("Alice")
	h.addPlayer("Bob")

	h.press("s", "c")
	h.press("space", "enter")         // Team A: Alice
	h.press("down", "space", "enter") // Team B: Bob
	h.press("enter")                  // Team A bats first
	h.press("backspace")
	h.typeText("1")
	h.press("enter")

	// Innings 1: Team A make 20. 2 is the outer bull, one wicket.
	h.typeText("60")
	h.press("enter")
	h.press("2")
	h.assertContains("1 wicket")
	h.press("enter")

	// Innings 2: Team B make 60 and lead.
	h.assertContains("Leg 1 of 2", "innings 2", "BATTING", "Bob")
	h.typeText("100")
	h.press("enter")
	h.press("2", "enter")

	// Leg 2: Team A trail, so they bat first, and overtake.
	h.assertContains("Leg 2 of 2", "innings 3", "BATTING", "Alice")
	h.typeText("140") // 100 runs, aggregate 120
	h.press("enter")
	h.press("2", "enter")

	// Team A lead, so Team B bat last, chasing on aggregate.
	h.assertContains("innings 4", "BATTING", "Bob")
	h.assertContains("chasing 120", "needs 61")

	h.typeText("140") // 100 runs: aggregate 160 beats 120
	h.press("enter")
	h.assertContains("Game over", "Team B win")

	// Undo reopens the finished match.
	h.press("u")
	h.assertContains("Undid")
	h.assertNotContains("Game over")
	h.assertContains("chasing 120")
}

// Enter is accepted however the terminal reports it, so the keypad Enter key
// works alongside the main Return key.
func TestKeypadEnterIsAccepted(t *testing.T) {
	h := newHarness(t)
	h.addPlayer("Alice")

	h.press("s", "5")
	h.press("space")
	// A line feed, which is what some terminals send for the keypad Enter.
	h.send(tea.KeyMsg{Type: tea.KeyCtrlJ})
	h.assertContains("Player", "Score", "501")

	h.typeText("100")
	h.send(tea.KeyMsg{Type: tea.KeyCtrlJ})
	h.assertContains("401")
}

func TestCricketOutOfTurnIsImpossibleThroughTheUI(t *testing.T) {
	h := newHarness(t)
	h.addPlayer("Alice")
	h.addPlayer("Bob")

	h.press("s", "c")
	h.press("space", "enter")
	h.press("down", "space", "enter")
	h.press("enter")
	h.press("enter") // accept the default wicket count

	// The screen offers batting only; there is no way to choose a thrower.
	h.assertContains("BATTING", "Alice")
	h.assertNotContains("choose who threw")
}

func TestCricketAbandon(t *testing.T) {
	h := newHarness(t)
	h.addPlayer("Alice")
	h.addPlayer("Bob")

	h.press("s", "c")
	h.press("space", "enter")
	h.press("down", "space", "enter")
	h.press("enter", "enter")

	h.typeText("60")
	h.press("enter")

	h.press("f")
	h.assertContains("Abandon this game?", "cannot be undone")
	h.press("y")
	h.assertContains("abandoned")
}

// The same calculator is available for cricket batting scores.
func TestCricketCalculatorAddsDartsUp(t *testing.T) {
	h := newHarness(t)
	h.addPlayer("Alice")
	h.addPlayer("Bob")

	h.press("s", "c")
	h.press("space", "enter")         // Team A: Alice
	h.press("down", "space", "enter") // Team B: Bob
	h.press("enter", "enter")         // Team A bats first, default wickets

	h.assertContains("BATTING", "Alice")

	h.press("=")
	h.assertContains("Add up the darts", "Sum", "Total")

	h.typeText("60")
	h.press("+")
	h.typeText("41")
	h.assertContains("60 + 41", "101")
	// The house rule is shown here too, so the runs are no surprise.
	h.assertContains("Runs", "61")

	h.press("enter")
	h.assertContains("Raw score", "101")
	h.assertContains("= 60 + 41")

	h.press("enter")
	h.assertContains("Alice scored 101")
}

// An impossible sum is corrected inside the cricket calculator, as in X01.
func TestCricketCalculatorRejectsAnImpossibleTotal(t *testing.T) {
	h := newHarness(t)
	h.addPlayer("Alice")
	h.addPlayer("Bob")

	h.press("s", "c")
	h.press("space", "enter")
	h.press("down", "space", "enter")
	h.press("enter", "enter")

	h.press("=")
	h.typeText("180")
	h.press("+")
	h.typeText("60")
	h.press("enter")
	h.assertContains("cannot be thrown")
	h.assertContains("Add up the darts")
}

// The calculator is only offered where a score is typed, not for bowling.
func TestCricketCalculatorNotOfferedWhileBowling(t *testing.T) {
	h := newHarness(t)
	h.addPlayer("Alice")
	h.addPlayer("Bob")

	h.press("s", "c")
	h.press("space", "enter")
	h.press("down", "space", "enter")
	h.press("enter", "enter")

	h.typeText("60")
	h.press("enter")
	h.assertContains("BOWLING")
	h.assertNotContains("add the darts up")

	// "=" does nothing here rather than opening a calculator for wickets.
	h.press("=")
	h.assertNotContains("Add up the darts")
	h.assertContains("BOWLING")
}

// Both game screens advertise the calculator, so it is discoverable without
// reading the manual.
func TestCalculatorIsAdvertisedOnBothGameScreens(t *testing.T) {
	t.Run("x01", func(t *testing.T) {
		h := newHarness(t)
		h.addPlayer("Alice")
		h.press("s", "5")
		h.press("space", "enter")

		// Visible before entry begins...
		h.assertContains("Press = or + to add the darts up")
		h.assertContains("add darts up") // and in the footer

		// ...and while typing a score.
		h.typeText("60")
		h.assertContains("Press = or + to add the darts up")
	})

	t.Run("cricket", func(t *testing.T) {
		h := newHarness(t)
		h.addPlayer("Alice")
		h.addPlayer("Bob")
		h.press("s", "c")
		h.press("space", "enter")
		h.press("down", "space", "enter")
		h.press("enter", "enter")

		h.assertContains("Press = or + to add the darts up")
		h.assertContains("add darts up") // footer
	})
}

// ------------------------------------------------------------- statistics

func TestLeaderboardSortAndProfileNavigation(t *testing.T) {
	h := newHarness(t)
	h.addPlayer("Alice")
	h.addPlayer("Bob")

	h.press("t")
	h.assertContains("Statistics", "Player", "Won", "Played", "X01 avg", "Runs", "Wkts")
	h.assertContains("not comparable")

	h.press("s") // change sort
	h.assertContains("Sorted by")

	h.press("enter") // open the highlighted player's profile
	h.assertContains("Lifetime summary")
}

func TestResumeListEmptyState(t *testing.T) {
	h := newHarness(t)
	h.press("r")
	h.assertContains("No unfinished games")
}

// Starting a game with nobody selected must warn rather than proceed.
func TestX01RequiresAtLeastOnePlayer(t *testing.T) {
	h := newHarness(t)
	h.addPlayer("Alice")

	h.press("s", "5")
	h.press("enter") // nothing selected
	h.assertContains("Select at least one player")
}

func TestCricketRequiresPlayersOnBothTeams(t *testing.T) {
	h := newHarness(t)
	h.addPlayer("Alice")

	h.press("s", "c")
	h.press("enter") // Team A empty
	h.assertContains("Pick at least one player")
}
