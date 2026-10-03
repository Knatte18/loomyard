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

func TestRemoveDetach_MarksRetiringBeforeSpawning(t *testing.T) {
	c, calls := newFakeCLI(t, &fakeStrandOps{})
	cmd := c.removeCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--name", "driver", "--detach"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	envelope.RequireOK(t, out.String())

	want := []string{"resolve driver", "mark guid-driver true", "spawn guid-driver"}
	if got := strings.Join(*calls, "|"); got != strings.Join(want, "|") {
		t.Fatalf("calls = %v; want %v", *calls, want)
	}
}

func TestRemoveDetach_FailedSpawnClearsTheMark(t *testing.T) {
	c, calls := newFakeCLI(t, &fakeStrandOps{})
	c.spawnRemove = func(guid string, recursive bool) error {
		*calls = append(*calls, "spawn "+guid)
		return errors.New("no exec")
	}
	cmd := c.removeCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--name", "driver", "--detach"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	envelope.RequireErr(t, out.String(), "no exec")

	want := []string{"resolve driver", "mark guid-driver true", "spawn guid-driver", "mark guid-driver false"}
	if got := strings.Join(*calls, "|"); got != strings.Join(want, "|") {
		t.Fatalf("calls = %v; want %v", *calls, want)
	}
}

func TestRemoveDetach_FailedClearIsNamedInTheError(t *testing.T) {
	c, _ := newFakeCLI(t, &fakeStrandOps{markErrs: map[bool]error{false: errors.New("state locked")}})
	c.spawnRemove = func(string, bool) error { return errors.New("no exec") }
	cmd := c.removeCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--name", "driver", "--detach"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	env := envelope.RequireErr(t, out.String(), "no exec")
	if !strings.Contains(env.Error, "state locked") {
		t.Fatalf("error = %q; want it to name the failed clear", env.Error)
	}
}

func TestRemoveWaitPID_UnknownGUIDIsACleanNoOp(t *testing.T) {
	old := pidAlive
	t.Cleanup(func() { pidAlive = old })
	pidAlive = func(int) bool { return false }

	c, _ := newFakeCLI(t, &fakeStrandOps{removeErr: fmt.Errorf("%w %q", reedengine.ErrUnknownStrand, "gone")})
	cmd := c.removeCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"gone", "--wait-pid", "4242"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	env := envelope.RequireOK(t, out.String())
	removed, ok := env.Raw["removed"].([]any)
	if !ok || len(removed) != 0 {
		t.Fatalf("removed = %#v; want an empty list", env.Raw["removed"])
	}
}

func TestRemovePositional_UnknownGUIDStillErrors(t *testing.T) {
	c, _ := newFakeCLI(t, &fakeStrandOps{removeErr: fmt.Errorf("%w %q", reedengine.ErrUnknownStrand, "gone")})
	cmd := c.removeCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"gone"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	envelope.RequireErr(t, out.String(), "unknown strand")
}

func TestStatus_ReportsRetiringPerStrand(t *testing.T) {
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
