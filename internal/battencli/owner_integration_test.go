//go:build integration

// owner_integration_test.go drives armAt over a real hub, the one owner check that needs the prime-name refusal ahead of it.

package battencli

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestArmAt_RunOwnedElsewhereWritesNoSeed asserts a first "lyx batten run" of a task another machine owns is refused naming the owner and "lyx adopt", before armSeed writes a seed.
func TestArmAt_RunOwnedElsewhereWritesNoSeed(t *testing.T) {
	t.Parallel()

	const slug = "owned-elsewhere"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	owners := boardengine.Owners{Prime: "here", Entries: map[string]boardengine.OwnerEntry{slug: {Owner: "there", IsTask: true}}}
	fake := &fakeOwners{machine: "here", owners: owners}
	c := &battenCLI{owner: fake.seams()}

	_, err := c.armAt(h.Location, slug, true, "run")

	if err == nil {
		t.Fatal("armAt(run) of a task owned elsewhere = nil; want a refusal")
	}
	for _, want := range []string{"there", "lyx adopt " + slug} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("armAt(run) error = %q; want it to carry %q", err.Error(), want)
		}
	}
	if _, found, readErr := shedrun.ReadSeed(h.Location, slug); readErr != nil || found {
		t.Errorf("ReadSeed(%q) = (found %v, err %v); want no seed after the refusal", slug, found, readErr)
	}
}
