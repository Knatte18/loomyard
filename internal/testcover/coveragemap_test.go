package testcover_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestWriteExitRecordReadExitRecord(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "testcover")
	if _, found, err := testcover.ReadExitRecord(dir); err != nil || found {
		t.Fatalf("ReadExitRecord before any write = found %v, err %v; want not found and no error", found, err)
	}
	full := testcover.ExitRecord{
		PID: 4322, PlannedHead: "c0ffee", Queued: 3,
		Plan:                testcover.PlanCounts{Current: 4, Affected: 1, Misses: map[testcover.MissReason]int{testcover.MissNoMap: 2}},
		SelfCheckMismatches: 1, AuditMismatches: 2,
		Packages: []testcover.PackageMeasure{{Package: "internal/x", BuildSeconds: 1.5, RunSeconds: 0.25, Tests: 2}},
	}
	for _, written := range []testcover.ExitRecord{{PID: 4321}, full} {
		if err := testcover.WriteExitRecord(dir, written); err != nil {
			t.Fatal(err)
		}
		read, found, err := testcover.ReadExitRecord(dir)
		if err != nil || !found || !reflect.DeepEqual(read, written) {
			t.Errorf("ReadExitRecord after writing %+v = %+v, found %v, err %v; want the written record", written, read, found, err)
		}
	}
	// A record an earlier writer left with the retired unbuilt count still reads.
	if err := os.WriteFile(testcover.ExitRecordPath(dir), []byte(`{"pid": 7, "unbuilt": 3}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if read, found, err := testcover.ReadExitRecord(dir); err != nil || !found || !reflect.DeepEqual(read, testcover.ExitRecord{PID: 7}) {
		t.Errorf("ReadExitRecord of a record holding unbuilt = %+v, found %v, err %v; want pid 7 alone", read, found, err)
	}
}

func TestMapFailedAt(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	entry := []testcover.MapTest{{Name: "TestA", Blocks: []int{0}}}
	tests := []struct {
		name string
		m    testcover.Map
		want bool
	}{
		{"FailureOnlyMapAtTheCommit", testcover.Map{Commit: "head", BuildFailure: "failed"}, true},
		{"FailureOnlyMapAtAnotherCommit", testcover.Map{Commit: "old", BuildFailure: "failed"}, false},
		{"FailureBesideEntriesAtTheCommit", testcover.Map{Commit: "old", Tests: entry, Failure: &testcover.MapFailure{Commit: "head", Error: "failed"}}, true},
		{"FailureBesideEntriesAtAnotherCommit", testcover.Map{Commit: "old", Tests: entry, Failure: &testcover.MapFailure{Commit: "older", Error: "failed"}}, false},
		{"NoFailure", testcover.Map{Commit: "head", Tests: entry}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.m.FailedAt("head"); got != tt.want {
				t.Errorf("FailedAt(head) = %v; want %v", got, tt.want)
			}
		})
	}
}

func TestWriteMapReadMap(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	dir := t.TempDir()
	const pkg = "internal/x"
	written := testcover.Map{
		Commit:  "0123abcd",
		BuiltAt: time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC),
		Files:   map[string]string{"x.go": "f1", "x_test.go": "f2"},
		Blocks: []testcover.BlockKey{
			{File: "x.go", Func: "A.Do", Hash: "h1"},
			{File: "x.go", Func: "B.Do", Hash: "h1"},
		},
		Tests: []testcover.MapTest{
			{Name: "TestA", Tier: 0, FuncHash: "t1", Seconds: 0.25, Blocks: []int{0}, Commit: "0123abcd", MeasuredAt: time.Date(2026, 11, 9, 11, 0, 0, 0, time.UTC)},
			{Name: "TestB", Tier: 1, FuncHash: "t2", Seconds: 1.5, Blocks: []int{0, 1}},
			{Name: "TestC", Tier: 0, FuncHash: "t3", Outcome: testcover.OutcomeFailed, Commit: "0123abcd", MeasuredAt: time.Date(2026, 11, 9, 11, 30, 0, 0, time.UTC)},
		},
		Failure: &testcover.MapFailure{Commit: "89abcdef", Error: "coverage binary: build failed"},
	}

	if _, found, err := testcover.ReadMap(dir, pkg); err != nil || found {
		t.Fatalf("ReadMap before any write = found %v, err %v; want not found and no error", found, err)
	}
	if err := testcover.WriteMap(dir, pkg, written); err != nil {
		t.Fatal(err)
	}
	read, found, err := testcover.ReadMap(dir, pkg)
	if err != nil || !found {
		t.Fatalf("ReadMap = found %v, err %v; want the written map", found, err)
	}
	if !reflect.DeepEqual(read, written) {
		t.Fatalf("ReadMap = %+v\nwant      %+v", read, written)
	}

	path := testcover.MapPath(dir, pkg)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err := json.Unmarshal(raw, &record); err != nil {
		t.Fatal(err)
	}
	record["newer_key"] = "kept"
	planted, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, planted, 0o644); err != nil {
		t.Fatal(err)
	}
	if read, _, err = testcover.ReadMap(dir, pkg); err != nil || !reflect.DeepEqual(read, written) {
		t.Fatalf("ReadMap of a record with an unknown key = %+v, %v; want the map, unrefused", read, err)
	}
	if err := testcover.WriteMap(dir, pkg, written); err != nil {
		t.Fatal(err)
	}
	if raw, err = os.ReadFile(path); err != nil {
		t.Fatal(err)
	}
	record = nil
	if err := json.Unmarshal(raw, &record); err != nil {
		t.Fatal(err)
	}
	if record["newer_key"] != "kept" {
		t.Errorf("rewritten record's newer_key = %v; want the unknown key kept", record["newer_key"])
	}

	replacement := testcover.Map{
		Commit:       "4567ef89",
		BuiltAt:      time.Date(2026, 11, 10, 8, 30, 0, 0, time.UTC),
		Files:        map[string]string{"x.go": "f3"},
		Blocks:       []testcover.BlockKey{{File: "x.go", Func: "C.Do", Hash: "h2", Ordinal: 1}},
		Tests:        []testcover.MapTest{{Name: "TestC", Tier: 0, FuncHash: "t3", Seconds: 0.5, Blocks: []int{0}}},
		BuildFailure: "coverage binary: build failed",
	}
	if err := testcover.WriteMap(dir, pkg, replacement); err != nil {
		t.Fatal(err)
	}
	if read, _, err = testcover.ReadMap(dir, pkg); err != nil || !reflect.DeepEqual(read, replacement) {
		t.Fatalf("ReadMap after a second write = %+v, %v; want the second map whole, with none of the first map's tests", read, err)
	}
}
