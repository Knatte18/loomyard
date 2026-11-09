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
	for _, written := range []testcover.ExitRecord{{PID: 4321}, {PID: 4322, Unbuilt: 3}} {
		if err := testcover.WriteExitRecord(dir, written); err != nil {
			t.Fatal(err)
		}
		read, found, err := testcover.ReadExitRecord(dir)
		if err != nil || !found || read != written {
			t.Errorf("ReadExitRecord after writing %+v = %+v, found %v, err %v; want the written record", written, read, found, err)
		}
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
			{Name: "TestA", Tier: 0, FuncHash: "t1", Seconds: 0.25, Blocks: []int{0}},
			{Name: "TestB", Tier: 1, FuncHash: "t2", Seconds: 1.5, Blocks: []int{0, 1}},
		},
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
