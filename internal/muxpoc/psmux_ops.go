// psmux_ops.go — higher-level psmux operations used by the muxpoc subcommands.
// All of them go through Runner (sanitized env, windowless), and use the explicit
// PowerShell 7 binary as the pane shell (bare `pwsh` is a broken alias under ConPTY).
package muxpoc

import (
	"crypto/rand"
	"fmt"
	"strings"
)

// Pane is one row of `list-panes` output.
type Pane struct {
	ID     string
	Dead   bool
	Width  int
	Height int
}

// hasServer reports whether the psmux server for this socket is up.
func (r *Runner) hasServer() bool {
	_, err := r.run("list-sessions")
	return err == nil
}

// hasSession reports whether a named session exists.
func (r *Runner) hasSession(name string) bool {
	_, err := r.run("has-session", "-t", name)
	return err == nil
}

// newSession starts a detached session whose single pane runs the explicit pwsh. Because
// this call starts the server (when it is not already up), its sanitized env becomes the
// env every pane inherits — the load-bearing env-hygiene step.
func (r *Runner) newSession(cfg Config, name, cwd string, w, h int) error {
	_, err := r.run("new-session", "-d", "-s", name,
		"-x", itoa(w), "-y", itoa(h), "-c", cwd, cfg.Pwsh)
	return err
}

// setRemainOnExit keeps a pane visible as "dead" when its process exits (so death is
// observable via pane_dead and the pane id can be reused with respawn-pane).
func (r *Runner) setRemainOnExit() error {
	_, err := r.run("set-option", "-g", "remain-on-exit", "on")
	return err
}

// sendLine types a line into a pane (literal text) and submits it with a separate Enter,
// matching the verified send-keys discipline.
func (r *Runner) sendLine(target, text string) error {
	if _, err := r.run("send-keys", "-t", target, "-l", text); err != nil {
		return err
	}
	_, err := r.run("send-keys", "-t", target, "Enter")
	return err
}

// capture returns the rendered visible text of a pane (primary buffer).
func (r *Runner) capture(target string) (string, error) {
	return r.run("capture-pane", "-t", target, "-p")
}

// splitV splits a pane vertically (new pane below) and returns the new pane id. The new
// pane runs the explicit pwsh; the caller sends it a launch command.
func (r *Runner) splitV(cfg Config, target, cwd string) (string, error) {
	out, err := r.run("split-window", "-v", "-t", target, "-c", cwd,
		"-P", "-F", "#{pane_id}", cfg.Pwsh)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// listPanes returns the panes of a target (session or window).
func (r *Runner) listPanes(target string) ([]Pane, error) {
	out, err := r.run("list-panes", "-t", target, "-F",
		"#{pane_id}\t#{pane_dead}\t#{pane_width}\t#{pane_height}")
	if err != nil {
		return nil, err
	}
	var panes []Pane
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 4 {
			continue
		}
		panes = append(panes, Pane{
			ID:     f[0],
			Dead:   f[1] == "1",
			Width:  atoi(f[2]),
			Height: atoi(f[3]),
		})
	}
	return panes, nil
}

// respawn revives a dead pane in place (same id) by respawning its shell, then the caller
// sends the resume command. -k kills first if still live.
func (r *Runner) respawn(cfg Config, target, cwd string) error {
	_, err := r.run("respawn-pane", "-k", "-t", target, "-c", cwd, cfg.Pwsh)
	return err
}

// killServer tears down the whole psmux server for this socket.
func (r *Runner) killServer() error {
	_, err := r.run("kill-server")
	return err
}

// newUUID returns a random RFC-4122 v4 UUID string (no external dependency).
func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return n
		}
		n = n*10 + int(c-'0')
	}
	return n
}
