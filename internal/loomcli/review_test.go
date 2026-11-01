package loomcli

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// fakeReviewTarget builds reviewTargetDeps over a hub with the given worktrees, each mapped to the recipe its run's seed records.
// An empty recipe reads as tapestry, as a worktree with no seed does.
func fakeReviewTarget(prime string, existing map[string]string) reviewTargetDeps {
	return reviewTargetDeps{
		primeName: func(*lymcwd.Location) (string, error) { return prime, nil },
		dirExists: func(path string) bool {
			_, ok := existing[filepath.Base(path)]
			return ok
		},
		resolveWorktree: func(root string) (*lymcwd.Location, error) {
			return &lymcwd.Location{HubPath: filepath.Dir(root), WorktreeName: filepath.Base(root)}, nil
		},
		seedRecipe: func(target *lymcwd.Location) (string, error) {
			return existing[target.WorktreeName], nil
		},
	}
}

func TestResolveReviewTarget(t *testing.T) {
	tiergate.Require(t, 0)
	hub := filepath.Join(t.TempDir(), "repo-LYMHUB")
	atPrime := &lymcwd.Location{HubPath: hub, WorktreeName: "repo"}
	atTask := &lymcwd.Location{HubPath: hub, WorktreeName: "task-a"}
	d := fakeReviewTarget("repo", map[string]string{
		"task-a":      "",
		"task-b":      "",
		"task-pawl":   shedrun.RecipePawl,
		"task-batten": shedrun.RecipeBatten,
		"task-beam":   "beam",
	})

	for _, group := range []string{"circling", "decision"} {
		t.Run(group, func(t *testing.T) {
			t.Run("task worktree addresses itself", func(t *testing.T) {
				got, err := resolveReviewTarget(group, "notify", atTask, "", d)
				if err != nil || got != atTask {
					t.Fatalf("got %v, %v; want the invoking location", got, err)
				}
			})
			t.Run("slug from the prime", func(t *testing.T) {
				got, err := resolveReviewTarget(group, "notify", atPrime, "task-b", d)
				if err != nil || got.WorktreeName != "task-b" {
					t.Fatalf("got %v, %v; want task-b", got, err)
				}
			})
			t.Run("a pawl target resolves", func(t *testing.T) {
				got, err := resolveReviewTarget(group, "notify", atPrime, "task-pawl", d)
				if err != nil || got.WorktreeName != "task-pawl" {
					t.Fatalf("got %v, %v; want task-pawl", got, err)
				}
			})
			t.Run("a batten target is refused naming the lym shed verbs", func(t *testing.T) {
				_, err := resolveReviewTarget(group, "notify", atPrime, "task-batten", d)
				if err == nil || !strings.Contains(err.Error(), "loom: "+group+" notify: run recipe \"batten\" is not a loom recipe") || !strings.Contains(err.Error(), "way forward:") || !strings.Contains(err.Error(), "lym shed") {
					t.Fatalf("err = %v; want the not-a-loom-recipe refusal naming %s and the lym shed verbs", err, group)
				}
			})
			t.Run("a target naming no embedded recipe is refused naming the re-seed", func(t *testing.T) {
				_, err := resolveReviewTarget(group, "notify", atPrime, "task-beam", d)
				if err == nil || !strings.Contains(err.Error(), "loom: "+group+" notify: run recipe \"beam\" names no recipe this lym knows") || !strings.Contains(err.Error(), "way forward:") || !strings.Contains(err.Error(), "seed the run again") {
					t.Fatalf("err = %v; want the unknown-recipe refusal naming %s and the re-seed", err, group)
				}
			})
			t.Run("slug required from the prime", func(t *testing.T) {
				_, err := resolveReviewTarget(group, "notify", atPrime, "", d)
				if err == nil || !strings.Contains(err.Error(), "loom: "+group+" notify: slug required from the prime") || !strings.Contains(err.Error(), "way forward:") || !strings.Contains(err.Error(), "lym board list") || !strings.Contains(err.Error(), "lym loom "+group+" notify <slug>") {
					t.Fatalf("err = %v; want the slug-required refusal naming %s and lym board list", err, group)
				}
			})
			t.Run("unknown slug", func(t *testing.T) {
				_, err := resolveReviewTarget(group, "notify", atTask, "nope", d)
				if err == nil || !strings.Contains(err.Error(), "loom: "+group+" notify: unknown slug \"nope\"") || !strings.Contains(err.Error(), "way forward:") || !strings.Contains(err.Error(), "lym board list") {
					t.Fatalf("err = %v; want the unknown-slug refusal naming %s and lym board list", err, group)
				}
			})
			t.Run("prime lookup failure", func(t *testing.T) {
				bad := d
				bad.primeName = func(*lymcwd.Location) (string, error) { return "", errors.New("boom") }
				if _, err := resolveReviewTarget(group, "notify", atTask, "", bad); err == nil {
					t.Fatal("err = nil; want the prime lookup failure")
				}
			})
			t.Run("a nil seed seam reads the target as tapestry", func(t *testing.T) {
				unseeded := d
				unseeded.seedRecipe = nil
				got, err := resolveReviewTarget(group, "notify", atPrime, "task-batten", unseeded)
				if err != nil || got.WorktreeName != "task-batten" {
					t.Fatalf("got %v, %v; want task-batten, since a nil seam judges no recipe", got, err)
				}
			})
		})
	}
}
