// bindings.go declares the server-wide key and mouse bindings of reed's navigation: the five Alt keys and the left click on the status bar.
// The builder is pure; pinBindingsLocked issues its argvs, each non-fatal, from the geometry pin path that boot and the attach pre-flight both run.

package reedengine

import (
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shell"
)

const (
	// zoomBindingCondition is true in a strand pane or in any zoomed window, so Selvage and a batten window are left alone.
	zoomBindingCondition = "#{||:#{@strand},#{window_zoomed_flag}}"

	// stepBindingCondition is true in a window reed marked as the strands' window.
	stepBindingCondition = "#{@lyx_strands}"

	// viewWindowID expands, at click time, to the id of the window carrying @lyx_strands, and to nothing when no window does.
	viewWindowID = "#{W:#{?#{@lyx_strands},#{window_id},}}"

	// selectStrandsWindowCommand selects the strands' window, and does nothing when no window carries the marker.
	// It is expanded by run-shell -C at click time.
	selectStrandsWindowCommand = "run-shell -C \"#{?" + viewWindowID + ",select-window -t " + viewWindowID + ",}\""

	// viewClickCommand selects the strands' window and unzooms it when zoomed.
	// The zoom flag it hands the nested if-shell is escaped once, its closing brace as `#}` because a bare `}` ends the enclosing conditional early.
	viewClickCommand = "run-shell -C \"#{?" + viewWindowID + ",select-window -t " + viewWindowID +
		" ; if-shell -F -t " + viewWindowID + " '##{window_zoomed_flag#}' 'resize-pane -Z -t " + viewWindowID + "',}\""

	// paneClickCommand selects the clicked pane in its window and zooms it, keeping the zoom on when it already is.
	// tmux resolves a pane range's `=` target only inside the client's current window, so the strands' window, the only one that carries pane ranges, is selected first.
	paneClickCommand = selectStrandsWindowCommand + " ; select-pane -Z -t= ; if-shell -F -t= '#{window_zoomed_flag}' '' 'resize-pane -Z -t='"
)

// stepCommand returns the tmux command that steps to the previous pane of the window (target `:.-`) or the next (`:.+`), both wrapping and keeping the zoom.
// Selvage is the one pane without @strand, so when a step lands on it one more step the same way passes it.
func stepCommand(target string) string {
	step := "select-pane -Z -t " + target
	return step + " ; if-shell -F -t : \"#{@strand}\" \"\" \"" + step + "\""
}

// statusClickBranch is one status range type of the left-click dispatch and the tmux command run for it.
type statusClickBranch struct {
	rangeType string
	command   string
}

// statusClickBranches lists the left-click dispatch on `#{mouse_status_range}`, in order.
var statusClickBranches = []statusClickBranch{
	{"view", viewClickCommand},
	{"pane", paneClickCommand},
	{"window", "select-window -t="},
	{"session", "switch-client -t="},
}

// bindingArgvs returns the `bind-key -n` argvs of the Alt keys and the status click, each in the root table.
// switchPrev and switchNext are the shell lines run-shell runs for M-Left and M-Right;
// an empty one leaves its key unbound.
// The status click is dispatched by chained if-shell commands, joined by tmux's escaped `\;` separator argument, since only one range type matches a click.
func bindingArgvs(switchPrev, switchNext string) [][]string {
	argvs := [][]string{
		{"bind-key", "-n", "M-z", "if-shell", "-F", zoomBindingCondition, "resize-pane -Z"},
		{"bind-key", "-n", "M-Up", "if-shell", "-F", stepBindingCondition, stepCommand(":.-")},
		{"bind-key", "-n", "M-Down", "if-shell", "-F", stepBindingCondition, stepCommand(":.+")},
	}
	if switchPrev != "" {
		argvs = append(argvs, []string{"bind-key", "-n", "M-Left", "run-shell", "-b", switchPrev})
	}
	if switchNext != "" {
		argvs = append(argvs, []string{"bind-key", "-n", "M-Right", "run-shell", "-b", switchNext})
	}

	click := []string{"bind-key", "-n", "MouseDown1Status"}
	for i, branch := range statusClickBranches {
		if i > 0 {
			click = append(click, `\;`)
		}
		click = append(click, "if-shell", "-F", "#{==:#{mouse_status_range},"+branch.rangeType+"}", branch.command)
	}
	return append(argvs, click)
}

// pinBindingsLocked issues the navigation bindings on the server.
// Each call's failure is logged via logger.Warn and ignored, and no GOOS branch guards it: psmux may refuse some or all of them.
// An unresolvable executable, socket path or tmux path logs a named warning and leaves M-Left and M-Right unbound, the other bindings in place.
// Assumes the op lock is already held.
func (e *Engine) pinBindingsLocked() {
	switchPrev, switchNext := e.switchBindingCommandsLocked()
	for _, argv := range bindingArgvs(switchPrev, switchNext) {
		if err := e.tmux.run(argv...); err != nil {
			logger.Warn("reed: failed to pin a key binding", "socket", e.Socket(), "session", e.SessionName(), "binding", argv[2], "err", err)
		}
	}
}

// switchBindingCommandsLocked returns the shell lines for M-Left and M-Right, both empty when the socket path, the tmux path or this binary cannot be resolved.
func (e *Engine) switchBindingCommandsLocked() (prev, next string) {
	socketPath, err := e.tmux.output("display-message", "-p", "#{socket_path}")
	socketPath = strings.TrimSpace(socketPath)
	if err != nil || socketPath == "" {
		logger.Warn("reed: could not read the server's socket path, pinning no session-switch binding", "socket", e.Socket(), "err", err)
		return "", ""
	}
	tmuxPath, err := exec.LookPath(e.cfg.Tmux)
	if err == nil {
		tmuxPath, err = filepath.Abs(tmuxPath)
	}
	if err != nil {
		logger.Warn("reed: could not resolve the tmux binary, pinning no session-switch binding", "tmux", e.cfg.Tmux, "err", err)
		return "", ""
	}
	sh := shell.ForGOOS()
	prev, prevOK := composeSwitchCommand(sh, false, socketPath, tmuxPath)
	next, nextOK := composeSwitchCommand(sh, true, socketPath, tmuxPath)
	if !prevOK || !nextOK {
		return "", ""
	}
	return prev, next
}
