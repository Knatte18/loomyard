// arbiterprompt.go implements the arbiter session's launch prompt and shuttle spec.
// Both are pure composers: the loom arbiter stencil filled over one stop's paths, and the spec the arbiter launches from.

package loomcli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// arbiterStencilName is the registered name of the arbiter's opening stencil.
const arbiterStencilName = "loom-template-arbiter"

// arbiterAbsentPath fills the report and brief markers when the stop has no such file.
const arbiterAbsentPath = "none"

// arbiterSkills is the skill list the arbiter loads before its launch prompt: none, since the prompt is its whole procedure.
var arbiterSkills = []string{}

// arbiterPromptInput carries what the arbiter stencil's markers are filled from.
// ReportPath, BriefPath and CaseBriefPath may be empty; every other field is required.
type arbiterPromptInput struct {
	RunID              string
	Slug               string
	Worktree           string
	EnvelopePath       string
	ReportPath         string
	BriefPath          string
	CaseBriefPath      string
	DecisionRecordPath string
	SupportLogPath     string
	VerdictPath        string
}

// arbiterPrompt composes the arbiter session's launch prompt.
// The arbiter stencil is read from stencilsDir through stencilstore.Read and filled with in, parentdirective.Directive(stencilsDir, "", false) and editdirective.Directive(stencilsDir).
// The no-parent directive is rendered because the verdict file is the arbiter's only channel.
// An empty report, brief or case brief path is filled as the literal "none".
// It returns the read or fill error wrapped with the stencil's name.
func arbiterPrompt(stencilsDir string, in arbiterPromptInput) (string, error) {
	directive, err := parentdirective.Directive(stencilsDir, "", false)
	if err != nil {
		return "", err
	}
	editDirective, err := editdirective.Directive(stencilsDir)
	if err != nil {
		return "", err
	}
	template, err := stencilstore.Read(stencilsDir, arbiterStencilName)
	if err != nil {
		return "", fmt.Errorf("loom: arbiter prompt: read stencil %q: %w", arbiterStencilName, err)
	}
	filled, err := stencil.Fill(template, map[string]string{
		"run_id":                   in.RunID,
		"slug":                     in.Slug,
		"worktree":                 in.Worktree,
		"envelope_path":            in.EnvelopePath,
		"report_path":              pathOrNone(in.ReportPath),
		"brief_path":               pathOrNone(in.BriefPath),
		"case_brief_path":          pathOrNone(in.CaseBriefPath),
		"decision_record_path":     in.DecisionRecordPath,
		"support_log_path":         in.SupportLogPath,
		"verdict_path":             in.VerdictPath,
		parentdirective.MarkerName: directive,
		editdirective.MarkerName:   editDirective,
	})
	if err != nil {
		return "", fmt.Errorf("loom: arbiter prompt: fill stencil %q: %w", arbiterStencilName, err)
	}
	return string(filled), nil
}

// pathOrNone returns path, or the literal "none" for an empty one.
func pathOrNone(path string) string {
	if path == "" {
		return arbiterAbsentPath
	}
	return path
}

// arbiterSpec composes the shuttleengine.Spec the arbiter session launches from, from an already-composed prompt, the verdict path and the resolved arbiter settings.
//
// Prompt is the argument verbatim. Skills is arbiterSkills, empty, so the prompt is the arbiter's whole procedure.
// OutputFiles is a single-entry slice holding verdictPath, so shuttle's completion check reads the verdict through allOutputFilesExist and reed drops a finished strand on resume.
// ModelChoice is the settings' choice whole, which carries the RESOLVED engine, provider model id, effort and version.
// Timeout is the settings' timeout, which bounds the arbiter's run.
// Role is loomengine.ArbiterRole, which reed forms into `<shortname>:<slug>:arbiter`; NameOverride is left empty so a second attempt numbers its role.
// Segment is the coordinator, the driver's, because the arbiter acts for the driver.
// Interactive is false, shuttle's autonomous posture, which adds the operator-prompt deny an unattended session needs.
// ForkSubagents is false and Parent is empty: the arbiter spawns nothing and has no session to message.
// Display is left at its zero value.
func arbiterSpec(prompt, verdictPath string, settings loomengine.ArbiterSettings) shuttleengine.Spec {
	return shuttleengine.Spec{
		Prompt:        prompt,
		Skills:        arbiterSkills,
		OutputFiles:   []string{verdictPath},
		ModelChoice:   settings.ModelChoice,
		Timeout:       settings.Timeout,
		Role:          loomengine.ArbiterRole,
		Segment:       segmentcolor.Coordinator,
		Interactive:   false,
		ForkSubagents: false,
		Parent:        "",
	}
}
