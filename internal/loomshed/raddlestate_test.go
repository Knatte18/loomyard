// raddlestate_test.go exercises the raddle run's state record and the one-time storing of the cold proposal.

package loomshed_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRaddleState(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("RoundTripsEveryField", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path, lockPath := filepath.Join(dir, "state.json"), filepath.Join(dir, "state.json.lock")

		absent, err := loomshed.ReadRaddleState(path, lockPath)
		if err != nil || !reflect.DeepEqual(absent, loomshed.RaddleState{}) {
			t.Fatalf("ReadRaddleState() of an absent file = %+v, %v; want the zero state and nil", absent, err)
		}

		want := loomshed.RaddleState{
			Mode: loomengine.RaddleModeInteractive, ColdStored: true, Session: 3, ProposedSession: 2, NoticeSession: 2,
			Approval: &loomshed.RaddleApprovalRecord{Hash: "abc", Session: 2}, Returns: 1, ReturnReason: "board moved",
			EntryPosition: 7, NoticeSent: true,
		}
		updated, err := loomshed.UpdateRaddleState(path, lockPath, func(stored *loomshed.RaddleState) { *stored = want })
		if err != nil || !reflect.DeepEqual(updated, want) {
			t.Fatalf("UpdateRaddleState() = %+v, %v; want %+v and nil", updated, err, want)
		}
		got, err := loomshed.ReadRaddleState(path, lockPath)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("ReadRaddleState() = %+v, %v; want %+v and nil", got, err, want)
		}
	})

	t.Run("KeepsAnUnknownKey", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path, lockPath := filepath.Join(dir, "state.json"), filepath.Join(dir, "state.json.lock")
		if err := os.WriteFile(path, []byte(`{"mode":"auto","future_key":7}`), 0o644); err != nil {
			t.Fatalf("write the state: %v", err)
		}

		got, err := loomshed.ReadRaddleState(path, lockPath)
		if err != nil || got.Mode != loomengine.RaddleModeAuto {
			t.Fatalf("ReadRaddleState() = %+v, %v; want mode auto and nil", got, err)
		}
		if _, err := loomshed.UpdateRaddleState(path, lockPath, func(stored *loomshed.RaddleState) { stored.Session++ }); err != nil {
			t.Fatalf("UpdateRaddleState() = %v; want nil", err)
		}
		data, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(data), "future_key") {
			t.Errorf("state after the update = %s, %v; want it to keep future_key", data, err)
		}
	})

	t.Run("StoreColdProposalStoresOnce", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		proposalPath, coldPath := filepath.Join(dir, "proposal.yaml"), filepath.Join(dir, "cold-proposal.yaml")
		path, lockPath := filepath.Join(dir, "state.json"), filepath.Join(dir, "state.json.lock")

		if err := os.WriteFile(proposalPath, []byte("first"), 0o644); err != nil {
			t.Fatalf("write the proposal: %v", err)
		}
		if err := loomshed.StoreColdProposal(proposalPath, coldPath, path, lockPath); err != nil {
			t.Fatalf("StoreColdProposal() first call = %v; want nil", err)
		}
		if err := os.WriteFile(proposalPath, []byte("second"), 0o644); err != nil {
			t.Fatalf("rewrite the proposal: %v", err)
		}
		if err := loomshed.StoreColdProposal(proposalPath, coldPath, path, lockPath); err != nil {
			t.Fatalf("StoreColdProposal() second call = %v; want nil", err)
		}

		cold, err := os.ReadFile(coldPath)
		if err != nil || string(cold) != "first" {
			t.Errorf("cold proposal = %q, %v; want %q left as the first call stored it", cold, err, "first")
		}
		got, err := loomshed.ReadRaddleState(path, lockPath)
		if err != nil || !got.ColdStored {
			t.Errorf("ReadRaddleState() = %+v, %v; want cold_stored set", got, err)
		}
	})

	t.Run("StoreColdProposalWithNoProposalIsAnErrorAndStoresNothing", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path, lockPath := filepath.Join(dir, "state.json"), filepath.Join(dir, "state.json.lock")
		coldPath := filepath.Join(dir, "cold-proposal.yaml")

		if err := loomshed.StoreColdProposal(filepath.Join(dir, "absent.yaml"), coldPath, path, lockPath); err == nil {
			t.Fatal("StoreColdProposal() with no proposal = nil; want an error")
		}
		got, err := loomshed.ReadRaddleState(path, lockPath)
		if err != nil || got.ColdStored {
			t.Errorf("ReadRaddleState() = %+v, %v; want cold_stored unset", got, err)
		}
		if _, err := os.Stat(coldPath); !os.IsNotExist(err) {
			t.Errorf("cold proposal stat = %v; want not-exist", err)
		}
	})
}
