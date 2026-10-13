// caps.go — a loom review perch's resolved soft and hard bounce caps, and the way forward that changes the loom.yaml key a cap comes from.

package loomengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/configengine"
)

// Perch names a loom review perch by its loom.yaml key prefix.
type Perch string

const (
	// PerchDiscussion is the Discussion-Review perch.
	PerchDiscussion Perch = "discussion"
	// PerchPlan is the Plan-Review perch.
	PerchPlan Perch = "plan"
	// PerchWebster is the Webster-Review perch.
	PerchWebster Perch = "webster"
	// PerchMerge is the Merge-Review perch.
	PerchMerge Perch = "merge"
)

// PerchCaps is a perch's resolved soft and hard cap and the loom.yaml key each resolves from.
// Soft is the bounce budget; Hard is the round number at or past which the perch never continues a round.
type PerchCaps struct {
	SoftKey string
	Soft    int
	HardKey string
	Hard    int
}

// Caps resolves p's soft and hard cap: the perch's own `<perch>_max_bounces` and `<perch>_hard_max_bounces` when above 0, else `review_max_bounces` and `review_hard_max_bounces`.
func (cfg Config) Caps(p Perch) PerchCaps {
	var ownSoft, ownHard int
	switch p {
	case PerchDiscussion:
		ownSoft, ownHard = cfg.DiscussionMaxBounces, cfg.DiscussionHardMaxBounces
	case PerchPlan:
		ownSoft, ownHard = cfg.PlanMaxBounces, cfg.PlanHardMaxBounces
	case PerchWebster:
		ownSoft, ownHard = cfg.WebsterMaxBounces, cfg.WebsterHardMaxBounces
	case PerchMerge:
		ownSoft, ownHard = cfg.MergeMaxBounces, cfg.MergeHardMaxBounces
	}
	caps := PerchCaps{
		SoftKey: "review_max_bounces",
		Soft:    cfg.ReviewMaxBounces,
		HardKey: "review_hard_max_bounces",
		Hard:    cfg.ReviewHardMaxBounces,
	}
	if ownSoft > 0 {
		caps.SoftKey, caps.Soft = string(p)+"_max_bounces", ownSoft
	}
	if ownHard > 0 {
		caps.HardKey, caps.Hard = string(p)+"_hard_max_bounces", ownHard
	}
	return caps
}

// CapRoute returns the way forward that changes the loom.yaml key, by the layer its resolved value comes from.
// A value from the local layer is changed there and in the task's run_settings, a worktree value in the task worktree, and a default or template value from the hub's prime worktree.
func CapRoute(dirs configengine.Dirs, key string) (string, error) {
	stack, err := configengine.StackLayers(dirs, ConfigSpec())
	if err != nil {
		return "", err
	}
	switch stack.Origins[key].Layer {
	case configengine.LayerLocal:
		return fmt.Sprintf(`run "lyx config loom --layer local --set %s=<n>" in the task worktree, and correct the task's run_settings with "lyx board upsert" too, so a later Seed-Child writes the same value`, key), nil
	case configengine.LayerWorktree:
		return fmt.Sprintf(`run "lyx config loom --set %s=<n>" in the task worktree`, key), nil
	default:
		return fmt.Sprintf(`run "lyx config loom --layer default --set %s=<n>" from the hub's prime worktree, where the operator or the hub orch runs it`, key), nil
	}
}
