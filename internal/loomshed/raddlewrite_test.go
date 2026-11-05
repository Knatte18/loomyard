// raddlewrite_test.go exercises the Raddle-Write producer over a stub inner producer and a run's state and files in a temp directory.

package loomshed

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// raddleInner is the stub session producer: it records the session the state held when it ran and answers the scripted verdict.
type raddleInner struct {
	outcome  shedengine.Outcome
	err      error
	fixture  *raddleWriteFixture
	sessions []int
}

func (r *raddleInner) Call(context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	stored, err := ReadRaddleState(r.fixture.deps.StatePath, r.fixture.deps.StateLockPath)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	r.sessions = append(r.sessions, stored.Session)
	return r.outcome, shedengine.OutputPointer{Path: r.fixture.deps.ProposalPath}, r.err
}

// raddleWriteFixture is a Raddle-Write producer over files in a temp directory and recording seams.
type raddleWriteFixture struct {
	deps             RaddleWriteDeps
	inner            *raddleInner
	commits          commitRecorder
	autoApprove      bool
	autoApproveErr   error
	autoApproveCalls int
	fresh            bool
	position         int
}

func newRaddleWriteFixture(t *testing.T) *raddleWriteFixture {
	t.Helper()
	dir := t.TempDir()
	f := &raddleWriteFixture{}
	f.inner = &raddleInner{outcome: shedengine.Done, fixture: f}
	f.deps = RaddleWriteDeps{
		StatePath:     filepath.Join(dir, "state.json"),
		StateLockPath: filepath.Join(dir, "state.json.lock"),
		ProposalPath:  filepath.Join(dir, "proposal.yaml"),
		ReturnedPath:  filepath.Join(dir, "returned-proposal.yaml"),
		ColdPath:      filepath.Join(dir, "cold-proposal.yaml"),
		ApprovalPath:  filepath.Join(dir, "approval.json"),
		AutoApprove: func() (bool, error) {
			f.autoApproveCalls++
			return f.autoApprove, f.autoApproveErr
		},
		FreshEntry: func() (bool, int, error) { return f.fresh, f.position, nil },
		StoreCold:  func() error { return nil },
		Commit:     f.commits.Commit,
	}
	return f
}

func (f *raddleWriteFixture) call() (shedengine.Outcome, error) {
	outcome, _, err := NewRaddleWrite(NameDarnWrite, f.inner, f.deps).Call(context.Background())
	return outcome, err
}

func (f *raddleWriteFixture) state(t *testing.T) RaddleState {
	t.Helper()
	stored, err := ReadRaddleState(f.deps.StatePath, f.deps.StateLockPath)
	if err != nil {
		t.Fatalf("ReadRaddleState() = %v; want nil", err)
	}
	return stored
}

