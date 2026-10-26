// landrequest.go holds the pawl run's file records: the names of the files under the beam's `.lyx/beam/` directory, Paths, and the land request with its write, read and remove.

package pawlengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/state"
)

const (
	// LandRequestFileName is the file name of the land request under the beam's `.lyx/beam/` directory.
	LandRequestFileName = "land-request.json"
	// DobbyStopMarkerName is the file name of the marker that says the landing stopped the beam's dobby.
	DobbyStopMarkerName = "dobby-stop"
	// AwaitMarkerName is the file name of the marker that says Beam-Await was entered before and whether its dobby-down notice was posted.
	AwaitMarkerName = "await-entered"
)

// Paths holds the absolute paths of the pawl run's files, which the hub wiring composes and tells.
type Paths struct {
	// LandRequest is the land request file.
	LandRequest string
	// DobbyStopMarker is the marker file the landing writes before it stops the dobby.
	DobbyStopMarker string
	// AwaitMarker is the marker file Beam-Await writes on its first entry; it also carries the line recording a posted dobby-down notice.
	AwaitMarker string
}

// LandRequest asks the pawl run to land its beam.
// It names the run it was written for, so a request left by an earlier run is recognised and dropped.
type LandRequest struct {
	// RunID is the seed id of the pawl run the request is for.
	RunID string `json:"run_id"`
	// RequestedAt is when the request was written.
	RequestedAt time.Time `json:"requested_at"`
}

// WriteLandRequest writes req to the land request file, replacing a request already there.
func WriteLandRequest(paths Paths, req LandRequest) error {
	if err := state.WriteJSON(paths.LandRequest, lockPath(paths.LandRequest), req); err != nil {
		return fmt.Errorf("pawlengine: write the land request: %w", err)
	}
	return nil
}

// ReadLandRequest reads the land request file and reports whether one stands.
// An absent file reads as none.
func ReadLandRequest(paths Paths) (LandRequest, bool, error) {
	req, found, err := state.ReadJSON[LandRequest](paths.LandRequest, lockPath(paths.LandRequest))
	if err != nil {
		return LandRequest{}, false, fmt.Errorf("pawlengine: read the land request: %w", err)
	}
	return req, found, nil
}

// RemoveLandRequest removes the land request file; removing an absent one succeeds.
func RemoveLandRequest(paths Paths) error {
	return removeFile(paths.LandRequest)
}

// lockPath is the lock file that guards the record at path.
func lockPath(path string) string { return path + ".lock" }

// removeFile removes the file at path and treats an absent one as removed.
func removeFile(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("pawlengine: remove %s: %w", path, err)
	}
	return nil
}

// writeMarker writes the marker file at path, creating its directory.
func writeMarker(path string, now time.Time) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("pawlengine: create the marker directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(now.UTC().Format(time.RFC3339)+"\n"), 0o644); err != nil {
		return fmt.Errorf("pawlengine: write %s: %w", path, err)
	}
	return nil
}

// markerStands reports whether the marker file at path exists.
func markerStands(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("pawlengine: stat %s: %w", path, err)
}

// downNoticeLine is the line the await marker carries once the dobby-down notice is posted for the current episode.
const downNoticeLine = "dobby-down-notified"

// downNoticeRecorded reports whether the await marker at path carries the sent dobby-down notice.
func downNoticeRecorded(path string) (bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("pawlengine: read %s: %w", path, err)
	}
	return strings.Contains(string(content), downNoticeLine), nil
}

// recordDownNotice rewrites the await marker at path with the sent dobby-down notice.
func recordDownNotice(path string, now time.Time) error {
	content := now.UTC().Format(time.RFC3339) + "\n" + downNoticeLine + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("pawlengine: write %s: %w", path, err)
	}
	return nil
}
