// tied_linux_test.go covers the Linux tree kill: Tied.Kill reaching a grandchild, KillTree's start-time gate and SelfRecord.
// It spawns real children, which this package's Test Tier Purity allowlist entry admits: process control is the package's subject.

package proc

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// processGone reports whether pid no longer runs; a zombie awaiting reaping by init counts as gone.
func processGone(pid int) bool {
	fields, ok := statFieldsAfterName(pid)
	return !ok || fields[0] == "Z"
}

func waitUntilGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !processGone(pid) {
		if time.Now().After(deadline) {
			t.Fatalf("process %d is still running", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestTiedStopEndsGrandchild(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// stop ends the child's tree; cancel is the context a CommandContext child was started under.
		stop func(t *testing.T, tied *Tied, cancel context.CancelFunc)
	}{
		{"Kill", func(t *testing.T, tied *Tied, _ context.CancelFunc) {
			if err := tied.Kill(); err != nil {
				t.Fatalf("Kill: %v", err)
			}
		}},
		{"context cancel", func(_ *testing.T, _ *Tied, cancel context.CancelFunc) { cancel() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cmd := exec.CommandContext(ctx, "sh", "-c", "sleep 60 & echo $!; wait")
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			tied, err := StartTied(cmd)
			if err != nil {
				t.Fatalf("StartTied: %v", err)
			}
			line, err := bufio.NewReader(stdout).ReadString('\n')
			if err != nil {
				t.Fatalf("read grandchild pid: %v", err)
			}
			grandchild, err := strconv.Atoi(strings.TrimSpace(line))
			if err != nil {
				t.Fatalf("parse grandchild pid %q: %v", line, err)
			}

			tt.stop(t, tied, cancel)
			_ = tied.Wait()
			waitUntilGone(t, grandchild)
		})
	}
}

func TestKillTree(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// shiftStart moves the recorded start time, so the record no longer matches a live member.
		shiftStart func(string) string
		wantKilled bool
	}{
		{"live group is killed", func(start string) string { return start }, true},
		{"group whose members started before the record is left alone", func(start string) string {
			value, _ := strconv.ParseUint(start, 10, 64)
			return strconv.FormatUint(value+1<<40, 10)
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tied, err := StartTied(exec.Command("sleep", "60"))
			if err != nil {
				t.Fatalf("StartTied: %v", err)
			}
			record := tied.Record()
			record.StartTime = tt.shiftStart(record.StartTime)

			killed, err := KillTree(record)
			if err != nil {
				t.Fatalf("KillTree: %v", err)
			}
			if killed != tt.wantKilled {
				t.Errorf("KillTree = %v; want %v", killed, tt.wantKilled)
			}
			if !tt.wantKilled {
				if processGone(record.PID) {
					t.Errorf("child %d died although KillTree reported nothing killed", record.PID)
				}
				if err := tied.Kill(); err != nil {
					t.Fatalf("Kill: %v", err)
				}
			}
			_ = tied.Wait()
			waitUntilGone(t, record.PID)
		})
	}
}

func TestSelfRecord(t *testing.T) {
	t.Parallel()

	record := SelfRecord()
	if record.PID != os.Getpid() || record.StartTime == "" {
		t.Errorf("SelfRecord() = %+v; want pid %d and a non-empty start time", record, os.Getpid())
	}
}
