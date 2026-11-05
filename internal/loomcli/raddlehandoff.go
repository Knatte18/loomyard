// raddlehandoff.go holds the hand-off line Beam-Up mails a beam's orch when a raddle run filed the beam's project.

package loomcli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// raddleHandOff reads the raddle store of the hub of location and, when a stored proposal filed the beam's project, returns the line that hands the beam's orch the Sketch and the proposal.
// found is false when no stored proposal filed project.
func raddleHandOff(location *lymcwd.Location, project string) (line string, found bool, err error) {
	store, err := boardengine.ReadRaddleStore(fabricengine.BoardDir(location.HubPath))
	if err != nil {
		return "", false, err
	}
	entry, held := store.Holds(project)
	if !held || entry.Project != project {
		return "", false, nil
	}
	return fmt.Sprintf("this beam was filed by a raddle run from the Sketch %s: read the Sketch's decision record with `lym board get %s --record` and the approved proposal with `lym board raddle %s`", entry.Sketch, project, project), true, nil
}
