// interruptpolicy.go declares the exported table mapping each of loom's durable row names
// to the operator-facing action an external supervisor should take when it finds `lyx loom step`
// interrupted mid-row, and the accessor that reads it.
// The two policy words themselves are shedverbs.InterruptPolicyReinvoke and shedverbs.InterruptPolicyHandback.

package loomshed

import "github.com/Knatte18/loomyard/internal/shedverbs"

// InterruptPolicies maps each of loom's durable row names (the Name* constants declared
// in loomshed.go) to the shedverbs.InterruptPolicyReinvoke/shedverbs.InterruptPolicyHandback action an external
// supervisor should take after finding that row interrupted. It is keyed by those constants rather
// than by repeated string literals, because the constants are the durable on-disk identity a rename
// would otherwise silently desynchronize this table from.
//
// Every row maps to shedverbs.InterruptPolicyReinvoke except NameWebster, which maps to shedverbs.InterruptPolicyHandback.
// On every row but Webster and Friction-Reflect, re-calling current_producer after an interrupted invocation does not double-spawn:
// internal/shedadapters/doc.go's "Every spawning adapter probes for a live agent first" section records that every spawning adapter probes for a live agent with the step's own OutputFiles and waits on a match before any archive.
// So a re-invocation reattaches to the live agent rather than spawning a second one.
//
// NameFrictionReflect's premise is the attach premise:
// frictionengine.Reflect probes shuttle's attach seam for a live reflection agent and archives a finished reflection's notes instead of spawning,
// so re-invoking an interrupted step never double-files.
//
// NameWebster is the exception because WebsterProducer inherits websterengine's own entry-time
// reclaim, which stops a leftover Master rather than attaching to it (reclaimEntryTimeStrands in
// internal/websterengine/runlevel.go), so re-invoking an interrupted Webster step kills the
// in-flight Master and restarts the batch run from state.json. Correctness survives that restart --
// recovery from state.json is what the reclaim exists for -- but the cost does not, Webster being
// the most expensive row in the list, and that cost is the operator's to accept rather than the
// supervisor skill's.
var InterruptPolicies = map[string]string{
	NamePreflight:         shedverbs.InterruptPolicyReinvoke,
	NameLoomPreflight:     shedverbs.InterruptPolicyReinvoke,
	NameDiscussionWrite:   shedverbs.InterruptPolicyReinvoke,
	NameDiscussionBouncer: shedverbs.InterruptPolicyReinvoke,
	NamePlanWrite:         shedverbs.InterruptPolicyReinvoke,
	NamePlanBouncer:       shedverbs.InterruptPolicyReinvoke,
	NameBatchifier:        shedverbs.InterruptPolicyReinvoke,
	NameWebster:           shedverbs.InterruptPolicyHandback,
	NameWebsterBouncer:    shedverbs.InterruptPolicyReinvoke,
	NameDescribe:          shedverbs.InterruptPolicyReinvoke,
	NameMergeIn:           shedverbs.InterruptPolicyReinvoke, // spawns only the conflict session the resolver reattaches
	NameMergeBouncer:      shedverbs.InterruptPolicyReinvoke,
	NamePublish:           shedverbs.InterruptPolicyReinvoke,
	NamePRAlign:           shedverbs.InterruptPolicyReinvoke, // re-entry reuses the live spawn or its verdict
	NamePRGate:            shedverbs.InterruptPolicyReinvoke, // spawns nothing
	NamePRRework:          shedverbs.InterruptPolicyReinvoke, // the single-session adapter reattaches rather than double-spawns
	NameDarnWrite:         shedverbs.InterruptPolicyReinvoke, // the single-session adapter reattaches rather than double-spawns
	NameDarnBouncer:       shedverbs.InterruptPolicyReinvoke,
	NameFinalize:          shedverbs.InterruptPolicyReinvoke,
	NameFrictionReflect:   shedverbs.InterruptPolicyReinvoke,

	// Both halves of every review segment probe for a live session and resume it rather than spawn a second one.
	NameDiscussionBurlerReview: shedverbs.InterruptPolicyReinvoke,
	NameDiscussionBurlerFix:    shedverbs.InterruptPolicyReinvoke,
	NamePlanBurlerReview:       shedverbs.InterruptPolicyReinvoke,
	NamePlanBurlerFix:          shedverbs.InterruptPolicyReinvoke,
	NameWebsterBurlerReview:    shedverbs.InterruptPolicyReinvoke,
	NameWebsterBurlerFix:       shedverbs.InterruptPolicyReinvoke,
	NameMergeBurlerReview:      shedverbs.InterruptPolicyReinvoke,
	NameMergeBurlerFix:         shedverbs.InterruptPolicyReinvoke,
	NameDarnBurlerReview:       shedverbs.InterruptPolicyReinvoke,
	NameDarnBurlerFix:          shedverbs.InterruptPolicyReinvoke,
}

// InterruptPolicyFor returns InterruptPolicies' entry for name, or the empty string when name is
// empty or names no row. The empty return is the caller's signal to omit the key rather than a
// third policy value.
func InterruptPolicyFor(name string) string {
	return InterruptPolicies[name]
}
