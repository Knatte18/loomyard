// report_test.go covers one seat's report over fake engines and a fixture seat catalog.

package shuttleparity

import (
	"errors"
	"io/fs"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestReport pins what a seat's report holds, which seat a name finds, its refusal of an unknown seat
// and the removal of its temporary directory.
func TestReport(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	notice := shuttleengine.ManifestItem{Channel: shuttleengine.ManifestChannelNotice, Key: "n", Delivery: shuttleengine.ManifestDelivered}
	context := shuttleengine.ManifestItem{Channel: shuttleengine.ManifestChannelPluginContext, Key: "p@m/emit", Delivery: shuttleengine.ManifestNative}
	unrun := context
	unrun.Delivery = shuttleengine.ManifestOmitted
	broken := errors.New("no stand-in")
	forkUnmet := map[string][]shuttleengine.Requirement{ComparedEngine: {shuttleengine.RequirementForks}}

	tests := []struct {
		name        string
		seat        string
		reference   []shuttleengine.ManifestItem
		compared    []shuttleengine.ManifestItem
		forks       bool
		brokenEntry bool
		// rolePrefix replaces the fixture table's, and wantSeat is the reported seat when it is not seat.
		rolePrefix  string
		wantSeat    string
		wantMissing []shuttleengine.ManifestItem
		wantUnmet   map[string][]shuttleengine.Requirement
		wantErr     []string
	}{
		{name: "matching manifests", seat: "fixture", reference: []shuttleengine.ManifestItem{notice}, compared: []shuttleengine.ManifestItem{notice}},
		{name: "a missing compared item", seat: "fixture", reference: []shuttleengine.ManifestItem{notice}, wantMissing: []shuttleengine.ManifestItem{notice}},
		{name: "an unrun plugin context", seat: "fixture", reference: []shuttleengine.ManifestItem{context}, compared: []shuttleengine.ManifestItem{unrun}},
		{name: "a seat the compared engine cannot start", seat: "fixture", forks: true, wantUnmet: forkUnmet},
		{name: "an unknown seat", seat: "nobody", wantErr: []string{`"nobody"`, "fixture"}},
		{name: "a seat beside a failed builder", seat: "fixture", brokenEntry: true},
		{name: "the seat of a failed builder", seat: "broken", brokenEntry: true, wantErr: []string{"second#Table", "no stand-in"}},
		{name: "a burler role under a recipe's prefix", seat: "code-burler-review", rolePrefix: "burler-review", wantSeat: "burler-review"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := seatsEnv(t, stencilkit.Seed(t))
			table := fixtureTable()
			table.Seats[0].ForkSubagents = tt.forks
			if tt.rolePrefix != "" {
				table.RolePrefix = tt.rolePrefix
			}
			wantSeat := tt.seat
			if tt.wantSeat != "" {
				wantSeat = tt.wantSeat
			}
			var dir string
			tables := []seatTable{{Builder: "first#Table", Build: func(env SeatEnv) ([]seatengine.Table, error) {
				dir = env.Dir
				return []seatengine.Table{table}, nil
			}}}
			if tt.brokenEntry {
				tables = append(tables, seatTable{Builder: "second#Table", Build: func(SeatEnv) ([]seatengine.Table, error) { return nil, broken }})
			}
			set := fakeSet(
				&manifestEngine{manifest: &shuttleengine.Manifest{Items: tt.reference}, forks: true},
				&manifestEngine{manifest: &shuttleengine.Manifest{Items: tt.compared}},
			)

			got, err := report(env, tables, tt.seat, set, shuttleengine.Config{Agents: shuttlefake.Agents(t)})

			if _, statErr := os.Stat(dir); dir == "" || !errors.Is(statErr, fs.ErrNotExist) {
				t.Errorf("report() left its temporary directory %q: %v", dir, statErr)
			}
			if tt.wantErr != nil {
				for _, want := range tt.wantErr {
					if err == nil || !strings.Contains(err.Error(), want) {
						t.Errorf("report() error = %v; want it to contain %q", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("report() error = %v", err)
			}
			if got.Seat != wantSeat || len(got.Manifests) != 2 {
				t.Errorf("report() = seat %q with %d manifests; want %q with 2", got.Seat, len(got.Manifests), wantSeat)
			}
			if len(got.Missing) != len(tt.wantMissing) || len(tt.wantMissing) > 0 && !reflect.DeepEqual(got.Missing, tt.wantMissing) {
				t.Errorf("report() missing = %+v; want %+v", got.Missing, tt.wantMissing)
			}
			if len(got.Unmet) != len(tt.wantUnmet) || len(tt.wantUnmet) > 0 && !reflect.DeepEqual(got.Unmet, tt.wantUnmet) {
				t.Errorf("report() unmet = %v; want %v", got.Unmet, tt.wantUnmet)
			}
		})
	}
}
