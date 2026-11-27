// interruptpolicy.go declares the exported InterruptPolicies table and the InterruptPolicyFor accessor that reads it.
// The table maps each durable row name to the action an external supervisor takes once `lym loom step` is interrupted.
// The two policy words are shedverbs.InterruptPolicyReinvoke and shedverbs.InterruptPolicyHandback.

package loomshed

import (
	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/shedverbs"
)

// InterruptPolicies maps each durable row name to the action an external supervisor takes when a row is interrupted.
// The action is shedverbs.InterruptPolicyReinvoke or shedverbs.InterruptPolicyHandback.
// The table is keyed by the row name constants rather than by repeated string literals.
// The constants are each row's durable on-disk identity; renaming one would silently desynchronize this table.
//
// Every row maps to shedverbs.InterruptPolicyReinvoke except Webster, which maps to shedverbs.InterruptPolicyHandback.
// On every row but Webster and Friction-Reflect, re-invoking an interrupted row does not double-spawn.
// Each spawning adapter probes for a live agent with the step's own output files first.
// So a re-invocation reattaches to the live agent rather than spawning a second one.
//
// Friction-Reflect probes for a live reflection agent and archives a finished reflection's notes instead of spawning.
// So re-invoking an interrupted step never double-files.
//
// Webster is the exception: its entry-time reclaim stops a leftover Master rather than attaching to it;
// so re-invoking an interrupted Webster step kills the in-flight Master and restarts the batch run from state.json.
// Correctness survives that restart, since recovery from state.json is the reason the reclaim exists.
// The cost is a different matter: Webster is the most expensive row in the list.
// Accepting that cost is the operator's call, not the supervisor's.
var InterruptPolicies = map[string]string{
	NamePreflight:         shedverbs.InterruptPolicyReinvoke,
	NameDiscussionWrite:   shedverbs.InterruptPolicyReinvoke,
	NameDiscussionBouncer: shedverbs.InterruptPolicyReinvoke,
	NamePlanWrite:         shedverbs.InterruptPolicyReinvoke,
	NamePlanBouncer:       shedverbs.InterruptPolicyReinvoke,
	NameWebster:           shedverbs.InterruptPolicyHandback,
	NameCodeBouncer:       shedverbs.InterruptPolicyReinvoke,
	NameDescribe:          shedverbs.InterruptPolicyReinvoke,
	NameParentAlign:       shedverbs.InterruptPolicyReinvoke, // spawns only the conflict session the resolver reattaches
	// PR-Review reattaches the live interactive session or resumes its recorded one, rather than spawning a second one.
	NamePRReview:        shedverbs.InterruptPolicyReinvoke,
	NameDarnWrite:       shedverbs.InterruptPolicyReinvoke, // the single-session adapter reattaches rather than double-spawns
	NameFinalize:        shedverbs.InterruptPolicyReinvoke,
	NameFrictionReflect: shedverbs.InterruptPolicyReinvoke,

	// Both halves of every review segment probe for a live session and resume it rather than spawn a second one.
	NameDiscussionBurlerReview: shedverbs.InterruptPolicyReinvoke,
	NameDiscussionBurlerFix:    shedverbs.InterruptPolicyReinvoke,
	NamePlanBurlerReview:       shedverbs.InterruptPolicyReinvoke,
	NamePlanBurlerFix:          shedverbs.InterruptPolicyReinvoke,
	NameCodeBurlerReview:       shedverbs.InterruptPolicyReinvoke,
	NameCodeBurlerFix:          shedverbs.InterruptPolicyReinvoke,

	// Record-Write writes its files only when missing and commits the pair; a re-invocation completes what a crash left.
	NameRecordWrite: shedverbs.InterruptPolicyReinvoke,

	// Agent-spawning Drift rows probe for a live chair and reattach to it; Drift-Await and Drift-Recheck spawn nothing.
	NameDriftBaseline: shedverbs.InterruptPolicyReinvoke,
	NameDriftAwait:    shedverbs.InterruptPolicyReinvoke,
	NameDriftExpect:   shedverbs.InterruptPolicyReinvoke,
	NameDriftVerify:   shedverbs.InterruptPolicyReinvoke,
	NameDriftRecheck:  shedverbs.InterruptPolicyReinvoke,

	// The parts rows probe for a live chair and resume it through the seat probe rather than spawn a second one.
	NamePartsWrite:     shedverbs.InterruptPolicyReinvoke,
	NamePartsPlanWrite: shedverbs.InterruptPolicyReinvoke,

	// The Motif producer returns Done at once, so re-invoking it repeats nothing.
	NameMotif: shedverbs.InterruptPolicyReinvoke,

	// Sampler-Session reattaches the live session or resumes the recorded one rather than spawning a second.
	NameSamplerSession: shedverbs.InterruptPolicyReinvoke,

	// Raddle-Write probes for a live session and resumes it rather than spawning a second one.
	NameRaddleWrite: shedverbs.InterruptPolicyReinvoke,
	// Every step of Raddle-File is idempotent, so a re-invocation finishes whatever a crash left.
	NameRaddleFile: shedverbs.InterruptPolicyReinvoke,

	// The split perch's rows probe for a live session and resume it, as every review segment's rows do.
	NameRaddleBouncer:      shedverbs.InterruptPolicyReinvoke,
	NameRaddleBurlerReview: shedverbs.InterruptPolicyReinvoke,
	NameRaddleBurlerFix:    shedverbs.InterruptPolicyReinvoke,

	// Each pawl row spawns no agent and is idempotent, so re-invoking an interrupted one repeats nothing.
	pawlengine.NameBeamUp:    shedverbs.InterruptPolicyReinvoke,
	pawlengine.NameBeamAwait: shedverbs.InterruptPolicyReinvoke,
	pawlengine.NameBeamDown:  shedverbs.InterruptPolicyReinvoke,
}

// InterruptPolicyFor returns the policy action for name, or the empty string when name is empty or names no row.
// The empty return tells the caller to omit the key rather than treat it as a third policy value.
func InterruptPolicyFor(name string) string {
	return InterruptPolicies[name]
}
