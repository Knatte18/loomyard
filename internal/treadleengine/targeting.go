// targeting.go implements treadle's third ephemeral judge framing: optional pre-round targeting, gated by Profile.PreRoundTargeting.
// Unlike runCircling/runMilestone in judge.go, this call produces no verdict — it reads the latest valid handoff and writes a free-form prose seed brief for the upcoming round's runner.
// It follows the exact same fail-safe posture as every other call in this package: any failure degrades to "no seed" with a logger.Warn, never an error, since a missed targeting call only costs the round the guidance it would have added, not correctness.

package treadleengine

import (
	"os"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
)

// runTargeting spawns the pre-round targeting call: reads a handoff and
// writes a prose seed brief. Fail-safe: any failure logs a Warn and returns
// ("", false), so the round runs without a seed.
func runTargeting(sh Shuttle, name string, round int, previousHandoffPath, seedPath, model, effort string) (string, bool) {
	values := map[string]string{
		"round":            strconv.Itoa(round),
		"previous_handoff": previousHandoffMarker(previousHandoffPath),
		"seed_path":        seedPath,
	}

	prompt, err := stencil.Fill(targetingTemplate, values)
	if err != nil {
		logger.Warn(name+": targeting judge failed, round runs without a seed", "round", round, "cause", err)
		return "", false
	}

	spec := shuttleengine.Spec{
		Prompt:      string(prompt),
		OutputFiles: []string{seedPath},
		Model:       model,
		Effort:      effort,
		Role:        "targeting",
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
