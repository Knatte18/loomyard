//go:build tier0

// flags_test.go pins the per-strand mail flags the store keeps in its flag directory.

package creelengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/testkit/creelfake"
)

// newFlaggedEnv is newEnv over a store told a flag directory, reconciled once as the daemon does at start.
// seed names flag files that stand in the directory before the store opens.
func newFlaggedEnv(t *testing.T, seed ...string) *env {
	t.Helper()
	root := t.TempDir()
	geom := creelengine.Geometry{
		HubPath: filepath.Join(root, "hub"), Shortname: "ly", StateDir: filepath.Join(root, "state"),
		JournalDir: filepath.Join(root, "state", "journal"), FlagDir: filepath.Join(root, "state", "flags"),
	}
	if err := os.MkdirAll(geom.FlagDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range seed {
		if err := os.WriteFile(filepath.Join(geom.FlagDir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	del := creelfake.NewDelivery()
	del.SetStrands(creelfake.LiveStrand(alpha), creelfake.LiveStrand(beta), creelfake.LiveStrand(orch), creelfake.LiveStrand(orch2))
	clock := creelfake.NewClock(epoch)
	store, err := creelengine.OpenStore(geom, del, clock)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.ReconcileFlags(); err != nil {
		t.Fatalf("ReconcileFlags: %v", err)
	}
	return &env{store: store, del: del, clock: clock, geom: geom}
}

// flags returns the file names in the env's flag directory, sorted.
func (e *env) flags(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(e.geom.FlagDir)
	if err != nil {
		t.Fatalf("read the flag directory: %v", err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestStore_MailFlags(t *testing.T) {
	t.Parallel()
	alphaFlag, betaFlag := agentname.FileName(alpha), agentname.FileName(beta)
	tests := []struct {
		name string
		seed []string
		run  func(t *testing.T, e *env)
	}{
		{"PostSetsTheReceiversFlagAndTheLastReadClearsIt", nil, func(t *testing.T, e *env) {
			e.post(t, strand(alpha), beta, "one")
			e.post(t, strand(alpha), beta, "two")
			if got := e.flags(t); !slices.Equal(got, []string{betaFlag}) {
				t.Fatalf("flags after two posts = %q; want %q", got, betaFlag)
			}
			e.readBodies(t, strand(beta), "")
			if got := e.flags(t); len(got) != 0 {
				t.Errorf("flags after the read = %q; want none", got)
			}
		}},
		{"ForwardMovesTheFlag", nil, func(t *testing.T, e *env) {
			id := e.post(t, strand(alpha), beta, "pass it on")
			if err := e.store.Forward(strand(beta), id, orch); err != nil {
				t.Fatalf("Forward: %v", err)
			}
			if got := e.flags(t); !slices.Equal(got, []string{agentname.FileName(orch)}) {
				t.Errorf("flags after the forward = %q; want only the orch's", got)
			}
		}},
		{"WithdrawClearsTheFlag", nil, func(t *testing.T, e *env) {
			e.post(t, program(batten), beta, "withdrawn")
			if _, err := e.store.Withdraw(program(batten), beta); err != nil {
				t.Fatalf("Withdraw: %v", err)
			}
			if got := e.flags(t); len(got) != 0 {
				t.Errorf("flags after the withdraw = %q; want none", got)
			}
		}},
		{"ReturnMovesTheFlagBackToTheSender", nil, func(t *testing.T, e *env) {
			e.post(t, strand(alpha), beta, "sent back")
			e.del.SetStrands(creelfake.LiveStrand(alpha), creelfake.LiveStrand(orch), creelfake.LiveStrand(orch2))
			e.start(t)
			e.sweepStep()
			e.sweepStep()
			if got := e.flags(t); !slices.Equal(got, []string{alphaFlag}) {
				t.Errorf("flags after the removed receiver's mail returned = %q; want only the sender's", got)
			}
		}},
		{"FlagFollowsThePoolWhenACallbackJournalsThenFails", nil, func(t *testing.T, e *env) {
			id := e.post(t, strand(alpha), beta, "read then failed")
			if err := e.store.ReadThenFail(id, beta); !errors.Is(err, creelengine.ErrAfterJournaling) {
				t.Fatalf("ReadThenFail = %v; want ErrAfterJournaling", err)
			}
			if got := e.flags(t); len(got) != 0 {
				t.Errorf("flags after a callback that journaled the read and failed = %q; want none", got)
			}
		}},
		{"OnlyAnOverlongMessageSetsNoFlag", nil, func(t *testing.T, e *env) {
			// A message an older binary stored past the hook field cap is never claimed, so it raises no flag.
			if _, err := e.store.PostUnchecked(strand(alpha), beta, strings.Repeat("x", 10001)); err != nil {
				t.Fatalf("PostUnchecked: %v", err)
			}
			if got := e.flags(t); len(got) != 0 {
				t.Errorf("flags after only an overlong message = %q; want none", got)
			}
		}},
		{"ProgramReceiverGetsNoFlag", nil, func(t *testing.T, e *env) {
			e.post(t, program(batten), alpha, "makes the program known")
			e.readBodies(t, strand(alpha), "")
			e.post(t, strand(alpha), batten, "to the program")
			if got := e.flags(t); len(got) != 0 {
				t.Errorf("flags after mail to a program = %q; want none", got)
			}
		}},
		{"AnOwedEchoKeepsTheFlagUntilTheClaimThatShowsItIsAcknowledged", nil, func(t *testing.T, e *env) {
			// newFlaggedEnv scripts pane-mail strands that are never rung, so the row re-scripts alpha as a hook strand before the scheduler starts.
			e.del.SetStrands(creelfake.HookStrand(alpha), creelfake.LiveStrand(beta))
			ringAndAck := func() {
				t.Helper()
				waiter, err := e.store.HookWait(strand(alpha), true)
				if err != nil {
					t.Fatalf("HookWait: %v", err)
				}
				outcome := awaitOutcome(t, waiter)
				if outcome.Reading == nil {
					t.Fatalf("the marked waiter was answered %+v; want a ring carrying a claim", outcome)
				}
				if err := outcome.Reading.Ack(); err != nil {
					t.Fatalf("Ack of the ring: %v", err)
				}
			}
			claimAndAck := func() {
				t.Helper()
				reading, err := e.store.ClaimAtBoundary(strand(alpha), creelengine.Boundary{})
				if err != nil {
					t.Fatalf("ClaimAtBoundary: %v", err)
				}
				if err := reading.Ack(); err != nil {
					t.Fatalf("Ack: %v", err)
				}
			}
			wantFlags := func(want []string, when string) {
				t.Helper()
				if got := e.flags(t); !slices.Equal(got, want) {
					t.Errorf("flags %s = %q; want %q", when, got, want)
				}
			}
			e.start(t)
			e.post(t, strand(beta), alpha, "one")
			ringAndAck()
			wantFlags([]string{alphaFlag}, "after the acknowledged ring, with no unread mail but an echo owed")
			claimAndAck()
			wantFlags(nil, "after the claim that showed the echo was acknowledged")

			// Two posts of 5000 bytes pass the claim budget together, so the ring takes the first and leaves the second unread.
			// The first ring's woken turn recorded no prompt submit, so a turn end after the ring stands in for the next turn's end.
			e.del.SetTurn(alpha, false, e.clock.Now().Add(time.Second))
			big := strings.Repeat("x", 5000)
			e.post(t, strand(beta), alpha, big)
			e.post(t, strand(beta), alpha, big)
			ringAndAck()
			wantFlags([]string{alphaFlag}, "after a ring that left the second post unread and owes the echo of the first")
			claimAndAck()
			wantFlags([]string{alphaFlag}, "after the echo claim, which the budget kept from the second post")
			claimAndAck()
			wantFlags(nil, "after the claim of the second post")
		}},
		{"ReconcileRestoresALostFlag", nil, func(t *testing.T, e *env) {
			e.post(t, strand(alpha), beta, "kept unread")
			if err := os.RemoveAll(e.geom.FlagDir); err != nil {
				t.Fatal(err)
			}
			if err := e.store.ReconcileFlags(); err != nil {
				t.Fatalf("ReconcileFlags: %v", err)
			}
			if got := e.flags(t); !slices.Equal(got, []string{betaFlag}) {
				t.Errorf("flags after reconciling a lost directory = %q; want beta's", got)
			}
		}},
		{"ReconcileRemovesAStaleFlag", []string{alphaFlag}, func(t *testing.T, e *env) {
			if got := e.flags(t); len(got) != 0 {
				t.Errorf("flags after reconciling an empty pool = %q; want the stale one removed", got)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t, newFlaggedEnv(t, tt.seed...))
		})
	}
}
