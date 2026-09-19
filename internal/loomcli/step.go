// step.go holds stepKindForBootstrapStage, loom's own mapping from seedAndCommitBootstrap's
// bootstrapStage classification onto shedverbs' refusal-kind vocabulary. step's own verb body now
// lives in arm.go's loomPreStep/loomPostStep hooks, called by shedverbs' generic step command.

package loomcli

import "github.com/Knatte18/loomyard/internal/shedverbs"

// stepKindForBootstrapStage maps a bootstrapStage -- seedAndCommitBootstrap's own classification of
// where in the bootstrap a failure occurred -- onto shedverbs' refusal-kind vocabulary.
// bootstrapStageSeed maps to shedverbs.KindUnseeded and bootstrapStageOwnership maps to
// shedverbs.KindOwnership, the two sub-steps with a more specific kind; bootstrapStageOrigin and
// bootstrapStageCommit both map to shedverbs.KindBootstrap, as does every other value including
// bootstrapStageNone, so an unclassifiable failure still carries a kind rather than an empty one.
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
