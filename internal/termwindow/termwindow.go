// Package termwindow opens a desktop terminal window that runs "lyx reed attach" in a worktree, so the operator can watch a run in a window of its own.
// The launcher is chosen per platform: Windows Terminal (wt.exe) into wsl.exe on WSL, Konsole on any other Linux with a graphical session.
// Elsewhere Resolve reports ErrNoLauncher and nothing opens.
// The window is a detached process: once it starts, lyx neither waits for it nor learns whether it stays open.
// It closes itself when the run ends: on a 0 attach exit, or on any other attach exit once the run's worktree is removed within a bounded wait.
// Otherwise it keeps an interactive shell and the attach's output.
package termwindow

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shell"
)

// ErrNoLauncher reports that no terminal launcher is known on this machine.
var ErrNoLauncher = errors.New("no terminal window launcher known here")

// wslInteropPath exists on every WSL machine whose Windows interop is enabled, the interop wt.exe needs.
const wslInteropPath = "/proc/sys/fs/binfmt_misc/WSLInterop"

// Launcher opens terminal windows through one platform's terminal program.
type Launcher struct {
	name    string
	program string
	// arguments returns the program's arguments for a window titled title, in dir, whose shell runs line.
	arguments func(dir, title, line string) []string
}

// probe carries the machine facts Resolve reads, so the choice is testable without the machine.
type probe struct {
	goos       string
	getenv     func(string) string
	lookPath   func(string) (string, error)
	fileExists func(string) bool
}

// Resolve returns the launcher for this machine, or an error wrapping ErrNoLauncher that names why there is none.
func Resolve() (Launcher, error) {
	return resolve(probe{
		goos:     runtime.GOOS,
		getenv:   os.Getenv,
		lookPath: exec.LookPath,
		fileExists: func(path string) bool {
			_, err := os.Stat(path)
			return err == nil
		},
	})
}

func resolve(p probe) (Launcher, error) {
	if p.goos != "linux" {
		return Launcher{}, fmt.Errorf("%w: no launcher for %s", ErrNoLauncher, p.goos)
	}
	if p.getenv("WSL_DISTRO_NAME") != "" || p.fileExists(wslInteropPath) {
		program, err := p.lookPath("wt.exe")
		if err != nil {
			return Launcher{}, fmt.Errorf("%w: WSL without wt.exe on PATH: %v", ErrNoLauncher, err)
		}
		return Launcher{name: "wt.exe", program: program, arguments: windowsTerminalArguments}, nil
	}
	if p.getenv("DISPLAY") == "" && p.getenv("WAYLAND_DISPLAY") == "" {
		return Launcher{}, fmt.Errorf("%w: no graphical session (neither DISPLAY nor WAYLAND_DISPLAY is set)", ErrNoLauncher)
	}
	program, err := p.lookPath("konsole")
	if err != nil {
		return Launcher{}, fmt.Errorf("%w: konsole is not on PATH: %v", ErrNoLauncher, err)
	}
	return Launcher{name: "konsole", program: program, arguments: konsoleArguments}, nil
}

// konsoleArguments opens a new Konsole window, never a tab, with its tab titled title.
func konsoleArguments(dir, title, line string) []string {
	return []string{"--workdir", dir, "-p", "tabtitle=" + title, "-e", "bash", "-lc", line}
}

// windowsTerminalArguments opens a new Windows Terminal window running wsl.exe in dir.
// Windows Terminal splits its own command line on ";", so each one in line is escaped as "\;".
func windowsTerminalArguments(dir, title, line string) []string {
	return []string{"-w", "new", "--title", title, "wsl.exe", "--cd", dir, "--", "bash", "-lc", strings.ReplaceAll(line, ";", `\;`)}
}

// Name returns the launcher's program name, for the operator's report.
func (l Launcher) Name() string {
	return l.name
}

// OpenAttach opens one window titled title whose login shell runs "<lyxPath> reed attach" in dir.
// The window closes when the attach exits 0, which includes an operator detach and an ended pair session, or, after any other attach exit, as soon as dir is removed within pairGoneWaitSeconds seconds; otherwise it stays open as a shell.
// It returns once the launcher process has started; an error means it could not start.
func (l Launcher) OpenAttach(dir, title, lyxPath string) error {
	cmd := exec.Command(l.program, l.arguments(dir, title, attachLine(lyxPath, dir))...)
	proc.Detach(cmd)
	logger.Info("termwindow: spawning terminal window", "launcher", l.name, "dir", dir, "title", title)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("termwindow: start %s: %w", l.name, err)
	}
	return cmd.Process.Release()
}

// pairGoneWaitSeconds is how long a window whose attach ended with a nonzero status waits for the run's worktree to be removed before it keeps an interactive shell.
const pairGoneWaitSeconds = 30

// attachLine is the window's shell line with the wait the window ends on a removed worktree for.
func attachLine(lyxPath, dir string) string {
	return attachLineWaiting(lyxPath, dir, pairGoneWaitSeconds)
}

// attachLineWaiting is the window's shell line: it runs the attach and keeps its status.
// On status 0 it exits.
// Otherwise it prints the status and that it waits up to waitSeconds seconds for dir to be removed.
// It polls once a second and exits as soon as dir is no longer a directory.
// When the wait ends with dir present it execs an interactive shell, keeping the attach's output on screen.
func attachLineWaiting(lyxPath, dir string, waitSeconds int) string {
	sh := shell.Posix()
	gone := "[ -d " + sh.Quote(dir) + " ] || exit 0"
	return sh.Chain(
		sh.Invoke(lyxPath)+" reed attach",
		"status=$?",
		"if [ $status -eq 0 ]; then exit 0; fi",
		"echo "+sh.Quote("reed attach ended with status")+" $status "+sh.Quote(fmt.Sprintf("- waiting up to %ds for the run worktree to be removed", waitSeconds)),
		"i=0",
		fmt.Sprintf("while [ $i -lt %d ]; do %s; sleep 1; i=$((i+1)); done", waitSeconds, gone),
		gone,
		"exec bash",
	)
}
