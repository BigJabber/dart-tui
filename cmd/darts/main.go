// Command darts is a terminal darts scoreboard for the office.
//
// It keeps score for 501, 301 and the office's own cricket variant, stores every
// visit in a local SQLite database, and derives player statistics from those
// visits. It needs no server, no browser and no administrator rights.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/BigJabber/dart-tui/internal/app"
	"github.com/BigJabber/dart-tui/internal/persistence"
	"github.com/BigJabber/dart-tui/internal/persistence/sqlite"
	"github.com/BigJabber/dart-tui/internal/tui"
)

// version is stamped at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "darts:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		portable    = flag.Bool("portable", false, "keep the database beside the executable instead of in the user's application data directory")
		dbPath      = flag.String("db", "", "path to the database file (overrides every other location)")
		showVersion = flag.Bool("version", false, "print the version and exit")
		showPath    = flag.Bool("where", false, "print the database location and exit")
	)
	flag.Usage = usage
	flag.Parse()

	if *showVersion {
		fmt.Printf("darts %s\n", version)
		return nil
	}

	location, err := persistence.Resolve(persistence.Options{
		Portable: *portable,
		Override: *dbPath,
	})
	if err != nil {
		return err
	}
	if *showPath {
		fmt.Println(location.Path)
		return nil
	}

	store, err := sqlite.Open(location.Path)
	if err != nil {
		return fmt.Errorf("open %s: %w", location.Path, err)
	}
	services := app.New(store)
	defer func() { _ = services.Close() }()

	// Cancelled on Ctrl+C or a terminal close, so in-flight database work stops
	// cleanly. Every visit is already committed by the time it is on screen, so
	// there is nothing to flush on the way out.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := services.Ping(ctx); err != nil {
		return fmt.Errorf("database at %s is not usable: %w", location.Path, err)
	}

	return tui.Run(ctx, services, location.Path)
}

func usage() {
	out := flag.CommandLine.Output()
	fmt.Fprintf(out, `darts %s — office darts scoreboard

Usage:
  darts [flags]

Games:
  501 and 301   Any number of players, thrown in any order, double out.
  Cricket       The office variant: two teams, batting and bowling innings.

Flags:
`, version)
	flag.PrintDefaults()
	fmt.Fprintf(out, `
Data:
  By default the database lives in the per-user application data directory
  (%%LOCALAPPDATA%%\DartsTUI on Windows). Use -where to print the exact path,
  -portable to keep it beside the executable, or the %s environment variable
  to point somewhere else.
`, persistence.EnvDBPath)
}
