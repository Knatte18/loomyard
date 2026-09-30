// steprecord_test.go covers the step body's own per-invocation record and status's last_step: the
// in-flight file exists while a producer runs, the envelope file equals the printed line after
// return, and last_step tracks the newest record.

package shedverbs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// TestStepCmd_InflightRecordExistsWhileProducerRuns has the producer inspect StepsDir from inside
// Call: the in-flight record must already be there, and no envelope record yet.
func TestStepCmd_InflightRecordExistsWhileProducerRuns(t *testing.T) {
	paths := newTestPaths(t)
	seedStatus(t, paths, "Only")
	stepsDir := filepath.Join(t.TempDir(), "steps")
	traceID := logger.TraceID()

	var sawInflight, sawEnvelope bool
	var rec inflightRecord
	probe := shedengine.ProducerDef{Name: "Only", Producer: &funcProducer{
		call: func(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
			data, err := os.ReadFile(filepath.Join(stepsDir, traceID+".inflight.json"))
			sawInflight = err == nil
			if err == nil {
				_ = json.Unmarshal(data, &rec)
			}
			_, envErr := os.Stat(filepath.Join(stepsDir, traceID+".json"))
			sawEnvelope = envErr == nil
			return shedengine.Done, shedengine.OutputPointer{}, nil
		},
	}}
	spec := &Spec{StepsDir: stepsDir, BuildShed: func() (*shedengine.Shed, error) {
		return newFakeShed(paths, []shedengine.ProducerDef{probe}), nil
	}}

	if _, code := execEnvelope(t, stepCmd(stepTexts(), spec), nil); code != 0 {
		t.Fatalf("exit code = %d; want 0", code)
	}
	if !sawInflight {
		t.Fatal("in-flight record was not present while the producer ran")
	}
	if sawEnvelope {
		t.Error("envelope record existed before the producer returned")
	}
	if rec.TraceID != traceID || rec.PID != os.Getpid() || rec.StartedAt.IsZero() {
		t.Errorf("in-flight record = %+v; want trace %q, pid %d and a start time", rec, traceID, os.Getpid())
	}
}

// TestStepCmd_EnvelopeRecordEqualsPrintedLine asserts the envelope file holds exactly the line
// step printed, for a success and for a refusal.
func TestStepCmd_EnvelopeRecordEqualsPrintedLine(t *testing.T) {
	tests := []struct {
		name     string
		wantCode int
		producer func() shedengine.ProducerDef
	}{
		{"Success", 0, func() shedengine.ProducerDef { return stubRow("Only") }},
		{"Refusal", 1, func() shedengine.ProducerDef { return erroringRow("Only", errors.New("boom")) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := newTestPaths(t)
			seedStatus(t, paths, "Only")
			stepsDir := filepath.Join(t.TempDir(), "steps")
			spec := &Spec{StepsDir: stepsDir, BuildShed: func() (*shedengine.Shed, error) {
				return newFakeShed(paths, []shedengine.ProducerDef{tt.producer()}), nil
			}}

			var buf bytes.Buffer
			if code := clihelp.Execute(stepCmd(stepTexts(), spec), &buf, nil); code != tt.wantCode {
				t.Fatalf("exit code = %d; want %d", code, tt.wantCode)
			}
			got, err := os.ReadFile(filepath.Join(stepsDir, logger.TraceID()+".json"))
			if err != nil {
				t.Fatalf("read envelope record: %v", err)
			}
			if !bytes.Equal(got, buf.Bytes()) {
				t.Errorf("envelope record = %q; want the printed %q", got, buf.Bytes())
			}
		})
	}
}

// TestStepCmd_NoStepsDirKeepsNoRecords asserts an empty StepsDir writes nothing anywhere.
func TestStepCmd_NoStepsDirKeepsNoRecords(t *testing.T) {
	paths := newTestPaths(t)
	seedStatus(t, paths, "Only")
	spec := &Spec{BuildShed: func() (*shedengine.Shed, error) {
		return newFakeShed(paths, []shedengine.ProducerDef{stubRow("Only")}), nil
	}}
	if _, code := execEnvelope(t, stepCmd(stepTexts(), spec), nil); code != 0 {
		t.Fatalf("exit code = %d; want 0", code)
	}
	entries, err := os.ReadDir(filepath.Dir(paths.StatusPath))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".json" && e.Name() != "status.json" {
			t.Errorf("unexpected record %q with no StepsDir", e.Name())
		}
	}
}

// TestStepCmd_RecordWriteFailureLeavesEnvelopeUnchanged points StepsDir at a path under a regular
// file, so every record write fails; the envelope and exit code are unaffected.
func TestStepCmd_RecordWriteFailureLeavesEnvelopeUnchanged(t *testing.T) {
	paths := newTestPaths(t)
	seedStatus(t, paths, "Only")
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := &Spec{StepsDir: filepath.Join(blocker, "steps"), BuildShed: func() (*shedengine.Shed, error) {
		return newFakeShed(paths, []shedengine.ProducerDef{stubRow("Only")}), nil
	}}
	env, code := execEnvelope(t, stepCmd(stepTexts(), spec), nil)
	if code != 0 || env["ok"] != true {
		t.Fatalf("code = %d, envelope = %v; want a normal success", code, env)
	}
}

