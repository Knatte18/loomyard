//go:build tier1

// markbeam_integration_test.go covers MarkBeam over a hubforge hub: the mark and its one commit, the idempotent re-run, and the two refusals for a pair with no origin record.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// recordsCommitCount returns the number of commits reachable from HEAD in the records worktree at weftPath.
func recordsCommitCount(t *testing.T, weftPath string) int {
	t.Helper()

	out, err := gitexec.Run([]string{"rev-list", "--count", "HEAD"}, weftPath)
	if err != nil {
		t.Fatalf("git rev-list --count HEAD in %s: %v", weftPath, err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		t.Fatalf("parse commit count %q: %v", out, err)
	}
	return n
}

func TestMarkBeam(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location

	t.Run("marks the record once and commits that one path", func(t *testing.T) {
		const slug = "mark-once"
		hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
		weftPath := fabricengine.RecordsWorktreePath(l, slug)
		before := recordsCommitCount(t, weftPath)
		recorded, found, err := fabricengine.ReadOriginFor(l, slug)
		if err != nil || !found {
			t.Fatalf("ReadOriginFor(%q) = (%+v, %v, %v); want a record", slug, recorded, found, err)
		}

		marked, err := fabricengine.MarkBeam(fabricengine.NewMutations(l.HubPath), l, slug)
		if err != nil || !marked {
			t.Fatalf("MarkBeam(%q) = (%v, %v); want (true, nil)", slug, marked, err)
		}

		got, found, err := fabricengine.ReadOriginFor(l, slug)
		if err != nil || !found {
			t.Fatalf("ReadOriginFor(%q) after the mark = (%+v, %v, %v); want a record", slug, got, found, err)
		}
		want := recorded
		want.Beam = true
		if got != want {
			t.Errorf("origin record after the mark = %+v; want %+v", got, want)
		}
		if after := recordsCommitCount(t, weftPath); after != before+1 {
			t.Errorf("records commit count = %d; want %d", after, before+1)
		}
		subject, err := gitexec.Run([]string{"log", "-1", "--format=%s"}, weftPath)
		if err != nil {
			t.Fatalf("git log -1 in %s: %v", weftPath, err)
		}
		if want := "fabric: mark beam " + slug; strings.TrimSpace(subject) != want {
			t.Errorf("mark commit subject = %q; want %q", strings.TrimSpace(subject), want)
		}

		again, err := fabricengine.MarkBeam(fabricengine.NewMutations(l.HubPath), l, slug)
		if err != nil || again {
			t.Errorf("second MarkBeam(%q) = (%v, %v); want (false, nil)", slug, again, err)
		}
		if after := recordsCommitCount(t, weftPath); after != before+1 {
			t.Errorf("records commit count after the re-run = %d; want %d", after, before+1)
		}
	})

	t.Run("refuses a pair whose record file is gone from a present records worktree", func(t *testing.T) {
		const slug = "mark-no-record"
		hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
		weftPath := fabricengine.RecordsWorktreePath(l, slug)
		if err := os.Remove(fabricengine.OriginRecordPathFor(l, slug)); err != nil {
			t.Fatalf("remove the origin record: %v", err)
		}
		before := recordsCommitCount(t, weftPath)

		marked, err := fabricengine.MarkBeam(fabricengine.NewMutations(l.HubPath), l, slug)
		if err == nil || marked {
			t.Fatalf("MarkBeam(%q) = (%v, %v); want a refusal", slug, marked, err)
		}
		recordPath := filepath.ToSlash(filepath.Join(l.AnchorRel, fabricengine.OriginRecordRel()))
		for _, want := range []string{slug, "git checkout HEAD -- " + recordPath} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("MarkBeam(%q) error = %v; want it to contain %q", slug, err, want)
			}
		}
		if after := recordsCommitCount(t, weftPath); after != before {
			t.Errorf("records commit count = %d; want %d (no commit)", after, before)
		}
	})

	t.Run("refuses a pair whose records worktree is gone", func(t *testing.T) {
		const slug = "mark-no-worktree"
		hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
		weftPath := fabricengine.RecordsWorktreePath(l, slug)
		if err := os.RemoveAll(weftPath); err != nil {
			t.Fatalf("remove the records worktree: %v", err)
		}

		marked, err := fabricengine.MarkBeam(fabricengine.NewMutations(l.HubPath), l, slug)
		if err == nil || marked {
			t.Fatalf("MarkBeam(%q) = (%v, %v); want a refusal", slug, marked, err)
		}
		for _, want := range []string{slug, "lyx fabric reconcile"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("MarkBeam(%q) error = %v; want it to contain %q", slug, err, want)
			}
		}
		if _, statErr := os.Stat(weftPath); statErr == nil {
			t.Errorf("MarkBeam(%q) recreated %s; want no commit and no worktree", slug, weftPath)
		}
	})
}
