//go:build tier0

package loomcli

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// fakeReviewTarget builds reviewTargetDeps over a hub with the given worktrees.
func fakeReviewTarget(prime string, existing ...string) reviewTargetDeps {
	return reviewTargetDeps{
		primeName: func(*lymcwd.Location) (string, error) { return prime, nil },
		dirExists: func(path string) bool {
			for _, e := range existing {
				if filepath.Base(path) == e {
					return true
				}
			}
			return false
		},
		resolveWorktree: func(root string) (*lymcwd.Location, error) {
			return &lymcwd.Location{HubPath: filepath.Dir(root), WorktreeName: filepath.Base(root)}, nil
		},
	}
}

func TestResolveReviewTarget(t *testing.T) {
	hub := filepath.Join(t.TempDir(), "repo-LYMHUB")
	atPrime := &lymcwd.Location{HubPath: hub, WorktreeName: "repo"}
	atTask := &lymcwd.Location{HubPath: hub, WorktreeName: "task-a"}
	d := fakeReviewTarget("repo", "task-a", "task-b")

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
		})
	}
}
