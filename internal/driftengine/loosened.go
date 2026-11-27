// loosened.go lists the files of the task's review range and reuses or collects the judge's rulings on them.
// It also words the halt and the verdict lines for a loosened check with no measured cause.

package driftengine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
)

// LoosenedCheckTrailerKey is the commit trailer key that names a loosened check and the measured cause behind it.
const LoosenedCheckTrailerKey = "Loosened-Check"

// loosenedTrailerLines returns the Loosened-Check trailer lines of the last paragraph of each message.
// The lines are trimmed, sorted and without duplicates.
func loosenedTrailerLines(messages []string) []string {
	prefix := LoosenedCheckTrailerKey + ":"
	var lines []string
	for _, message := range messages {
		paragraph := strings.TrimSpace(strings.ReplaceAll(message, "\r\n", "\n"))
		if cut := strings.LastIndex(paragraph, "\n\n"); cut >= 0 {
			paragraph = paragraph[cut+2:]
		}
		for _, line := range strings.Split(paragraph, "\n") {
			if line = strings.TrimSpace(line); strings.HasPrefix(line, prefix) {
				lines = append(lines, line)
			}
		}
	}
	slices.Sort(lines)
	return slices.Compact(lines)
}

// fileRulingKey returns the sha256, in hex, of a file's diff, a NUL and the trailer lines joined by newlines.
// A stored file ruling is reused only while its key is unchanged.
func fileRulingKey(diff string, trailerLines []string) string {
	sum := sha256.Sum256([]byte(diff + "\x00" + strings.Join(trailerLines, "\n")))
	return hex.EncodeToString(sum[:])
}

// listedFile is one file of the review range that no stored ruling covers under its current key.
type listedFile struct {
	path string
	key  string
	diff string
}

// listedFiles is what a pass read of the review range.
// It holds the files the judge still rules on, the stored rulings that cover the other files,
// and the range's commit messages.
type listedFiles struct {
	unruled  []listedFile
	reused   []JudgedFile
	messages []string
}

// listFiles reads the review range's changed files and commit messages and splits the files by their stored rulings.
// A run whose review base is not told has no review range and lists no file.
func (p *DriftVerify) listFiles(rec Records) (listedFiles, error) {
	d := p.deps
	if d.ReviewBase == nil {
		return listedFiles{}, nil
	}
	base, err := d.ReviewBase()
	if err != nil {
		return listedFiles{}, fmt.Errorf("drift: %s: read the review base: %w", p.name, err)
	}
	files, err := d.ChangedFiles(base)
	if err != nil {
		return listedFiles{}, fmt.Errorf("drift: %s: list the files changed since %s: %w", p.name, base, err)
	}
	messages, err := d.CommitMessages(base)
	if err != nil {
		return listedFiles{}, fmt.Errorf("drift: %s: read the commit messages since %s: %w", p.name, base, err)
	}

	trailers := loosenedTrailerLines(messages)
	listed := listedFiles{messages: messages}
	for _, file := range files {
		key := fileRulingKey(file.Diff, trailers)
		stored := slices.IndexFunc(rec.Judged.Files, func(held JudgedFile) bool { return held.Path == file.Path && held.Key == key })
		if stored >= 0 {
			listed.reused = append(listed.reused, rec.Judged.Files[stored])
			continue
		}
		listed.unruled = append(listed.unruled, listedFile{path: file.Path, key: key, diff: file.Diff})
	}
	return listed, nil
}

// judgeFileID is the id the judge rules on the unruled file at index in the request.
func judgeFileID(index int) string {
	return fmt.Sprintf("f%d", index+1)
}

// judgeFiles returns the unruled files as a judge request lists them.
func (l listedFiles) judgeFiles() []JudgeFile {
	var files []JudgeFile
	for i, file := range l.unruled {
		files = append(files, JudgeFile{ID: judgeFileID(i), Path: file.path, Diff: file.diff})
	}
	return files
}

