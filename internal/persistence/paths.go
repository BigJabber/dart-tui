// Package persistence resolves where the application keeps its data.
package persistence

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// DBFileName is the database file name in whichever directory is chosen.
const DBFileName = "darts.db"

// appDirName is the per-user directory the database lives in.
const appDirName = "DartsTUI"

// EnvDBPath lets an operator point the application at a specific database file,
// which is handy for testing and for sharing a database on a network drive.
const EnvDBPath = "DARTS_DB"

// Location describes where the database is and why.
type Location struct {
	Path string
	// Portable reports whether the database sits beside the executable rather
	// than in the per-user application data directory.
	Portable bool
	// Source explains the choice, for display on the main screen.
	Source string
}

// Options control database placement.
type Options struct {
	// Portable keeps the database beside the executable, for running from a USB
	// stick or a shared folder. Off by default: on Windows the executable may
	// sit somewhere the user cannot write.
	Portable bool
	// Override is an explicit path that beats every other rule.
	Override string
}

// Resolve decides where the database should live.
//
// Precedence, highest first:
//
//  1. An explicit path passed on the command line.
//  2. The DARTS_DB environment variable.
//  3. Portable mode: beside the executable.
//  4. The per-user application data directory. On Windows that is
//     %LOCALAPPDATA%\DartsTUI, which is per-user, writable without
//     administrator rights, and — unlike Roaming — not synced to a domain
//     profile, so a large game history never slows down login.
func Resolve(opts Options) (Location, error) {
	if opts.Override != "" {
		path, err := filepath.Abs(opts.Override)
		if err != nil {
			return Location{}, fmt.Errorf("resolve database path: %w", err)
		}
		if err := ensureDir(filepath.Dir(path)); err != nil {
			return Location{}, err
		}
		return Location{Path: path, Source: "command line"}, nil
	}

	if env := os.Getenv(EnvDBPath); env != "" {
		path, err := filepath.Abs(env)
		if err != nil {
			return Location{}, fmt.Errorf("resolve %s: %w", EnvDBPath, err)
		}
		if err := ensureDir(filepath.Dir(path)); err != nil {
			return Location{}, err
		}
		return Location{Path: path, Source: EnvDBPath + " environment variable"}, nil
	}

	if opts.Portable {
		dir, err := executableDir()
		if err != nil {
			return Location{}, err
		}
		if err := ensureDir(dir); err != nil {
			return Location{}, err
		}
		return Location{
			Path:     filepath.Join(dir, DBFileName),
			Portable: true,
			Source:   "portable mode (beside the executable)",
		}, nil
	}

	dir, err := DataDir()
	if err != nil {
		return Location{}, err
	}
	if err := ensureDir(dir); err != nil {
		return Location{}, err
	}
	return Location{
		Path:   filepath.Join(dir, DBFileName),
		Source: "application data directory",
	}, nil
}

// DataDir returns the per-user directory for application data.
func DataDir() (string, error) {
	switch runtime.GOOS {
	case "windows":
		// Prefer LOCALAPPDATA. os.UserConfigDir would give Roaming, which is
		// synced on domain-joined office machines and is the wrong home for a
		// database that only makes sense on this desktop.
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			return filepath.Join(local, appDirName), nil
		}
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, appDirName), nil
		}
	case "darwin":
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, "Library", "Application Support", appDirName), nil
		}
	default:
		// Linux and friends: follow the XDG base directory specification.
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
			return filepath.Join(xdg, "darts-tui"), nil
		}
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, ".local", "share", "darts-tui"), nil
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate a home directory for the database: %w", err)
	}
	return filepath.Join(home, "."+appDirName), nil
}

func executableDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate the executable for portable mode: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		resolved = exe
	}
	return filepath.Dir(resolved), nil
}

func ensureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create data directory %s: %w", dir, err)
	}
	return nil
}
