// publishretry_test.go covers the mapping of a task worktree's Publish failure record into the retry's seam type.

package battencli

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// TestReadChildPublishFailure_MapsTheRecord asserts readChildPublishFailure maps the record's head, target and write time, and reports found false for a task worktree with no record.
func TestReadChildPublishFailure_MapsTheRecord(t *testing.T) {
	t.Parallel()

	taskLocation := &lyxcwd.Location{RepoName: "example", HubPath: t.TempDir(), WorktreeName: "a-slug", AnchorRel: "."}

	if _, found, err := readChildPublishFailure(taskLocation); err != nil || found {
		t.Fatalf("readChildPublishFailure(no record) = found %v, err %v; want false, nil", found, err)
	}

	paths := verifytree.NewPaths(taskLocation.WorktreePath(), verifytree.Dir(taskLocation.AnchorPath()))
	if err := os.MkdirAll(verifytree.Dir(taskLocation.AnchorPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	written := verifytree.PublishFailure{Head: "abc123", Target: "Webster", WrittenAt: "2026-01-01T11:20:00Z", LogPath: "verify-1.log"}
	if err := verifytree.WritePublishFailure(paths, written); err != nil {
		t.Fatal(err)
	}

	got, found, err := readChildPublishFailure(taskLocation)
	want := battenshed.ChildPublishFailure{Head: "abc123", Target: "Webster", WrittenAt: "2026-01-01T11:20:00Z"}
	if err != nil || !found || got != want {
		t.Errorf("readChildPublishFailure() = %+v, %v, %v; want %+v, true, nil", got, found, err, want)
	}
}
