//go:build tier1

// owner_integration_test.go drives the run verb's persistent pre-run over a real hub, the one owner check that needs the prime-name refusal ahead of it.

package battencli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestArmAt_RunOwnedElsewhereWritesNoSeed asserts a first "lyx batten run" of a task another machine owns is refused naming the owner and "lyx adopt", with the owned-elsewhere exit code, before armSeed writes a seed.
func TestArmAt_RunOwnedElsewhereWritesNoSeed(t *testing.T) {
	t.Parallel()

	const slug = "owned-elsewhere"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	owners := boardengine.Owners{Prime: "here", Entries: map[string]boardengine.OwnerEntry{slug: {Owner: "there", IsTask: true}}}
	fake := &fakeOwners{machine: "here", owners: owners}
	c := &battenCLI{owner: fake.seams()}
	run := &cobra.Command{Use: "run", PersistentPreRunE: c.resolvePersistentPreRun, RunE: func(*cobra.Command, []string) error { return nil }}

	var out bytes.Buffer
	exitCode := clihelp.ExecuteIn(run, h.Location.WorktreePath(), &out, []string{slug})

	if exitCode != dobbyengine.PassExitOwnedElsewhere {
		t.Errorf("run exit code = %d; want %d", exitCode, dobbyengine.PassExitOwnedElsewhere)
	}
	for _, want := range []string{"there", "lyx adopt " + slug} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("run output = %q; want it to carry %q", out.String(), want)
		}
	}
	if _, found, readErr := shedrun.ReadSeed(h.Location, slug); readErr != nil || found {
		t.Errorf("ReadSeed(%q) = (found %v, err %v); want no seed after the refusal", slug, found, readErr)
	}
}
