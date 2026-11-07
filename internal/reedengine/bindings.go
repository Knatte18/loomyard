// bindings.go declares the server-wide key and mouse bindings of reed's navigation.
// They are the four Alt keys and the left click on the status bar.
// Alt+Up and Alt+Down step across windows in index order,
// and a left click on the shell button returns to the strands' window.
// The click dispatches on the shell, window and session ranges, the follow button and the button ranges.
// The builder is pure.
// pinBindingsLocked issues its argvs, each non-fatal.
// Its caller is the geometry pin path that boot and the attach pre-flight run.
// The pin stamps the server with the build that made it.
// Every op under the blocking op lock re-pins when the stamp is absent or unparseable.
// It also re-pins when another build's stamp has a commit time not after the running build's,
// so a deploy reaches a running server at its first reed op.
// Alt+Up, Alt+Down and the shell and window clicks write the follow state off.
// The follow button toggles it.
// @lym_follow is the one option these bindings write that a Go decision reads.

package reedengine

import (
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/buildvcs"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shell"
)

const (
	// hiddenWindowCondition is true in a window a step landed on that carries @lym_hidden.
	hiddenWindowCondition = "#{@lym_hidden}"

	// viewWindowID expands, at click time, to the id of the window carrying @lym_strands, and to nothing when no window does.
	viewWindowID = "#{W:#{?#{@lym_strands},#{window_id},}}"

	// selectStrandsWindowCommand selects the strands' window, and does nothing when no window carries the marker.
	// It is expanded by run-shell -C at click time.
	selectStrandsWindowCommand = "run-shell -C \"#{?" + viewWindowID + ",select-window -t " + viewWindowID + ",}\""

	// shellClickCommand selects the strands' window, the shell window.
	shellClickCommand = selectStrandsWindowCommand
)

// followCommandOption is the server-global user option holding the shell line the follow button runs.
// The button reads it through `#{E:...}` inside run-shell, which keeps the composed line out of tmux's nested command-string quoting.
const followCommandOption = "@lym_follow_command"

// buttonCommandOption is the server-global user option holding the shell line a click on a button range runs.
// The click reads it through `#{E:...}` inside run-shell, like followCommandOption.
const buttonCommandOption = "@lym_button_command"

// bindingsBuildOption is the server-global user option naming the build whose bindings the server holds.
// Its value is the build's VCS commit time in RFC 3339 UTC, a space, then its revision label.
// It is unset when no build pinned them, or when the one that did carries no VCS stamp.
const bindingsBuildOption = "@lym_bindings_build"

// bindingsBuildStamp returns the bindingsBuildOption value naming build, and the empty string for a build with no VCS stamp.
func bindingsBuildStamp(build buildvcs.Identity) string {
	if build.Revision == "" {
		return ""
	}
	return build.Time.UTC().Format(time.RFC3339) + " " + build.Label()
}

// bindingsStale reports whether running should re-pin the bindings of a server whose bindingsBuildOption reads stamp.
// A running build with no VCS stamp never re-pins.
// Otherwise an empty or unparseable stamp is stale, and running's own stamp is not.
// Another build's stamp is stale unless its commit time is after running's,
// so an older binary still running reed ops never pins its bindings back over a newer one's.
func bindingsStale(stamp string, running buildvcs.Identity) bool {
	if running.Revision == "" {
		return false
	}
	stamp = strings.TrimSpace(stamp)
	if stamp == bindingsBuildStamp(running) {
		return false
	}
	pinnedTime, _, ok := strings.Cut(stamp, " ")
	if !ok {
		return true
	}
	pinnedAt, err := time.Parse(time.RFC3339, pinnedTime)
	if err != nil {
		return true
	}
	return !pinnedAt.After(running.Time)
}

// followOffCommand writes follow off on the session of the client that pressed the key or clicked.
const followOffCommand = "set-option " + followSessionOption + " off"

// statusClickBranch is one status range type of the left-click dispatch and the tmux command run for it.
type statusClickBranch struct {
	rangeType string
	command   string
}

