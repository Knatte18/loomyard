// upkeep.go decides, for one package's coverage map, what an upkeep pass keeps, drops and queues at a new commit.
// It builds nothing and runs no test.

package testcover

import (
	"maps"
	"slices"
	"strings"
	"time"
)

// UpkeepVerdict is what an upkeep plan found for one coverage map.
type UpkeepVerdict string

// The verdicts of an upkeep plan.
const (
	// UpkeepCurrent is a map whose selection at the new commit has no affected test and no miss.
	UpkeepCurrent UpkeepVerdict = "current"
	// UpkeepAffected is a map whose selection has affected tests, whose entries the plan drops.
	UpkeepAffected UpkeepVerdict = "affected"
	// UpkeepMiss is a map the selector missed for: the plan leaves it as it is, and the package is rebuilt whole.
	UpkeepMiss UpkeepVerdict = "miss"
)

// UpkeepTree is a package's file hashes at the commit an upkeep plan moves its map to, in Map's own shape.
type UpkeepTree struct {
	Files      map[string]string
	OtherFiles map[string]string
	Linked     map[string]map[string]string
}

// UpkeepInput is what PlanUpkeep decides one map from.
type UpkeepInput struct {
	// Map is the package's coverage map.
	Map Map
	// Selection is the selector's verdict on Map with Commit as target.
	Selection Selection
	// Commit is the commit the plan moves the map to, and At the time of the plan.
	Commit string
	At     time.Time
	// Tree is the package's file hashes, and those of each package Map links, at Commit.
	Tree UpkeepTree
	// Declared are the tests of the package at Commit that a map holds: those of a mapped tier.
	Declared []TestFunc
	// Spawns returns the spawn scan of the package at Commit.
	// The plan calls it only when a declared test has no entry.
	Spawns func() (map[string]SpawnVerdict, error)
}

// UpkeepPlan is what an upkeep pass does with one map.
type UpkeepPlan struct {
	Verdict UpkeepVerdict
	// Miss is the selector's miss under UpkeepMiss, nil otherwise.
	Miss *Miss
	// Map is the map at the plan's commit, set unless Verdict is UpkeepMiss.
	Map Map
	// Changed reports whether Map differs from the input map, so the pass writes it.
	Changed bool
	// Queue names the declared tests Map holds no entry for, in declared order: the tests the pass measures.
	Queue []string
}

// PlanUpkeep plans one coverage map at a new commit from its selection there.
// A miss leaves the map as it is.
// Otherwise the map moves to the commit with the commit's file hashes, and keeps every entry but these, which it drops:
// an affected test's, the entry of a test no longer declared,
// and an outcome entry when any of the package's own files differs between the two commits.
// A failure the map records at another commit is cleared, since the map now stands at the commit.
// A declared test that then has no entry is queued, unless the spawn scan marks it out of process or unresolved:
// such a test gets an entry with its verdict and is never run.
// The error is the spawn scan's.
func PlanUpkeep(in UpkeepInput) (UpkeepPlan, error) {
	if in.Selection.Miss != nil {
		return UpkeepPlan{Verdict: UpkeepMiss, Miss: in.Selection.Miss}, nil
	}
	plan := UpkeepPlan{Verdict: UpkeepCurrent, Changed: in.Map.Commit != in.Commit}
	if len(in.Selection.Tests) > 0 {
		plan.Verdict = UpkeepAffected
	}
	ownChanged := !maps.Equal(in.Map.Files, in.Tree.Files) || !maps.Equal(in.Map.OtherFiles, in.Tree.OtherFiles)
	m := in.Map
	m.Commit = in.Commit
	m.Files, m.OtherFiles, m.Linked = in.Tree.Files, in.Tree.OtherFiles, in.Tree.Linked
	if m.Failure != nil && m.Failure.Commit != in.Commit {
		m.Failure = nil
		plan.Changed = true
	}
	m.Tests = nil
	declared := map[string]bool{}
	for _, fn := range in.Declared {
		declared[fn.Name] = true
	}
	entries := map[string]bool{}
	for _, test := range in.Map.Tests {
		if !declared[test.Name] || slices.Contains(in.Selection.Tests, test.Name) || (test.Outcome != "" && ownChanged) {
			plan.Changed = true
			continue
		}
		m.Tests = append(m.Tests, test)
		entries[test.Name] = true
	}
	var spawns map[string]SpawnVerdict
	for _, fn := range in.Declared {
		if entries[fn.Name] {
			continue
		}
		if spawns == nil {
			var err error
			if spawns, err = in.Spawns(); err != nil {
				return UpkeepPlan{}, err
			}
		}
		if verdict := spawns[fn.Name]; verdict.OutOfProcess || verdict.Unresolved {
			m.Tests = append(m.Tests, MapTest{Name: fn.Name, Tier: fn.Tier, FuncHash: fn.Hash, Spawn: verdict, Commit: in.Commit, MeasuredAt: in.At})
			plan.Changed = true
			continue
		}
		plan.Queue = append(plan.Queue, fn.Name)
	}
	slices.SortStableFunc(m.Tests, func(a, b MapTest) int { return strings.Compare(a.Name, b.Name) })
	plan.Map = m
	return plan, nil
}
