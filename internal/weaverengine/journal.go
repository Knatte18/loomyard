// journal.go names the files each Weaver strand writes, and appends the run journal.
// An attempt's report path and a fixer's reply path fix the strand's identity on disk before it spawns.

package weaverengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/runplan"
)

// AttemptReportPath returns the report file of one batch attempt, <reportsDir>/NN-<slug>-a<attempt>.yaml.
// It is unique to the batch and the attempt number, so every attempt's strand has its own output file.
func AttemptReportPath(reportsDir string, number int, slug string, attempt int) string {
	base := runplan.ReportFileName(number, slug)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	return filepath.Join(reportsDir, fmt.Sprintf("%s-a%d%s", stem, attempt, filepath.Ext(base)))
}

// FixerReplyPath returns the reply file of verify fixer number fixer, <reportsDir>/fix-<n>.md.
// It is the fixer's only output file.
func FixerReplyPath(reportsDir string, fixer int) string {
	return filepath.Join(reportsDir, fmt.Sprintf("fix-%d.md", fixer))
}

// JournalHeading returns the journal heading of one attempt of batch, `## Batch <NN-slug>, attempt <n>`.
func JournalHeading(batch string, attempt int) string {
	return fmt.Sprintf("## Batch %s, attempt %d", batch, attempt)
}

// AppendJournal appends heading and text to the journal at path, creating the file.
// It does nothing when the file already holds the heading as a line.
// Recording a report again after a crash therefore adds no second entry.
// An empty text appends the heading alone.
// It writes through a temp file and a rename, so a crash never leaves half an entry.
func AppendJournal(path, heading, text string) error {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("weaver: read journal %s: %w", path, err)
	}
	for _, line := range strings.Split(string(existing), "\n") {
		if line == heading {
			return nil
		}
	}

	entry := heading + "\n"
	if text != "" {
		entry += "\n" + strings.TrimRight(text, "\n") + "\n"
	}
	content := string(existing)
	if content != "" {
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += "\n"
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("weaver: create journal dir %s: %w", dir, err)
	}
	if err := runplan.WriteFileAtomic(dir, path, []byte(content+entry)); err != nil {
		return fmt.Errorf("weaver: write journal %s: %w", path, err)
	}
	return nil
}
