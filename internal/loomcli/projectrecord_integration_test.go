// projectrecord_integration_test.go drives the beam-side decision seams and the project record commit over a forged hub.
// This package's own testmain_test.go arms the hermetic git test environment for the whole binary,
// so this file adds no TestMain.

package loomcli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	testBeam  = "the-beam"
	testShort = "tst"
)

// beamHubRegistration is the template of a hub holding the beam pair testBeam and two pairs forked from it.
// Its board file lists the beam, a member with a pair and a member with none.
// Its board holds the shortname testShort.
// Both board files stay uncommitted.
var beamHubRegistration = &hubforge.Registration{
	Key:  "loomcli-beam-hub",
	Base: hubforge.Shape{Anchor: "."},
	Build: func(tb testing.TB, hub *hubforge.Hub) {
		hubforge.AddPair(tb, hub, testBeam)
		hubforge.AddPairWith(tb, hub, "member-with-pair", fabricengine.AddOptions{Parent: testBeam})
		hubforge.AddPairWith(tb, hub, "forked-child", fabricengine.AddOptions{Parent: testBeam})

		// A board write cannot make a beam, so the entries are written to the board file directly.
		board := `{"version":1,"entries":[` +
			`{"id":1,"slug":"` + testBeam + `","title":"Beam","kind":"project","labels":["bug"],"beam":true},` +
			`{"id":2,"slug":"member-with-pair","title":"With pair","kind":"task","labels":["bug"],"project":"` + testBeam + `"},` +
			`{"id":3,"slug":"member-no-pair","title":"No pair","kind":"task","labels":["bug"],"project":"` + testBeam + `"}]}`
		if err := os.WriteFile(filepath.Join(hub.BoardDir(), "board.json"), []byte(board), 0o644); err != nil {
			tb.Fatalf("write board.json: %v", err)
		}
		if err := os.WriteFile(filepath.Join(hub.BoardDir(), fabricengine.ShortnameFileName), []byte(testShort+"\n"), 0o644); err != nil {
			tb.Fatalf("write the shortname record: %v", err)
		}
	},
}

// beamHub copies the beam hub template into the test's own temp directory.
// It returns the hub and a location lookup by worktree name.
func beamHub(t *testing.T) (hub *hubforge.Hub, location func(name string) *lymcwd.Location) {
	t.Helper()
	hub = hubforge.CopyRegistered(t, beamHubRegistration)
	location = func(name string) *lymcwd.Location {
		t.Helper()
		loc, err := lymcwd.ResolveWorktree(hub.PairCodeWorktree(name))
		if err != nil {
			t.Fatalf("ResolveWorktree(%s) error = %v; want nil", name, err)
		}
		return loc
	}
	return hub, location
}

func TestMemberPairs_ReturnsEveryLocalPairUnderTheBeam(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	_, location := beamHub(t)

	pairs, err := memberPairs(location(testBeam), testBeam)
	if err != nil {
		t.Fatalf("memberPairs(%s) error = %v; want nil", testBeam, err)
	}
	var got []string
	for _, pair := range pairs {
		got = append(got, pair.WorktreeName)
	}
	slices.Sort(got)
	if want := []string{"forked-child", "member-with-pair"}; !slices.Equal(got, want) {
		t.Errorf("memberPairs(%s) = %v; want %v, the member with a pair and the forked pair, and not the member with no pair", testBeam, got, want)
	}
}

func TestDecisionReceivers_ReachABeamsMemberPairs(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	_, location := beamHub(t)
	beam := location(testBeam)
	strandsOf := func(worktree *lymcwd.Location) ([]string, error) {
		return []string{"strand-of-" + worktree.WorktreeName}, nil
	}

	tests := []struct {
		name   string
		record runRecord
		want   []string
	}{
		{"AProjectRecordReachesTheBeamAndEveryMemberPair", runRecord{project: true}, []string{"strand-of-forked-child", "strand-of-member-with-pair", "strand-of-" + testBeam}},
		{"ADiscussionRecordReachesOnlyTheTargetsOwnRuns", runRecord{}, []string{"strand-of-" + testBeam}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := decisionReceivers(beam, tt.record, strandsOf)
			if err != nil {
				t.Fatalf("decisionReceivers() error = %v; want nil", err)
			}
			slices.Sort(got)
			if !slices.Equal(got, tt.want) {
				t.Errorf("decisionReceivers() = %v; want %v", got, tt.want)
			}
		})
	}
}

func TestDecisionParent_ABeamsRecordIsItsOwnOrchs(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	_, location := beamHub(t)
	beam := location(testBeam)
	beamOrch, err := agentname.Format(testShort, testBeam, agentname.RoleOrch)
	if err != nil {
		t.Fatalf("Format() error = %v; want nil", err)
	}

	t.Run("TheBeamsOwnLiveOrchIsTheParent", func(t *testing.T) {
		t.Parallel()
		got, err := decisionParent(beam, runRecord{project: true}, func(*lymcwd.Location) (bool, error) { return true, nil })
		if err != nil || got.Name != beamOrch {
			t.Errorf("decisionParent() = %q, %v; want the beam's own orch %q", got.Name, err, beamOrch)
		}
	})

	t.Run("AHubWithNoShortnameIsRefusedNamingTheWayForward", func(t *testing.T) {
		t.Parallel()
		bareHub, bare := beamHub(t)
		if err := os.Remove(filepath.Join(bareHub.BoardDir(), fabricengine.ShortnameFileName)); err != nil {
			t.Fatalf("remove the shortname record: %v", err)
		}
		_, err := decisionParent(bare(testBeam), runRecord{project: true}, func(*lymcwd.Location) (bool, error) { return true, nil })
		if err == nil || !strings.Contains(err.Error(), "lym fabric shortname") {
			t.Errorf("decisionParent() error = %v; want a refusal naming `lym fabric shortname`", err)
		}
	})
}

// TestCommitRunRecord_ProjectRecordCommitsItsDirectory pins that a project record commit stages the project directory:
// its record reads back from HEAD, and the discussion directory is not staged.
func TestCommitRunRecord_ProjectRecordCommitsItsDirectory(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	_, location := beamHub(t)
	beam := location(testBeam)
	record := runRecordOf(beam, "pawl")
	if !record.project {
		t.Fatalf("runRecordOf(%s, pawl).project = false; want a project record", testBeam)
	}
	for path, content := range map[string]string{record.path: "## Goal\n", loomengine.DiscussionDecisionRecord(beam): "# discussion\n"} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v; want nil", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v; want nil", path, err)
		}
	}

	if err := commitRunRecord(beam, record); err != nil {
		t.Fatalf("commitRunRecord() error = %v; want nil", err)
	}

	got, found, err := fabricengine.CommittedAnchoredFile(beam, record.rel)
	if err != nil || !found || string(got) != "## Goal\n" {
		t.Errorf("CommittedAnchoredFile(%s) = %q, %v, %v; want the project record committed", record.rel, got, found, err)
	}
	if _, found, err := fabricengine.CommittedAnchoredFile(beam, loomengine.DiscussionDecisionRecordRel()); err != nil || found {
		t.Errorf("CommittedAnchoredFile(discussion record) found = %v, %v; want the discussion record left uncommitted", found, err)
	}
}
