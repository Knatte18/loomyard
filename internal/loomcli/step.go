// step.go holds stepKindForBootstrapStage, which maps bootstrapStage onto shedverbs' refusal-kind vocabulary.
// The verb's body lives in arm.go's loomPreStep and loomPostStep hooks, which shedverbs' generic step command calls.

package loomcli

import "github.com/Knatte18/loomyard/internal/shedverbs"

// stepKindForBootstrapStage maps a bootstrapStage onto shedverbs' refusal-kind vocabulary.
// A bootstrapStage classifies where in the bootstrap a failure occurred.
// bootstrapStageSeed maps to shedverbs.KindUnseeded, and bootstrapStageOwnership maps to shedverbs.KindOwnership.
// Those are the two sub-steps with a more specific kind.
// bootstrapStageOrigin and bootstrapStageCommit map to shedverbs.KindBootstrap.
// So does every other value, bootstrapStageNone included.
// An unclassifiable failure therefore still carries a kind rather than an empty one.
func stepKindForBootstrapStage(stage bootstrapStage) string {
	switch stage {
	case bootstrapStageSeed:
		return shedverbs.KindUnseeded
	case bootstrapStageOwnership:
		return shedverbs.KindOwnership
	default:
		return shedverbs.KindBootstrap
	}
}
