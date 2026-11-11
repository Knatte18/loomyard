// recordfile.go reads an upsert's --record-file, a decision record.
// The record must pass the record's format checks before anything is written.

package boardcli

import (
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/discussionparser"
)

// readRecordFile returns the decision record in the file at path.
// It refuses a file it cannot read, naming the path,
// and a record that fails the section check or the decision-id and acceptance check, listing every finding.
func readRecordFile(path string) ([]byte, error) {
	record, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read --record-file %s: %w; nothing was written; way forward: pass the path of a readable decision record", path, err)
	}
	findings := append(discussionparser.CheckRecordSections(record), discussionparser.CheckDecisionIDs(record)...)
	if len(findings) == 0 {
		return record, nil
	}
	details := make([]string, len(findings))
	for i, finding := range findings {
		details[i] = finding.Error()
	}
	return nil, fmt.Errorf("--record-file %s fails the decision record's format checks: %s; nothing was written; way forward: fix each finding in the file, then re-run the upsert",
		path, strings.Join(details, "; "))
}
