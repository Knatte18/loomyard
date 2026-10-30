// record.go declares the owner record: the Record type, its constructors, and the Register, List and Owners functions that write and read the record files.

package runproc

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/state"
)

// HubOwner is the owner of the processes that belong to the hub rather than to one run.
// It is a name the fabric slug check refuses as reserved hub geometry, so no slug can take it.
const HubOwner = "_board"

// The roles a registered process plays for its owner.
const (
	// RoleDriver is the Go driver of a shed run.
	RoleDriver = "driver"
	// RoleLoop is a step loop of a run.
	RoleLoop = "loop"
	// RoleBatten is the batten process that holds a run.
	RoleBatten = "batten"
	// RolePush is a detached push.
	RolePush = "push"
	// RoleSync is a detached board sync.
	RoleSync = "sync"
	// RoleDobby is a beam's dobby daemon.
	RoleDobby = "dobby"
	// RoleCapture is the detached child that runs a task's regression capture.
	RoleCapture = "capture"
	// RoleSession is the pane process of an interactive agent session.
	RoleSession = "session"
)

// Record is one registered process of a run.
type Record struct {
	// PID is the process id.
	PID int `json:"pid"`
	// PGID is the pid when the process leads its own process group, and zero when it does not or the platform has no group to signal.
	PGID int `json:"pgid"`
	// StartTime is the process's start time in the form proc.StartTime returns, empty where none can be read.
	StartTime string `json:"start_time"`
	// Role is what the process does for its owner.
	Role string `json:"role"`
	// Argv is the process's command line.
	Argv []string `json:"argv"`
	// RegisteredAt is when the record was built.
	RegisteredAt time.Time `json:"registered_at"`
}

// ChildRecord builds the record of a child a spawner has just started with proc.Detach.
// The child leads its own group on every platform but Windows, where a child's group cannot be signalled.
func ChildRecord(pid int, role string, argv []string) Record {
	startTime, _ := proc.StartTime(pid)
	record := Record{PID: pid, StartTime: startTime, Role: role, Argv: argv, RegisteredAt: time.Now().UTC()}
	if runtime.GOOS != "windows" {
		record.PGID = pid
	}
	return record
}

// SelfRecord builds the record of the calling process.
// Its group is recorded only when the process leads it, so a role started inside a shell's group is never group-signalled.
func SelfRecord(role string) Record {
	pid := os.Getpid()
	startTime, _ := proc.StartTime(pid)
	record := Record{PID: pid, StartTime: startTime, Role: role, Argv: os.Args, RegisteredAt: time.Now().UTC()}
	stats, err := proc.ListStats()
	if err != nil {
		return record
	}
	for _, entry := range stats {
		if entry.PID == pid && entry.PGID == pid {
			record.PGID = pid
		}
	}
	return record
}

// Register writes the record of one process of owner under dir, replacing the file of the same pid and start time.
// dir is told by the caller; the record lands at <dir>/<owner>/<pid>-<start time>.json.
func Register(dir, owner string, rec Record) error {
	path := recordPath(dir, owner, rec)
	if err := state.WriteJSON(path, path+".lock", rec); err != nil {
		return fmt.Errorf("runproc: register %s of %s: %w", rec.Role, owner, err)
	}
	logger.Info("runproc: registered a process", "owner", owner, "role", rec.Role, "pid", rec.PID, "pgid", rec.PGID)
	return nil
}

// recordPath returns the file holding rec under owner's directory in dir.
func recordPath(dir, owner string, rec Record) string {
	return filepath.Join(dir, owner, fmt.Sprintf("%d-%s.json", rec.PID, rec.StartTime))
}

// lister reads the records of an owner against the liveness seams, so a test reaches the Windows branch on Linux.
type lister struct {
	startTime func(pid int) (string, bool)
	isAlive   func(pid int) bool
}

// List returns the live records of one owner, ordered by pid.
// It removes every record whose process is gone or whose pid now carries another start time, with its lock file; a record file that does not decode is skipped.
// A missing owner directory lists nothing.
func List(dir, owner string) ([]Record, error) {
	return lister{startTime: proc.StartTime, isAlive: proc.IsAlive}.list(dir, owner)
}

// Owners returns List for every owner directory under dir, leaving out an owner with no live record.
func Owners(dir string) (map[string][]Record, error) {
	return lister{startTime: proc.StartTime, isAlive: proc.IsAlive}.owners(dir)
}

func (l lister) owners(dir string) (map[string][]Record, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string][]Record{}, nil
		}
		return nil, fmt.Errorf("runproc: list owners: %w", err)
	}
	owners := map[string][]Record{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		records, err := l.list(dir, entry.Name())
		if err != nil {
			return nil, err
		}
		if len(records) > 0 {
			owners[entry.Name()] = records
		}
	}
	return owners, nil
}

func (l lister) list(dir, owner string) ([]Record, error) {
	ownerDir := filepath.Join(dir, owner)
	entries, err := os.ReadDir(ownerDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("runproc: list %s: %w", owner, err)
	}
	var records []Record
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(ownerDir, entry.Name())
		record, found, err := state.ReadJSON[Record](path, path+".lock")
		if err != nil || !found {
			continue
		}
		if !l.live(record) {
			_ = os.Remove(path)
			_ = os.Remove(path + ".lock")
			continue
		}
		records = append(records, record)
	}
	slices.SortFunc(records, func(a, b Record) int { return a.PID - b.PID })
	return records, nil
}

// live reports whether the record's process is still the one that was registered.
// Where no start time can be read, as on Windows, a record with an empty start time lives while its pid does.
func (l lister) live(record Record) bool {
	if startTime, ok := l.startTime(record.PID); ok {
		return startTime == record.StartTime
	}
	return record.StartTime == "" && l.isAlive(record.PID)
}
