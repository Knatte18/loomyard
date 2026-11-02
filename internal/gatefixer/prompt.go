// prompt.go renders a fixer's prompt: the shared cold frame joined ahead of one caller body stencil and filled from told values.

package gatefixer

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// FrameStencil is the name of the shared cold frame every fixer prompt opens with.
const FrameStencil = "gatefix-frame-cold"

// patternDirectiveMarker is the frame's optional marker the PATTERN directive fills.
const patternDirectiveMarker = "pattern_directive"

// PromptInputs is what RenderPrompt fills the frame and one body stencil from.
type PromptInputs struct {
	// StencilsDir is the told stencils directory every stencil is read from.
	StencilsDir string
	// BodyStencil names the caller body stencil joined behind the frame.
	BodyStencil string
	// BodyValues are the body's marker values; a key naming the report or reply path is overwritten by the frame's own.
	BodyValues map[string]string
	// ReportPath is the report the fixer reads, and ReplyPath the reply file it writes; both are required.
	ReportPath string
	ReplyPath  string
	// RepoRoot and PatternChanges render the PATTERN directive.
	RepoRoot       string
	PatternChanges []string
	// ParentName renders the parent directive; an empty one renders its no-parent variant.
	ParentName string
	// NotePath is the friction note path, empty when friction capture is off.
	NotePath string
}

// RenderPrompt reads FrameStencil and in.BodyStencil from in.StencilsDir, joins them frame first and fills the result.
// The report and reply paths and every body value are filled, with the frame's own paths winning over a body value of the same key, beside the PATTERN, parent, edit and friction directives.
// It refuses an empty ReportPath or ReplyPath, and returns an error for a stencil that cannot be read or a directive that cannot be rendered.
// A friction directive that cannot be rendered is logged and left out.
func RenderPrompt(in PromptInputs) ([]byte, error) {
	if strings.TrimSpace(in.ReportPath) == "" {
		return nil, fmt.Errorf("gatefixer: render the %s prompt: ReportPath is empty", in.BodyStencil)
	}
	if strings.TrimSpace(in.ReplyPath) == "" {
		return nil, fmt.Errorf("gatefixer: render the %s prompt: ReplyPath is empty", in.BodyStencil)
	}

	frame, err := stencilstore.Read(in.StencilsDir, FrameStencil)
	if err != nil {
		return nil, fmt.Errorf("gatefixer: read the fixer frame: %w", err)
	}
	body, err := stencilstore.Read(in.StencilsDir, in.BodyStencil)
	if err != nil {
		return nil, fmt.Errorf("gatefixer: read the fixer body: %w", err)
	}
	template := []byte(stencil.StripLeadingComment(string(frame)) + "\n\n" + stencil.StripLeadingComment(string(body)))

	patternDirective, err := pattern.Directive(in.RepoRoot, in.StencilsDir, pattern.RoleImplementer, in.PatternChanges)
	if err != nil {
		return nil, fmt.Errorf("gatefixer: the %s prompt's PATTERN directive: %w", in.BodyStencil, err)
	}
	parentDirective, err := parentdirective.Directive(in.StencilsDir, in.ParentName, false)
	if err != nil {
		return nil, fmt.Errorf("gatefixer: the %s prompt's parent directive: %w", in.BodyStencil, err)
	}
	editDirective, err := editdirective.Directive(in.StencilsDir)
	if err != nil {
		return nil, fmt.Errorf("gatefixer: the %s prompt's edit directive: %w", in.BodyStencil, err)
	}
	frictionDirective, err := friction.Directive(in.NotePath, in.StencilsDir, friction.RoleImplementer)
	if err != nil {
		logger.Warn("gatefixer: friction directive failed, continuing without one", "stencil", in.BodyStencil, "error", err)
		frictionDirective = ""
	}

	values := make(map[string]string, len(in.BodyValues)+6)
	for key, value := range in.BodyValues {
		values[key] = value
	}
	values["report_path"] = in.ReportPath
	values["reply_path"] = in.ReplyPath
	values[patternDirectiveMarker] = patternDirective
	values[parentdirective.MarkerName] = parentDirective
	values[editdirective.MarkerName] = editDirective
	values[friction.MarkerName] = frictionDirective

	friction.WarnIfMarkerAbsent(template, FrameStencil, frictionDirective)
	prompt, err := stencil.FillOptional(template, values, []string{patternDirectiveMarker, friction.MarkerName, parentdirective.MarkerName})
	if err != nil {
		return nil, fmt.Errorf("gatefixer: fill the %s prompt: %w", in.BodyStencil, err)
	}
	return prompt, nil
}
