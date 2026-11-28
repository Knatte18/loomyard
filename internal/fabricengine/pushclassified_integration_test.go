// pushclassified_integration_test.go covers what one side's push reads of a real hub before and after it pushes.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccore"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestPushClassified_Snapshot(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("RecordsSideWithAnUpstreamReadsOriginBeforeItPushes", func(t *testing.T) {
		t.Parallel()
		p := newPairFixture(t, "snapshotpair")
		gitkit.CommitFile(t, p.weftPath, "local.txt", "local\n", "local")

		snapshot, err := fabricengine.PushClassifiedSnapshotForTest(p.hub.Location.HubPath, "records", p.weftPath, p.fake.options(nil))
		if err != nil {
			t.Fatalf("push error = %v; want nil", err)
		}

		facts := snapshot.Push
		if facts.Side != "records" || facts.Branch != p.weftBranch || facts.Prime || facts.Upstream != fabriccore.PushUpstreamTracked ||
			!facts.Origin.Read || facts.Origin.Gone || !facts.UnpushedRead || !facts.Unpushed || facts.Judgment.Read {
			t.Errorf("push facts = %+v; want a tracked records branch %s that origin holds, with unpushed commits and no judgment", facts, p.weftBranch)
		}
		if want := []fabriccore.Phase{fabriccore.PhaseUnderLock, fabriccore.PhaseOrigin}; !slices.Equal(snapshot.Phases, want) {
			t.Errorf("phases = %v; want %v", snapshot.Phases, want)
		}
		if got, want := bareBranchTip(t, p.hub.RecordsBare, p.weftBranch), gitkit.RevParse(t, p.weftPath, "HEAD"); got != want {
			t.Errorf("origin tip = %s; want the local HEAD %s", got, want)
		}
		requireNoFailedPush(t, p.hub.Location.HubPath)
	})

	t.Run("CodeBranchWithNoUpstreamReadsNoOrigin", func(t *testing.T) {
		t.Parallel()
		f, fake := newFailureFixture(t)
		prime := f.hub.PrimeWorktree()
		gitkit.Git(t, prime, "checkout", "-b", "fresh")
		local := gitkit.CommitFile(t, prime, "local.txt", "local\n", "local")

		snapshot, err := fabricengine.PushClassifiedSnapshotForTest(f.hub.Location.HubPath, "code", prime, fake.options(nil))
		if err != nil {
			t.Fatalf("push error = %v; want nil", err)
		}

		if want := []fabriccore.Phase{fabriccore.PhaseUnderLock}; !slices.Equal(snapshot.Phases, want) {
			t.Errorf("phases = %v; want %v, with no origin read", snapshot.Phases, want)
		}
		if snapshot.Push.Upstream != fabriccore.PushUpstreamNone || snapshot.Push.Origin.Read || snapshot.Push.Judgment.Read {
			t.Errorf("push facts = %+v; want no upstream, no lookup and no judgment", snapshot.Push)
		}
		if got := bareBranchTip(t, f.hub.CodeBare, "fresh"); got != local {
			t.Errorf("origin tip = %s; want the local commit %s", got, local)
		}
		requireNoFailedPush(t, f.hub.Location.HubPath)
	})
}
