// prompt.go composes the alignment agent's launch prompt and its shuttle spec.
// Both are pure composers: the agent's stencil filled over one attempt's paths, and the spec the session launches from.

package alignment

import (
	"fmt"
	"strconv"
	"time"

	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// stencilName is the registered name of the agent's opening stencil.
const stencilName = "loom-template-alignment"

// reviewFormatStencilName is the registered name of the review format the agent's report follows.
const reviewFormatStencilName = "burler-review-format"

// absentValue fills a marker whose input the point does not have.
const absentValue = "none"

// Settings is the agent's resolved model triple and the timeout of its run.
type Settings struct {
	Model   string
	Effort  string
	Version string
	Timeout time.Duration
}

// PromptInput carries what the stencil's markers are filled from.
// BriefPath, PRNumber and HeadSHA may be empty or zero; every other field is required.
type PromptInput struct {
	Point              Point
	Slug               string
	Worktree           string
	DecisionRecordPath string
	SupportLogPath     string
	PlanDir            string
	BriefPath          string
	ParentBranch       string
	PRNumber           int
	HeadSHA            string
	VerdictPath        string
	ReportPath         string
}

// Prompt composes the agent's launch prompt.
// The stencil is read from stencilsDir at call time and filled with in, the review-format path, the no-parent parent directive and the edit directive.
// The no-parent directive is rendered because the verdict file is the agent's only channel.
// An empty brief path, a zero PR number and an empty head SHA fill as the literal "none".
// It returns the read or fill error wrapped with the stencil's name.
func Prompt(stencilsDir string, in PromptInput) (string, error) {
	parentDirective, err := parentdirective.Directive(stencilsDir, "", false)
	if err != nil {
		return "", err
	}
	editDirective, err := editdirective.Directive(stencilsDir)
	if err != nil {
		return "", err
	}
	template, err := stencilstore.Read(stencilsDir, stencilName)
	if err != nil {
		return "", fmt.Errorf("alignment: prompt: read stencil %q: %w", stencilName, err)
	}
	prNumber := absentValue
	if in.PRNumber != 0 {
		prNumber = strconv.Itoa(in.PRNumber)
	}
	filled, err := stencil.Fill(template, map[string]string{
		"point":                    string(in.Point),
		"slug":                     in.Slug,
		"worktree":                 in.Worktree,
		"decision_record_path":     in.DecisionRecordPath,
		"support_log_path":         in.SupportLogPath,
		"plan_dir":                 in.PlanDir,
		"brief_path":               valueOrNone(in.BriefPath),
		"parent_branch":            in.ParentBranch,
		"pr_number":                prNumber,
		"head_sha":                 valueOrNone(in.HeadSHA),
		"verdict_path":             in.VerdictPath,
		"report_path":              in.ReportPath,
		"review_format_path":       stencilstore.Path(stencilsDir, reviewFormatStencilName),
		parentdirective.MarkerName: parentDirective,
		editdirective.MarkerName:   editDirective,
	})
	if err != nil {
		return "", fmt.Errorf("alignment: prompt: fill stencil %q: %w", stencilName, err)
	}
	return string(filled), nil
}

// valueOrNone returns value, or the literal "none" for an empty one.
func valueOrNone(value string) string {
	if value == "" {
		return absentValue
	}
	return value
}

// Spec composes the shuttleengine.Spec the agent launches from, from an already-composed prompt, the attempt and the resolved settings.
//
// Skills is empty, so the prompt is the agent's whole procedure.
// OutputFiles is the attempt's verdict path and then its report path, so shuttle counts the run done only once both exist.
// Model, Effort, Version and Timeout come from s, which carries the RESOLVED triple.
// Role is Role, which reed forms into the strand name; NameOverride is left empty so a second agent of a run numbers its role.
// Segment is segmentcolor.Review.
// Interactive is false, shuttle's autonomous posture.
// ForkSubagents is false and Parent is empty: the agent spawns nothing and has no session to message.
func Spec(prompt string, a Attempt, s Settings) shuttleengine.Spec {
	return shuttleengine.Spec{
		Prompt:        prompt,
		Skills:        []string{},
		OutputFiles:   []string{a.VerdictPath(), a.ReportPath()},
		Model:         s.Model,
		Effort:        s.Effort,
		Version:       s.Version,
		Timeout:       s.Timeout,
		Role:          Role,
		Segment:       segmentcolor.Review,
		Interactive:   false,
		ForkSubagents: false,
		Parent:        "",
	}
}
