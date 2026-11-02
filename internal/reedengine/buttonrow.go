// buttonrow.go renders a session's status-bar buttons into session options: the two drawn rows and the spec and run directory a click reads.
// The render runs in the geometry pins, at boot and at every attach pre-flight.

package reedengine

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shell"
)

const (
	// buttonSpecOption is the session user option holding the JSON spec of the drawn buttons, which a click reads to find what to run.
	buttonSpecOption = "@lym_button_spec"

	// buttonDirOption is the session user option holding the run directory as one word quoted in reed's shell dialect.
	// The pinned click line reads it to change into the directory before the click verb starts.
	buttonDirOption = "@lym_button_dir"

	// buttonRangePrefix starts the user range name of every drawn button, followed by the button's index in the spec.
	// With a two-digit index the name stays within tmux's 15-byte limit for range=user|X.
	buttonRangePrefix = "lymbtn"
)

// buttonSpec is the JSON stored in buttonSpecOption: how to run a click and the buttons the row draws.
type buttonSpec struct {
	// Shell is the absolute path of reed's configured shell.
	Shell string `json:"shell"`
	// Flag is the flag that makes Shell run its next argument as a command line.
	Flag string `json:"flag"`
	// Dir is the run directory as rendered; a click runs in the current anchor of the worktree holding it.
	Dir string `json:"dir"`
	// Entries are the drawn buttons, entry N being drawn as the range buttonRangePrefix followed by N.
	Entries []buttonSpecEntry `json:"entries"`
}

// buttonSpecEntry is one drawn button in a buttonSpec.
type buttonSpecEntry struct {
	// Index is the entry's position in the row, the number its range carries.
	Index int `json:"index"`
	// Label is the text drawn on the button.
	Label string `json:"label"`
	// Run is the command line with the static placeholders filled; {strand} stays for the click.
	Run string `json:"run"`
	// Confirm is the prompt asked before Run, with the static placeholders filled; empty asks nothing.
	Confirm string `json:"confirm"`
	// Strand reports that Run or Confirm still holds {strand} after the static fill.
	Strand bool `json:"strand"`
}

// buttonRangeFormat is one drawn button: a user range named by index, labelled with the entry's label and styled unlit.
func buttonRangeFormat(index int, label string) string {
	return fmt.Sprintf("#[range=user|%s%d %s] %s #[norange list=on default] ", buttonRangePrefix, index, unlitButtonStyle, label)
}

// composeButtonRowLocked renders the session's buttons: the plain row drawn in a window whose active pane is no strand pane, the agent row drawn in every window, and the spec a click reads.
// A standalone geometry, a failed button context, an unresolvable shell and a failed beam lookup each give empty rows and a zero spec, so line 3 shows follow alone.
// Reed's entries come before the machine's, each valid entry numbered in that order.
// An entry using {beam} is left out, with a Warn, when the beam's directory is absent on this machine.
func (e *Engine) composeButtonRowLocked() (plain, agent string, spec buttonSpec) {
	if e.geom.Buttons == nil {
		return "", "", buttonSpec{}
	}
	ctx, err := e.geom.Buttons()
	if err != nil {
		logger.Warn("reed: could not read the button context, rendering no buttons", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		return "", "", buttonSpec{}
	}
	shellPath, err := resolveShellPath(e.cfg.Shell)
	if err != nil {
		logger.Warn("reed: the shell does not resolve, rendering no buttons", "socket", e.Socket(), "session", e.SessionName(), "shell", e.cfg.Shell, "err", err)
		return "", "", buttonSpec{}
	}

	entries := placeButtons(e.cfg.Buttons, ctx.Place, "reed.yaml (the hub base or the worktree file)")
	entries = append(entries, placeButtons(ctx.MachineButtons, ctx.Place, "desktop.machine.yaml")...)
	if len(entries) == 0 {
		return "", "", buttonSpec{}
	}

	values := map[string]string{"slug": ctx.Slug, "worktree": ctx.Worktree, "session": ctx.Session}
	dir := e.geom.AnchorPath
	var beam ButtonBeam
	if ctx.Place == ButtonPlaceTask || usesBeamPlaceholder(entries) {
		beam, err = ctx.Beam()
		if err != nil {
			logger.Warn("reed: could not resolve the beam, rendering no buttons", "socket", e.Socket(), "session", e.SessionName(), "err", err)
			return "", "", buttonSpec{}
		}
		if beam.Anchor != "" {
			values["beam"] = beam.Anchor
		}
		if ctx.Place == ButtonPlaceTask {
			dir = beam.Anchor
			if dir == "" {
				dir = beam.PrimeAnchor
			}
		}
	}

	sh := shell.ForGOOS()
	spec = buttonSpec{Shell: shellPath, Flag: sh.CommandFlag(), Dir: dir}
	for _, entry := range entries {
		if beam.Anchor == "" && (strings.Contains(entry.Run, "{beam}") || strings.Contains(entry.Confirm, "{beam}")) {
			logger.Warn("reed: left out a button whose beam is not on this machine; take the beam onto this machine with lym adopt <beam>, and the row re-renders at the session's next attach",
				"socket", e.Socket(), "session", e.SessionName(), "label", entry.Label, "beam", beam.Name)
			continue
		}
		run := fillRunPlaceholders(sh, entry.Run, values)
		confirm := fillConfirmPlaceholders(entry.Confirm, values)
		index := len(spec.Entries)
		spec.Entries = append(spec.Entries, buttonSpecEntry{
			Index:   index,
			Label:   entry.Label,
			Run:     run,
			Confirm: confirm,
			Strand:  strings.Contains(run, "{strand}") || strings.Contains(confirm, "{strand}"),
		})
		fragment := buttonRangeFormat(index, entry.Label)
		agent += fragment
		if entry.Window != buttonWindowAgents {
			plain += fragment
		}
	}
	if len(spec.Entries) == 0 {
		return "", "", buttonSpec{}
	}
	return plain, agent, spec
}

// usesBeamPlaceholder reports whether any entry's run or confirm text names {beam}.
func usesBeamPlaceholder(entries []buttonEntry) bool {
	for _, entry := range entries {
		if strings.Contains(entry.Run, "{beam}") || strings.Contains(entry.Confirm, "{beam}") {
			return true
		}
	}
	return false
}

// pinButtonRowLocked sets the two row options, the spec option and the run directory option on target in one tmux invocation,
// so no click sees a row and a spec from different renders.
// An empty render sets all four to the empty string.
// A failure is logged at Warn and ignored, like every status pin.
// Assumes the op lock is already held.
func (e *Engine) pinButtonRowLocked(target string) {
	plain, agent, spec := e.composeButtonRowLocked()
	var specJSON, dirWord string
	if len(spec.Entries) > 0 {
		// A struct of strings, numbers and bools cannot fail to marshal.
		encoded, _ := json.Marshal(spec)
		specJSON = string(encoded)
		dirWord = shell.ForGOOS().Quote(spec.Dir)
	}
	argv := []string{
		"set-option", "-t", target, buttonRowPlainOption, plain, ";",
		"set-option", "-t", target, buttonRowAgentOption, agent, ";",
		"set-option", "-t", target, buttonSpecOption, specJSON, ";",
		"set-option", "-t", target, buttonDirOption, dirWord,
	}
	if err := e.tmux.run(argv...); err != nil {
		logger.Warn("reed: failed to pin the button row", "socket", e.Socket(), "session", e.SessionName(), "err", err)
	}
}
