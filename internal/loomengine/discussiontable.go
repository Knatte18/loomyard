// discussiontable.go composes the seatengine.Table of the Discussion-Write row.
// It resolves the lead's and each advisor's model and names the files the seats read and write.
// It does no spawning, polling or filesystem writing itself.

package loomengine

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
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
	// Self-check fragments: the first for a writer that runs the validator itself; the second for a cold-fixer gate.
	discussionSelfCheckStencil      = "loom-fragment-self-check-discussion"
	discussionSelfCheckGatedStencil = "loom-fragment-self-check-discussion-gated"
)

// DiscussionAdvisorNotes returns the path of the notes file of the advisor named name.
// The file name carries an "advisor-" prefix, so no advisor name takes the path of a lead output.
// It is anchored in the discussion directory of the location.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func DiscussionAdvisorNotes(l *lymcwd.Location, name string) string {
	return filepath.Join(DiscussionDir(l), "advisor-"+name+".md")
}

// discussionModeRules returns the mode_rules value of a discussion table.
// It is the single-agent text for the mode;
// in autonomous mode with advisors, one more sentence sends the chair's design questions to them.
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

// DiscussionTable builds the seatengine.Table of one Discussion-Write run.
// The lead writes the decision record, the support log and the relevant list.
// Each entry of cfg.DiscussionAdvisors adds one advisor, named by the entry,
// and each advisor writes a notes file for the lead to read.
// When cfg.DiscussionLenses is set, the lead alone may spawn fork subagents.
// Under the same condition, the lead carries the twill lens review directive.
// Its fan is resolved through lenses.yaml under dirs; the advisors never fork.
// The seats' opening stencils are read at call time from stencilsDir.
// The lead's prompt points at the format spec under specsDir.
// The seat engine renders every seat's parent directive from its geometry.
// cfg.SubagentModel names the model of the fresh subagents the lead's prompt tells it to hand lookups to.
// The table carries no gate; the producer entry sets it.
// A lead with no advisors decides each question itself, as its prompt says.
// The PATTERN directive renders the PATTERN changes the decision record declares;
// a malformed section fails the table, naming the way forward.
// A PATTERN directive or Tier 2 friction directive that cannot be rendered is left empty;
// the friction failure is logged, and it never fails the table.
// fixerOn reports whether a cold fixer clears the row's discussion gate,
// so the lead's prompt tells it to run no validator and the twill directive asks for none;
// false renders the prompt without that change.
func DiscussionTable(layout *lymcwd.Location, dirs configengine.Dirs, stencilsDir, specsDir string, cfg Config, reg modelspec.Registry, slug string, fixerOn bool) (seatengine.Table, error) {
	if slug == "" {
		return seatengine.Table{}, fmt.Errorf("loom: DiscussionTable: slug must not be empty")
	}

	chairChoice, err := resolveModelChoice(cfg.Discussion, reg)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("loom: DiscussionTable: discussion role model-spec: %w", err)
	}

	declaredChanges, err := discussionparser.DeclaredPatternChanges(DiscussionDecisionRecord(layout))
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("loom: DiscussionTable: %w re-step the row with %q", err, "lym loom start")
	}
	patternDirective, err := pattern.Directive(layout.WorktreePath(), stencilsDir, pattern.RoleDesigner, declaredChanges)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("loom: DiscussionTable: pattern directive: %w", err)
	}

	var frictionDir string
	if cfg.Friction != "" {
		frictionDir = LoomFrictionDir(layout)
	}
	frictionDirective, err := friction.Directive(friction.NotePath(frictionDir, "Discussion-Write"), stencilsDir, friction.RoleInterview)
	if err != nil {
		// A friction directive is optional bookkeeping,
		// so a stencil read failure here must never fail the Discussion-Write spawn.
		logger.Warn("loom: friction directive failed, continuing without one", "role", "discussion-write", "stencil", discussionChairStencil, "error", err)
		frictionDirective = ""
	}

	selfCheck, err := selfCheckBlock(stencilsDir, fixerOn, discussionSelfCheckStencil, discussionSelfCheckGatedStencil)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("loom: DiscussionTable: self-check step: %w", err)
	}
	forks := cfg.DiscussionLenses != ""
	leadValues := map[string]string{"self_check": selfCheck}
	if forks {
		lensDirective, err := discussionLensDirective(layout, dirs, stencilsDir, specsDir, cfg.DiscussionLenses, fixerOn)
		if err != nil {
			return seatengine.Table{}, fmt.Errorf("loom: DiscussionTable: %w", err)
		}
		leadValues[twilldirective.MarkerName] = setOffParagraph(lensDirective)
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
			ConfigRole:  agentconfig.RoleDiscussionAdvisor,
		})
	}
	lead := seatengine.Seat{
		Stencil:       discussionChairStencil,
		ModelChoice:   chairChoice,
		Inputs:        notes,
		Outputs:       []string{DiscussionDecisionRecord(layout), DiscussionSupportLog(layout), DiscussionRelevant(layout)},
		ConfigRole:    agentconfig.RoleDiscussion,
		Values:        leadValues,
		ForkSubagents: forks,
		ModelKey:      shuttleengine.ModelKey{Module: loomModule, Key: "discussion"},
	}
	if forks {
		// The lens directive tells the lead to fork and to end its turn while forks are outstanding.
		// A provider must wake the lead from that ended turn.
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
			"relevant_path":        DiscussionRelevant(layout),
			subagentModelKey:       cfg.SubagentModel,
			"mode_rules":           discussionModeRules(!cfg.DiscussionInteractive, len(cfg.DiscussionAdvisors) > 0),
			"pattern_directive":    patternDirective,
			friction.MarkerName:    frictionDirective,
		},
		Optional: []string{"pattern_directive", ProjectDirectiveMarker, friction.MarkerName, twilldirective.MarkerName},
		Seats:    append([]seatengine.Seat{lead}, seats...),
	}, nil
}
