// removedetach_test.go pins `reed remove --detach`'s retiring mark and the status verb's `retiring` key through a fake engine seam.
// It spawns nothing and drives no tmux.

package reedcli

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

// fakeStrandOps records the calls the remove and status verbs make, in order.
type fakeStrandOps struct {
	calls     *[]string
	removeErr error
	status    reedengine.StatusResult
	markErrs  map[bool]error
}

func (f *fakeStrandOps) ResolveStrandGUID(name string) (string, error) {
	*f.calls = append(*f.calls, "resolve "+name)
	return "guid-" + name, nil
}

func (f *fakeStrandOps) MarkRetiring(guid string, retiring bool) error {
	*f.calls = append(*f.calls, fmt.Sprintf("mark %s %v", guid, retiring))
	return f.markErrs[retiring]
}

func (f *fakeStrandOps) RemoveStrand(guid string, recursive bool) (reedengine.Removed, error) {
	*f.calls = append(*f.calls, "remove "+guid)
	return reedengine.Removed{}, f.removeErr
}

func (f *fakeStrandOps) Status() (reedengine.StatusResult, error) {
	return f.status, nil
}

func newFakeCLI(t *testing.T, f *fakeStrandOps) (*reedCLI, *[]string) {
	t.Helper()
	calls := &[]string{}
	f.calls = calls
	c := &reedCLI{strands: f}
	c.spawnRemove = func(guid string, recursive bool) error {
		*calls = append(*calls, "spawn "+guid)
		return nil
	}
	return c, calls
}

func TestRemoveDetach(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		// spawnErr is what the detached spawn returns; nil means it succeeds.
		spawnErr  error
		markErrs  map[bool]error
		wantCalls []string
		// wantErr is empty for an ok envelope, otherwise the envelope's error must contain it.
		wantErr string
		// wantErrAlso must also appear in the envelope's error.
		wantErrAlso string
	}{
		{
			name:      "MarksRetiringBeforeSpawning",
			wantCalls: []string{"resolve driver", "mark guid-driver true", "spawn guid-driver"},
		},
		{
			name:      "FailedSpawnClearsTheMark",
			spawnErr:  errors.New("no exec"),
			wantCalls: []string{"resolve driver", "mark guid-driver true", "spawn guid-driver", "mark guid-driver false"},
			wantErr:   "no exec",
		},
		{
			name:        "FailedClearIsNamedInTheError",
			spawnErr:    errors.New("no exec"),
			markErrs:    map[bool]error{false: errors.New("state locked")},
			wantCalls:   []string{"resolve driver", "mark guid-driver true", "spawn guid-driver", "mark guid-driver false"},
			wantErr:     "no exec",
			wantErrAlso: "state locked",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, calls := newFakeCLI(t, &fakeStrandOps{markErrs: tt.markErrs})
			c.spawnRemove = func(guid string, recursive bool) error {
				*calls = append(*calls, "spawn "+guid)
				return tt.spawnErr
			}
			cmd := c.removeCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetArgs([]string{"--name", "driver", "--detach"})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if tt.wantErr == "" {
				envelope.RequireOK(t, out.String())
			} else {
				env := envelope.RequireErr(t, out.String(), tt.wantErr)
				if !strings.Contains(env.Error, tt.wantErrAlso) {
					t.Fatalf("error = %q; want it to contain %q", env.Error, tt.wantErrAlso)
				}
			}
			if got, want := strings.Join(*calls, "|"), strings.Join(tt.wantCalls, "|"); got != want {
				t.Fatalf("calls = %v; want %v", *calls, tt.wantCalls)
			}
		})
	}
}

// TestRemove_UnknownGUID swaps the package-level pidAlive in its --wait-pid row, so it does not call t.Parallel.
func TestRemove_UnknownGUID(t *testing.T) {
	tests := []struct {
		name string
		args []string
		// wantRemovedEmpty selects the ok envelope with an empty removed list over an "unknown strand" error.
		wantRemovedEmpty bool
	}{
		{name: "WaitPIDIsACleanNoOp", args: []string{"gone", "--wait-pid", "4242"}, wantRemovedEmpty: true},
		{name: "PositionalStillErrors", args: []string{"gone"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old := pidAlive
			t.Cleanup(func() { pidAlive = old })
			pidAlive = func(int) bool { return false }

			c, _ := newFakeCLI(t, &fakeStrandOps{removeErr: fmt.Errorf("%w %q", reedengine.ErrUnknownStrand, "gone")})
			cmd := c.removeCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetArgs(tt.args)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if !tt.wantRemovedEmpty {
				envelope.RequireErr(t, out.String(), "unknown strand")
				return
			}
			env := envelope.RequireOK(t, out.String())
			removed, ok := env.Raw["removed"].([]any)
			if !ok || len(removed) != 0 {
				t.Fatalf("removed = %#v; want an empty list", env.Raw["removed"])
			}
		})
	}
}

func TestStatus_ReportsRetiringPerStrand(t *testing.T) {
	t.Parallel()
	c, _ := newFakeCLI(t, &fakeStrandOps{status: reedengine.StatusResult{
		Session: "s",
		Strands: []reedengine.StrandStatus{
			{GUID: "a", Name: "a", Retiring: true},
			{GUID: "b", Name: "b"},
		},
	}})
	cmd := c.statusCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs(nil)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	env := envelope.RequireOK(t, out.String())
	strands, _ := env.Raw["strands"].([]any)
	if len(strands) != 2 {
		t.Fatalf("strands = %#v; want 2", env.Raw["strands"])
	}
	for i, want := range []bool{true, false} {
		row, _ := strands[i].(map[string]any)
		got, present := row["retiring"].(bool)
		if !present || got != want {
			t.Fatalf("strand %d retiring = %v (present %v); want %v", i, got, present, want)
		}
	}
}
