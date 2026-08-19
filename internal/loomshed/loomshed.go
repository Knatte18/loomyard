// loomshed.go implements Deps and New: loom's own 12-row producer list assembled behind a
// *shedengine.Shed.

package loomshed

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// The twelve producer names, verbatim per manifest/designs/loom.md's producer table. The name is
// the durable on-disk identity in current_producer; a later rename breaks resume for any in-flight
// task, so every row below is built from these constants, never a repeated string literal.
//
// NamePublish and NameFinalize are not loom's own producers -- both are generic ShedProducers
// shared by reference with Hardener's own list (see manifest/designs/landing.md) -- but the name
// constants live here regardless, same as every other row, because loom's own producer table is
// what New assembles from them.
const (
	NamePreflight          = "Preflight"
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
	// MaxBounces is Shed's own told bounce budget. 0 means "use the internal default", never "no
	// bounces allowed" -- see shedengine.Shed.MaxBounces.
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
}

// New builds loom's 12-row producer list in table order and returns a *shedengine.Shed carrying it
// plus the four told Shed fields.
//
// The twelve rows, with their backing and OnStuck target: 1 Preflight (deps.Preflight, ""); 2
// Discussion-Write (stub, ""); 3 Discussion-Validate (real, bouncing to Discussion-Write); 4
// Discussion-Review (stub, bouncing to Discussion-Write); 5 Plan-Write (stub, ""); 6 Plan-Validate
// (real, bouncing to Plan-Write); 7 Plan-Review (stub, bouncing to Plan-Write); 8 Batchifier (real,
// ""); 9 Webster (the lazy wrapper, ""); 10 Webster-Review (stub, bouncing to Webster); 11 Publish
// (stub, ""); 12 Finalize (stub, ""). Every gate and validator bounces back to the producer whose
// artifact it guards, and a gate whose guarded artifact is produced by no row in the list escalates
// instead -- Preflight gates git and filesystem state and Batchifier gates the batch config, neither
// of which any row writes, so there is nothing to bounce to and a human is the only thing that can
// fix either.
//
// Plan-Sweep is deliberately not a row here: its scout-inventory work is deferred (see
// manifest/roadmap.md's Someday "loom: build Plan-Sweep for real" item) and it has no consumer --
// Plan-Write reads only decision-record.md today -- so a stub row with nothing to feed and nothing
// to bounce to would be dead weight, not a placeholder worth carrying. It returns as a row only
// alongside the task that builds it for real.
//
// New returns an error when deps.Preflight is nil, since a nil row would otherwise panic at the
// call step rather than failing loud; it does not otherwise pre-validate, because shedengine.Run
// validates every field before it touches anything.
func New(deps Deps) (*shedengine.Shed, error) {
	if deps.Preflight == nil {
		return nil, fmt.Errorf("loomshed: deps.Preflight must not be nil")
	}

	producers := []shedengine.ProducerDef{
		{Name: NamePreflight, Producer: deps.Preflight, OnStuck: ""},
		{Name: NameDiscussionWrite, Producer: newStub(NameDiscussionWrite), OnStuck: ""},
		{Name: NameDiscussionValidate, Producer: newDiscussionValidate(NameDiscussionValidate, deps.DecisionRecordPath, deps.SupportLogPath), OnStuck: NameDiscussionWrite},
		{Name: NameDiscussionReview, Producer: newStub(NameDiscussionReview), OnStuck: NameDiscussionWrite},
		{Name: NamePlanWrite, Producer: newStub(NamePlanWrite), OnStuck: ""},
		{Name: NamePlanValidate, Producer: newPlanValidate(NamePlanValidate, deps.AnchorPath, deps.WorktreeRoot), OnStuck: NamePlanWrite},
		{Name: NamePlanReview, Producer: newStub(NamePlanReview), OnStuck: NamePlanWrite},
		{Name: NameBatchifier, Producer: newBatchifier(NameBatchifier, deps.AnchorPath), OnStuck: ""},
		{Name: NameWebster, Producer: newWebsterProducer(NameWebster, deps.AnchorPath, deps.WebsterRun, deps.WebsterDeps), OnStuck: ""},
		{Name: NameWebsterReview, Producer: newStub(NameWebsterReview), OnStuck: NameWebster},
		{Name: NamePublish, Producer: newStub(NamePublish), OnStuck: ""},
		{Name: NameFinalize, Producer: newStub(NameFinalize), OnStuck: ""},
	}

	return &shedengine.Shed{
		Producers:      producers,
		StatusPath:     deps.StatusPath,
		LockPath:       deps.LockPath,
		StatusLockPath: deps.StatusLockPath,
		MaxBounces:     deps.MaxBounces,
	}, nil
}
