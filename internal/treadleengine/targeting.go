// targeting.go implements the pre-round targeting call, an ephemeral judge that runs before a round.
// The call runs only when Profile.PreRoundTargeting is set.
// It reads the latest valid handoff and writes a free-form prose seed brief for the round's runner.
// It produces no verdict.
// Any failure logs a warning through logger.Warn and degrades to running the round without a seed.
// It never returns an error, because a missed targeting call only costs the round guidance, not correctness.

package treadleengine

import (
	"os"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// targetingRole is the agent-name role the targeting spawn carries.
// The spawn runs in the review segment.
const targetingRole = "targeting"

// runTargeting spawns the pre-round targeting call, which reads a handoff and writes a prose seed brief.
// Any failure, including reading the prompt template, logs a Warn and returns ("", false),
// so the round runs without a seed.
// stencilsDir is the absolute stencils directory the prompt is read from.
func runTargeting(stencilsDir, parentName string, sh Shuttle, name string, round int, previousHandoffPath, seedPath string, choice shuttleengine.ModelChoice) (string, bool) {
	targetingTemplate, err := stencilstore.Read(stencilsDir, "treadle-template-targeting")
	if err != nil {
		logger.Warn(name+": targeting judge template unreadable, round runs without a seed", "round", round, "cause", err)
		return "", false
	}
	directive, err := parentdirective.Directive(stencilsDir, parentName, false)
	if err != nil {
		logger.Warn(name+": targeting judge parent directive unreadable, round runs without a seed", "round", round, "cause", err)
		return "", false
	}
	editDirective, err := editdirective.Directive(stencilsDir)
	if err != nil {
		logger.Warn(name+": targeting judge edit directive unreadable, round runs without a seed", "round", round, "cause", err)
		return "", false
	}
	values := map[string]string{
		"round":                    strconv.Itoa(round),
		"previous_handoff":         previousHandoffMarker(previousHandoffPath),
		"seed_path":                seedPath,
		parentdirective.MarkerName: directive,
		editdirective.MarkerName:   editDirective,
	}

	prompt, err := stencil.Fill(targetingTemplate, values)
	if err != nil {
		logger.Warn(name+": targeting judge failed, round runs without a seed", "round", round, "cause", err)
		return "", false
	}

	spec := shuttleengine.Spec{
		Prompt:      string(prompt),
		OutputFiles: []string{seedPath},
		ModelChoice: choice,
		Role:        targetingRole,
		Segment:     segmentcolor.Review,
		ConfigRole:  agentconfig.RoleTreadleTargeting,
		Round:       strconv.Itoa(round),
	}

	result, err := sh.Run(spec)
	if err != nil {
		logger.Warn(name+": targeting judge shuttle run failed, round runs without a seed", "round", round, "cause", err)
		return "", false
	}
	if result.Outcome != shuttleengine.OutcomeDone {
		logger.Warn(name+": targeting judge did not complete, round runs without a seed", "round", round, "outcome", result.Outcome)
		return "", false
	}

	content, err := os.ReadFile(seedPath)
	if err != nil {
		logger.Warn(name+": targeting judge seed file unreadable, round runs without a seed", "round", round, "cause", err)
		return "", false
	}
	if strings.TrimSpace(string(content)) == "" {
		logger.Warn(name+": targeting judge wrote an empty seed file, round runs without a seed", "round", round)
		return "", false
	}

	return string(content), true
}
