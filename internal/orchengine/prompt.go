// prompt.go renders the three orch stencils (orch-template-start, orch-template-handoff,
// orch-template-resume).
// Each is read from a told stencils directory at call time via stencilstore.Read, per the Stencil
// Ownership Invariant, and filled with stencil.Fill, which drops the leading comment.
// The handoff and resume renders are typed into the session through shuttle's Send, which refuses
// multi-line text, so each must render to one line; an operator override that breaks that fails
// here, naming the stencil to fix.

package orchengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

const (
	startStencilName   = "orch-template-start"
	handoffStencilName = "orch-template-handoff"
	resumeStencilName  = "orch-template-resume"
)

// RenderStartPrompt renders the fresh-launch prompt read from stencilsDir.
func RenderStartPrompt(stencilsDir string) (string, error) {
	return render(stencilsDir, startStencilName, nil, false)
}

// RenderHandoffInstruction renders the one-line handoff instruction, with handoffPath filled in.
func RenderHandoffInstruction(stencilsDir, handoffPath string) (string, error) {
	return render(stencilsDir, handoffStencilName, map[string]string{"handoff_path": handoffPath}, true)
}

// RenderResumePrompt renders the one-line resume prompt, with handoffPath filled in.
func RenderResumePrompt(stencilsDir, handoffPath string) (string, error) {
	return render(stencilsDir, resumeStencilName, map[string]string{"handoff_path": handoffPath}, true)
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
