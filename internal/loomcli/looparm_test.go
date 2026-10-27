//go:build tier0

// looparm_test.go pins the hook loom arms on the loop's step watchdog: what it stops and what it tells the interrupted cause.

package loomcli

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// TestRegisterRunProcess serves two roles: the loop's start hook and the driver's own call each register the test's pid under the slug, and a record directory that cannot be created registers nothing and warns once.
// Each role runs over its own hub, since one process keeps one record per owner.
// It stays serial because logcapture swaps the process-global logger.
func TestRegisterRunProcess(t *testing.T) {
	roles := []struct {
		name     string
		role     string
		register func(loc *lymcwd.Location)
	}{
		{"Loop", runproc.RoleLoop, func(loc *lymcwd.Location) { loopSpecFor(loc, loc.WorktreeName, 0, nil).OnStart() }},
		{"Driver", runproc.RoleDriver, func(loc *lymcwd.Location) { registerRunProcess(loc, loc.WorktreeName, runproc.RoleDriver) }},
	}
	for _, tt := range roles {
		t.Run(tt.name, func(t *testing.T) {
			loc := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "slug-1", AnchorRel: "."}

			tt.register(loc)

			records, err := runproc.List(fabricengine.RunProcDir(loc.HubPath), "slug-1")
			if err != nil {
				t.Fatalf("List() = %v; want nil", err)
			}
			if len(records) != 1 || records[0].Role != tt.role || records[0].PID != os.Getpid() {
				t.Errorf("records = %+v; want one %s record of the test's pid", records, tt.role)
			}
		})
	}

	blocked := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "slug-2", AnchorRel: "."}
	blockedDir := fabricengine.RunProcDir(blocked.HubPath)
	if err := os.MkdirAll(filepath.Dir(blockedDir), 0o755); err != nil {
		t.Fatalf("MkdirAll() = %v; want nil", err)
	}
	if err := os.WriteFile(blockedDir, nil, 0o644); err != nil {
		t.Fatalf("WriteFile() = %v; want nil", err)
	}
	buf := logcapture.Capture(t)

	registerRunProcess(blocked, blocked.WorktreeName, runproc.RoleDriver)

	if warns := strings.Count(buf.String(), "could not register the run process"); warns != 1 {
		t.Errorf("warnings = %d in %q; want exactly one", warns, buf.String())
	}
	if _, err := os.Stat(filepath.Join(blockedDir, "slug-2")); err == nil {
		t.Errorf("a record owner directory exists under the blocked record path; want nothing registered")
	}
}

func TestWatchdogKillNote_StopsTheKilledStepsRunsAndNamesTheRest(t *testing.T) {
	tests := []struct {
		name     string
		report   shuttleengine.StepStopReport
		stopErr  error
		wantNote string
	}{
		{name: "nothing live leaves an empty note"},
		{
			name:     "stopped and untied runs are both named",
			report:   shuttleengine.StepStopReport{Stopped: []string{"a:1:plan", "a:1:review"}, Untied: []string{"a:1:driver"}},
			wantNote: "stopped shuttle runs: a:1:plan, a:1:review; left live runs naming no step: a:1:driver",
		},
		{
			name:     "a run whose stop failed names its error",
			report:   shuttleengine.StepStopReport{Failed: []shuttleengine.StepStopFailure{{Strand: "a:1:plan", Error: "strand busy"}}},
			wantNote: "could not stop the run a:1:plan: strand busy",
		},
		{
			name:     "a failing stop renders its error",
			stopErr:  errors.New("probe reed: no server"),
			wantNote: "could not stop the step's runs: probe reed: no server",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ties []shuttleengine.StepTie
			stop := func(tie shuttleengine.StepTie) (shuttleengine.StepStopReport, error) {
				ties = append(ties, tie)
				return tt.report, tt.stopErr
			}

			note := watchdogKillNote(stop, "/steps")(shedverbs.WatchdogKill{TraceID: "trace-9"})

			if want := []shuttleengine.StepTie{{StepsDir: "/steps", TraceID: "trace-9"}}; !reflect.DeepEqual(ties, want) {
				t.Errorf("stop ties = %+v; want %+v", ties, want)
			}
			if note != tt.wantNote {
				t.Errorf("note = %q; want %q", note, tt.wantNote)
			}
		})
	}
}
