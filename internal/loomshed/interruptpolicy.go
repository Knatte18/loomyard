// interruptpolicy.go declares the exported table mapping each of loom's seventeen durable row names
// to the operator-facing action an external supervisor should take when it finds `lyx loom step`
// interrupted mid-row, and the accessor that reads it.

package loomshed

// InterruptPolicyReinvoke and InterruptPolicyHandback are the two closed policy values
// InterruptPolicies maps every row name to. There is no third value: InterruptPolicyFor's empty
// return is the caller's signal that name carries no entry at all, never a third policy word.
const (
	// InterruptPolicyReinvoke means an external supervisor may safely re-invoke `lyx loom step`
	// immediately after an interruption: the row's own adapter re-attaches to a still-live agent
	// rather than spawning a second one, so a re-invocation never double-spawns.
	InterruptPolicyReinvoke = "reinvoke"
	// InterruptPolicyHandback means an external supervisor should hand control back to the operator
	// rather than silently re-invoking: the row's own adapter reclaims (kills) a leftover agent
	// rather than attaching to it, so a re-invocation restarts in-flight work instead of resuming it.
	InterruptPolicyHandback = "handback"
)

// InterruptPolicies maps each of loom's seventeen durable row names (the Name* constants declared
// in loomshed.go) to the InterruptPolicyReinvoke/InterruptPolicyHandback action an external
// supervisor should take after finding that row interrupted. It is keyed by those constants rather
// than by repeated string literals, because the constants are the durable on-disk identity a rename
// would otherwise silently desynchronize this table from.
//
// Every row maps to InterruptPolicyReinvoke except NameWebster, which maps to
// InterruptPolicyHandback. On every row but Webster, re-calling current_producer after an
// interrupted invocation does not double-spawn: internal/shedadapters/doc.go's "Every spawning
// adapter probes for a live agent first" section records that SingleLLMProducer, Bouncer, and
// BurlerProducer all call shuttleengine's Attach seam with the step's own OutputFiles and wait on a
// match before any archive, so a re-invocation reattaches to the live agent rather than spawning a
// second one.
//
// NameWebster is the exception because WebsterProducer inherits websterengine's own entry-time
// reclaim, which stops a leftover Master rather than attaching to it (reclaimEntryTimeStrands in
// internal/websterengine/runlevel.go), so re-invoking an interrupted Webster step kills the
// in-flight Master and restarts the batch run from state.json. Correctness survives that restart --
// recovery from state.json is what the reclaim exists for -- but the cost does not, Webster being
// the most expensive row in the list, and that cost is the operator's to accept rather than the
// supervisor skill's.
var InterruptPolicies = map[string]string{
	NamePreflight:          InterruptPolicyReinvoke,
	NameLoomPreflight:      InterruptPolicyReinvoke,
	NameDiscussionWrite:    InterruptPolicyReinvoke,
	NameDiscussionValidate: InterruptPolicyReinvoke,
	NameDiscussionBouncer:  InterruptPolicyReinvoke,
	NameDiscussionBurler:   InterruptPolicyReinvoke,
	NamePlanWrite:          InterruptPolicyReinvoke,
	NamePlanValidate:       InterruptPolicyReinvoke,
	NamePlanBouncer:        InterruptPolicyReinvoke,
	NamePlanBurler:         InterruptPolicyReinvoke,
	NamePlanRevalidate:     InterruptPolicyReinvoke,
	NameBatchifier:         InterruptPolicyReinvoke,
	NameWebster:            InterruptPolicyHandback,
	NameWebsterBouncer:     InterruptPolicyReinvoke,
	NameWebsterBurler:      InterruptPolicyReinvoke,
	NamePublish:            InterruptPolicyReinvoke,
	NameFinalize:           InterruptPolicyReinvoke,
}

// InterruptPolicyFor returns InterruptPolicies' entry for name, or the empty string when name is
// empty or names no row. The empty return is the caller's signal to omit the key rather than a
// third policy value.
func InterruptPolicyFor(name string) string {
	return InterruptPolicies[name]
}
