// tidy.go implements dobby's board-tidy step: the count of landings, the board-tidy agent's prompt and launch spec, and the spawn, report and timeout of the agent every few landings.

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
	// tidySkill is the one skill the agent needs: the board, read and amended through `lym board`.
	tidySkill = "ly:board"
)

// TidyShuttle is the part of shuttle the board-tidy agent runs through.
type TidyShuttle interface {
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
}

// TidyPrompt composes the board-tidy agent's launch prompt.
// The stencil is read from stencilsDir at call time and filled with in, the parent directive naming in.Parent and the edit directive.
// It returns the read or fill error wrapped with the stencil's name.
func TidyPrompt(stencilsDir string, in TidyPromptInput) (string, error) {
	directive, err := parentdirective.Directive(stencilsDir, in.Parent, false)
	if err != nil {
		return "", err
	}
	editDirective, err := editdirective.Directive(stencilsDir)
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
		parentdirective.MarkerName: directive,
		editdirective.MarkerName:   editDirective,
	})
	if err != nil {
		return "", fmt.Errorf("dobby: board-tidy prompt: fill stencil %q: %w", tidyStencilName, err)
	}
	return string(filled), nil
}

// TidySpec composes the shuttle spec the board-tidy agent launches from.
// OutputFiles holds reportPath alone, and Skills only `ly:board`, the one skill the agent needs to read and amend the board.
// Segment is the coordinator, since the agent acts for the beam's driver, and the agent is autonomous with no parent session to message.
func TidySpec(prompt, reportPath string, model shuttleengine.ModelChoice, timeout time.Duration) shuttleengine.Spec {
	return shuttleengine.Spec{
		Prompt:      prompt,
		Skills:      []string{tidySkill},
		OutputFiles: []string{reportPath},
		ModelChoice: model,
		Timeout:     timeout,
		Role:        BoardTidyRole,
		Segment:     segmentcolor.Coordinator,
	}
}

// countLandings reads the landings the batten history of every run of this worktree holds and records each one not yet counted in the state record.
// The first read of a record baselines: every landing already there is marked seen and none is counted, so run records left from before the dobby, or a record rewritten fresh, trigger no tidy.
// While a board-tidy agent is in flight nothing is counted, so the record's slugs since the last tidy are exactly the agent's.
// A baselined record not yet marked machine-only has its landed-since-tidy list emptied once, ahead of the count, since it may hold other machines' landings.
func (d *daemon) countLandings(now time.Time) error {
	if d.deps.Landings == nil {
		return nil
	}
	landed, err := d.deps.Landings()
	if err != nil {
		return fmt.Errorf("read the landings: %w", err)
	}
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
		}
		return nil
	})
}

// tidyStep spawns the board-tidy agent at every board_tidy_every landings, unless one is in flight, and settles the one in flight.
// A report the agent wrote is posted to the nearest live orch by path and ends the agent; an agent past board_tidy_timeout_min is ended and its slugs carry to the next tidy.
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
	prompt, err := TidyPrompt(d.deps.StencilsDir, TidyPromptInput{Landed: landed, ReportPath: reportPath, Parent: orch})
	if err != nil {
		return err
	}
	timeout := time.Duration(cfg.BoardTidyTimeoutMin) * time.Minute
	guid, err := d.deps.TidyShuttle.Start(TidySpec(prompt, reportPath, d.deps.TidyModel, timeout))
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

// settleTidy ends the agent in flight once its report exists or its deadline has passed, and reports whether it is still running.
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

// stopTidyStrand ends the board-tidy agent's strand; a failure is logged, since the agent ends on its own at its timeout.
func (d *daemon) stopTidyStrand(guid string) {
	if err := d.deps.TidyShuttle.StopStrand(guid); err != nil {
		logger.Warn("dobbyengine: the board-tidy agent's strand did not stop", "worktree", d.geom.WorktreeName, "strand", guid, "error", err)
	}
}
