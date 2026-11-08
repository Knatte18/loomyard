// remoteteardown_childhold_integration_test.go covers the parent's side of the teardown handover.
// It runs over a started stand-in child process.
// It pins how spawnTeardownHold reads the child's answer and registers the child.
// It pins what childHold.finish writes to the child's input, how it closes it, and whether it waits for the child.

package fabricengine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// teardownStandInArg is the first argument that makes the test binary play the teardown child, as TestMain handles it.
const teardownStandInArg = "teardown-standin"

// runTeardownStandIn plays the teardown child of slug in the way mode names and returns the process's exit code.
// It runs from the hub's Bolt checkout, as the real child does.
// held creates the lock file, answers held and reads its input until it closes;
// busy answers busy;
// error answers an error line and fails;
// closed ends with no answer.
func runTeardownStandIn(mode, slug string) int {
	switch mode {
	case "held":
		workDir, err := os.Getwd()
		if err != nil {
			return 2
		}
		lockPath := TeardownLockPath(filepath.Dir(workDir), slug)
		if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
			return 2
		}
		if err := os.WriteFile(lockPath, nil, 0o644); err != nil {
			return 2
		}
		fmt.Println(teardownHeldSignal)
		_, _ = io.Copy(io.Discard, os.Stdin)
	case "busy":
		fmt.Println(teardownBusySignal)
	case "error":
		fmt.Println("error the stand-in refuses")
		return 1
	}
	return 0
}

// TestSpawnTeardownHold covers the process boundary of the handover over a stand-in child:
// a held answer returns a hold and records the lock file the child made;
// it registers the child under the slug with the teardown role.
// A busy answer returns the refusal of its mode.
// An error line or a closed output returns an error naming the answer, and records nothing.
func TestSpawnTeardownHold(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	const slug = "pair"
	const rerun = "re-run the stand-in command"
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v; want nil", err)
	}
	tests := []struct {
		name     string
		mode     string
		try      bool
		wantHeld bool
		wantErr  error
		// wantInErr are the texts the returned error holds.
		wantInErr []string
	}{
		{"HeldReturnsAHoldAndRecordsTheLock", "held", false, true, nil, nil},
		{"BusyWithTryIsBusy", "busy", true, false, ErrPairBusy, nil},
		{"BusyWithoutTryIsPushRunningAndNamesTheRerun", "busy", false, false, ErrPairPushRunning, []string{rerun}},
		{"ErrorLineNamesTheAnswer", "error", false, false, nil, []string{"error the stand-in refuses"}},
		{"ClosedOutputNamesTheReadError", "closed", false, false, nil, []string{"EOF"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			hub := t.TempDir()
			if err := os.MkdirAll(BoltDir(hub), 0o755); err != nil {
				t.Fatalf("create the Bolt checkout: %v", err)
			}
			rec := NewMutations(hub)

			hold, err := spawnTeardownHold(rec, &lymcwd.Location{HubPath: hub}, slug, tt.try, exe, []string{teardownStandInArg, tt.mode, slug}, rerun)

			if hold != nil {
				t.Cleanup(func() { hold.finish(nil) })
			}
			if !tt.wantHeld {
				if hold != nil || err == nil {
					t.Fatalf("spawnTeardownHold() = (%v, %v); want a nil hold and an error", hold, err)
				}
				if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
					t.Errorf("spawnTeardownHold() error = %v; want it to wrap %v", err, tt.wantErr)
				}
				for _, want := range tt.wantInErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("spawnTeardownHold() error = %q; want it to hold %q", err, want)
					}
				}
				if got := rec.Snapshot().Len(); got != 0 {
					t.Errorf("spawnTeardownHold() recorded %d mutations; want none", got)
				}
				return
			}

			if err != nil || hold == nil {
				t.Fatalf("spawnTeardownHold() = (%v, %v); want a hold and nil", hold, err)
			}
			lockPath := TeardownLockPath(hub, slug)
			want := []Mutation{{Kind: KindFileWritten, Target: hubRelativeTarget(hub, lockPath)}}
			if got := rec.Snapshot().Entries(); len(got) != 1 || got[0] != want[0] {
				t.Errorf("recorded mutations = %+v; want %+v", got, want)
			}
			records, err := runproc.List(RunProcDir(hub), slug)
			if err != nil || len(records) != 1 || records[0].Role != runproc.RoleTeardown {
				t.Errorf("runproc.List(%s) = (%+v, %v); want one record with role %q", slug, records, err, runproc.RoleTeardown)
			}
		})
	}
}

// recordingStdin is a child's input that records what is written to it and whether it was closed;
// it fails every write when writeErr is set.
type recordingStdin struct {
	bytes.Buffer
	writeErr error
	closed   bool
}

func (s *recordingStdin) Write(p []byte) (int, error) {
	if s.writeErr != nil {
		return 0, s.writeErr
	}
	return s.Buffer.Write(p)
}

func (s *recordingStdin) Close() error {
	s.closed = true
	return nil
}

// TestChildHold_Finish covers the parent's side of the handover:
// a job is written to the child and handed over at once;
// a job that cannot be written still closes the input;
// no job closes the input and waits for the child to exit.
func TestChildHold_Finish(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	job := remoteTeardownJob{Kind: remoteTeardownRemove, Slug: "pair", Worktree: "/w", WeftTip: "abc"}
	tests := []struct {
		name          string
		job           *remoteTeardownJob
		writeErr      error
		wantHandedOff bool
		wantWritten   bool
	}{
		{"NilJobWaitsForTheChild", nil, nil, false, false},
		{"JobIsWrittenAndHandedOff", &job, nil, true, true},
		{"UnwritableJobStillHandsOff", &job, errors.New("broken pipe"), true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cmd := exec.Command("git", "--version")
			if err := cmd.Start(); err != nil {
				t.Fatalf("start the stand-in child: %v", err)
			}
			stdin := &recordingStdin{writeErr: tt.writeErr}

			out := (&childHold{slug: "pair", cmd: cmd, stdin: stdin}).finish(tt.job)

			if !stdin.closed {
				t.Errorf("finish() left the child's input open; want it closed")
			}
			if out.handedOff != tt.wantHandedOff || out.ran {
				t.Errorf("finish() outcome = (handedOff %v, ran %v); want (%v, false)", out.handedOff, out.ran, tt.wantHandedOff)
			}
			if tt.job == nil && cmd.ProcessState == nil {
				t.Errorf("finish(nil) returned before the child was waited for")
			}
			if !tt.wantWritten {
				if stdin.Len() != 0 {
					t.Errorf("finish() wrote %q; want nothing", stdin.String())
				}
				return
			}
			var written remoteTeardownJob
			if err := json.Unmarshal(stdin.Bytes(), &written); err != nil || written != job {
				t.Errorf("finish() wrote %q; want the JSON of %+v (decode error %v)", stdin.String(), job, err)
			}
		})
	}
}
