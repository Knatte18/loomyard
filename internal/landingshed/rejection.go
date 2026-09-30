// rejection.go — the operator-rejection record.
//
// `lyx loom reject` writes the record and the PR gate reads it, both through Rejection, beside the approval record.
// The path is always told; this package derives none.

package landingshed

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// Rejection is the operator's recorded rejection of one pull request at one head commit.
type Rejection struct {
	// PRNumber is the rejected pull request's number.
	PRNumber int `json:"pr_number"`
	// HeadSHA is the head commit the operator rejected.
	HeadSHA string `json:"head_sha"`
	// RejectedAt is the rejection time, RFC 3339 UTC.
	RejectedAt string `json:"rejected_at"`
	// Findings is the operator's review findings text.
	Findings string `json:"findings"`
}

// WriteRejection writes r to path, creating the parent directory and replacing any earlier record atomically, so a reader never sees a partial file.
func WriteRejection(path string, r Rejection) error {
	return writeRecordAtomic(path, "rejection", r)
}

// ReadRejection reads the record at path.
// An absent file returns found == false with a nil error;
// unreadable or undecodable content, or a record missing any field, is an error.
// Blank findings count as missing.
func ReadRejection(path string) (Rejection, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Rejection{}, false, nil
	}
	if err != nil {
		return Rejection{}, false, fmt.Errorf("landingshed: read rejection: %w", err)
	}
	var r Rejection
	if err := json.Unmarshal(data, &r); err != nil {
		return Rejection{}, false, fmt.Errorf("landingshed: decode rejection %s: %w", path, err)
	}
	if r.PRNumber == 0 || r.HeadSHA == "" || r.RejectedAt == "" || strings.TrimSpace(r.Findings) == "" {
		return Rejection{}, false, fmt.Errorf("landingshed: rejection %s is missing a field", path)
	}
	return r, true, nil
}

// RemoveRecord deletes the decision record at path; an absent file is success.
func RemoveRecord(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("landingshed: remove decision record: %w", err)
	}
	return nil
}
