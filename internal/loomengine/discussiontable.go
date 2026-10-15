// discussiontable.go implements DiscussionTable, the Discussion-Write row's table factory: a pure composer that resolves the lead's and each advisor's model, names the files the seats read and write, and returns the seatengine.Table the row's seat producer runs.
// It does no spawning, polling or filesystem writing itself.

package loomengine

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/twilldirective"
)

const (
	// discussionChairStencil is the stencil that opens the chair seat.
	discussionChairStencil = "loom-template-discussion-chair"
	// discussionAdvisorStencil is the stencil that opens every advisor seat.
	discussionAdvisorStencil = "loom-template-discussion-advisor"
)

// discussionAdvisorSkills are the skills each advisor seat loads.
var discussionAdvisorSkills = []string{"scribe:prose"}

// DiscussionAdvisorNotes returns the path of the notes file of the advisor named name.
// The file name carries an "advisor-" prefix, so no advisor name takes the path of a lead output.
// It shares DiscussionDir's AnchorPath anchoring.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func DiscussionAdvisorNotes(l *lyxcwd.Location, name string) string {
	return filepath.Join(DiscussionDir(l), "advisor-"+name+".md")
}

// discussionModeRules returns the {{.mode_rules}} block of the seat-table producer: the single-agent text for the mode, plus, in autonomous mode with advisors, one sentence sending the chair's design questions to them.
func discussionModeRules(autonomous, advisors bool) string {
	rules := modeRules(autonomous)
	if autonomous && advisors {
		rules += " Send your design questions to the advisors named in this prompt, and self-pick what they cannot settle."
	}
	return rules
}

// resolveModelChoice parses raw and resolves it through reg, returning the resolved engine, model, effort and version.
func resolveModelChoice(raw string, reg modelspec.Registry) (shuttleengine.ModelChoice, error) {
	spec, err := modelspec.Parse(raw)
	if err != nil {
		return shuttleengine.ModelChoice{}, err
	}
	resolved, err := reg.Resolve(spec)
	if err != nil {
		return shuttleengine.ModelChoice{}, err
	}
	return shuttleengine.NewModelChoice(resolved), nil
}

// DiscussionTable builds the seatengine.Table of one Discussion-Write run: a lead that writes the decision record and the support log, and one advisor per entry of cfg.DiscussionAdvisors, named by the entry, that writes its notes file for the lead to read.
// When cfg.DiscussionLenses is set the lead alone is authorized to spawn fork subagents and carries the twill lens review directive, whose fan is resolved through burler.yaml under dirs; the advisors never fork.
// The seats' opening stencils are read at call time from stencilsDir, the lead's prompt points at the format spec under specsDir, and every seat's parent directive is rendered by the seat engine from its geometry.
// The table carries no gate; the producer entry sets it.
// A lead with no advisors decides each question itself, as its prompt says.
// A PATTERN directive or Tier 2 friction directive that cannot be rendered is left empty, the friction failure logged, and never fails the table.
func DiscussionTable(layout *lyxcwd.Location, dirs configengine.Dirs, stencilsDir, specsDir string, cfg Config, reg modelspec.Registry, slug string) (seatengine.Table, error) {
	if slug == "" {
		return seatengine.Table{}, fmt.Errorf("loom: DiscussionTable: slug must not be empty")
	}

	chairChoice, err := resolveModelChoice(cfg.Discussion, reg)
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

	forks := cfg.DiscussionLenses != ""
	var leadValues map[string]string
	if forks {
		lensDirective, err := discussionLensDirective(layout, dirs, stencilsDir, cfg.DiscussionLenses)
		if err != nil {
			return seatengine.Table{}, fmt.Errorf("loom: DiscussionTable: %w", err)
		}
		leadValues = map[string]string{twilldirective.MarkerName: setOffParagraph(lensDirective)}
	}

	seats := make([]seatengine.Seat, 0, 1+len(cfg.DiscussionAdvisors))
	var notes []string
	for n, advisor := range cfg.DiscussionAdvisors {
		choice, err := resolveModelChoice(advisor.Model, reg)
		if err != nil {
			return seatengine.Table{}, fmt.Errorf("loom: DiscussionTable: %s entry %d: %w; set the entry's model to a model-spec the registry defines", discussionAdvisorsKey, n+1, err)
		}
		path := DiscussionAdvisorNotes(layout, advisor.Name)
		notes = append(notes, path)
		seats = append(seats, seatengine.Seat{
			Name:        advisor.Name,
			Stencil:     discussionAdvisorStencil,
			ModelChoice: choice,
			ModelKey:    shuttleengine.ModelKey{Module: loomModule, Key: discussionAdvisorsKey, Entry: advisor.Name},
			Outputs:     []string{path},
			Skills:      discussionAdvisorSkills,
		})
	}
	lead := seatengine.Seat{
		Stencil:       discussionChairStencil,
		ModelChoice:   chairChoice,
		Inputs:        notes,
		Outputs:       []string{DiscussionDecisionRecord(layout), DiscussionSupportLog(layout)},
		Skills:        discussionSkills,
		Values:        leadValues,
		ForkSubagents: forks,
		ModelKey:      shuttleengine.ModelKey{Module: loomModule, Key: "discussion"},
	}
	if forks {
		// The lens directive tells the lead to fork and to end its turn while forks are outstanding, which a provider must wake it from.
		lead.Requires = []shuttleengine.Requirement{shuttleengine.RequirementBackgroundWake}
	}

	return seatengine.Table{
		RolePrefix:  discussionRole,
		Segment:     segmentcolor.Discussion,
		Timeout:     time.Duration(cfg.DiscussionTimeoutMin) * time.Minute,
		Interactive: cfg.DiscussionInteractive,
		Values: map[string]string{
			"slug":                 slug,
			"specs_dir":            specsDir,
			"decision_record_path": DiscussionDecisionRecord(layout),
			"support_log_path":     DiscussionSupportLog(layout),
			"mode_rules":           discussionModeRules(!cfg.DiscussionInteractive, len(cfg.DiscussionAdvisors) > 0),
			"pattern_directive":    patternDirective,
			friction.MarkerName:    frictionDirective,
		},
		Optional: []string{"pattern_directive", friction.MarkerName, twilldirective.MarkerName},
		Seats:    append([]seatengine.Seat{lead}, seats...),
	}, nil
}
