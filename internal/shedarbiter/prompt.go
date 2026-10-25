// prompt.go implements the arbiter session's launch prompt, shuttle spec and verdict gate.
// The prompt and spec are pure composers: the arbiter stencil filled over one stop's paths and the recipe's description, and the spec the arbiter launches from.

package shedarbiter

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// stencilName is the registered name of the arbiter's opening stencil.
const stencilName = "loom-template-arbiter"

// AbsentPath fills a path marker, and the recipe marker, when the stop has no such file.
const AbsentPath = "none"

// verdictGateAttempts is how many times the verdict gate re-prompts an arbiter whose verdict file does not read.
const verdictGateAttempts = 2

// skills is the skill list the arbiter loads before its launch prompt: none, since the prompt is its whole procedure.
var skills = []string{}

// PromptInput carries what the arbiter stencil's markers are filled from.
// Recipe, ReportPath, BriefPath, CaseBriefPath, DecisionRecordPath, SupportLogPath and AmendmentTarget may be empty; every other field is required.
// Recipe is the description of the run's recipe, including the repair verbs the arbiter may run.
// AmendmentTarget is the row a plan amendment routes to.
type PromptInput struct {
	Recipe             string
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
	AmendmentTarget    string
}

// Prompt composes the arbiter session's launch prompt.
// The arbiter stencil is read from stencilsDir through stencilstore.Read and filled with in, parentdirective.Directive(stencilsDir, "", false) and editdirective.Directive(stencilsDir).
// The no-parent directive is rendered because the verdict file is the arbiter's only channel.
// An empty recipe description, report, brief, case brief, decision record, support log or amendment target is filled as the literal "none".
// It returns the read or fill error wrapped with the stencil's name.
func Prompt(stencilsDir string, in PromptInput) (string, error) {
	directive, err := parentdirective.Directive(stencilsDir, "", false)
	if err != nil {
		return "", err
	}
	editDirective, err := editdirective.Directive(stencilsDir)
	if err != nil {
		return "", err
	}
	template, err := stencilstore.Read(stencilsDir, stencilName)
	if err != nil {
		return "", fmt.Errorf("loom: arbiter prompt: read stencil %q: %w", stencilName, err)
	}
	filled, err := stencil.Fill(template, map[string]string{
		"recipe":                   pathOrNone(in.Recipe),
		"run_id":                   in.RunID,
		"slug":                     in.Slug,
		"worktree":                 in.Worktree,
		"envelope_path":            in.EnvelopePath,
		"report_path":              pathOrNone(in.ReportPath),
		"brief_path":               pathOrNone(in.BriefPath),
		"case_brief_path":          pathOrNone(in.CaseBriefPath),
		"decision_record_path":     pathOrNone(in.DecisionRecordPath),
		"support_log_path":         pathOrNone(in.SupportLogPath),
		"verdict_path":             in.VerdictPath,
		"amendment_target":         pathOrNone(in.AmendmentTarget),
		parentdirective.MarkerName: directive,
		editdirective.MarkerName:   editDirective,
	})
	if err != nil {
		return "", fmt.Errorf("loom: arbiter prompt: fill stencil %q: %w", stencilName, err)
	}
	return string(filled), nil
}

// pathOrNone returns path, or the literal "none" for an empty one.
func pathOrNone(path string) string {
	if path == "" {
		return AbsentPath
	}
	return path
}

// Spec composes the shuttleengine.Spec the arbiter session launches from, from an already-composed prompt, the verdict path and the resolved arbiter settings.
//
// Prompt is the argument verbatim. Skills is empty, so the prompt is the arbiter's whole procedure.
// OutputFiles is a single-entry slice holding verdictPath, so shuttle's completion check reads the verdict through allOutputFilesExist and reed drops a finished strand on resume.
// ModelChoice is the settings' choice whole, which carries the RESOLVED engine, provider model id, effort and version.
// Timeout is the settings' timeout, which bounds the arbiter's run.
// Role is loomengine.ArbiterRole, which reed forms into `<shortname>:<slug>:arbiter`; NameOverride is left empty so a second attempt numbers its role.
// Segment is the coordinator, the driver's, because the arbiter acts for the driver.
// Interactive is false, shuttle's autonomous posture, which adds the operator-prompt deny an unattended session needs.
// ForkSubagents is false and Parent is empty: the arbiter spawns nothing and has no session to message.
// Display is left at its zero value.
func Spec(prompt, verdictPath string, settings sheddriver.ArbiterSettings) shuttleengine.Spec {
	return shuttleengine.Spec{
		Prompt:        prompt,
		Skills:        skills,
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

// VerdictGate returns the gate the arbiter's run carries, which re-prompts the arbiter while the verdict file at verdictPath does not read as a valid verdict.
// An absent file passes, so a run that ends without a verdict still reads as missing.
// A file that fails the reader is a failed result whose findings carry the reader's error and the path to rewrite.
// The entry lets the run through once its budget is spent, so the call's own read after the run still hands on with cause unparseable.
func VerdictGate(verdictPath string) shuttleengine.GateSpec {
	gate := func() (shuttleengine.GateResult, error) {
		if _, _, err := loomengine.ReadArbiterVerdict(verdictPath); err != nil {
			return shuttleengine.GateResult{
				Findings: fmt.Sprintf("%s\n\nRewrite the verdict file (%s) so it reads: write reason, question and each verbs item as a literal block scalar (|-).", err.Error(), verdictPath),
			}, nil
		}
		return shuttleengine.GateResult{Passed: true}, nil
	}
	return shuttleengine.GateSpec{{Name: "verdict", Gate: gate, Attempts: verdictGateAttempts, PassOnCap: true}}
}
