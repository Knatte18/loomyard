// beamland.go holds the refusal that stops a beam's landing while anything still works under the beam: an open member entry, a local pair forked from it, or its orch.

package battencli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/orchengine"
)

// beamMember is an open board entry whose parent is a beam: Slug names it, and HasPair says whether this machine holds its pair.
type beamMember struct {
	Slug    string
	HasPair bool
}

// landingSeams carries the reads Worktree-Create makes for a beam's landing, as function values.
// A nil Project reads every entry as a task, which only a hand-populated receiver has.
type landingSeams struct {
	// Project reports whether the slug's board entry is a project.
	Project func() (bool, error)
	// Refusal applies BeamLandRefusal to the slug.
	Refusal func() error
}

// hubLandingSeams builds the landingSeams over the hub of location.
func hubLandingSeams(location *lyxcwd.Location, slug string) landingSeams {
	return landingSeams{
		Project: func() (bool, error) {
			owners, err := boardengine.ReadOwners(fabricengine.BoardDir(location.HubPath))
			if err != nil {
				return false, err
			}
			return owners.Entries[slug].IsProject, nil
		},
		Refusal: func() error { return BeamLandRefusal(location, slug) },
	}
}

// entryIsProject reports whether the receiver's slug is a project entry, which only a beam's landing runs.
func (c *battenCLI) entryIsProject() (bool, error) {
	if c.beam.Project == nil {
		return false, nil
	}
	return c.beam.Project()
}

// BeamLandRefusal reads what still works under the beam slug and refuses its landing while anything does, naming each way forward.
// The facts are the beam's open member entries, the local pairs forked from it, and whether its orch strand or watcher is live.
func BeamLandRefusal(prime *lyxcwd.Location, slug string) error {
	members, err := beamMembers(prime, slug)
	if err != nil {
		return err
	}
	childPairs, err := beamChildPairs(prime, slug)
	if err != nil {
		return err
	}
	orchLive, watcherLive, err := beamOrchLive(prime, slug)
	if err != nil {
		return err
	}
	return beamLandRefusal(slug, members, childPairs, orchLive, watcherLive)
}

// beamLandRefusal judges the facts BeamLandRefusal read and words one refusal naming every blocker with its way forward, or returns nil when none stands.
func beamLandRefusal(slug string, members []beamMember, childPairs []string, orchLive, watcherLive bool) error {
	var blockers []string
	for _, member := range members {
		if member.HasPair {
			blockers = append(blockers, fmt.Sprintf("the open entry %s is a member of the beam: land it with `lyx batten run %s`, or end it with `lyx batten cancel %s --remove`", member.Slug, member.Slug, member.Slug))
			continue
		}
		blockers = append(blockers, fmt.Sprintf("the open entry %s is a member of the beam and has no pair on this machine: move its `project` off the beam with `lyx board upsert`, or land it where it runs", member.Slug))
	}
	for _, pair := range childPairs {
		blockers = append(blockers, fmt.Sprintf("the pair %s was forked from the beam: land it with `lyx batten run %s`, or end it with `lyx batten cancel %s --remove`", pair, pair, pair))
	}
	switch {
	case orchLive && watcherLive:
		blockers = append(blockers, fmt.Sprintf("the orch and its watcher of the beam %s are live: run `lyx orch stop` in the beam's worktree, and re-run once `lyx orch status` there reports the watcher gone", slug))
	case orchLive:
		blockers = append(blockers, fmt.Sprintf("the orch of the beam %s is live: run `lyx orch stop` in the beam's worktree, and re-run once `lyx orch status` there reports the watcher gone", slug))
	case watcherLive:
		blockers = append(blockers, fmt.Sprintf("the watcher of the beam %s is live: run `lyx orch stop` in the beam's worktree, and re-run once `lyx orch status` there reports the watcher gone", slug))
	}
	if len(blockers) == 0 {
		return nil
	}
	return fmt.Errorf("battencli: the beam %s cannot land while work remains under it; way forward: %s", slug, strings.Join(blockers, "; "))
}

// beamMembers lists, sorted, the open tasks and beam projects whose parent, through any depth of grouping projects, is the beam slug.
func beamMembers(prime *lyxcwd.Location, slug string) ([]beamMember, error) {
	tasks, err := boardengine.ReadTasks(fabricengine.BoardDir(prime.HubPath))
	if err != nil {
		return nil, fmt.Errorf("battencli: read the board for the members of the beam %s: %w", slug, err)
	}
	var members []beamMember
	for _, task := range tasks {
		if task.Slug == slug || (task.Status != nil && *task.Status == "done") {
			continue
		}
		if task.Kind != boardengine.KindTask && !(task.Kind == boardengine.KindProject && task.Beam) {
			continue
		}
		parent, err := hubgeom.ParentOf(prime, task.Slug)
		if err != nil {
			return nil, fmt.Errorf("battencli: read the parent of %s for the beam %s: %w", task.Slug, slug, err)
		}
		if parent != slug {
			continue
		}
		hasPair, err := pairPresent(prime, task.Slug)
		if err != nil {
			return nil, err
		}
		members = append(members, beamMember{Slug: task.Slug, HasPair: hasPair})
	}
	slices.SortFunc(members, func(a, b beamMember) int { return strings.Compare(a.Slug, b.Slug) })
	return members, nil
}

// pairPresent reports whether this machine holds the worktree of slug's pair.
func pairPresent(prime *lyxcwd.Location, slug string) (bool, error) {
	info, err := os.Stat(fabricengine.WorktreePath(prime, slug))
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil && info.IsDir(), err
}

// beamChildPairs lists, sorted, the local pairs whose origin record names the beam slug as their parent worktree.
func beamChildPairs(prime *lyxcwd.Location, slug string) ([]string, error) {
	worktrees, err := fabricengine.List(prime.AnchorPath())
	if err != nil {
		return nil, fmt.Errorf("battencli: list the worktrees for the pairs forked from the beam %s: %w", slug, err)
	}
	var pairs []string
	for _, worktree := range worktrees {
		if worktree.Main || worktree.Prunable {
			continue
		}
		name := filepath.Base(filepath.Clean(filepath.FromSlash(worktree.Path)))
		if name == slug {
			continue
		}
		origin, found, err := fabricengine.ReadOriginFor(prime, name)
		if err != nil {
			return nil, fmt.Errorf("battencli: read the origin record of %s for the beam %s: %w", name, slug, err)
		}
		if found && origin.ParentWorktree == slug {
			pairs = append(pairs, name)
		}
	}
	slices.Sort(pairs)
	return pairs, nil
}

// beamOrchLive reports whether the orch strand and the watcher of the beam slug are live; a beam with no pair on this machine has neither.
func beamOrchLive(prime *lyxcwd.Location, slug string) (orchLive, watcherLive bool, err error) {
	present, err := pairPresent(prime, slug)
	if err != nil || !present {
		return false, false, err
	}
	beam, err := lyxcwd.ResolveWorktree(fabricengine.WorktreePath(prime, slug))
	if err != nil {
		return false, false, fmt.Errorf("battencli: resolve the beam %s to read its orch: %w", slug, err)
	}
	orchLive, err = orchcli.OrchLive(beam)
	if err != nil {
		return false, false, fmt.Errorf("battencli: read whether the orch of the beam %s is live: %w", slug, err)
	}
	watcherLive, err = orchengine.WatcherLive(orchcli.PrimePaths(beam))
	if err != nil {
		return false, false, fmt.Errorf("battencli: read whether the watcher of the beam %s is live: %w", slug, err)
	}
	return orchLive, watcherLive, nil
}
