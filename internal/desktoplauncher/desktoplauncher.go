// desktoplauncher.go names, writes, removes and lists the launcher files on a located desktop.

package desktoplauncher

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/standalonestate"
)

// Form is the kind of launcher file a desktop takes.
type Form string

const (
	// FormFreedesktop is a `.desktop` file that runs the verb in a terminal on Linux.
	FormFreedesktop Form = "freedesktop"
	// FormWindowsWSL is a `.cmd` file on the Windows desktop that runs the verb in a WSL distribution through wsl.exe.
	FormWindowsWSL Form = "windows-wsl"
	// FormWindows is a `.cmd` file that runs the verb on native Windows.
	FormWindows Form = "windows"
)

const (
	// hubPrefix leads the file name of a hub's launcher, before the hub's shortname and the extension.
	hubPrefix = "lym-hub-"
	// allHubsName is the file name of the all-hubs launcher, before the extension.
	allHubsName = "lym-hubs"
)

// Desktop is this machine's desktop directory and the form of the launchers written there.
type Desktop struct {
	// Dir is the absolute desktop directory, as a path this process opens.
	Dir string
	// Form is the form every launcher in Dir takes.
	Form Form
	// Distro is the WSL distribution a FormWindowsWSL launcher runs the verb in; empty runs wsl.exe's default one.
	Distro string
}

// Add writes the launcher of the hub shortname and the all-hubs launcher, each running `<lymPath> machine start`.
// It rewrites a launcher that stands, so one naming a lym binary that moved names the current one.
// A lym path a `.cmd` launcher under WSL cannot quote is an error, and nothing is written.
// Both writes happen under the desktop's lock, so a concurrent Remove never sees one without the other.
func (d Desktop) Add(lymPath, shortname string) error {
	hub, err := d.content(lymPath, "start the hub "+shortname, shortname)
	if err != nil {
		return err
	}
	all, err := d.content(lymPath, "start every hub", "")
	if err != nil {
		return err
	}
	held, err := d.acquireLock()
	if err != nil {
		return err
	}
	defer func() { _ = held.Release() }()
	for path, content := range map[string]string{d.hubPath(shortname): hub, d.allHubsPath(): all} {
		if err := os.WriteFile(path, []byte(content), d.fileMode()); err != nil {
			return fmt.Errorf("desktoplauncher: write the launcher %s: %w", path, err)
		}
	}
	return nil
}

// Remove removes the launcher of the hub shortname, and the all-hubs launcher once no hub's launcher stands.
// A launcher already gone is no error.
// The removal, the read of the launchers left and the all-hubs removal happen under the desktop's lock,
// so another process's Add never lands between them.
func (d Desktop) Remove(shortname string) error {
	held, err := d.acquireLock()
	if err != nil {
		return err
	}
	defer func() { _ = held.Release() }()
	if err := removeIfPresent(d.hubPath(shortname)); err != nil {
		return err
	}
	left, err := d.Shortnames()
	if err != nil || len(left) > 0 {
		return err
	}
	return removeIfPresent(d.allHubsPath())
}

// Shortnames lists the shortnames of the hubs whose launcher stands in the desktop directory, in name order.
func (d Desktop) Shortnames() ([]string, error) {
	entries, err := os.ReadDir(d.Dir)
	if err != nil {
		return nil, fmt.Errorf("desktoplauncher: list the desktop %s: %w", d.Dir, err)
	}
	var shortnames []string
	for _, entry := range entries {
		rest, isHub := strings.CutPrefix(entry.Name(), hubPrefix)
		shortname, hasExtension := strings.CutSuffix(rest, d.extension())
		if isHub && hasExtension && shortname != "" && !entry.IsDir() {
			shortnames = append(shortnames, shortname)
		}
	}
	return shortnames, nil
}

// hubPath returns the path of the launcher of the hub shortname.
func (d Desktop) hubPath(shortname string) string {
	return filepath.Join(d.Dir, hubPrefix+shortname+d.extension())
}

// allHubsPath returns the path of the all-hubs launcher.
func (d Desktop) allHubsPath() string {
	return filepath.Join(d.Dir, allHubsName+d.extension())
}