// TestStatusCmd_LastStep walks last_step through no record, an unfinished record and a finished
// one, and checks the newest record wins.
func TestStatusCmd_LastStep(t *testing.T) {
	stepsDir := filepath.Join(t.TempDir(), "steps")
	spec := seededStatusSpec(t, Hooks{})
	spec.StepsDir = stepsDir

	lastStep := func() map[string]any {
		env, code := execEnvelope(t, statusCmd(statusTexts(), spec), nil)
		if code != 0 {
			t.Fatalf("exit code = %d; want 0", code)
		}
		v, present := env["last_step"]
		if !present {
			t.Fatalf("envelope missing last_step: %v", env)
		}
		m, _ := v.(map[string]any)
		return m
	}

	if got := lastStep(); got != nil {
		t.Fatalf("last_step = %v; want null with no records", got)
	}

	writeRecord := func(traceID, startedAt string) {
		t.Helper()
		if err := os.MkdirAll(stepsDir, 0o755); err != nil {
			t.Fatal(err)
		}
		data := `{"trace_id":"` + traceID + `","pid":1,"started_at":"` + startedAt + `"}`
		if err := os.WriteFile(filepath.Join(stepsDir, traceID+".inflight.json"), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeRecord("aaaa", "2026-01-01T00:00:00Z")
	got := lastStep()
	if got["trace_id"] != "aaaa" || got["finished"] != false {
		t.Errorf("last_step = %v; want aaaa unfinished", got)
	}
	if _, has := got["envelope_path"]; has {
		t.Errorf("unfinished last_step carries envelope_path: %v", got)
	}

	envPath := filepath.Join(stepsDir, "aaaa.json")
	if err := os.WriteFile(envPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got = lastStep()
	if got["finished"] != true || got["envelope_path"] != envPath {
		t.Errorf("last_step = %v; want finished with envelope_path %q", got, envPath)
	}

	writeRecord("bbbb", "2026-01-02T00:00:00Z")
	got = lastStep()
	if got["trace_id"] != "bbbb" || got["finished"] != false {
		t.Errorf("last_step = %v; want the newer bbbb, unfinished", got)
	}
}

// TestLastStepOf_BuildIdentity has a recorder write its identity, and checks lastStepOf reports it
// back with binary_changed false for the same running identity and true for a different known one;
// a hand-written record without identity fields never reports a change.
func TestLastStepOf_BuildIdentity(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "steps")
	built := BuildIdentity{Revision: "abc", Modified: true}
	newStepRecorder(dir, "aaaa", built).begin()

	same := lastStepOf(dir, built)
	if same == nil || same.VCSRevision != "abc" || !same.VCSModified || same.BinaryChanged {
		t.Errorf("same identity: last step = %+v; want abc/modified, binary_changed false", same)
	}
	other := lastStepOf(dir, BuildIdentity{Revision: "def"})
	if other == nil || other.VCSRevision != "abc" || !other.BinaryChanged {
		t.Errorf("different identity: last step = %+v; want binary_changed true", other)
	}

	legacy := filepath.Join(t.TempDir(), "steps")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	data := `{"trace_id":"bbbb","pid":1,"started_at":"2026-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(legacy, "bbbb.inflight.json"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	old := lastStepOf(legacy, built)
	if old == nil || old.VCSRevision != "" || old.BinaryChanged {
		t.Errorf("legacy record: last step = %+v; want unknown identity, binary_changed false", old)
	}
}

// TestStepRecords_NothingUnderStatusTree runs a step with the status file under one temp tree and
// StepsDir under another, and asserts nothing new appears beside the status file.
func TestStepRecords_NothingUnderStatusTree(t *testing.T) {
	paths := newTestPaths(t)
	seedStatus(t, paths, "Only")
	statusTree := filepath.Dir(paths.StatusPath)
	before, err := os.ReadDir(statusTree)
	if err != nil {
		t.Fatal(err)
	}

	stepsDir := filepath.Join(t.TempDir(), ".lyx", "steps")
	spec := &Spec{StepsDir: stepsDir, BuildShed: func() (*shedengine.Shed, error) {
		return newFakeShed(paths, []shedengine.ProducerDef{stubRow("Only")}), nil
	}}
	if _, code := execEnvelope(t, stepCmd(stepTexts(), spec), nil); code != 0 {
		t.Fatalf("exit code = %d; want 0", code)
	}

	after, err := os.ReadDir(statusTree)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, e := range before {
		seen[e.Name()] = true
	}
	for _, e := range after {
		if !seen[e.Name()] && e.Name() != "run.lock" && e.Name() != "status.lock" && e.Name() != "status.json.lock" {
			t.Errorf("new entry %q appeared beside the status file", e.Name())
		}
	}
}
