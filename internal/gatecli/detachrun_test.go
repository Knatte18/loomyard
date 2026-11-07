// detachrun_test.go covers the refusals of `lym gate run-detached`.
// A run directory the runner will not run leaves no exit file,
// and its refusal names the gate verbs that start a run of their own.

package gatecli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestGateRunner_Refusals(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	// A refusal ends before the run is claimed, so it never reaches the card fixer's seams.
	seams := defaultDetachSeams()
	seams.cardFixer = func(detachRequest) (cardFixerKit, bool, error) {
		t.Error("a refused run built a card fixer")
		return cardFixerKit{}, false, nil
	}
	validRequest := detachRequest{Verb: verbTest, Cwd: "/work"}
	tests := []struct {
		name string
		// prepare builds the run directory's files.
		prepare func(t *testing.T, run detachRun)
		want    string
	}{
		{"NoLease", func(t *testing.T, run detachRun) { writeRequest(t, run, validRequest) }, "no live lease"},
		{"PassedLease", func(t *testing.T, run detachRun) {
			writeRequest(t, run, validRequest)
			if err := run.writeLease(time.Now().Add(-time.Minute)); err != nil {
				t.Fatal(err)
			}
		}, "no live lease"},
		{"NoRequest", func(t *testing.T, run detachRun) { writeLiveLease(t, run) }, "request that cannot be read"},
		{"UnknownVerb", func(t *testing.T, run detachRun) {
			writeLiveLease(t, run)
			writeRequest(t, run, detachRequest{Verb: "bogus", Cwd: "/work"})
		}, "bogus"},
		{"CardVerifyWithoutCard", func(t *testing.T, run detachRun) {
			writeLiveLease(t, run)
			writeRequest(t, run, detachRequest{Verb: verbCardVerify, Cwd: "/work"})
		}, "names no card"},
		{"AlreadyClaimed", func(t *testing.T, run detachRun) {
			writeLiveLease(t, run)
			writeRequest(t, run, validRequest)
			if err := run.claim(os.Getpid(), "1"); err != nil {
				t.Fatal(err)
			}
		}, "already claimed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			tt.prepare(t, detachRun{dir: dir})

			var out bytes.Buffer
			command := newCommand(defaultGoBinary, defaultSamplePeriod, defaultPreTierCeiling, time.Minute, defaultRerunDelay, defaultPipeWaitDelay, seams)
			if code := clihelp.ExecuteIn(command, dir, &out, []string{"run-detached", dir}); code != 1 {
				t.Errorf("run-detached exit = %d, stdout %q; want 1", code, out.String())
			}
			for _, want := range []string{tt.want, "lym gate test", "lym gate card-verify"} {
				if !strings.Contains(out.String(), want) {
					t.Errorf("run-detached stdout = %q; want it to name %q", out.String(), want)
				}
			}
			if _, err := os.Stat(filepath.Join(dir, exitFile)); !os.IsNotExist(err) {
				t.Errorf("exit file stat error = %v; want none written", err)
			}
		})
	}
}

// writeLiveLease gives run a lease that holds for the length of a test.
func writeLiveLease(t *testing.T, run detachRun) {
	t.Helper()
	if err := run.writeLease(time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
}

// writeRequest writes request as the run's request file.
func writeRequest(t *testing.T, run detachRun, request detachRequest) {
	t.Helper()
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(run.dir, requestFile), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDetachRun_LeaseLapsed(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	now := time.Now()
	tests := []struct {
		name    string
		prepare func(t *testing.T, run detachRun)
		want    bool
	}{
		{"LeaseHolds", func(t *testing.T, run detachRun) { writeLiveLease(t, run) }, false},
		{"LeasePassed", func(t *testing.T, run detachRun) {
			if err := run.writeLease(now.Add(-time.Minute)); err != nil {
				t.Fatal(err)
			}
		}, true},
		{"LeaseAbsent", func(*testing.T, detachRun) {}, true},
		{"LeaseUnreadable", func(t *testing.T, run detachRun) {
			if err := os.WriteFile(run.path(leaseFile), []byte("not json"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			run := detachRun{dir: t.TempDir()}
			tt.prepare(t, run)
			if got := run.leaseLapsed(now); got != tt.want {
				t.Errorf("leaseLapsed = %v; want %v", got, tt.want)
			}
		})
	}
}

func TestDetachRun_Claim(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	run := detachRun{dir: t.TempDir()}
	if err := run.claim(42, "start-a"); err != nil {
		t.Fatal(err)
	}
	if got, found := readRunFile[processRecord](run.path(processFile)); !found || got != (processRecord{PID: 42, Start: "start-a"}) {
		t.Errorf("process file = %+v, %v; want the claimed record", got, found)
	}
	if err := run.claim(43, "start-b"); !errors.Is(err, os.ErrExist) {
		t.Errorf("second claim error = %v; want os.ErrExist", err)
	}
}

func TestDetachRun_ReadProcessRecord(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name      string
		content   *string
		want      processRecord
		wantFound bool
	}{
		{"Absent", nil, processRecord{}, false},
		{"Undecodable", ptr(""), processRecord{}, false},
		{"Complete", ptr(`{"pid":42,"start":"start-a"}`), processRecord{PID: 42, Start: "start-a"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			run := detachRun{dir: t.TempDir()}
			if tt.content != nil {
				if err := os.WriteFile(run.path(processFile), []byte(*tt.content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if got, found := run.readProcessRecord(); found != tt.wantFound || got != tt.want {
				t.Errorf("readProcessRecord = %+v, %v; want %+v, %v", got, found, tt.want, tt.wantFound)
			}
		})
	}
}

// ptr returns a pointer to value.
func ptr(value string) *string { return &value }
