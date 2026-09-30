// approval.go — the operator-approval record.
//
// `lyx loom approve` writes the record and the PR gate reads it, both through Approval, so the format
// lives in the package that reads it. The path is always told; this package derives none.

package landingshed

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Approval is the operator's recorded approval of one pull request at one head commit.
type Approval struct {
	// PRNumber is the approved pull request's number.
	PRNumber int `json:"pr_number"`
	// HeadSHA is the head commit the operator approved.
	HeadSHA string `json:"head_sha"`
	// ApprovedAt is the approval time, RFC 3339 UTC.
	ApprovedAt string `json:"approved_at"`
}

// WriteApproval writes a to path, creating the parent directory and replacing any earlier record
// atomically, so a reader never sees a partial file.
func WriteApproval(path string, a Approval) error {
	return writeRecordAtomic(path, "approval", a)
}

// writeRecordAtomic encodes v as JSON and writes it to path through a temp file in the same
// directory and a rename, creating the parent directory; kind names the record in error messages.
func writeRecordAtomic(path, kind string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("landingshed: encode %s: %w", kind, err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("landingshed: create %s directory: %w", kind, err)
	}
	tmp, err := os.CreateTemp(dir, "."+kind+"-*.tmp")
	if err != nil {
		return fmt.Errorf("landingshed: create %s temp file: %w", kind, err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("landingshed: write %s: %w", kind, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("landingshed: close %s temp file: %w", kind, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("landingshed: replace %s: %w", kind, err)
	}
	return nil
}

// ReadApproval reads the record at path. An absent file returns found == false with a nil error;
// unreadable or undecodable content, or a record missing any field, is an error.
func ReadApproval(path string) (Approval, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Approval{}, false, nil
	}
	if err != nil {
		return Approval{}, false, fmt.Errorf("landingshed: read approval: %w", err)
	}
	var a Approval
	if err := json.Unmarshal(data, &a); err != nil {
		return Approval{}, false, fmt.Errorf("landingshed: decode approval %s: %w", path, err)
	}
	if a.PRNumber == 0 || a.HeadSHA == "" || a.ApprovedAt == "" {
		return Approval{}, false, fmt.Errorf("landingshed: approval %s is missing a field", path)
	}
	return a, true, nil
}
