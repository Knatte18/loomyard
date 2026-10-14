// reviewbudget.go — the review budget key names, categories, cap resolution, layer routes and bound checks.

package reviewbudget

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/configengine"
)

const (
	// RunSoftKey is the run-wide bounce budget key.
	RunSoftKey = "review_max_bounces"
	// RunHardKey is the run-wide hard cap key.
	RunHardKey = "review_hard_max_bounces"
)

// SoftKey returns the key of perch's own bounce budget.
func SoftKey(perch string) string { return perch + "_max_bounces" }

// HardKey returns the key of perch's own hard cap.
func HardKey(perch string) string { return perch + "_hard_max_bounces" }

// AutoAcceptKey returns the key of perch's auto-accept flag.
func AutoAcceptKey(perch string) string { return perch + "_auto_accept" }

// Categories returns the category of every key of the family for perches.
// The budget and hard-cap keys are CategoryNone and the auto-accept keys CategoryGuard.
// With no perches it returns the two run-wide keys alone.
func Categories(perches []string) map[string]configengine.Category {
	categories := map[string]configengine.Category{
		RunSoftKey: configengine.CategoryNone,
		RunHardKey: configengine.CategoryNone,
	}
	for _, perch := range perches {
		categories[SoftKey(perch)] = configengine.CategoryNone
		categories[HardKey(perch)] = configengine.CategoryNone
		categories[AutoAcceptKey(perch)] = configengine.CategoryGuard
	}
	return categories
}

// Caps is a perch's resolved soft and hard cap and the key each resolves from.
// Soft is the bounce budget, 0 meaning no review round; Hard is the round number at or past which the perch never continues a round.
type Caps struct {
	SoftKey string
	Soft    int
	HardKey string
	Hard    int
}

// Resolve returns perch's caps.
// The soft cap is *ownSoft when ownSoft is non-nil, an own 0 included, else runSoft.
// The hard cap is ownHard when above 0, else runHard.
func Resolve(perch string, runSoft, runHard int, ownSoft *int, ownHard int) Caps {
	caps := Caps{SoftKey: RunSoftKey, Soft: runSoft, HardKey: RunHardKey, Hard: runHard}
	if ownSoft != nil {
		caps.SoftKey, caps.Soft = SoftKey(perch), *ownSoft
	}
	if ownHard > 0 {
		caps.HardKey, caps.Hard = HardKey(perch), ownHard
	}
	return caps
}

// Values holds the budget keys of one loaded config.
// PerchSoft and PerchHard are keyed by perch name; a nil PerchSoft entry takes RunSoft and a PerchHard entry of 0 takes RunHard.
type Values struct {
	RunSoft   int
	RunHard   int
	Perches   []string
	PerchSoft map[string]*int
	PerchHard map[string]int
}

// Route returns the way forward that changes key, by the layer the resolved value comes from.
// A local value is changed there and in the task's run_settings, a worktree value in the task worktree, and any other value in the default file from the prime worktree.
func Route(dirs configengine.Dirs, spec configengine.Spec, key string) (string, error) {
	stack, err := configengine.StackLayers(dirs, spec)
	if err != nil {
		return "", err
	}
	switch stack.Origins[key].Layer {
	case configengine.LayerLocal:
		return fmt.Sprintf(`run "lyx config %s --layer local --set %s=<n>" in the task worktree, and correct the task's run_settings with "lyx board upsert" too, so a later Seed-Child writes the same value`, spec.Name, key), nil
	case configengine.LayerWorktree:
		return fmt.Sprintf(`run "lyx config %s --set %s=<n>" in the task worktree`, spec.Name, key), nil
	default:
		if spec.HubWide {
			return fmt.Sprintf(`run "lyx config %s --set %s=<n>" from the prime`, spec.Name, key), nil
		}
		return fmt.Sprintf(`run "lyx config %s --layer default --set %s=<n>" from the hub's prime worktree, where the operator or the hub orch runs it`, spec.Name, key), nil
	}
}

// CheckResolved refuses the first value of the family below its bound, and a resolved hard cap below its resolved soft cap.
// Each refusal names the key and the Route of the layer its value comes from.
func CheckResolved(dirs configengine.Dirs, spec configengine.Spec, values Values) error {
	refuse := func(key, detail string) error {
		route, err := Route(dirs, spec, key)
		if err != nil {
			return err
		}
		return fmt.Errorf("%s config key %q: %s; way forward: %s, then run \"lyx loom resume\"", spec.Name, key, detail, route)
	}

	if values.RunSoft < 0 {
		return refuse(RunSoftKey, fmt.Sprintf("must not be negative, got %d; set it to 0 for no review round, or to a positive integer", values.RunSoft))
	}
	for _, perch := range values.Perches {
		if own := values.PerchSoft[perch]; own != nil && *own < 0 {
			return refuse(SoftKey(perch), fmt.Sprintf("must not be negative, got %d; set it to 0 for no review round, to a positive integer, or to null to take %s", *own, RunSoftKey))
		}
	}
	if values.RunHard < 1 {
		return refuse(RunHardKey, fmt.Sprintf("must be at least 1, got %d", values.RunHard))
	}
	for _, perch := range values.Perches {
		if own := values.PerchHard[perch]; own < 0 {
			return refuse(HardKey(perch), fmt.Sprintf("must not be negative, got %d; set it to 0 to take %s, or to a positive integer", own, RunHardKey))
		}
	}

	// Equal caps pass: a round at the budget is already at the hard cap.
	perches := values.Perches
	if len(perches) == 0 {
		perches = []string{""}
	}
	for _, perch := range perches {
		caps := Resolve(perch, values.RunSoft, values.RunHard, values.PerchSoft[perch], values.PerchHard[perch])
		if caps.Hard >= caps.Soft {
			continue
		}
		hardRoute, err := Route(dirs, spec, caps.HardKey)
		if err != nil {
			return err
		}
		softRoute, err := Route(dirs, spec, caps.SoftKey)
		if err != nil {
			return err
		}
		return fmt.Errorf("%s config key %q: resolved hard cap %d is below %q's resolved soft cap %d; way forward: raise the hard cap: %s, or lower the soft cap: %s, then run \"lyx loom resume\"", spec.Name, caps.HardKey, caps.Hard, caps.SoftKey, caps.Soft, hardRoute, softRoute)
	}
	return nil
}

// CheckLocal refuses a budget key of the family present in set that is below its bound or not an integer.
// set is a local-layer document decoded into a map; every other key is left alone.
// A nil value passes only for a perch's own soft key, where null takes the run-wide budget.
// Every refusal is marked configengine.ErrInvalid.
func CheckLocal(set map[string]any, perches []string) error {
	type bound struct {
		key     string
		min     int
		nilable bool
	}
	bounds := []bound{{RunSoftKey, 0, false}, {RunHardKey, 1, false}}
	for _, perch := range perches {
		bounds = append(bounds, bound{SoftKey(perch), 0, true}, bound{HardKey(perch), 0, false})
	}
	for _, b := range bounds {
		value, present := set[b.key]
		if !present {
			continue
		}
		if value == nil && b.nilable {
			continue
		}
		number, ok := value.(int)
		if !ok {
			return configengine.MarkInvalid(fmt.Errorf("key %q: must be an integer", b.key))
		}
		if number < b.min {
			return configengine.MarkInvalid(fmt.Errorf("key %q: must be at least %d, got %d; correct the task's run_settings with \"lyx board upsert\"", b.key, b.min, number))
		}
	}
	return nil
}
