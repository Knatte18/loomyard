// prompt.go renders the orch role and the orch stencils.
// The role and its on-demand files are filled from their embedded bytes, since no hub may edit them.
// Every other stencil is read at call time from the caller's stencils directory, per the Stencil Ownership Invariant.
// Each text is filled with stencil.Fill, which drops the leading comment of the stencil.
// The role, its on-demand files and the note template render to files a session reads;
// the two note requests render to the mail body creel posts.
// Every other render is one creel text line or a shuttle launch pointer, so it must render to one line;
// an operator override that breaks that fails here, naming the stencil to fix.
// WithPresence appends the presence line to a pointer while the machine's presence is slack.

package orchengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/contracts/orchrole"
	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/slackengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// presenceLine is appended to a pointer's own line while the machine's presence is slack.
const presenceLine = " Presence is slack: also post every message meant for the operator to the hub's SLACK program, as the orch role's Slack rules say."

// WithPresence returns pointer with the presence line appended while the machine's presence is slack.
// The presence is read from p.SlackPresencePath.
// The line joins the pointer's own line, since a typed pointer is one creel text line.
// An empty path, a hub with no presence record and a record that cannot be read all leave the pointer as it is;
// a failed read is logged at Debug.
func WithPresence(p Paths, pointer string) string {
	if p.SlackPresencePath == "" {
		return pointer
	}
	presence, err := slackengine.ReadPresence(p.SlackPresencePath)
	if err != nil {
		logger.Debug("orch: the presence could not be read; the pointer carries no presence line", "path", p.SlackPresencePath, "error", err)
		return pointer
	}
	if presence != slackengine.PresenceSlack {
		return pointer
	}
	return pointer + presenceLine
}

const (
	// roleName names the orch role in a render error.
	roleName        = "orch role"
	noteStencilName = "orch-template-note"

	startStencilName   = "orch-template-start"
	handoffStencilName = "orch-template-handoff"
	resumeStencilName  = "orch-template-resume"

	softHandoffStencilName = "orch-template-handoff-soft"
	compactStencilName     = "orch-template-compact"
	reloadStencilName      = "orch-template-reload"
	usageLimitWakeStencil  = "orch-template-usage-limit-wake"
)

// decisionNoteTitleMarker is the role's marker that the decision-note title form fills.
const decisionNoteTitleMarker = "decision_note_title"

// RenderRoleFile renders the orch role to path and each of its on-demand files beside it, creating the directory.
// The role is the same for every orch in every repository.
// The texts come from the bytes built into the binary, never from a board copy, so no hub can change them.
// The decision-note title marker is filled with the title form for the placeholder slug "<slug>".
// The on-demand files are written first, so a role a session reads never names a file not yet there.
// Each write is a temporary file and a rename, so a reader never sees a partial file when two renders overlap.
func RenderRoleFile(path string) error {
	for name, text := range orchrole.OnDemand {
		if err := writeFilled(name, text, filepath.Join(filepath.Dir(path), name), nil); err != nil {
			return err
		}
	}
	return writeFilled(roleName, orchrole.Role, path, map[string]string{decisionNoteTitleMarker: DecisionNoteTitle("<slug>")})
}

// DecisionNoteTitle returns the title prefix a decision note for the held task slug starts with: "Decision for <slug>:".
// It is the one declaration of the form: the orch role render and the no-record way forward of "lym loom decision add" both call it.
// The colon keeps a slug from matching a longer slug that starts with it, and the match is case-sensitive.
func DecisionNoteTitle(slug string) string {
	return "Decision for " + slug + ":"
}

// RenderNoteTemplateFile renders the note template stencil to path, creating its directory.
func RenderNoteTemplateFile(stencilsDir, path string) error {
	return renderFile(stencilsDir, noteStencilName, path, nil)
}

// RenderStartPrompt renders the one-line fresh-launch pointer, with rolePath filled in.
func RenderStartPrompt(stencilsDir, rolePath string) (string, error) {
	return render(stencilsDir, startStencilName, map[string]string{"role_path": rolePath}, true)
}

