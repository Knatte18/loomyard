// Package termwindow opens a desktop terminal window that runs "lym reed attach" in a worktree,
// so the operator can watch a run in a window of its own.
// The launcher is chosen per platform:
// Windows Terminal (wt.exe) opens on native Windows and, through wsl.exe, on WSL.
// Konsole opens on any other Linux with a graphical session.
// Elsewhere Resolve reports ErrNoLauncher and nothing opens.
// The window is a detached process: once it starts, lym neither waits for it nor learns whether it stays open.
// On Linux and WSL it closes itself when the run ends:
// on a 0 attach exit, or on any other attach exit once the run's worktree is removed within a bounded wait.
// Otherwise it keeps an interactive shell and the attach's output.
// On native Windows the window runs the attach itself,
// so the Windows Terminal profile's closeOnExit decides whether it closes when the attach exits.
package termwindow

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

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
	// arguments returns the program's arguments for a window titled title, in dir, that runs "<lymPath> reed attach".
	arguments func(dir, title, lymPath string) []string
	// separate returns the program's arguments for the same window run in a process of its own;
	// it is nil for a program that offers none.
	separate func(dir, title, lymPath string) []string
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
	if p.goos == "windows" {
		program, err := p.lookPath("wt.exe")
		if err != nil {
			return Launcher{}, fmt.Errorf("%w: wt.exe is not on PATH: %v", ErrNoLauncher, err)
		}
		return Launcher{name: "wt.exe", program: program, arguments: nativeWindowsTerminalArguments}, nil
	}
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
	return Launcher{name: "konsole", program: program, arguments: konsoleArguments, separate: separateKonsoleArguments}, nil
}

// IsKonsole reports whether the launcher is Konsole, the one program that can run a window in a process of its own.
func (l Launcher) IsKonsole() bool {
	return l.separate != nil
}

// Open opens one window titled title that runs "<lymPath> reed attach" in dir, and returns once it has started.
// With separate false it opens the window as OpenAttach does and returns pid 0.
// With separate true the window runs in a process of its own and the returned pid is that process's.
// It is an error to ask for separate on a launcher IsKonsole reports false for.
func (l Launcher) Open(dir, title, lymPath string, separate bool) (int, error) {
	if !separate {
		return 0, l.OpenAttach(dir, title, lymPath)
	}
	if l.separate == nil {
		return 0, fmt.Errorf("termwindow: launcher %s cannot open a window in a process of its own; the caller must check IsKonsole first", l.name)
	}
	cmd := exec.Command(l.program, l.separate(dir, title, lymPath)...)
	proc.Detach(cmd)
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("termwindow: start %s: %w", l.name, err)
	}
	pid := cmd.Process.Pid
	logger.Info("termwindow: spawned terminal window", "launcher", l.name, "dir", dir, "title", title, "pid", pid)
	proc.Reap(cmd)
	return pid, nil
}

// konsoleArguments opens a new Konsole window, never a tab, with its tab titled title.
// Its login shell runs the attach line.
func konsoleArguments(dir, title, lymPath string) []string {
	return []string{"--workdir", dir, "-p", "tabtitle=" + title, "-e", "bash", "-lc", attachLine(lymPath, dir)}
}

// separateKonsoleArguments opens a new titled window running the attach line in a Konsole process of its own.
// The operator's single-process setting does not change that.
func separateKonsoleArguments(dir, title, lymPath string) []string {
	return append([]string{"--separate"}, konsoleArguments(dir, title, lymPath)...)
}

// windowsTerminalArguments opens a new Windows Terminal window running wsl.exe in dir.
// Its login shell runs the attach line.
func windowsTerminalArguments(dir, title, lymPath string) []string {
	return []string{"-w", "new", "--title", title, "wsl.exe", "--cd", dir, "--", "bash", "-lc", escapeWindowsTerminal(attachLine(lymPath, dir))}
}

// nativeWindowsTerminalArguments opens a new Windows Terminal window in dir that runs the attach with no shell around it.
func nativeWindowsTerminalArguments(dir, title, lymPath string) []string {
	return []string{"-w", "new", "--title", escapeWindowsTerminal(title), "-d", escapeWindowsTerminal(dir), escapeWindowsTerminal(lymPath), "reed", "attach"}
}

// escapeWindowsTerminal escapes each ";" in argument as "\;", since Windows Terminal splits its own command line on ";".
func escapeWindowsTerminal(argument string) string {
	return strings.ReplaceAll(argument, ";", `\;`)
}

// OpenAttach opens one window titled title that runs "<lymPath> reed attach" in dir.
// On Linux and WSL the window closes when the attach exits 0.
// That includes an operator detach and an ended pair session.
// After any other attach exit, it closes as soon as dir is removed within pairGoneWaitSeconds seconds;
// otherwise it stays open as a shell.
// On native Windows the Windows Terminal profile decides whether the window closes when the attach exits.
// It returns once the launcher process has started; an error means it could not start.
func (l Launcher) OpenAttach(dir, title, lymPath string) error {
	cmd := exec.Command(l.program, l.arguments(dir, title, lymPath)...)
	proc.Detach(cmd)
	logger.Info("termwindow: spawning terminal window", "launcher", l.name, "dir", dir, "title", title)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("termwindow: start %s: %w", l.name, err)
	}
	proc.Reap(cmd)
	return nil
}

// pairGoneWaitSeconds bounds how long a window waits for the run's worktree to be removed after a nonzero attach exit.
// Past the bound, the window keeps an interactive shell.
const pairGoneWaitSeconds = 30

// attachLine is the window's shell line, with the wait on a removed worktree set to pairGoneWaitSeconds one-second polls.
func attachLine(lymPath, dir string) string {
	return attachLineWaiting(lymPath, dir, pairGoneWaitSeconds, time.Second)
}

// attachLineWaiting is the window's shell line: it runs the attach and keeps its status.
// On status 0 it exits.
// Otherwise it prints the status and that it waits up to polls times pollInterval for dir to be removed.
// It polls once per pollInterval and exits as soon as dir is no longer a directory.
// A test passes a short pollInterval so a removal during the wait is seen without a second of real time per poll.
// When the wait ends with dir present it execs an interactive shell, keeping the attach's output on screen.
func attachLineWaiting(lymPath, dir string, polls int, pollInterval time.Duration) string {
	sh := shell.Posix()
	pollSleep := strconv.FormatFloat(pollInterval.Seconds(), 'f', -1, 64)
	waitBound := time.Duration(polls) * pollInterval
	gone := "[ -d " + sh.Quote(dir) + " ] || exit 0"
	return sh.Chain(
		sh.Invoke(lymPath)+" reed attach",
		"status=$?",
		"if [ $status -eq 0 ]; then exit 0; fi",
		"echo "+sh.Quote("reed attach ended with status")+" $status "+sh.Quote(fmt.Sprintf("- waiting up to %s for the run worktree to be removed", waitBound)),
		"i=0",
		fmt.Sprintf("while [ $i -lt %d ]; do %s; sleep %s; i=$((i+1)); done", polls, gone, pollSleep),
		gone,
		"exec bash",
	)
}
