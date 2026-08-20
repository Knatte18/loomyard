// loomshed.go implements Deps and New: loom's own 13-row producer list assembled behind a
// *shedengine.Shed.

package loomshed

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// The thirteen producer names, verbatim per manifest/designs/loom.md's producer table. The name is
// the durable on-disk identity in current_producer; a later rename breaks resume for any in-flight
// task, so every row below is built from these constants, never a repeated string literal.
//
// NamePublish and NameFinalize are not loom's own producers -- both are generic ShedProducers
// shared by reference with Hardener's own list (see internal/landingshed's own package
// documentation) -- but the name constants live here regardless, same as every other row, because
// loom's own producer table is what New assembles from them.
const (
	NamePreflight          = "Preflight"
	NameLoomPreflight      = "Loom-Preflight"
	NameDiscussionWrite    = "Discussion-Write"
	NameDiscussionValidate = "Discussion-Validate"
	NameDiscussionReview   = "Discussion-Review"
	NamePlanWrite          = "Plan-Write"
	NamePlanValidate       = "Plan-Validate"
	NamePlanReview         = "Plan-Review"
	NameBatchifier         = "Batchifier"
	NameWebster            = "Webster"
	NameWebsterReview      = "Webster-Review"
	NamePublish            = "Publish"
	NameFinalize           = "Finalize"
)

// Deps carries every told value New needs to assemble loom's producer list.
//
// There is deliberately no separate base-directory field: the batch-config lookup
// (batcher.Active) stats an _lyx directory under the directory it is given and returns it
// unchanged, which is exactly the directory the plan-directory constructor (planparser.PlanDir)
// anchors on, so two fields would only invite silent divergence -- both use AnchorPath.
type Deps struct {
	// StatusPath, LockPath, and StatusLockPath are Shed's own told values -- see shedengine.Shed's
	// own field docs.
	StatusPath     string
	LockPath       string
	StatusLockPath string
	// MaxBounces is the default an unset per-producer ProducerDef.MaxBounces inherits. 0 means
	// "use the internal default", never "no bounces allowed" -- see shedengine.Shed.MaxBounces.
	// The budget it seeds is per-producer and episode-scoped, not run-wide -- see
	// shedengine.Shed.MaxBounces's own field doc for the full inversion this task made.
	MaxBounces int

	// AnchorPath feeds both the plan directory (planparser.PlanDir) and the batch-config lookup
	// (batcher.Active).
	AnchorPath string
	// WorktreeRoot is kept separate from AnchorPath because planparser.Validate genuinely takes a
	// different value from planparser.PlanDir.
	WorktreeRoot string

	// DecisionRecordPath and SupportLogPath are told rather than derived, because loomengine's
	// accessors for them (DiscussionDecisionRecord/DiscussionSupportLog) take a *lyxcwd.Location
	// this package may not import.
	DecisionRecordPath string
	SupportLogPath     string

	// Preflight is injected pre-constructed, because it is the only row that spawns git.
	Preflight shedengine.ShedProducer

	// WebsterRun and WebsterDeps are injected as parts, rather than as a single ShedProducer,
	// because the lazy wrapper around them (websterProducer) is owned here.
	WebsterRun  shedadapters.WebsterRunner
	WebsterDeps websterengine.RunDeps

	// Landing is the told-value passthrough for landingshed.Deps, backing rows 12 (Publish) and 13
	// (Finalize). It is a single passthrough rather than landing's fields flattened into this
	// struct, deliberately: that keeps landing-specific fields out of Deps, and gives a future
	// sibling product the same struct to fill from its own geometry with no duplication.
	Landing landingshed.Deps
}

