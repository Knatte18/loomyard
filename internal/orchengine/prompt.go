// prompt.go renders the orch stencils (orch-template-role, orch-template-note, orch-template-start, orch-template-handoff, orch-template-resume, orch-template-adopt, orch-template-handoff-soft, orch-template-compact, orch-template-reload).
// Each is read from a told stencils directory at call time via stencilstore.Read, per the Stencil Ownership Invariant, and filled with stencil.Fill, which drops the leading comment.
// The role and note stencils render to files the session reads, and the two note requests render to the mail body creel posts.
// Every other render is injected as one creel text line or handed to shuttle as its launch pointer, and a creel text inject is one line,
// so each of those must render to one line;
// an operator override that breaks that fails here, naming the stencil to fix.

package orchengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

const (
	roleStencilName = "orch-template-role"
	noteStencilName = "orch-template-note"

	startStencilName   = "orch-template-start"
	handoffStencilName = "orch-template-handoff"
	resumeStencilName  = "orch-template-resume"

	adoptStencilName       = "orch-template-adopt"
	softHandoffStencilName = "orch-template-handoff-soft"
	compactStencilName     = "orch-template-compact"
	reloadStencilName      = "orch-template-reload"
)

const (
	// commandIndexMarker is the role stencil's marker that index fills.
	commandIndexMarker = "command_index"
	// decisionNoteTitleMarker is the role stencil's marker that the decision-note title form fills.
	decisionNoteTitleMarker = "decision_note_title"
	// hubRulesMarker is the role stencil's marker that the hub-wide rules file fills.
	hubRulesMarker = "hub_rules"
	// orchRulesMarker is the role stencil's marker that this orch's own rules file fills.
	orchRulesMarker = "orch_rules"
	// noHubRules fills hubRulesMarker when the hub-wide rules file is absent or blank; its verb is the path.
	noHubRules = "This hub has no hub-wide orch rules; they go in %s."
	// noOwnRules fills orchRulesMarker when this orch's own rules file is absent or blank; its verb is the path.
	noOwnRules = "This orch has no rules of its own; they go in %s."
)

// RenderRoleFile renders the role stencil to path, creating its directory.
// The command-index marker is filled with index, the hub-rules marker with the file at hubRulesPath and the orch-rules marker with the file at ownRulesPath.
// The decision-note title marker is filled with the title form for the placeholder slug "<slug>", since the role stencil is hub-wide.
// A role stencil that lacks any of the four markers fails the render naming the stencil, so an operator-edited copy never silently renders without them.
// A rules file that cannot be read returns its error and writes no file.
// The write is a temporary file and a rename, so a reader never sees a partial file when two renders overlap.
// The session reads the file through the one-line pointers, so a stencil edit applies from the next delivery.
func RenderRoleFile(stencilsDir, path, index, hubRulesPath, ownRulesPath string) error {
	template, err := stencilstore.Read(stencilsDir, roleStencilName)
	if err != nil {
		return fmt.Errorf("orch: read %s: %w", roleStencilName, err)
	}
	markers, err := stencil.TopLevelMarkers(template)
	if err != nil {
		return fmt.Errorf("orch: read markers of %s: %w", roleStencilName, err)
	}
	for _, marker := range []string{commandIndexMarker, hubRulesMarker, orchRulesMarker, decisionNoteTitleMarker} {
		if !slices.Contains(markers, marker) {
			return fmt.Errorf("orch: %s declares no {{.%s}} marker: restore the marker, or delete the override so the shipped stencil applies", roleStencilName, marker)
		}
	}
	hubRules, err := readRules(hubRulesPath, noHubRules)
	if err != nil {
		return err
	}
	ownRules, err := readRules(ownRulesPath, noOwnRules)
	if err != nil {
		return err
	}
	return renderFile(stencilsDir, roleStencilName, path, map[string]string{commandIndexMarker: index, hubRulesMarker: hubRules, orchRulesMarker: ownRules, decisionNoteTitleMarker: DecisionNoteTitle("<slug>")})
}

// DecisionNoteTitle returns the title prefix a decision note for the held task slug starts with: "Decision for <slug>:".
// It is the one declaration of the form: the orch role render and the no-record way forward of "lyx loom decision add" both call it.
// The colon keeps a slug from matching a longer slug that starts with it, and the match is case-sensitive.
func DecisionNoteTitle(slug string) string {
	return "Decision for " + slug + ":"
}

// readRules returns the content of the rules file at path, as it stands.
// A missing file, or one holding only whitespace, yields placeholder with path filled in.
func readRules(path, placeholder string) (string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Sprintf(placeholder, path), nil
	}
	if err != nil {
		return "", fmt.Errorf("orch: read rules %s: %w; fix the file or remove it", path, err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return fmt.Sprintf(placeholder, path), nil
	}
	return string(data), nil
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

// RenderAdoptPrompt renders the one-line launch pointer for an adopted session, with rolePath filled in.
func RenderAdoptPrompt(stencilsDir, rolePath string) (string, error) {
	return render(stencilsDir, adoptStencilName, map[string]string{"role_path": rolePath}, true)
}

// RenderSoftHandoffInstruction renders the soft-trigger note request mail body, with handoffPath and noteTemplatePath filled in.
func RenderSoftHandoffInstruction(stencilsDir, handoffPath, noteTemplatePath string) (string, error) {
	return renderMailBody(stencilsDir, softHandoffStencilName, map[string]string{"handoff_path": handoffPath, "note_template_path": noteTemplatePath})
}

// renderMailBody renders one stencil with values to a mail body: surrounding whitespace is trimmed and any number of lines is kept.
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

// RenderCompactFocus renders the one-line focus text typed after `/compact`.
func RenderCompactFocus(stencilsDir string) (string, error) {
	return render(stencilsDir, compactStencilName, nil, true)
}

// renderFile renders one stencil with values and writes it to path, creating the directory.
func renderFile(stencilsDir, name, path string, values map[string]string) error {
	out, err := render(stencilsDir, name, values, false)
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
