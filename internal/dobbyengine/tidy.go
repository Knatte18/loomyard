// tidy.go implements dobby's board-tidy step.
// It counts landings, composes the board-tidy agent's prompt and launch spec, and spawns and settles the agent.

package dobbyengine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

const (
	// BoardTidyRole is the agent-name role of the board-tidy agent's strand.
	BoardTidyRole = "board-tidy"
	// tidyStencilName is the registered name of the board-tidy agent's opening stencil.
	tidyStencilName = "beam-template-board-tidy"
	// tidyUpkeepStencilName is the registered name of the board-tidy prompt's step that runs the upkeep checks.
	tidyUpkeepStencilName = "beam-fragment-board-tidy-upkeep"
	// tidyNoUpkeepStencilName is the registered name of the board-tidy prompt's step that runs no upkeep checks.
	tidyNoUpkeepStencilName = "beam-fragment-board-tidy-no-upkeep"
)

// AgentShuttle is the part of shuttle the agents dobby spawns run through: the board-tidy agent and the describer agent.
type AgentShuttle interface {
	// EngineName names the engine a launch with choice resolves to, which the prompt is rendered for.
	// It returns an error when the shuttle behind the launch cannot be built, so no prompt is rendered for it.
	EngineName(choice shuttleengine.ModelChoice) (string, error)
	// Start launches spec in a fresh window and returns the new strand's GUID once the agent is running.
	Start(spec shuttleengine.Spec) (strandGUID string, err error)
	// StopStrand ends the strand guid names through shuttle's stop verb.
	StopStrand(guid string) error
}

// TidyPromptInput carries what the board-tidy stencil's markers are filled from.
type TidyPromptInput struct {
	// Landed holds the slugs landed since the last tidy.
	Landed []string
	// ReportPath is the one file the agent writes.
	ReportPath string
	// Parent is the name of the nearest live orch, empty when there is none.
	Parent string
	// UpkeepCall is the gate call of the upkeep checks the agent runs and files notes from; empty runs none.
	UpkeepCall string
}

// TidyPrompt composes the board-tidy agent's launch prompt for the engine named engine.
// It reads the board-tidy stencil from stencilsDir at call time and fills its markers from in.
// The parent directive names in.Parent, and the edit directive is the one of engine.
// The upkeep step runs in.UpkeepCall, and a prompt with no call gets the step that runs no upkeep.
// A stencil read or fill error is returned wrapped with the stencil's name.
func TidyPrompt(stencilsDir, engine string, in TidyPromptInput) (string, error) {
	upkeep, err := tidyUpkeepStep(stencilsDir, in.UpkeepCall)
	if err != nil {
		return "", err
	}
	directive, err := parentdirective.Directive(stencilsDir, in.Parent, false)
	if err != nil {
		return "", err
	}
	editDirective, err := editdirective.Directive(stencilsDir, engine)
	if err != nil {
		return "", err
	}
	template, err := stencilstore.Read(stencilsDir, tidyStencilName)
	if err != nil {
		return "", fmt.Errorf("dobby: board-tidy prompt: read stencil %q: %w", tidyStencilName, err)
	}
	var landed strings.Builder
	for _, slug := range in.Landed {
		fmt.Fprintf(&landed, "- `%s`\n", slug)
	}
	filled, err := stencil.Fill(template, map[string]string{
		"landed":                   strings.TrimSuffix(landed.String(), "\n"),
		"report_path":              in.ReportPath,
		"upkeep":                   upkeep,
		parentdirective.MarkerName: directive,
		editdirective.MarkerName:   editDirective,
	})
	if err != nil {
		return "", fmt.Errorf("dobby: board-tidy prompt: fill stencil %q: %w", tidyStencilName, err)
	}
	return string(filled), nil
}

// tidyUpkeepStep returns the board-tidy prompt's upkeep step.
// The step is the upkeep fragment filled with call, or the no-upkeep fragment when call is empty.
// A stencil read or fill error is returned wrapped with the fragment's name.
func tidyUpkeepStep(stencilsDir, call string) (string, error) {
	name := tidyNoUpkeepStencilName
	if call != "" {
		name = tidyUpkeepStencilName
	}
	template, err := stencilstore.Read(stencilsDir, name)
	if err != nil {
		return "", fmt.Errorf("dobby: board-tidy prompt: read stencil %q: %w", name, err)
	}
	values := map[string]string{}
	if call != "" {
		values["upkeep_call"] = call
	}
	filled, err := stencil.Fill(template, values)
	if err != nil {
		return "", fmt.Errorf("dobby: board-tidy prompt: fill stencil %q: %w", name, err)
	}
	return strings.TrimSpace(string(filled)), nil
}