// New builds loom's 13-row producer list in table order and returns a *shedengine.Shed carrying it
// plus the four told Shed fields.
//
// The thirteen rows, with their backing, OnStuck target, and OnDone target: 1 Preflight
// (deps.Preflight, "", -> Loom-Preflight); 2 Loom-Preflight (real, "", -> Discussion-Write); 3
// Discussion-Write (stub, "", -> Discussion-Validate); 4 Discussion-Validate (real, bouncing to
// Discussion-Write, -> Discussion-Review); 5 Discussion-Review (stub, bouncing to Discussion-Write,
// -> Plan-Write); 6 Plan-Write (stub, "", -> Plan-Validate); 7 Plan-Validate (real, bouncing to
// Plan-Write, -> Plan-Review); 8 Plan-Review (stub, bouncing to Plan-Write, -> Batchifier); 9
// Batchifier (real, "", -> Webster); 10 Webster (the lazy wrapper, "", -> Webster-Review); 11
// Webster-Review (stub, bouncing to Webster, -> Publish); 12 Publish (real, "", -> Finalize); 13
// Finalize (real, "", -> "", the empty OnDone that finishes the whole run). Every gate and
// validator bounces back to the producer whose artifact it guards, and a gate whose guarded
// artifact is produced by no row in the list escalates instead -- Preflight gates git and
// filesystem state and Batchifier gates the batch config, neither of which any row writes, so
// there is nothing to bounce to and a human is the only thing that can fix either.
// Loom-Preflight shares that same escalate-only posture for its own reason: no row in the list
// produces loom's own status file, so there is nothing to bounce to and a human is the only thing
// that can fix an incoherent or half-finished seed. Publish and Finalize share that same
// escalate-only posture: neither bounces, because nothing in the list produces what these two gate
// -- an unresolvable conflict against the parent's current state, an unreachable remote service, a
// drifting parent, and a pull request awaiting human review are all things only a human fixes.
//
// The list's physical order above is preserved as the display and enumeration order this comment
// and the table below walk in, and each row's own OnDone is what actually routes a Done verdict to
// its successor, forward or backward, regardless of list position.
//
// Plan-Sweep is deliberately not a row here: its scout-inventory work is deferred (see
// manifest/roadmap.md's Someday "loom: build Plan-Sweep for real" item) and it has no consumer --
// Plan-Write reads only decision-record.md today -- so a stub row with nothing to feed and nothing
// to bounce to would be dead weight, not a placeholder worth carrying. It returns as a row only
// alongside the task that builds it for real.
//
// New returns an error when deps.Preflight is nil, since a nil row would otherwise panic at the
// call step rather than failing loud; it does not otherwise pre-validate, because shedengine.Run
// validates every field before it touches anything. Building the Publish and Finalize rows can
// itself fail -- both constructors reject a nil closure in deps.Landing -- and New surfaces that
// construction failure rather than discarding it.
func New(deps Deps) (*shedengine.Shed, error) {
	if deps.Preflight == nil {
		return nil, fmt.Errorf("loomshed: deps.Preflight must not be nil")
	}

	publish, err := landingshed.NewPublish(deps.Landing)
	if err != nil {
		return nil, fmt.Errorf("loomshed: build Publish row: %w", err)
	}
	finalize, err := landingshed.NewFinalize(deps.Landing)
	if err != nil {
		return nil, fmt.Errorf("loomshed: build Finalize row: %w", err)
	}

	producers := []shedengine.ProducerDef{
		{Name: NamePreflight, Producer: deps.Preflight, OnStuck: "", OnDone: NameLoomPreflight},
		{Name: NameLoomPreflight, Producer: newLoomPreflight(NameLoomPreflight, deps.StatusPath, deps.StatusLockPath), OnStuck: "", OnDone: NameDiscussionWrite},
		{Name: NameDiscussionWrite, Producer: newStub(NameDiscussionWrite), OnStuck: "", OnDone: NameDiscussionValidate},
		{Name: NameDiscussionValidate, Producer: newDiscussionValidate(NameDiscussionValidate, deps.DecisionRecordPath, deps.SupportLogPath), OnStuck: NameDiscussionWrite, OnDone: NameDiscussionReview},
		{Name: NameDiscussionReview, Producer: newStub(NameDiscussionReview), OnStuck: NameDiscussionWrite, OnDone: NamePlanWrite},
		{Name: NamePlanWrite, Producer: newStub(NamePlanWrite), OnStuck: "", OnDone: NamePlanValidate},
		{Name: NamePlanValidate, Producer: newPlanValidate(NamePlanValidate, deps.AnchorPath, deps.WorktreeRoot), OnStuck: NamePlanWrite, OnDone: NamePlanReview},
		{Name: NamePlanReview, Producer: newStub(NamePlanReview), OnStuck: NamePlanWrite, OnDone: NameBatchifier},
		{Name: NameBatchifier, Producer: newBatchifier(NameBatchifier, deps.AnchorPath), OnStuck: "", OnDone: NameWebster},
		{Name: NameWebster, Producer: newWebsterProducer(NameWebster, deps.AnchorPath, deps.WebsterRun, deps.WebsterDeps), OnStuck: "", OnDone: NameWebsterReview},
		{Name: NameWebsterReview, Producer: newStub(NameWebsterReview), OnStuck: NameWebster, OnDone: NamePublish},
		{Name: NamePublish, Producer: publish, OnStuck: "", OnDone: NameFinalize},
		{Name: NameFinalize, Producer: finalize, OnStuck: "", OnDone: ""},
	}

	return &shedengine.Shed{
		Producers:      producers,
		StatusPath:     deps.StatusPath,
		LockPath:       deps.LockPath,
		StatusLockPath: deps.StatusLockPath,
		MaxBounces:     deps.MaxBounces,
	}, nil
}
