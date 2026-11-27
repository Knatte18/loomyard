// removethrough_test.go pins what a teardown request hands the pair's removal.

package pairteardown

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// recordingRemover is a pairRemover that records the arguments of its origin removal.
type recordingRemover struct {
	slug          string
	force, remote bool
	rerun         string
}

func (r *recordingRemover) Remove(_ *lymcwd.Location, slug string, force, remote bool, rerun string) (fabriclayout.RemoveResult, error) {
	r.slug, r.force, r.remote, r.rerun = slug, force, remote, rerun
	return fabriclayout.RemoveResult{}, nil
}

func (r *recordingRemover) RemoveLocal(*lymcwd.Location, string, bool) (fabriclayout.RemoveResult, error) {
	return fabriclayout.RemoveResult{}, nil
}

func TestRemoveThrough_HandsTheRequestToTheOriginRemoval(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	got := &recordingRemover{}
	req := Request{Slug: "slug", Force: true, Remote: true, Rerun: "lym batten cancel slug --remove"}

	if _, err := removeThrough(got, nil, req); err != nil {
		t.Fatalf("removeThrough() error = %v; want nil", err)
	}

	want := recordingRemover{slug: "slug", force: true, remote: true, rerun: "lym batten cancel slug --remove"}
	if *got != want {
		t.Errorf("removeThrough() asked for %+v; want %+v", *got, want)
	}
}
