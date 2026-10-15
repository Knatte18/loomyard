// tacklerprompt.go implements the tackler session's launch prompt and shuttle spec.
// Both are pure composers: the shed tackler stencil filled over one stop's paths, and the spec the tackler launches from.

package sheddriver

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// tacklerStencilName is the registered name of the tackler's opening stencil.
const tacklerStencilName = "shed-template-tackler"

// tacklerSkills is the skill list the tackler loads before its launch prompt: none, since the prompt is its whole procedure.
var tacklerSkills = []string{}

// TacklerPromptInput carries what the tackler stencil's markers are filled from.
// ReportPath may be empty; every other field is required.
type TacklerPromptInput struct {
	RunID string
	Slug  string
	// Recipe is the one-line description of the recipe the run belongs to.
	Recipe string
	// GotoRows names the rows a resume may name in its goto.
	GotoRows      string
	Worktree      string
	EnvelopePath  string
	ReportPath    string
	TraceCopy     string
	StderrPath    string
	TraceDir      string
	DriverLog     string
	VerdictPath   string
	CaseBriefPath string
}

// tacklerPrompt composes the tackler session's launch prompt.
// The tackler stencil is read from stencilsDir through stencilstore.Read and filled with in, parentdirective.Directive(stencilsDir, "", false) and editdirective.Directive(stencilsDir).
// The no-parent directive is rendered because the verdict file is the tackler's only channel.
// An empty report path is filled as the literal "none".
// An empty Recipe or GotoRows is a wiring fault and is refused, naming the marker.
// It returns the read or fill error wrapped with the stencil's name.
func tacklerPrompt(stencilsDir string, in TacklerPromptInput) (string, error) {
	for _, required := range []struct{ marker, value string }{{"recipe", in.Recipe}, {"goto_rows", in.GotoRows}} {
		if required.value == "" {
			return "", fmt.Errorf("sheddriver: tackler prompt: the %s marker is empty", required.marker)
		}
	}
	directive, err := parentdirective.Directive(stencilsDir, "", false)
	if err != nil {
		return "", err
	}
	editDirective, err := editdirective.Directive(stencilsDir)
	if err != nil {
		return "", err
	}
	template, err := stencilstore.Read(stencilsDir, tacklerStencilName)
	if err != nil {
		return "", fmt.Errorf("sheddriver: tackler prompt: read stencil %q: %w", tacklerStencilName, err)
	}
	filled, err := stencil.Fill(template, map[string]string{
		"run_id":                   in.RunID,
		"slug":                     in.Slug,
		"recipe":                   in.Recipe,
		"goto_rows":                in.GotoRows,
		"worktree":                 in.Worktree,
		"envelope_path":            in.EnvelopePath,
		"report_path":              pathOrNone(in.ReportPath),
		"trace_copy":               in.TraceCopy,
		"stderr_path":              in.StderrPath,
		"trace_dir":                in.TraceDir,
		"driver_log":               in.DriverLog,
		"verdict_path":             in.VerdictPath,
		"case_brief_path":          in.CaseBriefPath,
		parentdirective.MarkerName: directive,
		editdirective.MarkerName:   editDirective,
	})
	if err != nil {
		return "", fmt.Errorf("sheddriver: tackler prompt: fill stencil %q: %w", tacklerStencilName, err)
	}
	return string(filled), nil
}

// pathOrNone returns path, or the literal "none" for an empty one.
func pathOrNone(path string) string {
	if path == "" {
		return "none"
	}
	return path
}

// tacklerSpec composes the shuttleengine.Spec the tackler session launches from, from an already-composed prompt, the verdict and case-brief paths and the resolved tackler settings.
//
// Prompt is the argument verbatim. Skills is tacklerSkills, empty, so the prompt is the tackler's whole procedure.
// OutputFiles is a single-entry slice holding verdictPath, so shuttle's completion check reads the verdict through allOutputFilesExist and reed drops a finished strand on resume.
// The case brief is not an output file, since only an escalate writes it; caseBriefPath reaches the prompt input alone, which is why it is not read here.
// Model, Effort and Version come from settings, which carries the RESOLVED triple -- a provider model id plus its effort and version.
// Timeout is the settings' timeout, which bounds the tackler's run.
// Role is TacklerRole, which reed forms into `<shortname>:<slug>:tackler`; NameOverride is left empty so a second attempt numbers its role.
// Segment is the coordinator, the driver's, because the tackler acts for the driver.
// Interactive is false, shuttle's autonomous posture, which adds the operator-prompt deny an unattended session needs.
// ForkSubagents is false and Parent is empty: the tackler spawns nothing and has no session to message.
func tacklerSpec(prompt, verdictPath, caseBriefPath string, settings TacklerSettings) shuttleengine.Spec {
	return shuttleengine.Spec{
		Prompt:        prompt,
		Skills:        tacklerSkills,
		OutputFiles:   []string{verdictPath},
		Model:         settings.Model,
		Effort:        settings.Effort,
		Version:       settings.Version,
		Timeout:       settings.Timeout,
		Role:          TacklerRole,
		Segment:       segmentcolor.Coordinator,
		Interactive:   false,
		ForkSubagents: false,
		Parent:        "",
	}
}
