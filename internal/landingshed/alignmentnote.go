// alignmentnote.go — the alignment note: the record PR-Align leaves when it applies no approval or rejection, so PR-Gate can name the agent's report to the run's parent.
// The path is always told; this package derives none.

package landingshed

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// AlignmentNote says why PR-Align left a pull request to the run's parent, for one pull request at one head commit.
type AlignmentNote struct {
	// PRNumber is the pull request the note concerns.
	PRNumber int `json:"pr_number"`
	// HeadSHA is the pull request head the note concerns.
	HeadSHA string `json:"head_sha"`
	// Report is the path of the agent's report, empty when it wrote none.
	Report string `json:"report"`
	// Reason says why the review was not settled.
	Reason string `json:"reason"`
}

// WriteAlignmentNote writes n to path, creating the parent directory and replacing any earlier note atomically.
func WriteAlignmentNote(path string, n AlignmentNote) error {
	return writeRecordAtomic(path, "alignment-note", n)
}

// ReadAlignmentNote reads the note at path.
// An absent file returns found == false with a nil error; unreadable or undecodable content is an error.
func ReadAlignmentNote(path string) (AlignmentNote, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return AlignmentNote{}, false, nil
	}
	if err != nil {
		return AlignmentNote{}, false, fmt.Errorf("landingshed: read alignment note: %w", err)
	}
	var n AlignmentNote
	if err := json.Unmarshal(data, &n); err != nil {
		return AlignmentNote{}, false, fmt.Errorf("landingshed: decode alignment note %s: %w", path, err)
	}
	return n, true, nil
}
