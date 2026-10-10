// discussiontable.go implements DiscussionTable, the seat-table Discussion producer's table factory: a pure composer that resolves the chair's and each advisor's model, names the files the seats read and write, and returns the seatengine.Table the DiscussionSeats producer runs.
// It does no spawning, polling or filesystem writing itself.

package loomengine

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
)

const (
	// discussionChairStencil is the stencil that opens the chair seat.
	discussionChairStencil = "loom-template-discussion-chair"
	// discussionAdvisorStencil is the stencil that opens every advisor seat.
	discussionAdvisorStencil = "loom-template-discussion-advisor"
)

// discussionAdvisorSkills are the skills each advisor seat loads.
var discussionAdvisorSkills = []string{"scribe:prose"}

// DiscussionAdvisorNotes returns the path of advisor n's notes file, counting from 1.
// It shares DiscussionDir's AnchorPath anchoring.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func DiscussionAdvisorNotes(l *lyxcwd.Location, n int) string {
	return filepath.Join(DiscussionDir(l), fmt.Sprintf("advisor-%d.md", n))
}

// discussionModeRules returns the {{.mode_rules}} block of the seat-table producer: the single-agent text for the mode, plus, in autonomous mode with advisors, one sentence sending the chair's design questions to them.
func discussionModeRules(autonomous, advisors bool) string {
	rules := modeRules(autonomous)
	if autonomous && advisors {
		rules += " Send your design questions to the advisors named in this prompt, and self-pick what they cannot settle."
	}
	return rules
}

// resolveModelChoice parses raw and resolves it through reg, returning the model, effort and version.
func resolveModelChoice(raw string, reg modelspec.Registry) (model, effort, version string, err error) {
	spec, err := modelspec.Parse(raw)
	if err != nil {
		return "", "", "", err
	}
	resolved, err := reg.Resolve(spec)
	if err != nil {
		return "", "", "", err
	}
	return resolved.Model, resolved.Params["effort"], resolved.Params["version"], nil
}

// DiscussionTable builds the seatengine.Table of one seat-table Discussion-Write run: a chair that writes the decision record and the support log, and one advisor per entry of cfg.DiscussionAdvisors that writes its notes file for the chair to read.
// The seats' opening stencils are read at call time from stencilsDir, and every seat's parent directive is rendered by the seat engine from its geometry.
// The table carries no gate; the producer entry sets it.
// A PATTERN directive or Tier 2 friction directive that cannot be rendered is left empty, the friction failure logged, and never fails the table.
func DiscussionTable(layout *lyxcwd.Location, stencilsDir string, cfg Config, reg modelspec.Registry, slug string) (seatengine.Table, error) {
	if slug == "" {
		return seatengine.Table{}, fmt.Errorf("loom: DiscussionTable: slug must not be empty")
	}

	chairModel, chairEffort, chairVersion, err := resolveModelChoice(cfg.Discussion, reg)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("loom: DiscussionTable: discussion role model-spec: %w", err)
	}

	patternDirective, err := pattern.Directive(layout.WorktreePath(), stencilsDir, pattern.RoleDesigner)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("loom: DiscussionTable: pattern directive: %w", err)
	}

	var frictionDir string
	if cfg.Friction != "" {
		frictionDir = LoomFrictionDir(layout)
	}
	frictionDirective, err := friction.Directive(friction.NotePath(frictionDir, "Discussion-Write"), stencilsDir, friction.RoleInterview)
	if err != nil {
		// A friction directive is optional bookkeeping, so a stencil read failure here must never fail the Discussion-Write spawn.
		logger.Warn("loom: friction directive failed, continuing without one", "role", "discussion-write", "stencil", discussionChairStencil, "error", err)
		frictionDirective = ""
	}

	seats := make([]seatengine.Seat, 0, 1+len(cfg.DiscussionAdvisors))
	var notes []string
	for n, raw := range cfg.DiscussionAdvisors {
		model, effort, version, err := resolveModelChoice(raw, reg)
		if err != nil {
			return seatengine.Table{}, fmt.Errorf("loom: DiscussionTable: %s entry %d: %w; set the entry to a model-spec the registry defines", discussionAdvisorsKey, n+1, err)
		}
		path := DiscussionAdvisorNotes(layout, n+1)
		notes = append(notes, path)
		seats = append(seats, seatengine.Seat{
			Name:    seatengine.AdvisorName(n + 1),
			Stencil: discussionAdvisorStencil,
			Model:   model,
			Effort:  effort,
			Version: version,
			Outputs: []string{path},
			Skills:  discussionAdvisorSkills,
		})
	}
	chair := seatengine.Seat{
		Name:    seatengine.RoleChair,
		Stencil: discussionChairStencil,
		Model:   chairModel,
		Effort:  chairEffort,
		Version: chairVersion,
		Inputs:  notes,
		Outputs: []string{DiscussionDecisionRecord(layout), DiscussionSupportLog(layout)},
		Skills:  discussionSkills,
	}

	return seatengine.Table{
		RolePrefix:  discussionRole,
		Segment:     segmentcolor.Discussion,
		Timeout:     time.Duration(cfg.DiscussionTimeoutMin) * time.Minute,
		Interactive: cfg.DiscussionInteractive,
		Values: map[string]string{
			"slug":                 slug,
			"decision_record_path": DiscussionDecisionRecord(layout),
			"support_log_path":     DiscussionSupportLog(layout),
			"mode_rules":           discussionModeRules(!cfg.DiscussionInteractive, len(cfg.DiscussionAdvisors) > 0),
			"pattern_directive":    patternDirective,
			friction.MarkerName:    frictionDirective,
		},
		Optional: []string{"pattern_directive", friction.MarkerName},
		Seats:    append([]seatengine.Seat{chair}, seats...),
	}, nil
}
