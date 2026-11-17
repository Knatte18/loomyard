// report_test.go covers the manifest report of one seat over fake engines and fixture seat tables.

package shuttleparity

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymtemp"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// forkableSeatTable returns a seat table whose one seat is called seatName and forks its subagents when forks is set.
func forkableSeatTable(seatName string, forks bool) seatTable {
	return seatTable{Builder: "internal/x#First", Build: func(env SeatEnv) ([]seatengine.Table, error) {
		return []seatengine.Table{{
			RolePrefix: seatName,
			Seats: []seatengine.Seat{{
				Stencil:       "parity-test-bare",
				Outputs:       []string{filepath.Join(env.Dir, "out.md")},
				ConfigRole:    agentconfig.RoleDarn,
				ForkSubagents: forks,
			}},
		}}, nil
	}}
}

// TestReport_RemovesTheTemporaryDirectoryOnARefusal checks that a refused report leaves no report directory behind.
// It lists the process's temporary root, which a parallel test would add to, so it does not run in parallel.
func TestReport_RemovesTheTemporaryDirectoryOnARefusal(t *testing.T) {
	tiergate.Require(t, 0)

	stencils := stencilkit.Seed(t)
	writeFixtureFile(t, stencilstore.Path(stencils, "parity-test-bare"), "Lead {{.seat_name}}.\n")
	cfg := shuttleengine.Config{Agents: shuttlefake.Agents(t)}
	failing := func(string, shuttleengine.Spec) error { return errors.New("prepare refused") }
	tempRoot, err := lymtemp.Dir()
	if err != nil {
		t.Fatalf("lymtemp.Dir() error: %v", err)
	}
	tests := []struct {
		name string
		seat string
		prep func(string, shuttleengine.Spec) error
	}{
		{"UnknownSeat", "nope", writesManifest(manifestOf())},
		{"FailedPrepare", "parity", failing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before, err := filepath.Glob(filepath.Join(tempRoot, "manifest-report-*"))
			if err != nil {
				t.Fatalf("Glob: %v", err)
			}
			env := SeatEnv{
				StencilsDir: stencils,
				Location:    &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "wt", AnchorRel: "."},
				Shortname:   "ly", Slug: "task", ParentName: "ly:orch",
			}
			engines := engineSetOf(newManifestEngine(true, tt.prep), newManifestEngine(true, tt.prep))
			if _, err := report(env, []seatTable{forkableSeatTable("parity", false)}, tt.seat, engines, cfg); err == nil {
				t.Fatal("report() error = nil; want a refusal")
			}
			after, err := filepath.Glob(filepath.Join(tempRoot, "manifest-report-*"))
			if err != nil {
				t.Fatalf("Glob: %v", err)
			}
			if len(after) != len(before) {
				t.Errorf("report directories after the refusal = %v; want the %d before it", after, len(before))
			}
		})
	}
}

