// twilldirective.go implements Directive, the renderer a spawning module calls to read the twill directive stencil from a told directory and fill it with its host values.
// See doc.go for the package-level rationale.

package twilldirective

import (
	"fmt"
	"maps"
	"strings"

	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// MarkerName is the template marker a host carries for the rendered lens review directive.
const MarkerName = "twill_directive"

// directiveStencil names the stencil Directive reads.
const directiveStencil = "twill-directive"

// writerForkStencil names the part stencil Directive reads when a writer fork is asked for.
const writerForkStencil = "twill-writer-fork"

// relaunchStencil names the part stencil Directive reads when no writer fork is asked for.
const relaunchStencil = "twill-relaunch"

// criticalJudgeStencil names the part stencil Directive reads when the critical judge is asked for.
const criticalJudgeStencil = "twill-judge-critical"

// plainJudgeStencil names the part stencil Directive reads when no critical judge is asked for.
const plainJudgeStencil = "twill-judge-plain"

// Lens is one review lens: its name and the prose its fork is given.
type Lens struct {
	Name string
	Text string
}

// Values are the host-specific values Directive fills into the directive stencil's markers.
type Values struct {
	// ReviewedFiles is the phrase naming the files each fork reviews, rendered verbatim with the host's own backticks.
	ReviewedFiles string
	// SpawnPoint is the sentence that opens the procedure and says when the writer runs it, rendered verbatim.
	SpawnPoint string
	// RejectionRule is the sentence that tells the writer how to fix or reject a finding, rendered verbatim on its own line under the judge step; read only when CriticalJudge is nil.
	RejectionRule string
	// SeverityRule is the phrase that tells each fork how to grade a finding, rendered verbatim after "each with".
	SeverityRule string
	// RubricPath is the rubric the writer reads before spawning.
	RubricPath string
	// Lenses are the lenses to spawn, one fork each.
	Lenses []Lens
	// FormatStep names the host step that tells the writer how to write the reviewed files.
	FormatStep string
	// SelfCheckStep names the host step that tells the writer to run the self-check.
	SelfCheckStep string
	// SelfCheckCommand is the command the writer runs as the self-check.
	SelfCheckCommand string
	// FinalSelfCheckCommand is the command the writer runs as the final self-check, after the lens review is recorded.
	FinalSelfCheckCommand string
	// DispositionPlace names where the writer records the lens review.
	DispositionPlace string
	// WriterFork, when set, has one fork write the reviewed files and replaces the relaunch paragraphs; nil asks for no writer fork.
	WriterFork *WriterFork
	// CriticalJudge, when set, has the writer judge each finding as a critical fixer; nil asks for the plain judge, which needs RejectionRule.
	CriticalJudge *CriticalJudge
}

// CriticalJudge holds the host values only the critical-judge part needs, each rendered verbatim.
type CriticalJudge struct {
	// GoverningInput is the phrase naming the input a finding's premise is weighed against, rendered verbatim after "against the code and".
	GoverningInput string
	// DisputeEvidence is the phrase listing the evidence kinds a dispute cites, rendered verbatim after "opened in this session,".
	DisputeEvidence string
}

// WriterFork holds the host values only the writer-fork part needs, each rendered verbatim.
type WriterFork struct {
	// ExploreStep names the host step the writer explores in.
	ExploreStep string
	// InputRecord is the phrase naming the input fork0's context holds.
	InputRecord string
	// LastFile is the file written last, which marks the reviewed files complete, with the host's own backticks.
	LastFile string
	// FilesDir is the directory the reviewed files are written into, with the host's own backticks.
	FilesDir string
}

// Directive returns the lens review directive read from stencilsDir, filled with v, without its leading banner and trailing newlines.
// Every marker is required: an empty value, or an empty Lenses, fails the fill with an error naming the marker.
// With v.WriterFork set the directive carries the writer-fork part, otherwise the relaunch paragraphs.
// With v.CriticalJudge set it carries the critical-judge part, otherwise the plain-judge part.
// Every error names the stencil it comes from.
func Directive(stencilsDir string, v Values) (string, error) {
	values := map[string]string{
		"reviewed_files":           v.ReviewedFiles,
		"spawn_point":              v.SpawnPoint,
		"rejection_rule":           v.RejectionRule,
		"severity_rule":            v.SeverityRule,
		"rubric_path":              v.RubricPath,
		"lens_list":                bulletList(v.Lenses, func(l Lens) string { return l.Name + ": " + strings.Join(strings.Fields(l.Text), " ") }),
		"format_step":              v.FormatStep,
		"self_check_step":          v.SelfCheckStep,
		"self_check_command":       v.SelfCheckCommand,
		"final_self_check_command": v.FinalSelfCheckCommand,
		"disposition_place":        v.DispositionPlace,
	}
	if v.WriterFork != nil {
		part, err := fillPart(stencilsDir, writerForkStencil, mergedValues(values, map[string]string{
			"explore_step": v.WriterFork.ExploreStep,
			"input_record": v.WriterFork.InputRecord,
			"last_file":    v.WriterFork.LastFile,
			"files_dir":    v.WriterFork.FilesDir,
		}), nil)
		if err != nil {
			return "", err
		}
		values["writer_fork"] = part + "\n\n"
	} else {
		part, err := fillPart(stencilsDir, relaunchStencil, values, nil)
		if err != nil {
			return "", err
		}
		values["relaunch_rules"] = part + "\n\n"
	}
	judge, err := judgePart(stencilsDir, values, v.CriticalJudge)
	if err != nil {
		return "", err
	}
	values["judge_steps"] = judge
	return fillPart(stencilsDir, directiveStencil, values, []string{"writer_fork", "relaunch_rules"})
}

// judgePart returns the judge part for the host: the critical judge filled over values and the critical judge's own values when critical is set, otherwise the plain judge filled over values.
func judgePart(stencilsDir string, values map[string]string, critical *CriticalJudge) (string, error) {
	if critical == nil {
		return fillPart(stencilsDir, plainJudgeStencil, values, nil)
	}
	return fillPart(stencilsDir, criticalJudgeStencil, mergedValues(values, map[string]string{
		"governing_input":  critical.GoverningInput,
		"dispute_evidence": critical.DisputeEvidence,
	}), nil)
}

// fillPart reads stencil name from stencilsDir and returns it filled with values, without trailing newlines.
// The names in optional may be empty.
// Every error names the stencil.
func fillPart(stencilsDir, name string, values map[string]string, optional []string) (string, error) {
	content, err := stencilstore.Read(stencilsDir, name)
	if err != nil {
		return "", fmt.Errorf("twilldirective: read stencil %s: %w", name, err)
	}
	filled, err := stencil.FillOptional(content, values, optional)
	if err != nil {
		return "", fmt.Errorf("twilldirective: fill stencil %s: %w", name, err)
	}
	return strings.TrimRight(string(filled), "\r\n"), nil
}

// mergedValues returns a copy of base with extra added.
func mergedValues(base, extra map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(extra))
	maps.Copy(merged, base)
	maps.Copy(merged, extra)
	return merged
}

// bulletList renders one "- " bullet per item on its own line, or the empty string for no items.
func bulletList[T any](items []T, text func(T) string) string {
	lines := make([]string, len(items))
	for i, item := range items {
		lines[i] = "- " + text(item)
	}
	return strings.Join(lines, "\n")
}
