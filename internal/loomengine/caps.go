// caps.go defines the loom review perches and resolves their soft and hard bounce caps through reviewbudget.

package loomengine

import (
	"github.com/Knatte18/loomyard/internal/reviewbudget"
)

// Perch names a loom review perch by the loom.yaml area that holds its keys.
type Perch string

const (
	// PerchDiscussion is the Discussion-Review perch.
	PerchDiscussion Perch = "discussion"
	// PerchPlan is the Plan-Review perch.
	PerchPlan Perch = "plan"
	// PerchCode is the Code-Review perch.
	PerchCode Perch = "code"
	// PerchSplit is the split perch, the Raddle-Review perch over a raddle proposal.
	PerchSplit Perch = "split"
)

// reviewPerches lists the review perches whose keys loom.yaml holds.
// Config loading, its spec and local-value validation take the perches from here.
// The plan perch's keys live in tapestry.yaml, so it is not listed here.
var reviewPerches = []Perch{PerchDiscussion, PerchCode, PerchSplit}

// reviewPerchNames returns reviewPerches' names as reviewbudget takes them.
func reviewPerchNames() []string {
	names := make([]string, len(reviewPerches))
	for i, perch := range reviewPerches {
		names[i] = string(perch)
	}
	return names
}

// Caps resolves p's soft and hard bounce caps.
// The soft cap is the perch's own `<perch>.max_bounces` when that is non-null, else `review.max_bounces`.
// The hard cap is the perch's own `<perch>.hard_max_bounces` when that is above 0, else `review.hard_max_bounces`.
func (cfg Config) Caps(p Perch) reviewbudget.Caps {
	var ownSoft *int
	var ownHard int
	switch p {
	case PerchDiscussion:
		ownSoft, ownHard = cfg.Discussion.MaxBounces, cfg.Discussion.HardMaxBounces
	case PerchPlan:
		ownSoft, ownHard = cfg.Plan.MaxBounces, cfg.Plan.HardMaxBounces
	case PerchCode:
		ownSoft, ownHard = cfg.Code.MaxBounces, cfg.Code.HardMaxBounces
	case PerchSplit:
		ownSoft, ownHard = cfg.Split.MaxBounces, cfg.Split.HardMaxBounces
	}
	return reviewbudget.Resolve(string(p), cfg.Review.MaxBounces, cfg.Review.HardMaxBounces, ownSoft, ownHard)
}
