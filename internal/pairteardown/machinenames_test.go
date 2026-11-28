// machinenames_test.go pins the names machineNamesOf says read as this machine under a board's registry.

package pairteardown

import (
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestMachineNamesOf(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name     string
		here     machinename.Identity
		registry boardengine.Registry
		want     fabricengine.MachineNames
	}{
		{
			name: "NoEntryReadsTheFilesNames",
			here: machinename.Identity{ID: "id1", Name: "new", Former: []string{"old"}},
			want: fabricengine.MachineNames{Current: "new", Former: []string{"old"}},
		},
		{
			name:     "AnEntryHoldingAFormerNameTheFileLacksAddsIt",
			here:     machinename.Identity{ID: "id1", Name: "new", Former: []string{"old"}},
			registry: boardengine.Registry{"id1": {Name: "new", Former: []string{"older", "old"}}},
			want:     fabricengine.MachineNames{Current: "new", Former: []string{"old", "older"}},
		},
		{
			name:     "AnEntryOfAnotherIdAddsNothing",
			here:     machinename.Identity{ID: "id1", Name: "new", Former: []string{"old"}},
			registry: boardengine.Registry{"id2": {Name: "other", Former: []string{"elsewhere"}}},
			want:     fabricengine.MachineNames{Current: "new", Former: []string{"old"}},
		},
		{
			name:     "AFormerNameOfTheFileAnotherEntryHoldsAsItsCurrentNameIsLeftOut",
			here:     machinename.Identity{Name: "new", Former: []string{"old", "taken"}},
			registry: boardengine.Registry{"id2": {Name: "taken"}},
			want:     fabricengine.MachineNames{Current: "new", Former: []string{"old"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := machineNamesOf(tt.here, tt.registry); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("machineNamesOf() = %+v; want %+v", got, tt.want)
			}
		})
	}
}
