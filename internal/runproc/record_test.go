//go:build tier0

// record_test.go tests the owner record: registering, listing with pruning, owner grouping and the liveness seams.

package runproc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/proc"
)

// deadPID is above the kernel's pid_max (at most 2^22), so no process carries it.
const deadPID = 1 << 30

func listPIDs(t *testing.T, records []Record) []int {
	t.Helper()
	var pids []int
	for _, record := range records {
		pids = append(pids, record.PID)
	}
	return pids
}

func dirEntries(t *testing.T, dir string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*"))
	if err != nil {
		t.Fatalf("Glob(%s): %v", dir, err)
	}
	return matches
}

func TestRegisterAndList(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	self := SelfRecord(RoleDriver)
	if self.PID != os.Getpid() || self.StartTime == "" || self.Role != RoleDriver || !slices.Equal(self.Argv, os.Args) {
		t.Fatalf("SelfRecord(driver) = %+v; want the test process's pid, a start time, the role and os.Args", self)
	}
	wantGroup := 0
	stats, err := proc.ListStats()
	if err != nil {
		t.Fatalf("ListStats: %v", err)
	}
	for _, entry := range stats {
		if entry.PID == os.Getpid() && entry.PGID == os.Getpid() {
			wantGroup = os.Getpid()
		}
	}
	if self.PGID != wantGroup {
		t.Errorf("SelfRecord(driver).PGID = %d; want %d", self.PGID, wantGroup)
	}

	if err := Register(dir, "slug", self); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := Register(dir, "slug", self); err != nil {
		t.Fatalf("Register again: %v", err)
	}
	if files := dirEntries(t, filepath.Join(dir, "slug")); len(files) != 2 {
		t.Errorf("owner directory after two registers = %v; want one record file and its lock file", files)
	}
	listed, err := List(dir, "slug")
	if err != nil || len(listed) != 1 {
		t.Fatalf("List(slug) = %+v, %v; want the one record", listed, err)
	}
	got := listed[0]
	if got.PID != self.PID || got.PGID != self.PGID || got.StartTime != self.StartTime || got.Role != self.Role || !slices.Equal(got.Argv, self.Argv) || !got.RegisteredAt.Equal(self.RegisteredAt) {
		t.Errorf("List(slug)[0] = %+v; want %+v", got, self)
	}

	child := ChildRecord(os.Getpid(), RolePush, []string{"git", "push"})
	if child.PGID != os.Getpid() || child.StartTime != self.StartTime || child.Role != RolePush {
		t.Errorf("ChildRecord = %+v; want its own group, the pid's start time and the role", child)
	}

	stale := []Record{
		{PID: deadPID, StartTime: "1", Role: RoleLoop},
		{PID: os.Getpid(), StartTime: "reused", Role: RoleLoop},
	}
	for _, record := range stale {
		if err := Register(dir, "slug", record); err != nil {
			t.Fatalf("Register(%+v): %v", record, err)
		}
	}
	if listed, err = List(dir, "slug"); err != nil || !slices.Equal(listPIDs(t, listed), []int{os.Getpid()}) {
		t.Errorf("List(slug) with a dead pid and a reused pid = %v, %v; want only the live record", listed, err)
	}
	for _, record := range stale {
		path := recordPath(dir, "slug", record)
		for _, name := range []string{path, path + ".lock"} {
			if _, err := os.Stat(name); !os.IsNotExist(err) {
				t.Errorf("Stat(%s) after the read = %v; want the stale record's file removed", name, err)
			}
		}
	}

	if err := os.MkdirAll(filepath.Join(dir, "other"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "other", "7-1.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	withUnknownKey := map[string]any{"pid": os.Getpid(), "start_time": self.StartTime, "role": RoleSync, "future_key": 1}
	body, err := json.Marshal(withUnknownKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "other", "8-1.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Register(dir, "empty", Record{PID: deadPID, StartTime: "1"}); err != nil {
		t.Fatal(err)
	}
	owners, err := Owners(dir)
	if err != nil {
		t.Fatalf("Owners: %v", err)
	}
	if len(owners) != 2 || len(owners["slug"]) != 1 || len(owners["other"]) != 1 || owners["other"][0].Role != RoleSync {
		t.Errorf("Owners = %+v; want slug and other with one record each, the undecodable file skipped, the unknown key read, and no empty owner", owners)
	}

	// Where no start time can be read, a record with an empty one lives while its pid does.
	if err := Register(dir, "windows", Record{PID: 42, Role: RoleBatten}); err != nil {
		t.Fatal(err)
	}

	alive := true
	unreadable := lister{
		startTime: func(int) (string, bool) { return "", false },
		isAlive:   func(int) bool { return alive },
	}
	if listed, err := unreadable.list(dir, "windows"); err != nil || len(listed) != 1 {
		t.Errorf("list with the pid alive = %+v, %v; want the record kept", listed, err)
	}
	alive = false
	if listed, err := unreadable.list(dir, "windows"); err != nil || len(listed) != 0 {
		t.Errorf("list with the pid dead = %+v, %v; want the record pruned", listed, err)
	}
}
