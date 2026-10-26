// beamland.go holds the refusal that stops a beam's landing while anything still works under the beam: an open member entry or a local pair forked from it.

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
)

// BeamLandMember is an open board entry whose parent is a beam.
// Slug names it, HasPair says whether this machine holds its pair, and Running says whether its board status is a run status, so a run is in flight on some machine.
type BeamLandMember struct {
	Slug    string
	HasPair bool
	Running bool
}

// BeamLandBlockers is what still works under a beam: its open members and the local pairs forked from it.
type BeamLandBlockers struct {
	Members    []BeamLandMember
	ChildPairs []string
}

// Empty reports whether nothing works under the beam, so its landing may go on.
func (b BeamLandBlockers) Empty() bool {
	return len(b.Members) == 0 && len(b.ChildPairs) == 0
}

// landingSeams carries the reads Worktree-Create makes for a beam project's run, as function values.
// A nil Project reads every entry as a task, which only a hand-populated receiver has.
type landingSeams struct {
	// Project reports whether the slug's board entry is a project.
	Project func() (bool, error)
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
	}
}

// entryIsProject reports whether the receiver's slug is a project entry, which only a beam project's run makes a pair for from its seed's anchor.
func (c *battenCLI) entryIsProject() (bool, error) {
	if c.beam.Project == nil {
		return false, nil
	}
	return c.beam.Project()
}

// ReadBeamLandBlockers reads what still works under the beam slug: its open member entries, through any depth of grouping projects, and the local pairs forked from it.
// The beam's own orch is no blocker, since it runs until the beam comes down.
func ReadBeamLandBlockers(prime *lyxcwd.Location, slug string) (BeamLandBlockers, error) {
	members, err := beamMembers(prime, slug)
	if err != nil {
		return BeamLandBlockers{}, err
	}
	childPairs, err := beamChildPairs(prime, slug)
	if err != nil {
		return BeamLandBlockers{}, err
	}
	return BeamLandBlockers{Members: members, ChildPairs: childPairs}, nil
}

// BeamLandRefusal refuses the landing of the beam slug while anything works under it, naming each way forward.
func BeamLandRefusal(prime *lyxcwd.Location, slug string) error {
	blockers, err := ReadBeamLandBlockers(prime, slug)
	if err != nil {
		return err
	}
	return beamLandRefusal(slug, blockers)
}

// beamLandRefusal words one refusal naming every blocker with its way forward, or returns nil when none stands.
func beamLandRefusal(slug string, blockers BeamLandBlockers) error {
	if blockers.Empty() {
		return nil
	}
	var ways []string
	for _, member := range blockers.Members {
		if member.HasPair {
			ways = append(ways, fmt.Sprintf("the open entry %s is a member of the beam: land it with `lyx batten run %s`, or end it with `lyx batten cancel %s --remove`", member.Slug, member.Slug, member.Slug))
			continue
		}
		ways = append(ways, fmt.Sprintf("the open entry %s is a member of the beam and has no pair on this machine: move its `project` off the beam with `lyx board upsert`, or land it where it runs", member.Slug))
	}
	for _, pair := range blockers.ChildPairs {
		ways = append(ways, fmt.Sprintf("the pair %s was forked from the beam: land it with `lyx batten run %s`, or end it with `lyx batten cancel %s --remove`", pair, pair, pair))
	}
	return fmt.Errorf("battencli: the beam %s cannot land while work remains under it; way forward: %s", slug, strings.Join(ways, "; "))
}

// beamMembers lists, sorted, the open tasks and beam projects whose parent, through any depth of grouping projects, is the beam slug.
func beamMembers(prime *lyxcwd.Location, slug string) ([]BeamLandMember, error) {
	tasks, err := boardengine.ReadTasks(fabricengine.BoardDir(prime.HubPath))
	if err != nil {
		return nil, fmt.Errorf("battencli: read the board for the members of the beam %s: %w", slug, err)
	}
	var members []BeamLandMember
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
		members = append(members, BeamLandMember{Slug: task.Slug, HasPair: hasPair, Running: boardengine.IsRunStatus(task.Status)})
	}
	slices.SortFunc(members, func(a, b BeamLandMember) int { return strings.Compare(a.Slug, b.Slug) })
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
