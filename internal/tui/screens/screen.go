// Package screens holds the Bubble Tea models for each screen.
//
// Screens consume the application layer and render its results. None of them
// contains a darts rule: a screen asks the engine what a visit did and displays
// the answer, so the same rules apply whether a game is played, resumed from
// disk, or exercised by a unit test with no terminal attached.
//
// Data is read from the store synchronously rather than through commands. The
// database is a local SQLite file opened by a single-user desktop application,
// so a read costs microseconds; routing every list through an asynchronous
// message round trip would add a great deal of code for no perceptible gain.
// Anything genuinely slow would need a command, and none of it is.
package screens

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pourmoi/dart-tui/internal/app"
	"github.com/pourmoi/dart-tui/internal/tui/components"
)

// Context is what every screen needs to do its job.
type Context struct {
	Services *app.Services
	// Ctx bounds database work; it is cancelled when the application exits.
	Ctx context.Context
	// DBPath is shown on the main menu so the operator knows where their data
	// lives without hunting for it.
	DBPath string
}

// Screen is one view in the application.
type Screen interface {
	// Init returns any command to run when the screen becomes visible.
	Init() tea.Cmd
	// Update handles a message and returns the screen to display next. A screen
	// normally returns itself.
	Update(tea.Msg) (Screen, tea.Cmd)
	// View renders the screen body into the space available between the header
	// and the footer.
	View(width, height int) string
	// Title is shown in the header.
	Title() string
	// Hints are the contextual key bindings shown in the footer.
	Hints() []components.KeyHint
}

// ------------------------------------------------------------- navigation

// PushMsg asks the router to open a screen on top of the current one.
type PushMsg struct{ Screen Screen }

// PopMsg asks the router to go back one screen.
type PopMsg struct{}

// PopToRootMsg returns to the main menu, used when a game ends.
type PopToRootMsg struct{}

// ReplaceMsg swaps the current screen for another without growing the stack.
type ReplaceMsg struct{ Screen Screen }

// QuitMsg exits the application.
type QuitMsg struct{}

// StatusKind classifies a status line.
type StatusKind int

// Status line kinds. Each is rendered with a word as well as a colour.
const (
	StatusInfo StatusKind = iota
	StatusSuccess
	StatusWarning
	StatusError
)

// StatusMsg shows a transient message under the header.
type StatusMsg struct {
	Text string
	Kind StatusKind
}

// Push opens a screen.
func Push(s Screen) tea.Cmd {
	return func() tea.Msg { return PushMsg{Screen: s} }
}

// Pop goes back one screen.
func Pop() tea.Cmd {
	return func() tea.Msg { return PopMsg{} }
}

// PopToRoot returns to the main menu.
func PopToRoot() tea.Cmd {
	return func() tea.Msg { return PopToRootMsg{} }
}

// Replace swaps the current screen.
func Replace(s Screen) tea.Cmd {
	return func() tea.Msg { return ReplaceMsg{Screen: s} }
}

// Quit exits the application.
func Quit() tea.Cmd {
	return func() tea.Msg { return QuitMsg{} }
}

// Status shows an informational message.
func Status(text string) tea.Cmd {
	return func() tea.Msg { return StatusMsg{Text: text, Kind: StatusInfo} }
}

// Success shows a confirmation message.
func Success(text string) tea.Cmd {
	return func() tea.Msg { return StatusMsg{Text: text, Kind: StatusSuccess} }
}

// Warn shows a warning message.
func Warn(text string) tea.Cmd {
	return func() tea.Msg { return StatusMsg{Text: text, Kind: StatusWarning} }
}

// Fail shows an error. Errors are always recoverable input problems: they are
// reported here and the operator corrects the entry, never a crash.
func Fail(err error) tea.Cmd {
	if err == nil {
		return nil
	}
	return func() tea.Msg { return StatusMsg{Text: err.Error(), Kind: StatusError} }
}