// TidySpec composes the shuttle spec the board-tidy agent launches from.
// PromptEngine is engine, the engine the prompt was rendered for, which must be the one model resolves to.
// OutputFiles holds reportPath alone.
// ConfigRole is the board-tidy role, whose skills come from agents.yaml (role `board-tidy`).
// Segment is the coordinator, since the agent acts for the beam's driver,
// and the agent is autonomous with no parent session to message.
func TidySpec(prompt, reportPath, engine string, model shuttleengine.ModelChoice, timeout time.Duration) shuttleengine.Spec {
	return shuttleengine.Spec{
		Prompt:       prompt,
		PromptEngine: engine,
		ConfigRole:   agentconfig.RoleBoardTidy,
		OutputFiles:  []string{reportPath},
		ModelChoice:  model,
		Timeout:      timeout,
		Role:         BoardTidyRole,
		Segment:      segmentcolor.Coordinator,
	}
}

// countLandings reads the landings the batten history of every run of this worktree holds.
// It records each one not yet counted in the state record.
// The first read of a record baselines it: every landing already there is marked seen and none is counted.
// So run records left from before the dobby, or a record rewritten fresh, trigger no tidy.
// While a board-tidy agent is in flight nothing is counted,
// so the record's slugs since the last tidy are exactly the agent's.
// A dobby that rewrites its beam's description also records each landing it counts as one to describe.
// A baselined record not yet marked machine-only has its landed-since-tidy list emptied once, ahead of the count.
// The list may hold other machines' landings.
func (d *daemon) countLandings(now time.Time) error {
	if d.deps.Landings == nil {
		return nil
	}
	landed, err := d.deps.Landings()
	if err != nil {
		return fmt.Errorf("read the landings: %w", err)
	}
	d.landed = landed
	return UpdateState(d.geom.StatePath, func(s *State) error {
		if s.CountedLandings == nil {
			s.CountedLandings = map[string]bool{}
		}
		if !s.LandingsBaselined {
			for _, slug := range landed {
				s.CountedLandings[slug] = true
			}
			s.LandingsBaselined = true
			s.LandingsMachineOnly = true
			return nil
		}
		if s.Tidy != nil {
			return nil
		}
		if !s.LandingsMachineOnly {
			s.LandedSinceTidy = nil
			s.LandingsMachineOnly = true
		}
		fresh := slices.DeleteFunc(slices.Clone(landed), func(slug string) bool { return s.CountedLandings[slug] })
		sort.Strings(fresh)
		for _, slug := range fresh {
			s.CountedLandings[slug] = true
			s.LandedSinceTidy = append(s.LandedSinceTidy, slug)
			if d.describes() {
				s.LandedSinceDescribe = append(s.LandedSinceDescribe, slug)
			}
		}
		return nil
	})
}

// tidyStep spawns the board-tidy agent at every beam.board_tidy_every landings unless one is in flight.
// It also settles the agent in flight.
// A report the agent wrote is posted to the nearest live orch by path and ends the agent;
// an agent past beam.board_tidy_timeout_min is ended, and its slugs carry to the next tidy.
// An agent in flight keeps the loop out of its idle backoff.
func (d *daemon) tidyStep(ctx context.Context, now time.Time) error {
	if d.deps.TidyShuttle == nil {
		return nil
	}
	cfg, err := d.deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("load beam.yaml: %w", err)
	}
	current, _, err := ReadState(d.geom.StatePath)
	if err != nil {
		return err
	}
	if current.Tidy != nil {
		waiting, err := d.settleTidy(ctx, now, current)
		d.busy = d.busy || waiting
		return err
	}
	if cfg.BoardTidyEvery == 0 || len(current.LandedSinceTidy) < cfg.BoardTidyEvery {
		return nil
	}
	return d.spawnTidy(now, cfg, current.LandedSinceTidy)
}

