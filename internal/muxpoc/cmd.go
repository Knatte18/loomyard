// cmd.go — low-level psmux command helpers.
//
// Config holds psmux and shell paths plus dimensions. PsmuxCmd wraps low-level
// psmux operations: run (discarding output), output (capturing stdout), hasSession
// (checking existence), and listPanes (parsing pane format). LivePane represents
// a single pane state. expandTpl is a template helper for launch/resume commands.

package muxpoc

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Config holds paths and dimensions for muxpoc operations.
type Config struct {
	PsmuxPath  string
	PwshPath   string
	ClaudePath string
	LaunchTpl  string
	ResumeTpl  string
	Width      int
	Height     int
}

// PsmuxCmd wraps low-level psmux operations.
type PsmuxCmd struct {
	cfg Config
}

// NewPsmuxCmd creates a new PsmuxCmd with the given config.
func NewPsmuxCmd(cfg Config) PsmuxCmd {
	return PsmuxCmd{cfg: cfg}
}

// run builds an exec.Command with -L <socket> prepended and runs it,
// discarding stdout and stderr. Returns cmd.Run() error.
func (p PsmuxCmd) run(args ...string) error {
	fullArgs := append([]string{"-L", socketArg(p.cfg)}, args...)
	cmd := exec.Command(p.cfg.PsmuxPath, fullArgs...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

// output builds an exec.Command with -L <socket> prepended and runs it,
// capturing stdout. Returns (stdout string, error).
func (p PsmuxCmd) output(args ...string) (string, error) {
	fullArgs := append([]string{"-L", socketArg(p.cfg)}, args...)
	cmd := exec.Command(p.cfg.PsmuxPath, fullArgs...)
	out, err := cmd.Output()
	return string(out), err
}

// hasSession checks whether the named session exists. Returns (true, nil) on
// exit 0, (false, nil) on exit 1 (session absent — normal, not an error).
// Returns (false, err) on any other error.
func (p PsmuxCmd) hasSession(name string) (bool, error) {
	err := p.run("has-session", "-t", name)
	if err == nil {
		return true, nil
	}

	// Check if it's an ExitError with code 1 (session absent)
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false, nil
	}

	return false, err
}

// LivePane represents the state of a single pane.
type LivePane struct {
	ID     string `json:"id"`
	Dead   bool   `json:"dead"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// listPanes returns all panes in the session. Parses output from
// list-panes -F "#{pane_id} #{pane_dead} #{pane_width} #{pane_height}".
// Returns nil, nil if output is empty (no panes).
func (p PsmuxCmd) listPanes(session string) ([]LivePane, error) {
	out, err := p.output("list-panes", "-t", session, "-F", "#{pane_id} #{pane_dead} #{pane_width} #{pane_height}")
	if err != nil {
		return nil, err
	}

	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}

	var panes []LivePane
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 4 {
			return nil, fmt.Errorf("invalid pane format: %q", line)
		}

		dead := parts[1] == "1"
		width, err := strconv.Atoi(parts[2])
		if err != nil {
			return nil, fmt.Errorf("invalid pane width: %s", parts[2])
		}
		height, err := strconv.Atoi(parts[3])
		if err != nil {
			return nil, fmt.Errorf("invalid pane height: %s", parts[3])
		}

		panes = append(panes, LivePane{
			ID:     parts[0],
			Dead:   dead,
			Width:  width,
			Height: height,
		})
	}

	return panes, nil
}

// socketArg is a helper that calls os.Getwd() and returns socketName(cwd).
// Used by run and output to inject the per-repo -L <socket> argument.
func socketArg(cfg Config) string {
	cwd, _ := os.Getwd()
	return socketName(cwd)
}

// expandTpl replaces %SID% with sid and %TASK% with task in tpl.
// Used by up.go and daemon.go to build claude launch/resume commands.
func expandTpl(tpl, sid, task string) string {
	result := strings.ReplaceAll(tpl, "%SID%", sid)
	result = strings.ReplaceAll(result, "%TASK%", task)
	return result
}
