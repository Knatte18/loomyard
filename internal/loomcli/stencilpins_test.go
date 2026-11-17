// stencilpins_test.go pins that a start or resume that puts a driver to work warns the parent orch.
// The warning names each pinned stencil changed since the run started.

package loomcli

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// editInstalledStencil appends a line to the body of name's board copy under stencilsDir.
func editInstalledStencil(t *testing.T, stencilsDir, name string) {
	t.Helper()
	path := stencilstore.Path(stencilsDir, name)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(content, []byte("\nedited after the run started\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
}

// assertParentNotices asserts the notices a recording notifyParent received.
// An empty want asserts none; otherwise exactly one notice that names edited and the diff command.
func assertParentNotices(t *testing.T, notices []string, want []string) {
	t.Helper()
	if len(want) == 0 {
		if len(notices) != 0 {
			t.Errorf("parent notices = %q; want none", notices)
		}
		return
	}
	if len(notices) != 1 {
		t.Fatalf("parent notices = %q; want one", notices)
	}
	for _, name := range append(append([]string{}, want...), `"lym stencil diff <name>"`) {
		if !strings.Contains(notices[0], name) {
			t.Errorf("parent notice %q lacks %q", notices[0], name)
		}
	}
}

// TestResumeVerb_NamesStencilsChangedSinceTheRunStarted resumes a halted run.
// The run gets a spawned Go driver or a woken parked one.
// It asserts the envelope's stencils_changed and the notice the parent orch gets,
// and that a failed post keeps the verb's success.
func TestResumeVerb_NamesStencilsChangedSinceTheRunStarted(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	edited := stencils.Registry().Names()[0]
	tests := []struct {
		name string
		pin  bool
		edit bool
		// parked resumes a parked live driver instead of spawning a Go driver.
		parked bool
		// notifyErr is what the parent post returns.
		notifyErr error
		want      []string
	}{
		{name: "RunWithoutPinsWarnsOfNothing", edit: true},
		{name: "UnchangedStencilsWarnOfNothing", pin: true},
		{name: "EditedStencilIsNamedToTheParent", pin: true, edit: true, want: []string{edited}},
		{name: "ParkedDriverWakeNamesTheEditedStencil", pin: true, edit: true, parked: true, want: []string{edited}},
		{name: "FailedParentPostStillListsTheEditedStencil", pin: true, edit: true, notifyErr: errors.New("creel is down"), want: []string{edited}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			directory := fakeDriverDirectory{}
			if tt.parked {
				directory = fakeDriverDirectory{found: true, row: reedengine.DirectoryRow{Name: driverStrandDisplayName, GUID: "g-drv", Live: true}}
			}
			c, _, _ := newResumeVerbReceiver(t, &fakeDriverSender{}, directory)
			c.spawnGoDriver = func() (int, error) { return 4242, nil }
			var notices []string
			c.notifyParent = func(line string) error {
				notices = append(notices, line)
				return tt.notifyErr
			}
			if err := shedrun.WriteSeed(c.location, c.runID, shedrun.Seed{Recipe: shedrun.RecipeTapestry, Driver: shedrun.DriverGo}); err != nil {
				t.Fatalf("write seed: %v", err)
			}
			writeTestRunState(t, c, shedengine.StateBlocked)
			if tt.parked {
				putParkMarker(t, c)
			}
			if tt.pin {
				c.writeStencilPins()
			}
			if tt.edit {
				editInstalledStencil(t, c.runDeps.Geom.StencilsDir, edited)
			}

			env, code := runResume(t, c)

			if !env.OK || code != 0 {
				t.Fatalf("resume envelope ok = %v, exit code = %d (%s); want a spawned or woken driver", env.OK, code, env.Error)
			}
			if !reflect.DeepEqual(env.StencilsChanged, tt.want) {
				t.Errorf("stencils_changed = %v; want %v", env.StencilsChanged, tt.want)
			}
			assertParentNotices(t, notices, tt.want)
		})
	}
}

// TestStartSuccessFields asserts that a start warns the parent orch of the changed stencils.
// It warns only when it put a driver to work.
func TestStartSuccessFields(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	edited := stencils.Registry().Names()[0]
	tests := []struct {
		name            string
		edit            bool
		putDriverToWork bool
		want            []string
	}{
		{"SpawnedDriverNamesTheEditedStencil", true, true, []string{edited}},
		{"SpawnedDriverWithUnchangedStencilsWarnsOfNothing", false, true, nil},
		{"WorkingDriverLeftAloneWarnsOfNothing", true, false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, _ := newTestSpawnAndWaitReceiver(t, &fakeDriverStarter{}, &fakeDriverPaneProbeFull{strandsFn: noStrands})
			var notices []string
			c.notifyParent = func(line string) error {
				notices = append(notices, line)
				return nil
			}
			c.writeStencilPins()
			if tt.edit {
				editInstalledStencil(t, c.runDeps.Geom.StencilsDir, edited)
			}

			fields := c.startSuccessFields("go", "pair", tt.putDriverToWork)

			got, listed := fields[stencilsChangedField]
			if tt.want == nil && listed {
				t.Errorf("stencils_changed = %v; want the key absent", got)
			}
			if tt.want != nil && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("stencils_changed = %v; want %v", got, tt.want)
			}
			assertParentNotices(t, notices, tt.want)
		})
	}
}