// spawnTidy starts the board-tidy agent over landed and records it in the state record.
// A record that cannot be written ends the agent again, so no unrecorded agent is left running.
func (d *daemon) spawnTidy(now time.Time, cfg Config, landed []string) error {
	orch, err := d.deps.NearestOrch()
	if err != nil {
		return fmt.Errorf("read the nearest live orch: %w", err)
	}
	if err := os.MkdirAll(d.geom.TidyDir, 0o755); err != nil {
		return err
	}
	reportPath := filepath.Join(d.geom.TidyDir, now.UTC().Format("20060102T150405Z")+".md")
	engine, err := d.deps.TidyShuttle.EngineName(d.deps.TidyModel)
	if err != nil {
		return fmt.Errorf("resolve the board-tidy agent's engine: %w", err)
	}
	prompt, err := TidyPrompt(d.deps.StencilsDir, engine, TidyPromptInput{Landed: landed, ReportPath: reportPath, Parent: orch, UpkeepCall: d.upkeepCall()})
	if err != nil {
		return err
	}
	timeout := time.Duration(cfg.BoardTidyTimeoutMin) * time.Minute
	guid, err := d.deps.TidyShuttle.Start(TidySpec(prompt, reportPath, engine, d.deps.TidyModel, timeout))
	if err != nil {
		return fmt.Errorf("start the board-tidy agent: %w", err)
	}
	err = UpdateState(d.geom.StatePath, func(s *State) error {
		s.Tidy = &TidyState{StrandGUID: guid, ReportPath: reportPath, Deadline: now.Add(timeout)}
		return nil
	})
	if err != nil {
		d.stopTidyStrand(guid)
		return err
	}
	logger.Info("dobbyengine: the board-tidy agent started", "worktree", d.geom.WorktreeName, "landings", len(landed), "report", reportPath)
	d.logLine(now, fmt.Sprintf("board tidy started over %d landings: %s", len(landed), reportPath))
	return nil
}

// upkeepCall returns the upkeep checks' gate call for the board-tidy agent, empty when the dobby runs none.
// A call that does not render is logged, and the tidy runs without the upkeep checks.
func (d *daemon) upkeepCall() string {
	if d.deps.UpkeepCall == nil {
		return ""
	}
	call, err := d.deps.UpkeepCall()
	if err != nil {
		logger.Warn("dobbyengine: the upkeep gate call does not render; the board tidy runs no upkeep checks", "worktree", d.geom.WorktreeName, "error", err)
		return ""
	}
	return call
}

// settleTidy ends the agent in flight once its report exists or its deadline has passed.
// It reports whether the agent is still running.
// The notice goes out before the state record is cleared, so a notice that fails is retried on the next tick.
func (d *daemon) settleTidy(ctx context.Context, now time.Time, current State) (waiting bool, err error) {
	tidy := current.Tidy
	landings := len(current.LandedSinceTidy)
	if _, statErr := os.Stat(tidy.ReportPath); statErr == nil {
		line := fmt.Sprintf("board tidy after %d landings: report %s; put each removal or scope change to the operator", landings, tidy.ReportPath)
		if err := d.deps.Notice(ctx, line); err != nil {
			return false, fmt.Errorf("post the board-tidy report: %w", err)
		}
		d.logLine(now, "board tidy report "+tidy.ReportPath)
		d.stopTidyStrand(tidy.StrandGUID)
		return false, UpdateState(d.geom.StatePath, func(s *State) error {
			s.Tidy, s.LandedSinceTidy = nil, nil
			return nil
		})
	}
	if now.Before(tidy.Deadline) {
		return true, nil
	}
	d.stopTidyStrand(tidy.StrandGUID)
	line := fmt.Sprintf("board tidy after %d landings gave no report; its slugs carry to the next tidy: %s", landings, strings.Join(current.LandedSinceTidy, ", "))
	if err := d.deps.Notice(ctx, line); err != nil {
		return false, fmt.Errorf("post the board-tidy timeout: %w", err)
	}
	d.logLine(now, "board tidy timed out: "+strings.Join(current.LandedSinceTidy, ", "))
	return false, UpdateState(d.geom.StatePath, func(s *State) error {
		s.Tidy = nil
		return nil
	})
}

// stopTidyStrand ends the board-tidy agent's strand;
// a failure is logged, since the agent ends on its own at its timeout.
func (d *daemon) stopTidyStrand(guid string) {
	if err := d.deps.TidyShuttle.StopStrand(guid); err != nil {
		logger.Warn("dobbyengine: the board-tidy agent's strand did not stop", "worktree", d.geom.WorktreeName, "strand", guid, "error", err)
	}
}