// loosenedChecksOf returns the loosened checks a ruling on the file at path holds.
func loosenedChecksOf(path string, rulings []LoosenedRuling) []LoosenedCheck {
	checks := make([]LoosenedCheck, len(rulings))
	for i, ruling := range rulings {
		checks[i] = LoosenedCheck{Path: path, Test: ruling.Test, Loosened: ruling.Loosened, Ruling: ruling.Ruling, Cause: ruling.Cause, Reason: ruling.Reason}
	}
	return checks
}

// newRulings returns the file rulings the judge gave the unruled files, each stored with the key of what it judged,
// and the loosened checks those rulings hold.
func (l listedFiles) newRulings(judgement Judgement, strand, at string) ([]JudgedFile, []LoosenedCheck) {
	byID := make(map[string]listedFile, len(l.unruled))
	for i, file := range l.unruled {
		byID[judgeFileID(i)] = file
	}
	var rulings []JudgedFile
	var checks []LoosenedCheck
	for _, ruling := range judgement.Files {
		file := byID[ruling.ID]
		rulings = append(rulings, JudgedFile{Path: file.path, Key: file.key, Ruling: ruling.Ruling, Reason: ruling.Reason, Checks: ruling.Checks, Strand: strand, At: at})
		checks = append(checks, loosenedChecksOf(file.path, ruling.Checks)...)
	}
	return rulings, checks
}

// allChecks returns the loosened checks of the reused rulings followed by fresh, ordered by file path.
func (l listedFiles) allChecks(fresh []LoosenedCheck) []LoosenedCheck {
	var checks []LoosenedCheck
	for _, stored := range l.reused {
		checks = append(checks, loosenedChecksOf(stored.Path, stored.Checks)...)
	}
	checks = append(checks, fresh...)
	slices.SortStableFunc(checks, func(a, b LoosenedCheck) int { return strings.Compare(a.Path, b.Path) })
	return checks
}

// storeFileRulings stores each ruling in judged, replacing an earlier ruling on the same path.
func storeFileRulings(judged *JudgedRecord, rulings []JudgedFile) {
	for _, ruling := range rulings {
		judged.Files = replaceOrAppend(judged.Files, ruling, func(held JudgedFile) bool { return held.Path == ruling.Path })
	}
}

// uncausedChecks returns the checks ruled to have no measured cause.
func uncausedChecks(checks []LoosenedCheck) []LoosenedCheck {
	return slices.DeleteFunc(slices.Clone(checks), func(check LoosenedCheck) bool { return check.Ruling != RulingUncaused })
}

// uncausedReason words the Stuck over loosened checks with no measured cause.
// It names each check and the two ways forward: a trailer naming the cause, or a revert of the loosening.
func uncausedReason(checks []LoosenedCheck) string {
	described := make([]string, len(checks))
	for i, check := range checks {
		described[i] = fmt.Sprintf("%s: %s: %s: %s", check.Path, check.Test, check.Loosened, check.Reason)
	}
	return fmt.Sprintf("a test the change loosened has no measured cause: %s; way forward: commit a \"%s: <test> — <what was loosened> — <measured cause>\" trailer on any commit of the range, with no change if need be, or commit a revert of the loosening, then lym loom resume",
		strings.Join(described, "; "), LoosenedCheckTrailerKey)
}

// haltReasons joins the does-not-match reason, when there is one, and the uncaused reason, when a check is uncaused.
// It returns empty when neither holds.
func haltReasons(noMatch string, uncaused []LoosenedCheck) string {
	var reasons []string
	if noMatch != "" {
		reasons = append(reasons, noMatch)
	}
	if len(uncaused) > 0 {
		reasons = append(reasons, uncausedReason(uncaused))
	}
	return strings.Join(reasons, "; ")
}

// loosenedLines renders each loosened check with its ruling and cause, for the regression check section.
func loosenedLines(checks []LoosenedCheck) []string {
	if len(checks) == 0 {
		return nil
	}
	lines := []string{"- Loosened checks:"}
	for _, check := range checks {
		cause := check.Cause
		if cause == "" {
			cause = "none"
		}
		lines = append(lines, fmt.Sprintf("  - `%s` in `%s`: %s; %s; cause: %s", check.Test, check.Path, check.Loosened, check.Ruling, cause))
	}
	return lines
}
