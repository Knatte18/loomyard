// prompt.go renders the orch stencils (orch-template-role, orch-template-note, orch-template-start, orch-template-handoff, orch-template-resume, orch-template-adopt, orch-template-handoff-soft, orch-template-compact, orch-template-reload).
// Each is read from a told stencils directory at call time via stencilstore.Read, per the Stencil Ownership Invariant, and filled with stencil.Fill, which drops the leading comment.
// The role and note stencils render to files the session reads; every other render is typed into the session or handed to shuttle as its launch pointer,
// and shuttle's Send refuses multi-line text, so each of those must render to one line;
// an operator override that breaks that fails here, naming the stencil to fix.

package orchengine

import (
	"fmt"
	"os"
	"path/filepath"
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

// RenderRoleFile renders the role stencil to path, creating its directory.
// The write is a temporary file and a rename, so a reader never sees a partial file when two renders overlap.
// The session reads the file through the one-line pointers, so a stencil edit applies from the next delivery.
func RenderRoleFile(stencilsDir, path string) error {
	return renderFile(stencilsDir, roleStencilName, path)
}

// RenderNoteTemplateFile renders the note template stencil to path, creating its directory.
func RenderNoteTemplateFile(stencilsDir, path string) error {
	return renderFile(stencilsDir, noteStencilName, path)
}

// RenderStartPrompt renders the one-line fresh-launch pointer, with rolePath filled in.
func RenderStartPrompt(stencilsDir, rolePath string) (string, error) {
	return render(stencilsDir, startStencilName, map[string]string{"role_path": rolePath}, true)
}

// RenderHandoffInstruction renders the one-line note request, with handoffPath and noteTemplatePath filled in.
func RenderHandoffInstruction(stencilsDir, handoffPath, noteTemplatePath string) (string, error) {
	return render(stencilsDir, handoffStencilName, map[string]string{"handoff_path": handoffPath, "note_template_path": noteTemplatePath}, true)
}

// RenderResumePrompt renders the one-line resume pointer, with rolePath and handoffPath filled in.
func RenderResumePrompt(stencilsDir, rolePath, handoffPath string) (string, error) {
	return render(stencilsDir, resumeStencilName, map[string]string{"role_path": rolePath, "handoff_path": handoffPath}, true)
}

// RenderAdoptPrompt renders the one-line launch pointer for an adopted session, with rolePath filled in.
func RenderAdoptPrompt(stencilsDir, rolePath string) (string, error) {
	return render(stencilsDir, adoptStencilName, map[string]string{"role_path": rolePath}, true)
}

// RenderSoftHandoffInstruction renders the one-line soft-trigger note request, with handoffPath and noteTemplatePath filled in.
func RenderSoftHandoffInstruction(stencilsDir, handoffPath, noteTemplatePath string) (string, error) {
	return render(stencilsDir, softHandoffStencilName, map[string]string{"handoff_path": handoffPath, "note_template_path": noteTemplatePath}, true)
}

// RenderReloadPrompt renders the one-line reload pointer typed after an auto-compaction, with rolePath filled in.
func RenderReloadPrompt(stencilsDir, rolePath string) (string, error) {
	return render(stencilsDir, reloadStencilName, map[string]string{"role_path": rolePath}, true)
}

// RenderCompactFocus renders the one-line focus text typed after `/compact`.
func RenderCompactFocus(stencilsDir string) (string, error) {
	return render(stencilsDir, compactStencilName, nil, true)
}

// renderFile renders one marker-free stencil and writes it to path, creating the directory.
func renderFile(stencilsDir, name, path string) error {
	out, err := render(stencilsDir, name, nil, false)
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
		return "", fmt.Errorf("orch: %s renders to more than one line; shuttle's Send refuses multi-line text, so fix the stencil", name)
	}
	return out, nil
}
