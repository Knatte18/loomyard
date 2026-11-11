// dobbydeps_test.go pins which pause entries the dobby's owner-pause lift removes.

package beamcli

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestLiftOwnerPause_LeavesAnAdoptOrHoldEntryStanding(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	for _, caller := range []string{pauserecord.AdoptCaller, pauserecord.HoldCaller} {
		t.Run(caller, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "pause.json")
			entry := pauserecord.Entry{Scope: "alpha", Caller: caller, PausedAt: time.Unix(1000, 0)}
			if _, err := pauserecord.Update(path, time.Unix(2000, 0), func(record *pauserecord.Record) error {
				record.Live = []pauserecord.Entry{entry}
				return nil
			}); err != nil {
				t.Fatalf("Update(pause record) = %v", err)
			}

			if err := liftOwnerPause(nil, path, "alpha"); err != nil {
				t.Fatalf("liftOwnerPause() = %v, want none", err)
			}
			record, err := pauserecord.Read(path)
			if err != nil {
				t.Fatalf("Read(pause record) = %v", err)
			}
			if len(record.Live) != 1 || record.Live[0].Caller != caller || len(record.Resumed) != 0 {
				t.Errorf("record = live %+v, resumed %+v; want the %s entry standing", record.Live, record.Resumed, caller)
			}
		})
	}
}