// TestReport covers report.
// It checks the manifests read back, what Codex lacks and the requirements an engine does not meet.
// It also checks the refusal of an unknown seat, a builder that failed, and that the temporary directory is left empty.
func TestReport(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const seatName = "parity"
	const failedSeatName = "parity-failed"
	stencils := stencilkit.Seed(t)
	writeFixtureFile(t, stencilstore.Path(stencils, "parity-test-bare"), "Lead {{.seat_name}}.\n")
	cfg := shuttleengine.Config{Agents: shuttlefake.Agents(t)}
	cause := errors.New("no stand-in")

	tableFor := func(forks bool) seatTable { return forkableSeatTable(seatName, forks) }
	failingTable := seatTable{Builder: "internal/x#Second", Build: func(SeatEnv) ([]seatengine.Table, error) { return nil, cause }}
	shared := item(shuttleengine.ChannelStandingPassage, "agents-prompt", "")
	only := item(shuttleengine.ChannelInstructionSource, "heddle-file/agents-file", "")
	unrun := shuttleengine.ManifestItem{Channel: shuttleengine.ChannelPluginContext, Key: "p@m/emit", Delivery: shuttleengine.DeliveryOmitted, Reason: "unrun"}
	held := item(shuttleengine.ChannelPluginContext, "p@m/emit", "")

	tests := []struct {
		name        string
		tables      []seatTable
		seat        string
		claude      shuttleengine.Manifest
		codex       shuttleengine.Manifest
		codexForks  bool
		wantMissing []shuttleengine.ManifestItem
		wantUnmet   map[string][]shuttleengine.Requirement
		// wantErr are the parts the refusal names.
		wantErr []string
	}{
		{name: "MatchingManifestsLackNothing", tables: []seatTable{tableFor(false)}, seat: seatName, claude: manifestOf(shared), codex: manifestOf(shared), codexForks: true},
		{
			name: "CodexLackingAnItemGivesExactlyThatItem", tables: []seatTable{tableFor(false)}, seat: seatName,
			claude: manifestOf(shared, only), codex: manifestOf(shared), wantMissing: []shuttleengine.ManifestItem{only},
		},
		{name: "OmittedPluginContextLacksNothing", tables: []seatTable{tableFor(false)}, seat: seatName, claude: manifestOf(held), codex: manifestOf(unrun)},
		{
			name: "CodexMeetingNoRequirementIsNamedForAForksSeat", tables: []seatTable{tableFor(true)}, seat: seatName,
			claude: manifestOf(shared), codex: manifestOf(shared),
			wantUnmet: map[string][]shuttleengine.Requirement{ComparedEngine: {shuttleengine.RequirementForks}},
		},
		{name: "UnknownSeatNamesTheSeats", tables: []seatTable{tableFor(false)}, seat: "nope", wantErr: []string{"nope", seatName, "way forward"}},
		{
			name: "FailedBuilderCostsOnlyItsOwnSeats", tables: []seatTable{tableFor(false), failingTable}, seat: seatName,
			claude: manifestOf(shared), codex: manifestOf(shared),
		},
		{
			name: "FailedBuildersSeatIsRefusedNamingTheBuilder", tables: []seatTable{tableFor(false), failingTable}, seat: failedSeatName,
			wantErr: []string{failedSeatName, "internal/x#Second", cause.Error(), "way forward"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var reportDir string
			prepares := func(m shuttleengine.Manifest) func(string, shuttleengine.Spec) error {
				return func(runDir string, spec shuttleengine.Spec) error {
					reportDir = filepath.Dir(runDir)
					return writesManifest(m)(runDir, spec)
				}
			}
			engines := engineSetOf(newManifestEngine(true, prepares(tt.claude)), newManifestEngine(tt.codexForks, prepares(tt.codex)))
			env := SeatEnv{
				StencilsDir: stencils,
				Location:    &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "wt", AnchorRel: "."},
				Shortname:   "ly", Slug: "task", ParentName: "ly:orch",
			}

			got, err := report(env, tt.tables, tt.seat, engines, cfg)
			if len(tt.wantErr) > 0 {
				if err == nil {
					t.Fatal("report() error = nil; want a refusal")
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("report() error %q does not contain %q", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("report() error: %v", err)
			}
			if got.Seat != tt.seat {
				t.Errorf("report() seat = %q; want %q", got.Seat, tt.seat)
			}
			for _, engine := range []string{ReferenceEngine, ComparedEngine} {
				if _, ok := got.Manifests[engine]; !ok {
					t.Errorf("report() manifests = %v; want one for %s", got.Manifests, engine)
				}
			}
			if len(got.Missing) != len(tt.wantMissing) || (len(got.Missing) > 0 && !reflect.DeepEqual(got.Missing, tt.wantMissing)) {
				t.Errorf("report() missing = %+v; want %+v", got.Missing, tt.wantMissing)
			}
			if len(got.Unmet) != len(tt.wantUnmet) || (len(got.Unmet) > 0 && !reflect.DeepEqual(got.Unmet, tt.wantUnmet)) {
				t.Errorf("report() unmet = %v; want %v", got.Unmet, tt.wantUnmet)
			}
			if _, err := os.Stat(reportDir); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("report directory %s after the report: stat error = %v; want it removed", reportDir, err)
			}
		})
	}
}