// extension returns the file extension of the desktop's launchers.
func (d Desktop) extension() string {
	if d.Form == FormFreedesktop {
		return ".desktop"
	}
	return ".cmd"
}

// fileMode returns the mode a launcher is written with.
// A `.desktop` file is executable, which a file manager asks of a launcher it trusts.
func (d Desktop) fileMode() os.FileMode {
	if d.Form == FormFreedesktop {
		return 0o755
	}
	return 0o644
}

// content returns the text of a launcher that runs `<lymPath> machine start`, with shortname as its argument when set.
// purpose says what the launcher does, for its name and comment.
func (d Desktop) content(lymPath, purpose, shortname string) (string, error) {
	verb := strings.TrimSpace("machine start " + shortname)
	sh := shell.Posix()
	posixRun := sh.Invoke(lymPath) + " " + verb
	switch d.Form {
	case FormFreedesktop:
		line := sh.Chain(posixRun, "echo", "read -r -p "+sh.Quote("Press Enter to close this window")+" _")
		return strings.Join([]string{
			"[Desktop Entry]",
			"Type=Application",
			"Name=lym: " + purpose,
			"Comment=Runs lym " + verb + " on this machine",
			"Exec=bash -lc " + execArgument(line),
			"Terminal=true",
			"Icon=utilities-terminal",
			"",
		}, "\n"), nil
	case FormWindowsWSL:
		if strings.ContainsAny(lymPath+shortname, "\"$`\\") {
			return "", fmt.Errorf("desktoplauncher: the lym path %q or shortname %q holds a double quote, dollar sign, backquote or backslash, which a .cmd launcher cannot pass to wsl.exe; way forward: install lym under a path without one", lymPath, shortname)
		}
		run := "wsl.exe"
		if d.Distro != "" {
			run += ` -d "` + d.Distro + `"`
		}
		return cmdFile(purpose, run+` -- bash -lc "`+posixRun+`"`), nil
	default:
		return cmdFile(purpose, `"`+lymPath+`" `+verb), nil
	}
}

// cmdFile returns a `.cmd` launcher that runs run and pauses, so its window shows the outcome until a key is pressed.
// Each "%" is doubled, since a batch file reads "%" as a variable reference.
func cmdFile(purpose, run string) string {
	lines := []string{"@echo off", "rem lym: " + purpose, run, "pause", ""}
	return strings.ReplaceAll(strings.Join(lines, "\r\n"), "%", "%%")
}

// execArgument quotes argument as one argument of a desktop entry's Exec key.
// Inside the double quotes each double quote, backquote, "$" and backslash gets a backslash.
// Each "%" is then doubled, since Exec reads "%" as a field code,
// and each backslash is doubled again by the string escape every desktop entry value goes through.
func execArgument(argument string) string {
	var quoted strings.Builder
	quoted.WriteByte('"')
	for _, r := range argument {
		if strings.ContainsRune("\"`$\\", r) {
			quoted.WriteByte('\\')
		}
		quoted.WriteRune(r)
	}
	quoted.WriteByte('"')
	return strings.NewReplacer(`\`, `\\`, "%", "%%").Replace(quoted.String())
}

// acquireLock takes the exclusive lock that serializes the launcher writes and removals on d across processes.
// The lock file sits in the per-user state directory, so none shows on the desktop.
// One lock serves every desktop of the user, since a machine has one.
func (d Desktop) acquireLock() (*lock.FileLock, error) {
	stateDir, err := standalonestate.UserStateDir()
	if err != nil {
		return nil, fmt.Errorf("desktoplauncher: place the lock of the desktop %s: %w", d.Dir, err)
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return nil, fmt.Errorf("desktoplauncher: create %s: %w", stateDir, err)
	}
	held, err := lock.AcquireWriteLock(filepath.Join(stateDir, "launchers.lock"))
	if err != nil {
		return nil, fmt.Errorf("desktoplauncher: lock the desktop %s: %w", d.Dir, err)
	}
	return held, nil
}

// removeIfPresent removes the file at path; a file already gone is no error.
func removeIfPresent(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("desktoplauncher: remove the launcher %s: %w", path, err)
	}
	return nil
}