// statusClickBranches lists the left-click dispatch on `#{mouse_status_range}`, in order.
// The shell and window branches write follow off first, also for a click on the current window;
// the session branch leaves the follow state alone.
var statusClickBranches = []statusClickBranch{
	{"shell", followOffCommand + " ; " + shellClickCommand},
	{"window", followOffCommand + " ; select-window -t="},
	{"session", "switch-client -t="},
}

// followClickCommand returns the command the follow button's click runs.
// From off it writes follow on and, when followJump holds, runs the jump line in the background;
// from on it writes follow off and changes no window.
func followClickCommand(followJump bool) string {
	turnOn := "set-option " + followSessionOption + " on"
	if followJump {
		turnOn += ` ; run-shell -b "#{E:` + followCommandOption + `}"`
	}
	return "if-shell -F '#{==:#{" + followSessionOption + "},off}' '" + turnOn + "' '" + followOffCommand + "'"
}

// bindingArgvs returns the argvs of the Alt keys and the status click, each in the root table.
// The first argv is an `unbind-key -n M-z` that removes the Alt+z an older build bound.
// A `bind-key` re-pin never removes a binding, so the unbind is needed.
// The `bind-key -n` argvs follow it.
// switchPrev and switchNext are the shell lines run-shell runs for M-Left and M-Right;
// an empty one leaves its key unbound.
// followJump reports that the follow button's jump line is pinned in followCommandOption.
// A click that turns follow on then runs it.
// M-Down and M-Up write follow off, then step to the next and previous window with tmux's own wrap, in every window,
// and the step then goes on if it landed on a window carrying @lym_hidden.
// The write comes first so it lands also when the step wraps onto the current window.
// Hidden windows sit after the shell window, so one conditional step reaches a visible window.
// Forward, the step lands on the session's first window, the lowest-index overview;
// back, it lands on the strands' window, the shell.
// The status click and each step are dispatched by chained commands, joined by tmux's escaped `\;` separator argument;
// only one range type matches a click.
// The click dispatch ends in the follow button's branch.
// When buttonClick holds, a click on a range named by buttonRangePrefix runs the button branch.
// That branch runs the line pinned in buttonCommandOption, resolved through the session's own options.
// buttonClick reports that the line is pinned; when false no button branch is bound.
func bindingArgvs(switchPrev, switchNext string, followJump, buttonClick bool) [][]string {
	argvs := [][]string{
		{"unbind-key", "-n", "M-z"},
		{"bind-key", "-n", "M-Up", "set-option", followSessionOption, "off", `\;`, "previous-window", `\;`, "if-shell", "-F", "-t", ":", hiddenWindowCondition, selectStrandsWindowCommand},
		{"bind-key", "-n", "M-Down", "set-option", followSessionOption, "off", `\;`, "next-window", `\;`, "if-shell", "-F", "-t", ":", hiddenWindowCondition, "select-window -t :^"},
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
	click = append(click, `\;`, "if-shell", "-F", "#{==:#{mouse_status_range},follow}", followClickCommand(followJump))
	if buttonClick {
		click = append(click, `\;`, "if-shell", "-F", "#{m:"+buttonRangePrefix+"*,#{mouse_status_range}}", `run-shell -b "#{E:`+buttonCommandOption+`}"`)
	}
	return append(argvs, click)
}

// pinBindingsLocked issues the navigation bindings on the server.
// Each call's failure is logged via logger.Warn and ignored.
// No GOOS branch guards it, since psmux may refuse some or all of them.
// An unresolvable executable, socket path or tmux path logs a named warning.
// Then M-Left and M-Right stay unbound and the follow button goes without its jump; the other bindings are still pinned.
// The follow jump line is set in followCommandOption when it resolves and unset otherwise,
// so a stale line never outlives the binary that pinned it.
// The button click line is set in buttonCommandOption and unset the same way.
// This binary's build stamp is then set in bindingsBuildOption, ahead of the bindings themselves.
// It is unset for a binary with no VCS stamp.
// Assumes the op lock is already held.
func (e *Engine) pinBindingsLocked() {
	switchPrev, switchNext, follow, button := e.switchBindingCommandsLocked()
	if follow != "" {
		if err := e.tmux.run("set-option", "-g", followCommandOption, follow); err != nil {
			logger.Warn("reed: failed to pin the follow jump line", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		}
	} else if err := e.tmux.run("set-option", "-gu", followCommandOption); err != nil {
		logger.Warn("reed: failed to unset the follow jump line", "socket", e.Socket(), "session", e.SessionName(), "err", err)
	}
	if button != "" {
		if err := e.tmux.run("set-option", "-g", buttonCommandOption, button); err != nil {
			logger.Warn("reed: failed to pin the button click line", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		}
	} else if err := e.tmux.run("set-option", "-gu", buttonCommandOption); err != nil {
		logger.Warn("reed: failed to unset the button click line", "socket", e.Socket(), "session", e.SessionName(), "err", err)
	}
	if stamp := bindingsBuildStamp(e.build); stamp != "" {
		if err := e.tmux.run("set-option", "-g", bindingsBuildOption, stamp); err != nil {
			logger.Warn("reed: failed to stamp the bindings' build", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		}
	} else if err := e.tmux.run("set-option", "-gu", bindingsBuildOption); err != nil {
		logger.Warn("reed: failed to unset the bindings' build stamp", "socket", e.Socket(), "session", e.SessionName(), "err", err)
	}
	for _, argv := range bindingArgvs(switchPrev, switchNext, follow != "", button != "") {
		if err := e.tmux.run(argv...); err != nil {
			logger.Warn("reed: failed to pin a key binding", "socket", e.Socket(), "session", e.SessionName(), "binding", argv[2], "err", err)
		}
	}
}

// repinStaleBindingsLocked re-pins the global status formats and the navigation bindings another build pinned.
// It acts only when `bindingsStale` finds the stamp in `bindingsBuildOption` stale against this binary.
// A binary with no VCS stamp reads nothing.
// A failed read, for example when no server is up, is logged at Debug and pins nothing.
// Assumes the op lock is already held.
func (e *Engine) repinStaleBindingsLocked() {
	if e.build.Revision == "" {
		return
	}
	stamp, err := e.tmux.output("display-message", "-p", "#{"+bindingsBuildOption+"}")
	if err != nil {
		logger.Debug("reed: could not read the bindings' build stamp, leaving the bindings as they are", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		return
	}
	if !bindingsStale(stamp, e.build) {
		return
	}
	logger.Info("reed: re-pinning the navigation bindings another build pinned", "socket", e.Socket(), "session", e.SessionName(), "pinned", strings.TrimSpace(stamp), "running", bindingsBuildStamp(e.build))
	for _, argv := range statusFormatPinArgvs() {
		if err := e.tmux.run(argv...); err != nil {
			logger.Warn("reed: failed to re-pin a status bar option another build pinned", "socket", e.Socket(), "session", e.SessionName(), "option", argv[2], "err", err)
		}
	}
	e.pinBindingsLocked()
}

// switchBindingCommandsLocked returns the shell lines for M-Left, M-Right, the follow button's jump and a button click.
// All four are empty when the socket path, the tmux path or this binary cannot be resolved.
func (e *Engine) switchBindingCommandsLocked() (prev, next, follow, button string) {
	socketPath, err := e.tmux.output("display-message", "-p", "#{socket_path}")
	socketPath = strings.TrimSpace(socketPath)
	if err != nil || socketPath == "" {
		logger.Warn("reed: could not read the server's socket path, pinning no session-switch binding", "socket", e.Socket(), "err", err)
		return "", "", "", ""
	}
	tmuxPath, err := exec.LookPath(e.cfg.Tmux)
	if err == nil {
		tmuxPath, err = filepath.Abs(tmuxPath)
	}
	if err != nil {
		logger.Warn("reed: could not resolve the tmux binary, pinning no session-switch binding", "tmux", e.cfg.Tmux, "err", err)
		return "", "", "", ""
	}
	sh := shell.ForGOOS()
	prev, prevOK := composeSwitchCommand(sh, e.executablePath, false, socketPath, tmuxPath)
	next, nextOK := composeSwitchCommand(sh, e.executablePath, true, socketPath, tmuxPath)
	follow, followOK := composeFollowCommand(sh, e.executablePath, socketPath, tmuxPath)
	button, buttonOK := composeButtonCommand(sh, e.executablePath, socketPath, tmuxPath)
	if !prevOK || !nextOK || !followOK || !buttonOK {
		return "", "", "", ""
	}
	return prev, next, follow, button
}
