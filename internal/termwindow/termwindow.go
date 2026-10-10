// Package termwindow opens a desktop terminal window that runs "lyx reed attach" in a worktree, so the operator can watch a run in a window of its own.
// The launcher is chosen per platform: Windows Terminal (wt.exe) into wsl.exe on WSL, Konsole on any other Linux with a graphical session.
// Elsewhere Resolve reports ErrNoLauncher and nothing opens.
// The window is a detached process: once it starts, lyx neither waits for it nor learns whether it stays open.
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

// OpenAttach opens one window titled title whose login shell runs "<lyxPath> reed attach" in dir and stays open as a shell once the attach returns.
// It returns once the launcher process has started; an error means it could not start.
func (l Launcher) OpenAttach(dir, title, lyxPath string) error {
	cmd := exec.Command(l.program, l.arguments(dir, title, attachLine(lyxPath))...)
	proc.Detach(cmd)
	logger.Info("termwindow: spawning terminal window", "launcher", l.name, "dir", dir, "title", title)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("termwindow: start %s: %w", l.name, err)
	}
	return cmd.Process.Release()
}

// attachLine is the window's shell line: the attach, then an interactive shell that keeps the window open.
func attachLine(lyxPath string) string {
	sh := shell.Posix()
	return sh.Chain(sh.Invoke(lyxPath)+" reed attach", "exec bash")
}