// RenderHandoffInstruction renders the note request mail body, with handoffPath and noteTemplatePath filled in.
// The body may span several lines and its first line stays the same for the same paths.
func RenderHandoffInstruction(stencilsDir, handoffPath, noteTemplatePath string) (string, error) {
	return renderMailBody(stencilsDir, handoffStencilName, map[string]string{"handoff_path": handoffPath, "note_template_path": noteTemplatePath})
}

// RenderResumePrompt renders the one-line resume pointer, with rolePath and handoffPath filled in.
func RenderResumePrompt(stencilsDir, rolePath, handoffPath string) (string, error) {
	return render(stencilsDir, resumeStencilName, map[string]string{"role_path": rolePath, "handoff_path": handoffPath}, true)
}

// RenderSoftHandoffInstruction renders the soft-trigger note request mail body, with handoffPath and noteTemplatePath filled in.
func RenderSoftHandoffInstruction(stencilsDir, handoffPath, noteTemplatePath string) (string, error) {
	return renderMailBody(stencilsDir, softHandoffStencilName, map[string]string{"handoff_path": handoffPath, "note_template_path": noteTemplatePath})
}

// renderMailBody renders one stencil with values to a mail body:
// surrounding whitespace is trimmed and any number of lines is kept.
func renderMailBody(stencilsDir, name string, values map[string]string) (string, error) {
	out, err := render(stencilsDir, name, values, false)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// RenderReloadPrompt renders the one-line reload pointer typed after an auto-compaction, with rolePath filled in.
func RenderReloadPrompt(stencilsDir, rolePath string) (string, error) {
	return render(stencilsDir, reloadStencilName, map[string]string{"role_path": rolePath}, true)
}

// RenderUsageLimitWake renders the one-line wake line typed once a usage limit that stopped the orch should have reset.
// A zero resetsAt renders that the reset time could not be read; any other renders it in its own zone.
func RenderUsageLimitWake(stencilsDir string, resetsAt time.Time) (string, error) {
	reset := "has a reset time that could not be read, so this is a retry"
	if !resetsAt.IsZero() {
		reset = "reset at " + resetsAt.Format("2006-01-02 15:04 -07:00")
	}
	return render(stencilsDir, usageLimitWakeStencil, map[string]string{"reset": reset}, true)
}

// RenderCompactFocus renders the one-line focus text typed after `/compact`.
func RenderCompactFocus(stencilsDir string) (string, error) {
	return render(stencilsDir, compactStencilName, nil, true)
}

// renderFile reads one stencil, fills it with values and writes it to path.
func renderFile(stencilsDir, name, path string, values map[string]string) error {
	template, err := stencilstore.Read(stencilsDir, name)
	if err != nil {
		return fmt.Errorf("orch: read %s: %w", name, err)
	}
	return writeFilled(name, template, path, values)
}

// writeFilled fills template, named name in errors, with values and writes it to path, creating the directory.
func writeFilled(name string, template []byte, path string, values map[string]string) error {
	out, err := fill(name, template, values, false)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("orch: create dir for %s: %w", name, err)
	}
	if err := fsx.AtomicWriteBytes(path, []byte(out)); err != nil {
		return fmt.Errorf("orch: write %s: %w", name, err)
	}
	return nil
}

// render reads and fills one stencil.
// singleLine trims surrounding whitespace and errors when a newline remains.
func render(stencilsDir, name string, values map[string]string, singleLine bool) (string, error) {
	template, err := stencilstore.Read(stencilsDir, name)
	if err != nil {
		return "", fmt.Errorf("orch: read %s: %w", name, err)
	}
	return fill(name, template, values, singleLine)
}

// fill fills template, named name in errors, with values.
// singleLine trims surrounding whitespace and errors when a newline remains.
func fill(name string, template []byte, values map[string]string, singleLine bool) (string, error) {
	filled, err := stencil.Fill(template, values)
	if err != nil {
		return "", fmt.Errorf("orch: fill %s: %w", name, err)
	}
	out := string(filled)
	if !singleLine {
		return out, nil
	}
	out = strings.TrimSpace(out)
	if strings.ContainsAny(out, "\r\n") {
		return "", fmt.Errorf("orch: %s renders to more than one line; a creel text inject is one line, so fix the stencil", name)
	}
	return out, nil
}
