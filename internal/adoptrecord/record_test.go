// record_test.go pins the adopt record's read and write contract.
// It also pins the record's state and resume judgments and its request rendering.

package adoptrecord_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/adoptrecord"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRead(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	full := adoptrecord.Record{
		Request:    adoptrecord.Request{Slugs: []string{"a", "b"}, Root: true, KeepLocal: true},
		Subjects:   []string{"a", "b", "root"},
		PID:        42,
		StartTicks: "1234",
		Start:      time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC),
		Steps:      []adoptrecord.StepRecord{{Name: adoptrecord.StepReconcile, EndedAt: time.Date(2026, 11, 9, 10, 1, 0, 0, time.UTC), Outcome: json.RawMessage(`{}`)}},
		InFlight:   adoptrecord.StepPairs,
		Inputs: adoptrecord.Inputs{
			LocalRuns: []string{"a"}, Moved: []string{"a"}, Runs: []string{"a", "b"}, Adoptable: []string{"a"},
			RootGoing: true, HeldBack: []string{"b"}, OwnerStood: []string{"a"}, PrimeRecorded: true, Paused: []string{"a"},
		},
		State:       adoptrecord.StateStopped,
		Error:       "stopped",
		WayForward:  "re-run",
		StoppedPart: json.RawMessage(`{}`),
		Envelope:    map[string]any{"ok": true},
	}
	tests := []struct {
		name string
		// file is the content planted at the record's path, nil for none.
		file []byte
		// write is a record written through Write instead of a planted file.
		write      *adoptrecord.Record
		wantFound  bool
		want       adoptrecord.Record
		wantDecode bool
		// wantScratchAbsent expects Read to leave the scratch directory uncreated.
		wantScratchAbsent bool
	}{
		{name: "AbsentRecord", wantScratchAbsent: true},
		{name: "WrittenRecordReadsBackEqual", write: &full, wantFound: true, want: full},
		{name: "UnknownKeyStillReads", file: []byte(`{"state":"done","future_key":1}`), wantFound: true, want: adoptrecord.Record{State: adoptrecord.StateDone}},
		{name: "UndecodableFile", file: []byte(`{not json`), wantDecode: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			hub := t.TempDir()
			if tt.file != nil {
				path := adoptrecord.Path(hub)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, tt.file, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if tt.write != nil {
				if err := adoptrecord.Write(hub, *tt.write); err != nil {
					t.Fatal(err)
				}
			}

			got, found, err := adoptrecord.Read(hub)

			if tt.wantDecode {
				if !errors.Is(err, adoptrecord.ErrDecode) || !strings.Contains(err.Error(), "lym adopt --discard") || !strings.Contains(err.Error(), adoptrecord.Path(hub)) {
					t.Fatalf("Read() error = %v; want ErrDecode naming the path and lym adopt --discard", err)
				}
				return
			}
			if err != nil || found != tt.wantFound || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Read() = %+v, %v, %v; want %+v, %v, nil", got, found, err, tt.want, tt.wantFound)
			}
			if _, statErr := os.Stat(fabricengine.AdoptScratchDir(hub)); tt.wantScratchAbsent && !errors.Is(statErr, os.ErrNotExist) {
				t.Errorf("scratch directory stat = %v; want it absent after a read of an absent record", statErr)
			}
		})
	}
}

