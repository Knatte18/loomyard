// requiredseam.go holds the seams fabric needs supplied from outside and the rule that refuses a run lacking one.

package fabriccore

import "strings"

// RequiredSeam names a seam fabric needs to keep its behaviour, supplied to the process from outside fabric.
type RequiredSeam string

const (
	// SeamRunRecordPaths is where the records of runs sit.
	SeamRunRecordPaths RequiredSeam = "run_record_paths"
	// SeamRunLocks is the locks a run is held by.
	SeamRunLocks RequiredSeam = "run_locks"
	// SeamStatusCodec is the reading of a run's status record from its bytes.
	SeamStatusCodec RequiredSeam = "status_codec"
)

// RefuseUnsupplied refuses a run that would use a seam of needed that the snapshot does not report supplied.
// The refusal names every such seam in the order needed lists them.
// It returns nil when each is supplied, and a nil Snapshot.Seams reads every seam as unsupplied.
func RefuseUnsupplied(snapshot Snapshot, needed ...RequiredSeam) *Refusal {
	var missing []string
	for _, seam := range needed {
		if !snapshot.Seams[seam] {
			missing = append(missing, string(seam))
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return &Refusal{
		Reason: "fabric's required seams are not supplied: " + strings.Join(missing, ", "),
		// The lym binary supplies the seams at start, so only a test binary meets this refusal.
		WayForward: "way forward: build the hub through internal/hubforge, which installs the seams once per test binary, " +
			"or call hubforge.InstallRequiredSeams before the verb",
	}
}
