package tui_test

import (
	"context"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/BigJabber/dart-tui/internal/domain/darts"

	"github.com/BigJabber/dart-tui/internal/app"
	"github.com/BigJabber/dart-tui/internal/persistence/sqlite"
	"github.com/BigJabber/dart-tui/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
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

// resize changes the terminal size, for the screens that lay themselves out
// from the space they are given.
func (h *harness) resize(width, height int) {
	h.t.Helper()
	h.send(tea.WindowSizeMsg{Width: width, Height: height})
}

// assertFitsScreen checks the rendered view is no taller than the terminal.
//
// Bubble Tea keeps the LAST height lines of a view that overflows, so a screen
// one row too tall silently loses its header and its scoreboard with no other
// symptom. That failure is quiet enough to be worth a test of its own.
func (h *harness) assertFitsScreen(height int) {
	h.t.Helper()
	v := h.view()
	if got := strings.Count(v, "\n") + 1; got > height {
		h.t.Errorf("view is %d rows in a %d-row terminal.\n---\n%s\n---", got, height, v)
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

// startAt opens a 501 game for one player and reduces them to the score wanted,
// so the checkout panel can be inspected at a specific remaining score.
func startAt(t *testing.T, remaining int) *harness {
	t.Helper()
	h := newHarness(t)
	h.addPlayer("James")
	h.press("s", "5")
	h.press("space", "enter")

	// Three visits of at most 180 will reach anything down to 2 from 501.
	for _, points := range visitsToReach(501, remaining) {
		h.typeText(strconv.Itoa(points))
		h.press("enter")
	}
	return h
}

// visitsToReach splits the reduction into legal visits of 180 or less.
func visitsToReach(from, to int) []int {
	var out []int
	for from > to {
		step := from - to
		if step > 180 {
			step = 180
		}
		// 179 and its neighbours cannot be thrown; step down to a total that can.
		for step > 0 && !dartsReachable(step) {
			step--
		}
		out = append(out, step)
		from -= step
	}
	return out
}

func dartsReachable(points int) bool { return darts.Reachable(points, 3) }

// Checkout suggestions appear for the active player when their score can be
// finished.
func TestX01ShowsCheckoutSuggestions(t *testing.T) {
	h := startAt(t, 170)
	h.assertContains("170", "Checkout", "T20  T20  Bull")
}

// Several sensible finishes are offered, numbered.
func TestX01ShowsCheckoutAlternatives(t *testing.T) {
	h := startAt(t, 80)
	h.assertContains("Checkout", "1. T20  D10", "2. T16  D16")
}

// A score with one sensible finish is not padded out with alternatives.
func TestX01ShowsASingleRouteWithoutNumbering(t *testing.T) {
	h := startAt(t, 170)
	h.assertContains("Checkout", "T20  T20  Bull")
	h.assertNotContains("1. T20")
	// The trailing space is deliberate: what this test means is "no list
	// numbering", and a bare "2." would also match any decimal ending in a two.
	h.assertNotContains("2. ")
}

// The panel is hidden entirely when there is no checkout, rather than showing
// something misleading.
func TestX01HidesCheckoutWhenThereIsNone(t *testing.T) {
	for _, score := range []int{169, 168, 166, 165, 163, 162, 159} {
		t.Run(strconv.Itoa(score), func(t *testing.T) {
			h := startAt(t, score)
			h.assertContains(strconv.Itoa(score))
			h.assertNotContains("Checkout")
		})
	}
}

// A fresh 501 is far too high to finish, so nothing is shown.
func TestX01HidesCheckoutAtTheStartOfALeg(t *testing.T) {
	h := newHarness(t)
	h.addPlayer("James")
	h.press("s", "5")
	h.press("space", "enter")
	h.assertContains("501")
	h.assertNotContains("Checkout")
}

// The suggestion stays visible while a score is being typed, alongside the
// current game average rather than in place of it.
func TestX01ShowsCheckoutDuringScoreEntry(t *testing.T) {
	h := startAt(t, 170)
	h.typeText("60")
	h.assertContains("this game:", "Checkout", "T20  T20  Bull", "Scored", "60")
}

// One-dart finishes show just the double.
func TestX01ShowsOneDartFinishes(t *testing.T) {
	cases := map[int]string{40: "D20", 32: "D16", 50: "Bull"}
	for score, want := range cases {
		t.Run(strconv.Itoa(score), func(t *testing.T) {
			h := startAt(t, score)
			h.assertContains("Checkout", want)
		})
	}
}

// The suggestion follows the player whose turn is being recorded.
func TestX01CheckoutFollowsTheSelectedPlayer(t *testing.T) {
	h := newHarness(t)
	h.addPlayer("James")
	h.addPlayer("Bob")
	h.press("s", "5")
	h.press("space", "down", "space", "enter")

	// The roster is alphabetical, so the cursor starts on Bob. Take him to 170
	// and leave James untouched on 501.
	h.typeText("140")
	h.press("enter")
	h.typeText("140")
	h.press("enter")
	h.typeText("51")
	h.press("enter")
	h.assertContains("Ready for Bob", "Checkout", "T20  T20  Bull")

	// Selecting James, who is still on 501 and cannot finish, hides the panel.
	h.press("down")
	h.assertContains("Ready for James")
	h.assertNotContains("Checkout")
}

// ------------------------------------------------- the large "throwing" panel

// On a terminal with room to spare the panel names the thrower and draws their
// remaining score out of blocks.
func TestX01SpotlightNamesTheThrower(t *testing.T) {
	h := startAt(t, 170)
	h.resize(120, 50)

	h.assertContains("NOW THROWING", "LEFT", "CHECKOUT")
	// The name and the number are both drawn in blocks here, which no assertion
	// can read. Their plain forms have to survive in the table and the prompt
	// below — everything that shrinks or disappears depends on that, so it is
	// pinned here.
	h.assertContains("170", "James")
	h.assertFitsScreen(50)
}

// One size down there is no longer room for a block name, and it falls back to
// spaced capitals rather than to nothing.
func TestX01SpotlightNameFallsBackToText(t *testing.T) {
	h := startAt(t, 170)
	h.resize(120, 40)
	h.assertContains("NOW THROWING", "J A M E S")
	h.assertFitsScreen(40)
}

// The panel gives up its size before the prompt gives up any of its own, and
// then disappears rather than being clipped.
func TestX01SpotlightDegradesOnAShortTerminal(t *testing.T) {
	h := startAt(t, 170)

	h.resize(100, 30)
	h.assertContains("NOW THROWING", "170 LEFT")
	h.assertFitsScreen(30)

	h.resize(100, 22)
	h.assertNotContains("NOW THROWING")
	// The things the operator actually types into are all still there.
	h.assertContains("Player", "Score", "Ready for James")
}

// The panel must not move or change size when a visit is recorded: a wall
// display that jumps a row after every throw is worse than one that does not.
func TestX01SpotlightDoesNotMoveWhenAVisitIsRecorded(t *testing.T) {
	h := startAt(t, 170)
	h.resize(100, 30)
	before := strings.Count(h.view(), "\n")

	h.typeText("20")
	h.press("enter") // leaves a status message behind
	if after := strings.Count(h.view(), "\n"); after != before {
		t.Errorf("the screen was %d rows and became %d after a visit", before+1, after+1)
	}
}

// The remaining score shown beside "did they finish on a double?" would be the
// one from before the visit being asked about, so the panel gets out of the way.
func TestX01SpotlightIsHiddenDuringConfirmations(t *testing.T) {
	h := startAt(t, 40)
	h.resize(120, 50)
	h.assertContains("NOW THROWING")

	h.typeText("40")
	h.press("tab", "tab") // one dart
	h.press("enter")
	h.assertContains("Did they finish on a double?")
	h.assertNotContains("NOW THROWING")

	h.press("esc") // back to the score
	h.assertContains("NOW THROWING")
}

// Cricket shows the innings total until the last innings, and the runs needed
// once there is something to chase.
func TestCricketSpotlightShowsTheInningsThenTheChase(t *testing.T) {
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
	h.resize(120, 50)

	// Innings 1 is not a chase, so the panel counts the innings instead.
	h.assertContains("▶ BATTING", "Alice", "RUNS", "TEAM A BATTING")
	h.assertNotContains("TO WIN")
	h.assertNotContains("chasing")
	h.assertFitsScreen(50)

	// The number belongs to the innings, not the thrower, so it stays put when
	// the bowler comes on — and the panel names the side it belongs to.
	h.typeText("60")
	h.press("enter")
	h.assertContains("▶ BOWLING", "Bob", "RUNS", "TEAM A BATTING")
	h.press("2", "enter")

	// Innings 2 and 3.
	h.typeText("100")
	h.press("enter")
	h.press("2", "enter")
	h.typeText("140")
	h.press("enter")
	h.press("2", "enter")

	// Innings 4: Team B chase 120 and need 61.
	h.assertContains("innings 4", "TO WIN", "TEAM B NEED 61", "TARGET 120")
	h.assertContains("chasing 120", "needs 61")
	h.assertFitsScreen(50)
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