func TestRecordReadState(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	alive := func(pid int, startTicks string) bool { return pid == 1 && startTicks == "t1" }
	tests := []struct {
		name   string
		record adoptrecord.Record
		want   string
	}{
		{"RunningWithLiveWorker", adoptrecord.Record{State: adoptrecord.StateRunning, PID: 1, StartTicks: "t1"}, adoptrecord.StateRunning},
		{"RunningWithDeadPid", adoptrecord.Record{State: adoptrecord.StateRunning, PID: 2, StartTicks: "t1"}, adoptrecord.StateInterrupted},
		{"RunningWithOtherTicks", adoptrecord.Record{State: adoptrecord.StateRunning, PID: 1, StartTicks: "t2"}, adoptrecord.StateInterrupted},
		{"StoppedAsWritten", adoptrecord.Record{State: adoptrecord.StateStopped, PID: 2}, adoptrecord.StateStopped},
		{"DoneAsWritten", adoptrecord.Record{State: adoptrecord.StateDone, PID: 2}, adoptrecord.StateDone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.record.ReadState(alive); got != tt.want {
				t.Errorf("ReadState() = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestRecordResumeStep(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		done []string
		want string
	}{
		{"NothingDone", nil, adoptrecord.StepReconcile},
		{"ReconcileDone", []string{adoptrecord.StepReconcile}, adoptrecord.StepReconcile},
		{"PairsDone", []string{adoptrecord.StepReconcile, adoptrecord.StepPairs}, adoptrecord.StepReconcile},
		{"PauseDone", []string{adoptrecord.StepReconcile, adoptrecord.StepPairs, adoptrecord.StepPause}, adoptrecord.StepReconcile},
		{"OwnerDone", []string{adoptrecord.StepReconcile, adoptrecord.StepPairs, adoptrecord.StepPause, adoptrecord.StepOwner}, adoptrecord.StepDeploy},
		{"DeployDone", []string{adoptrecord.StepReconcile, adoptrecord.StepPairs, adoptrecord.StepPause, adoptrecord.StepOwner, adoptrecord.StepDeploy}, adoptrecord.StepBeam},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var record adoptrecord.Record
			for _, name := range tt.done {
				record.Steps = append(record.Steps, adoptrecord.StepRecord{Name: name})
			}
			if got := record.ResumeStep(); got != tt.want {
				t.Errorf("ResumeStep() = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestRequestSameSubjects(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		a, b adoptrecord.Request
		want bool
	}{
		{"AllAgainstAll", adoptrecord.Request{All: true}, adoptrecord.Request{All: true, KeepLocal: true}, true},
		{"SameSlugsInAnotherOrder", adoptrecord.Request{Slugs: []string{"a", "b"}}, adoptrecord.Request{Slugs: []string{"b", "a"}}, true},
		{"AnotherSlug", adoptrecord.Request{Slugs: []string{"a"}}, adoptrecord.Request{Slugs: []string{"b"}}, false},
		{"MissingRoot", adoptrecord.Request{Slugs: []string{"a"}, Root: true}, adoptrecord.Request{Slugs: []string{"a"}}, false},
		{"AllAgainstNamedSlugs", adoptrecord.Request{All: true}, adoptrecord.Request{Slugs: []string{"a"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.a.SameSubjects(tt.b); got != tt.want {
				t.Errorf("SameSubjects(%+v, %+v) = %v; want %v", tt.a, tt.b, got, tt.want)
			}
			if got := tt.b.SameSubjects(tt.a); got != tt.want {
				t.Errorf("SameSubjects(%+v, %+v) = %v; want %v", tt.b, tt.a, got, tt.want)
			}
		})
	}
}

func TestRequestCommand(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name    string
		request adoptrecord.Request
		want    string
	}{
		{"SlugsSorted", adoptrecord.Request{Slugs: []string{"b", "a"}}, "lym adopt a b"},
		{"Root", adoptrecord.Request{Root: true}, "lym adopt root"},
		{"All", adoptrecord.Request{All: true}, "lym adopt --all"},
		{"KeepLocal", adoptrecord.Request{Slugs: []string{"a"}, KeepLocal: true}, "lym adopt a --keep-local"},
		{"DropLocal", adoptrecord.Request{Slugs: []string{"a"}, DropLocal: true}, "lym adopt a --drop-local"},
		{"NoOrch", adoptrecord.Request{Slugs: []string{"a"}, NoOrch: true}, "lym adopt a --no-orch"},
		{"EverythingInOrder", adoptrecord.Request{Slugs: []string{"b", "a"}, Root: true, All: true, KeepLocal: true, DropLocal: true, NoOrch: true}, "lym adopt a b root --all --keep-local --drop-local --no-orch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.request.Command(); got != tt.want {
				t.Errorf("Command() = %q; want %q", got, tt.want)
			}
		})
	}
}