func (f *raddleWriteFixture) write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestRaddleWrite(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("RecordsTheModeAtTheFirstCallOnlyAndRaisesTheSessionAtEveryCall", func(t *testing.T) {
		t.Parallel()
		for _, tt := range []struct {
			name string
			auto bool
			want RaddleMode
		}{
			{"KeyOffIsInteractive", false, RaddleModeInteractive},
			{"KeyOnIsAuto", true, RaddleModeAuto},
		} {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newRaddleWriteFixture(t)
				f.autoApprove = tt.auto
				if outcome, err := f.call(); err != nil || outcome != shedengine.Done {
					t.Fatalf("first Call() = %q, %v; want Done", outcome, err)
				}
				if got := f.state(t); got.Mode != tt.want || got.Session != 1 {
					t.Errorf("state after the first call = %+v; want mode %q and session 1", got, tt.want)
				}

				// A key changed after the first spawn changes no mode of the run.
				f.autoApprove = !tt.auto
				f.call()
				f.call()
				if got := f.state(t); got.Mode != tt.want || got.Session != 3 {
					t.Errorf("state after three calls = %+v; want mode %q kept and session 3", got, tt.want)
				}
				if f.autoApproveCalls != 1 {
					t.Errorf("AutoApprove calls = %d; want 1, the key is read once", f.autoApproveCalls)
				}
				if want := []int{1, 2, 3}; !slices.Equal(f.inner.sessions, want) {
					t.Errorf("sessions the inner producer saw = %v; want %v, raised before it runs", f.inner.sessions, want)
				}
			})
		}
	})

	t.Run("AnAutoApproveErrorIsReturnedAndRecordsNothing", func(t *testing.T) {
		t.Parallel()
		f := newRaddleWriteFixture(t)
		f.autoApproveErr = errors.New("setting unreadable")

		_, err := f.call()
		if err == nil || !strings.Contains(err.Error(), "setting unreadable") || !strings.Contains(err.Error(), "re-step") {
			t.Fatalf("Call() error = %v; want the fault with a re-step way forward", err)
		}
		if got := f.state(t); got != (RaddleState{}) {
			t.Errorf("state after the error = %+v; want nothing recorded", got)
		}
		if len(f.inner.sessions) != 0 {
			t.Errorf("inner producer ran %d times; want 0", len(f.inner.sessions))
		}
	})

	t.Run("AFreshEntryStartsANewSessionOncePerHistoryPosition", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name          string
			auto          bool
			approvalState bool
			returnReason  string
		}{
			{name: "ReturnInAutoMode", auto: true, returnReason: "the board moved"},
			{name: "ReturnInInteractiveMode", approvalState: true, returnReason: "the board moved"},
			{name: "GotoWithAnApprovalRecordPresent"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newRaddleWriteFixture(t)
				f.autoApprove = tt.auto
				f.call()
				f.write(t, f.deps.ProposalPath, "proposal")
				f.write(t, f.deps.ApprovalPath, `{"hash":"h","session":1}`)
				if _, err := UpdateRaddleState(f.deps.StatePath, f.deps.StateLockPath, func(stored *RaddleState) {
					stored.ReturnReason = tt.returnReason
					if tt.approvalState {
						stored.Approval = &RaddleApprovalRecord{Hash: "h", Session: 1}
					}
				}); err != nil {
					t.Fatalf("UpdateRaddleState() = %v; want nil", err)
				}
				f.fresh, f.position = true, 4

				f.call()
				got := f.state(t)
				if got.EntryPosition != 4 || got.Approval != nil {
					t.Errorf("state after the fresh entry = %+v; want position 4 and no approval", got)
				}
				if tt.auto {
					if exists(f.deps.ProposalPath) {
						t.Error("proposal still in place; want it moved to the set-aside path")
					}
					if aside, err := os.ReadFile(f.deps.ReturnedPath); err != nil || string(aside) != "proposal" {
						t.Errorf("set-aside proposal = %q, %v; want the proposal", aside, err)
					}
				} else {
					if exists(f.deps.ApprovalPath) {
						t.Error("approval record still in place; want it removed")
					}
					if !exists(f.deps.ProposalPath) {
						t.Error("proposal removed; want it kept in interactive mode")
					}
				}

				// The resume of the new session at the same position clears nothing again.
				f.write(t, f.deps.ProposalPath, "revised")
				f.write(t, f.deps.ApprovalPath, `{"hash":"r","session":2}`)
				f.call()
				if !exists(f.deps.ProposalPath) || !exists(f.deps.ApprovalPath) {
					t.Errorf("proposal exists = %v, approval exists = %v after the resume; want both left in place", exists(f.deps.ProposalPath), exists(f.deps.ApprovalPath))
				}
			})
		}
	})

	t.Run("DoneClearsTheReturnReasonAndCommitsOnce", func(t *testing.T) {
		t.Parallel()
		f := newRaddleWriteFixture(t)
		f.call()
		if _, err := UpdateRaddleState(f.deps.StatePath, f.deps.StateLockPath, func(stored *RaddleState) { stored.ReturnReason = "the board moved" }); err != nil {
			t.Fatalf("UpdateRaddleState() = %v; want nil", err)
		}
		commitsBefore := f.commits.calls

		if outcome, err := f.call(); err != nil || outcome != shedengine.Done {
			t.Fatalf("Call() = %q, %v; want Done", outcome, err)
		}
		if f.commits.calls != commitsBefore+1 {
			t.Errorf("commits = %d; want one more than %d", f.commits.calls, commitsBefore)
		}
		if got := f.state(t); got.ReturnReason != "" {
			t.Errorf("return reason after Done = %q; want it cleared", got.ReturnReason)
		}
	})

	t.Run("WithoutADoneVerdictNothingIsCommitted", func(t *testing.T) {
		t.Parallel()
		innerFault := errors.New("session failed")
		for _, tt := range []struct {
			name    string
			outcome shedengine.Outcome
			err     error
			want    shedengine.Outcome
		}{
			{"InnerError", "", innerFault, ""},
			{"InnerStuck", shedengine.Stuck, nil, shedengine.Stuck},
		} {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newRaddleWriteFixture(t)
				f.inner.outcome, f.inner.err = tt.outcome, tt.err

				outcome, err := f.call()
				if !errors.Is(err, tt.err) || outcome != tt.want {
					t.Errorf("Call() = %q, %v; want %q, %v", outcome, err, tt.want, tt.err)
				}
				if f.commits.calls != 0 {
					t.Errorf("commits = %d; want 0", f.commits.calls)
				}
			})
		}
	})

	t.Run("ACommitErrorIsReturnedNamingTheProducer", func(t *testing.T) {
		t.Parallel()
		f := newRaddleWriteFixture(t)
		f.commits.commitErr = errors.New("git commit failed")

		_, err := f.call()
		if !errors.Is(err, f.commits.commitErr) || !strings.Contains(err.Error(), NameDarnWrite) {
			t.Errorf("Call() error = %v; want the commit fault naming the producer", err)
		}
	})
}
