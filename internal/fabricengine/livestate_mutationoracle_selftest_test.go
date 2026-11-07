// Self-tests for AssertRecordMatchesDiff against hand-built Mutations and Change values, with no hub, git or verb.
// They cover each rule the oracle's doc comments state, including the omission and commission directions.

package fabricengine_test

import (
	"fmt"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// failureRecorder embeds a real testing.TB, which satisfies its unexported methods by delegation.
// It intercepts Error, Errorf, Fatal and Fatalf, so a case can assert a failure without failing the enclosing test.
type failureRecorder struct {
	testing.TB
	failed   bool
	messages []string
}

// Helper is a no-op: the recorder is the assertion target itself, not a helper whose caller should be blamed.
func (f *failureRecorder) Helper() {}

// Error implements testing.TB by recording the failure rather than failing the embedded TB.
func (f *failureRecorder) Error(args ...any) {
	f.failed = true
	f.messages = append(f.messages, fmt.Sprint(args...))
}

// Errorf implements testing.TB by recording the failure rather than failing the embedded TB.
func (f *failureRecorder) Errorf(format string, args ...any) {
	f.failed = true
	f.messages = append(f.messages, fmt.Sprintf(format, args...))
}

// Fatal implements testing.TB by recording the failure rather than failing the embedded TB.
func (f *failureRecorder) Fatal(args ...any) {
	f.failed = true
	f.messages = append(f.messages, fmt.Sprint(args...))
}

// Fatalf implements testing.TB by recording the failure instead of failing the embedded, real TB.
func (f *failureRecorder) Fatalf(format string, args ...any) {
	f.failed = true
	f.messages = append(f.messages, fmt.Sprintf(format, args...))
}

// buildRecord constructs a fabricengine.Mutations value from entries in order.
// It records through a recorder with an empty hub root, so each Target is kept exactly as written:
// Append's hub-relative conversion does nothing when the hub root is empty.
// A case can therefore spell a Target as the bare hub-relative string the oracle operates on.
func buildRecord(entries ...fabricengine.Mutation) fabricengine.Mutations {
	m := fabricengine.NewMutations("")
	for _, e := range entries {
		m.Append(e.Kind, e.Target, e.Detail)
	}
	return m.Snapshot()
}

// mutation is a short constructor for a detail-less fabricengine.Mutation literal, for table readability.
func mutation(kind fabricengine.Kind, target string) fabricengine.Mutation {
	return fabricengine.Mutation{Kind: kind, Target: target}
}

// mutationWithDetail builds a fabricengine.Mutation that carries a Detail.
// For a path_removed, the Detail is "recursive" or "single", which decides whether the removal destroys a whole subtree.
func mutationWithDetail(kind fabricengine.Kind, target, detail string) fabricengine.Mutation {
	return fabricengine.Mutation{Kind: kind, Target: target, Detail: detail}
}

// addedChange returns a ChangeAdded Change at path with the given entry kind, for a hand-built unfiltered diff.
func addedChange(path string, kind Kind) Change {
	e := Entry{Kind: kind}
	return Change{Path: path, Kind: ChangeAdded, After: &e}
}

// removedChange returns a ChangeRemoved Change at path with the given entry kind.
func removedChange(path string, kind Kind) Change {
	e := Entry{Kind: kind}
	return Change{Path: path, Kind: ChangeRemoved, Before: &e}
}

// TestAssertRecordMatchesDiff drives AssertRecordMatchesDiff with hand-built Mutations and Change values.
// It has one case per rule the oracle carries, and each case records failures through failureRecorder.
func TestAssertRecordMatchesDiff(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name       string
		rec        fabricengine.Mutations
		unfiltered []Change
		wantFail   bool
	}{
		{
			name:       "OmissionFires_UncoveredDiffChange",
			rec:        buildRecord(mutation(fabricengine.KindDirCreated, "unrelated")),
			unfiltered: []Change{addedChange("some/path", KindFile)},
			wantFail:   true,
		},
		{
			name:       "CommissionFires_UnbackedManifestObservableEntry",
			rec:        buildRecord(mutation(fabricengine.KindWorktreeCreated, "myslug")),
			unfiltered: nil,
			wantFail:   true,
		},
		{
			name:       "SegmentWiseContainment_DescendantCovered",
			rec:        buildRecord(mutation(fabricengine.KindWorktreeCreated, "_portals/x")),
			unfiltered: []Change{addedChange("_portals/x/file.txt", KindFile)},
			wantFail:   false,
		},
		{
			// "_portalsfoo/y" shares a name prefix with the "_portals/x" root but is not a segment-wise descendant of it.
			// The oracle must not treat it as one.
			name: "SegmentWiseContainment_SiblingPrefixNotCovered",
			rec:  buildRecord(mutation(fabricengine.KindWorktreeCreated, "_portals/x")),
			unfiltered: []Change{
				addedChange("_portals/x/file.txt", KindFile),
				addedChange("_portalsfoo/y", KindFile),
			},
			wantFail: true,
		},
		{
			name:       "WorktreeAdmin_WarpSideCovered",
			rec:        buildRecord(mutation(fabricengine.KindWorktreeRemoved, "myslug")),
			unfiltered: []Change{removedChange("warp-bare/.git/worktrees/myslug", KindDir)},
			wantFail:   false,
		},
		{
			name:       "WorktreeAdmin_WarpSideNameMismatchUncovered",
			rec:        buildRecord(mutation(fabricengine.KindWorktreeRemoved, "myslug")),
			unfiltered: []Change{removedChange("warp-bare/.git/worktrees/othername", KindDir)},
			wantFail:   true,
		},
		{
			name:       "WorktreeAdmin_WeftSideCovered",
			rec:        buildRecord(mutation(fabricengine.KindWorktreeRemoved, weftname.SiblingPath("", "myslug"))),
			unfiltered: []Change{removedChange("warp-bare-weft/.git/worktrees/myslug-weft", KindDir)},
			wantFail:   false,
		},
		{
			// A derivation that feeds a weft-suffixed target into the warp path looks in the wrong place:
			// it finds the admin entry under "warp-bare", not "warp-bare-weft".
			// This case proves that wrong location does not cover a weft-side worktree_removed.
			name:       "WorktreeAdmin_WeftSideNaiveWarpDerivationUncovered",
			rec:        buildRecord(mutation(fabricengine.KindWorktreeRemoved, weftname.SiblingPath("", "myslug"))),
			unfiltered: []Change{removedChange("warp-bare/.git/worktrees/myslug-weft", KindDir)},
			wantFail:   true,
		},
		{
			name:       "GitStateKind_BranchCreated_NoDiffPasses",
			rec:        buildRecord(mutation(fabricengine.KindBranchCreated, "myslug")),
			unfiltered: nil,
			wantFail:   false,
		},
		{
			name:       "GitStateKind_CommitCreated_NoDiffPasses",
			rec:        buildRecord(mutation(fabricengine.KindCommitCreated, fabricengine.BoltDirName)),
			unfiltered: nil,
			wantFail:   false,
		},
		{
			name:       "GitStateKind_WorktreeReset_NoDiffPasses",
			rec:        buildRecord(mutation(fabricengine.KindWorktreeReset, "warp-bare")),
			unfiltered: nil,
			wantFail:   false,
		},
		{
			name:       "GitStateKind_PushSpawned_NoDiffPasses",
			rec:        buildRecord(mutation(fabricengine.KindPushSpawned, "myslug")),
			unfiltered: nil,
			wantFail:   false,
		},
		{
			name:       "GitStateKind_RepoAdvanced_NoDiffPasses",
			rec:        buildRecord(mutation(fabricengine.KindRepoAdvanced, "warp-bare")),
			unfiltered: nil,
			wantFail:   false,
		},
		{
			name:       "FileWritten_UnderGitExempt",
			rec:        buildRecord(mutation(fabricengine.KindFileWritten, "warp-bare/.git/info/exclude")),
			unfiltered: nil,
			wantFail:   false,
		},
		{
			name:       "FileWritten_OutsideGitNotExempt",
			rec:        buildRecord(mutation(fabricengine.KindFileWritten, "_lym/config/fabric.yaml")),
			unfiltered: nil,
			wantFail:   true,
		},
		{
			name: "NetCommission_DirCreatedInvertedByLaterPathRemoved_Exempt",
			rec: buildRecord(
				mutation(fabricengine.KindDirCreated, "x"),
				mutationWithDetail(fabricengine.KindPathRemoved, "x", "recursive"),
			),
			unfiltered: nil,
			wantFail:   false,
		},
		{
			// The rule is LATER inverts EARLIER:
			// a path_removed that precedes a dir_created at the same Target does not exempt it.
			name: "NetCommission_ReverseOrder_LaterInvertsEarlierOnly_NotExempt",
			rec: buildRecord(
				mutationWithDetail(fabricengine.KindPathRemoved, "x", "recursive"),
				mutation(fabricengine.KindDirCreated, "x"),
			),
			unfiltered: nil,
			wantFail:   true,
		},
		{
			name: "NetCommission_LinkCreatedInvertedByLaterLinkRemoved_Exempt",
			rec: buildRecord(
				mutation(fabricengine.KindLinkCreated, "y"),
				mutation(fabricengine.KindLinkRemoved, "y"),
			),
			unfiltered: nil,
			wantFail:   false,
		},
		{
			name: "NetCommission_LinkReverseOrder_NotExempt",
			rec: buildRecord(
				mutation(fabricengine.KindLinkRemoved, "y"),
				mutation(fabricengine.KindLinkCreated, "y"),
			),
			unfiltered: nil,
			wantFail:   true,
		},
		{
			name:       "DotTarget_ExemptFromCommissionRegardlessOfKind",
			rec:        buildRecord(mutation(fabricengine.KindLinkCreated, ".")),
			unfiltered: nil,
			wantFail:   false,
		},
		{
			name:       "DotTarget_DirCreatedDoesNotSatisfyOmission",
			rec:        buildRecord(mutation(fabricengine.KindDirCreated, ".")),
			unfiltered: []Change{addedChange("something/else", KindFile)},
			wantFail:   true,
		},
		{
			name:       "DotTarget_PathRemovedSatisfiesOmission",
			rec:        buildRecord(mutation(fabricengine.KindPathRemoved, ".")),
			unfiltered: []Change{addedChange("something/else", KindFile)},
			wantFail:   false,
		},
		{
			name: "AncestorCoverage_KindDirCovered",
			rec:  buildRecord(mutation(fabricengine.KindWorktreeCreated, "a/b/c")),
			unfiltered: []Change{
				addedChange("a/b/c/file.txt", KindFile),
				removedChange("a/b", KindDir),
			},
			wantFail: false,
		},
		{
			// A KindFile change beside a recorded target stays uncovered:
			// the change is never taken as an implicitly-created ancestor directory.
			name: "AncestorCoverage_KindFileNotCovered",
			rec:  buildRecord(mutation(fabricengine.KindWorktreeCreated, "a/b/c")),
			unfiltered: []Change{
				addedChange("a/b/c/file.txt", KindFile),
				removedChange("a/b", KindFile),
			},
			wantFail: true,
		},
		{
			name:       "EmptyRecordEmptyDiff_Passes",
			rec:        buildRecord(),
			unfiltered: nil,
			wantFail:   false,
		},
		{
			name:       "EmptyRecordNonEmptyDiff_Fails",
			rec:        buildRecord(),
			unfiltered: []Change{addedChange("some/path", KindFile)},
			wantFail:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &failureRecorder{TB: t}
			AssertRecordMatchesDiff(rec, tt.rec, tt.unfiltered)
			if rec.failed != tt.wantFail {
				t.Errorf("AssertRecordMatchesDiff() failed = %v; want %v (messages: %v)", rec.failed, tt.wantFail, rec.messages)
			}
		})
	}
}
