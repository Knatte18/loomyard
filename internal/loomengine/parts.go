// parts.go implements PartsTable, the Parts-Write row's seat-table builder.
// Like the Plan producer (plan.go), the Parts producer is a prompt fed to the seat engine, one lead seat producing one artifact.
// Its inputs are the decision record and the relevant list; its output is the parts file, `_lym/parts/parts.md`, which splits the task into parts with a contract each.
// PartsTable is a pure builder: it reads the stencils it composes from, and does not stat the decision record, create `_lym/parts/` or spawn anything.

package loomengine

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/partsparser"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// partsRole is the agent-name role this module's Parts-Write spawn carries.
const partsRole = "parts"

// partsStencil is the stencil that opens the Parts-Write lead seat.
const partsStencil = "loom-template-parts"

// partsDepthFragments maps each parts_depth value to the stencil holding that reading depth's text.
var partsDepthFragments = map[string]string{
	"light":  "loom-fragment-parts-light",
	"medium": "loom-fragment-parts-medium",
}

// PartsTable builds the seatengine.Table of one Parts-Write run: a lead that reads the decision record and the relevant list and writes the parts file.
// The seat's opening stencil and the reading-depth fragment cfg.PartsDepth names are read at call time from stencilsDir, and the seat engine renders the parent and edit directives from its geometry.
// specsDir is told, never derived: this package is bound by the Told-Geometry Invariant.
// The table carries no gate; the producer entry sets it.
// A PATTERN directive that cannot be rendered fails the table; a friction directive that cannot be rendered is left empty, the failure logged.
func PartsTable(layout *lymcwd.Location, stencilsDir, specsDir string, cfg Config, reg modelspec.Registry) (seatengine.Table, error) {
	choice, err := resolveModelChoice(cfg.Parts, reg)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("loom: PartsTable: parts role model-spec: %w", err)
	}

	fragment, ok := partsDepthFragments[cfg.PartsDepth]
	if !ok {
		return seatengine.Table{}, fmt.Errorf("loom: PartsTable: parts_depth %q is neither light nor medium", cfg.PartsDepth)
	}
	depthBlock, err := stencilstore.Read(stencilsDir, fragment)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("loom: PartsTable: depth fragment %q: %w", fragment, err)
	}

	declaredChanges, err := discussionparser.DeclaredPatternChanges(DiscussionDecisionRecord(layout))
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("loom: PartsTable: %w re-step the row with %q", err, "lym loom start")
	}
	patternDirective, err := pattern.Directive(layout.WorktreePath(), stencilsDir, pattern.RoleImplementer, declaredChanges)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("loom: PartsTable: %w", err)
	}

	var frictionDir string
	if cfg.Friction != "" {
		frictionDir = LoomFrictionDir(layout)
	}
	frictionDirective, err := friction.Directive(friction.NotePath(frictionDir, "Parts-Write"), stencilsDir, friction.RoleImplementer)
	if err != nil {
		// A friction directive is optional bookkeeping, so a stencil read failure here must never fail the Parts-Write spawn.
		logger.Warn("loom: friction directive failed, continuing without one", "role", "parts-write", "stencil", partsStencil, "error", err)
		frictionDirective = ""
	}

	partsFile := partsparser.File(layout.AnchorPath())
	lead := seatengine.Seat{
		Stencil:     partsStencil,
		ModelChoice: choice,
		ModelKey:    shuttleengine.ModelKey{Module: tapestryModule, Key: "parts"},
		Inputs:      []string{DiscussionDecisionRecord(layout), DiscussionRelevant(layout)},
		Outputs:     []string{partsFile},
		Skills:      planSkills,
	}
	return seatengine.Table{
		RolePrefix: partsRole,
		Segment:    segmentcolor.Plan,
		Timeout:    time.Duration(cfg.PlanTimeoutMin) * time.Minute,
		Values: map[string]string{
			"decision_record_path": DiscussionDecisionRecord(layout),
			"relevant_path":        DiscussionRelevant(layout),
			"parts_path":           partsFile,
			"parts_dir":            partsparser.Dir(layout.AnchorPath()),
			"parts_max":            strconv.Itoa(cfg.PartsMax),
			"depth_block":          strings.TrimSpace(stencil.StripLeadingComment(string(depthBlock))),
			"specs_dir":            specsDir,
			"pattern_directive":    patternDirective,
			friction.MarkerName:    frictionDirective,
		},
		Optional: []string{"pattern_directive", friction.MarkerName},
		Seats:    []seatengine.Seat{lead},
	}, nil
}
