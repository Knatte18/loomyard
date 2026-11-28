// judgesnapshot.go records the state of both checkouts and the scratch directory before the judge runs.
// It also compares that state with the current one to name what the judge session changed.

package driftengine

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/state"
)

const (
	// judgeSnapshotFileName is the snapshot record's file name under the snapshot directory.
	judgeSnapshotFileName = "snapshot.json"
	// judgeSnapshotUntrackedDir holds the copies of the untracked files, one subdirectory per checkout.
	judgeSnapshotUntrackedDir = "untracked"
	// judgeSnapshotRecordCopy is the copy of the decision record as the records checkout held it, beside the record.
	judgeSnapshotRecordCopy = "decision-record.md"
	// absentHash is the content hash recorded for a path that is not on disk.
	absentHash = "absent"
)

// CheckoutSnapshot is what one checkout held when the snapshot was taken.
type CheckoutSnapshot struct {
	// Root is the checkout's absolute directory.
	Root string `json:"root"`
	// Head is the checkout's HEAD commit.
	Head string `json:"head"`
	// Changed maps each changed tracked path to the hash of its content, or to the absent marker for a deleted path.
	Changed map[string]string `json:"changed,omitempty"`
	// Untracked maps each untracked path to the hash of its content.
	Untracked map[string]string `json:"untracked,omitempty"`
}

// JudgeSnapshot is the state of both checkouts and the current capture's scratch directory when the judge was spawned.
type JudgeSnapshot struct {
	// Code is the state of the code checkout when the judge was spawned.
	Code CheckoutSnapshot `json:"code"`
	// Records is the state of the records checkout when the judge was spawned.
	Records CheckoutSnapshot `json:"records"`
	// ScratchDir is the current capture's scratch directory.
	ScratchDir string `json:"scratch_dir,omitempty"`
	// Scratch maps each file of the scratch directory, by slash-separated path, to the hash of its content.
	Scratch map[string]string `json:"scratch,omitempty"`
}

// GuardChange is one change the judge snapshot names.
type GuardChange struct {
	// Text names the change, such as "code: a.go was changed".
	Text string
	// Path is the changed file's absolute path, and empty for a moved HEAD or a deleted path.
	Path string
}

// lockDecisionRecord takes the run's decision lock when Deps.DecisionRecord and Deps.LockDecisionRecord are set,
// so no read of the record sees a decision verb's write before its commit.
// It returns the release, which does nothing when no lock was taken.
func lockDecisionRecord(d Deps) (func(), error) {
	if d.DecisionRecord == "" || d.LockDecisionRecord == nil {
		return func() {}, nil
	}
	release, err := d.LockDecisionRecord()
	if err != nil {
		return nil, fmt.Errorf("drift: take the run's decision lock: %w", err)
	}
	return func() { _ = release() }, nil
}

// contentHash returns the sha256 of the file at path, or the absent marker when the file does not exist.
func contentHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return absentHash, nil
	}
	if err != nil {
		return "", fmt.Errorf("drift: hash %s: %w", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// hashDirectory maps each regular file under dir, by slash-separated relative path, to the hash of its content.
func hashDirectory(dir string) (map[string]string, error) {
	hashes := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		relative, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		hash, err := contentHash(path)
		if err != nil {
			return err
		}
		hashes[filepath.ToSlash(relative)] = hash
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("drift: hash the directory %s: %w", dir, err)
	}
	return hashes, nil
}

// snapshotCheckout records the checkout's HEAD and the content of its changed and untracked paths.
// It copies each untracked file under copyDir.
func snapshotCheckout(checkout fabricengine.CheckoutState, copyDir string) (CheckoutSnapshot, error) {
	snapshot := CheckoutSnapshot{Root: checkout.Root, Head: checkout.Head, Changed: map[string]string{}, Untracked: map[string]string{}}
	for _, rel := range checkout.Changed {
		hash, err := contentHash(filepath.Join(checkout.Root, filepath.FromSlash(rel)))
		if err != nil {
			return CheckoutSnapshot{}, err
		}
		snapshot.Changed[rel] = hash
	}
	for _, rel := range checkout.Untracked {
		source := filepath.Join(checkout.Root, filepath.FromSlash(rel))
		hash, err := contentHash(source)
		if err != nil {
			return CheckoutSnapshot{}, err
		}
		snapshot.Untracked[rel] = hash
		if hash == absentHash {
			continue
		}
		if err := copyFile(source, filepath.Join(copyDir, filepath.FromSlash(rel))); err != nil {
			return CheckoutSnapshot{}, err
		}
	}
	return snapshot, nil
}

// judgeSnapshotLock returns the lock file of the snapshot record, beside it.
func judgeSnapshotLock(d Deps) string {
	return filepath.Join(d.JudgeSnapshotDir, judgeSnapshotFileName+".lock")
}

// TakeJudgeSnapshot records the state of both checkouts and of scratchDir.
// It replaces any earlier snapshot in Deps.JudgeSnapshotDir.
// The record holds each checkout's HEAD and a content hash of every changed tracked path and every untracked path.
// It also holds the path and hash of every file under scratchDir.
// A copy of each untracked file lies beside the record.
// So does a copy of the decision record Deps.DecisionRecord names when the records checkout holds it.
// It holds the run's decision lock throughout, so the copy is never a decision verb's uncommitted write.
// Only the copies are cleared first.
// The earlier record stays until one atomic write replaces it,
// so a reader never finds the snapshot gone while one is taken.
func TakeJudgeSnapshot(d Deps, scratchDir string) (JudgeSnapshot, error) {
	err := requireSeams("the judge snapshot",
		seam{"Checkouts", d.Checkouts != nil},
		seam{"JudgeSnapshotDir", d.JudgeSnapshotDir != ""},
	)
	if err != nil {
		return JudgeSnapshot{}, err
	}
	release, err := lockDecisionRecord(d)
	if err != nil {
		return JudgeSnapshot{}, err
	}
	defer release()
	code, records, err := d.Checkouts()
	if err != nil {
		return JudgeSnapshot{}, fmt.Errorf("drift: read both checkouts for the judge snapshot: %w", err)
	}
	copies := filepath.Join(d.JudgeSnapshotDir, judgeSnapshotUntrackedDir)
	if err := os.RemoveAll(copies); err != nil {
		return JudgeSnapshot{}, fmt.Errorf("drift: clear the judge snapshot's copies %s: %w", copies, err)
	}
	recordCopy := filepath.Join(d.JudgeSnapshotDir, judgeSnapshotRecordCopy)
	if err := os.Remove(recordCopy); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return JudgeSnapshot{}, fmt.Errorf("drift: clear the judge snapshot's copy of the decision record %s: %w", recordCopy, err)
	}

	snapshot := JudgeSnapshot{ScratchDir: scratchDir}
	if snapshot.Code, err = snapshotCheckout(code, filepath.Join(copies, "code")); err != nil {
		return JudgeSnapshot{}, err
	}
	if snapshot.Records, err = snapshotCheckout(records, filepath.Join(copies, "records")); err != nil {
		return JudgeSnapshot{}, err
	}
	if d.DecisionRecord != "" {
		source, err := recordsCheckoutPath(d, records.Root, d.DecisionRecord)
		if err != nil {
			return JudgeSnapshot{}, err
		}
		if _, statErr := os.Stat(source); statErr == nil {
			if err := copyFile(source, recordCopy); err != nil {
				return JudgeSnapshot{}, err
			}
		}
	}
	if scratchDir != "" {
		if _, statErr := os.Stat(scratchDir); statErr == nil {
			if snapshot.Scratch, err = hashDirectory(scratchDir); err != nil {
				return JudgeSnapshot{}, err
			}
		}
	}
	if err := state.WriteJSON(filepath.Join(d.JudgeSnapshotDir, judgeSnapshotFileName), judgeSnapshotLock(d), snapshot); err != nil {
		return JudgeSnapshot{}, fmt.Errorf("drift: write the judge snapshot: %w", err)
	}
	return snapshot, nil
}

// ReadJudgeSnapshot reads the judge snapshot record, answering found false when it is gone.
func ReadJudgeSnapshot(d Deps) (JudgeSnapshot, bool, error) {
	if err := requireSeams("the judge snapshot", seam{"JudgeSnapshotDir", d.JudgeSnapshotDir != ""}); err != nil {
		return JudgeSnapshot{}, false, err
	}
	snapshot, found, err := state.ReadJSON[JudgeSnapshot](filepath.Join(d.JudgeSnapshotDir, judgeSnapshotFileName), judgeSnapshotLock(d))
	if err != nil {
		return JudgeSnapshot{}, false, fmt.Errorf("drift: read the judge snapshot: %w", err)
	}
	return snapshot, found, nil
}

// Changes names each change the judge session made.
// It compares both checkouts and the scratch directory with the snapshot, and it sorts the names within each part.
// It names a moved code HEAD and each changed, added or deleted path of either checkout that is not exempt.
// Both tracked and untracked paths count.
// It also names each changed, added or deleted scratch file, and a scratch directory that is gone is left out.
// The exempt paths are the judgement file, which the judge writes, and its timestamped siblings.
// The siblings keep an earlier judge's file when the next judge starts.
// On the records checkout, the exempt paths are what lym writes itself around a session:
// every path under a loom records directory or starting with one of its prefixes, and the drift logs.
// The scope.json and judged.json records are exempt only while both match their newest Drift-Records commit,
// so a hand edit of either is still a change.
// The decision record is exempt when the records checkout holds it committed.
// It must also differ only by entries added after the Discussion.
// The base is its copy in the snapshot,
// or the record as the newest `lym loom decision replace` commit made since the snapshot's records head wrote it.
// Every entry and carry-over block that base held must be kept unchanged.
// The comparison holds the run's decision lock, so a decision verb's write is read only once committed.
// A path is judged by content.
// A path the snapshot recorded is a change only when its content now differs from the recorded content,
// so a later commit of content that was already there is no change.
// A deletion already uncommitted at the snapshot and still absent is no change either.
// A moved records HEAD counts only through the records paths that changed since the snapshot's head.
// Of those, only the paths that are not exempt and not unchanged count.
func (s JudgeSnapshot) Changes(d Deps) ([]GuardChange, error) {
	err := requireSeams("the judge snapshot",
		seam{"Checkouts", d.Checkouts != nil},
		seam{"RecordsChangedSince", d.RecordsChangedSince != nil},
		seam{"TrailerCommit", d.TrailerCommit != nil},
		seam{"FileAt", d.FileAt != nil},
	)
	if err != nil {
		return nil, err
	}
	release, err := lockDecisionRecord(d)
	if err != nil {
		return nil, err
	}
	defer release()
	code, records, err := d.Checkouts()
	if err != nil {
		return nil, fmt.Errorf("drift: read both checkouts for the judge snapshot: %w", err)
	}
	judgementRel, err := worktreeRel(d, d.Paths.Judgement)
	if err != nil {
		return nil, err
	}
	lymWritten, err := lymWrittenRecords(d)
	if err != nil {
		return nil, err
	}
	appendedRecord, err := appendedDecisionRecord(d, records, s.Records.Head)
	if err != nil {
		return nil, err
	}

	var changes []GuardChange
	if code.Head != s.Code.Head {
		changes = append(changes, GuardChange{Text: fmt.Sprintf("code: HEAD moved from %q to %q", s.Code.Head, code.Head)})
	}
	codeChanges, err := checkoutChanges("code", s.Code, code, nil, func(rel string) bool { return isJudgementPath(judgementRel, rel) })
	if err != nil {
		return nil, err
	}
	changes = append(changes, codeChanges...)

	var committed []string
	if records.Head != s.Records.Head && s.Records.Head != "" {
		if committed, err = d.RecordsChangedSince(s.Records.Head); err != nil {
			return nil, fmt.Errorf("drift: list the records paths changed since the snapshot: %w", err)
		}
	}
	recordsExempt := func(rel string) bool {
		return isJudgementPath(judgementRel, rel) || isLoomRecordsPath(lymWritten, rel) || rel == appendedRecord
	}
	recordsChanges, err := checkoutChanges("records", s.Records, records, committed, recordsExempt)
	if err != nil {
		return nil, err
	}
	changes = append(changes, recordsChanges...)

	scratchChanges, err := s.scratchChanges()
	if err != nil {
		return nil, err
	}
	return append(changes, scratchChanges...), nil
}

// worktreeRel returns path relative to the worktree root, slash-separated, the form a checkout's listed paths take.
func worktreeRel(d Deps, path string) (string, error) {
	rel, err := filepath.Rel(d.WorktreeRoot, path)
	if err != nil {
		return "", fmt.Errorf("drift: relate %s to the worktree: %w", path, err)
	}
	return filepath.ToSlash(rel), nil
}

// recordsCheckoutPath returns the file of the records checkout rooted at recordsRoot that path names in the worktree.
func recordsCheckoutPath(d Deps, recordsRoot, path string) (string, error) {
	rel, err := worktreeRel(d, path)
	if err != nil {
		return "", err
	}
	return filepath.Join(recordsRoot, filepath.FromSlash(rel)), nil
}

// appendedDecisionRecord returns the decision record's path relative to the worktree root, or empty.
// It returns the path when the record is committed in the records checkout.
// The record must differ only by entries added after the Discussion from one of two bases.
// One base is the snapshot's copy.
// The other is the record as the newest `lym loom decision replace` commit made after snapshotHead wrote it,
// so a replace that predates the snapshot is no base.
// Every entry and carry-over block the base held must be kept unchanged.
// It returns empty when Deps.DecisionRecord is unset, neither base exists or the record is gone.
// It also returns empty when the checkout lists the record as changed or untracked, which names an uncommitted write.
// It also returns empty when the record differs in any other way.
// Carry-over blocks the record gained are left out of the comparison, since lym settles them.
func appendedDecisionRecord(d Deps, records fabricengine.CheckoutState, snapshotHead string) (string, error) {
	if d.DecisionRecord == "" {
		return "", nil
	}
	rel, err := worktreeRel(d, d.DecisionRecord)
	if err != nil {
		return "", err
	}
	if slices.Contains(records.Changed, rel) || slices.Contains(records.Untracked, rel) {
		return "", nil
	}
	current := filepath.Join(records.Root, filepath.FromSlash(rel))
	now, err := os.ReadFile(current)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("drift: read the decision record %s: %w", current, err)
	}
	appendedTo := func(base []byte) bool {
		return bytes.Equal(discussionparser.IdentityText(base), discussionparser.IdentityText(now)) &&
			len(discussionparser.CheckAddedEntriesSubset(now, base)) == 0 &&
			len(discussionparser.CheckCarryOversNotLost(base, now)) == 0
	}

	before, err := os.ReadFile(filepath.Join(d.JudgeSnapshotDir, judgeSnapshotRecordCopy))
	switch {
	case err == nil && appendedTo(before):
		return rel, nil
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return "", fmt.Errorf("drift: read the judge snapshot's copy of the decision record: %w", err)
	}
	if d.ReplacedDecisionRecord == nil {
		return "", nil
	}
	replaced, found, err := d.ReplacedDecisionRecord(snapshotHead)
	if err != nil {
		return "", fmt.Errorf("drift: read the decision record as its newest replace wrote it: %w", err)
	}
	if found && appendedTo(replaced) {
		return rel, nil
	}
	return "", nil
}

// lymWrittenRecords returns the records paths lym writes itself around a judge session.
// They are the loom records entries and the drift logs.
// The scope and judged records join them while both match their newest Drift-Records commit.
// Every lym write of a guarded record lands in a Drift-Records commit,
// so a guarded record that differs from that commit is no write of lym's and is left out.
func lymWrittenRecords(d Deps) ([]string, error) {
	logs, err := worktreeRel(d, d.Paths.LogsDir)
	if err != nil {
		return nil, err
	}
	written := append(slices.Clone(d.LoomRecordsDirs), logs)

	records, err := ReadRecords(d)
	if err != nil {
		return nil, err
	}
	if !records.Committed || records.Mismatch {
		return written, nil
	}
	for _, guarded := range []string{d.Paths.Scope, d.Paths.Judged} {
		rel, err := worktreeRel(d, guarded)
		if err != nil {
			return nil, err
		}
		written = append(written, rel)
	}
	return written, nil
}

// isJudgementPath reports whether rel is the judgement file or a timestamped sibling of it.
// A sibling shares the judgement file's directory and extension.
func isJudgementPath(judgementRel, rel string) bool {
	if rel == judgementRel {
		return true
	}
	extension := path.Ext(judgementRel)
	if path.Dir(rel) != path.Dir(judgementRel) || path.Ext(rel) != extension {
		return false
	}
	archivedFrom, archived := shedadapters.ArchivedRunDirSegment(strings.TrimSuffix(path.Base(rel), extension))
	return archived && archivedFrom == strings.TrimSuffix(path.Base(judgementRel), extension)
}

// isLoomRecordsPath reports whether rel lies under a loomRecordsDirs directory or starts with one of its `-` prefixes.
func isLoomRecordsPath(loomRecordsDirs []string, rel string) bool {
	for _, entry := range loomRecordsDirs {
		if strings.HasSuffix(entry, "-") {
			if strings.HasPrefix(rel, entry) {
				return true
			}
			continue
		}
		if rel == entry || strings.HasPrefix(rel, entry+"/") {
			return true
		}
	}
	return false
}

// checkoutChanges names each path of one checkout the judge changed, judged by content against the snapshot.
// committed holds paths a later commit changed, which the checkout's own lists no longer show.
func checkoutChanges(label string, snapshot CheckoutSnapshot, now fabricengine.CheckoutState, committed []string, exempt func(rel string) bool) ([]GuardChange, error) {
	candidates := map[string]bool{}
	for _, group := range [][]string{now.Changed, now.Untracked, committed} {
		for _, rel := range group {
			candidates[rel] = true
		}
	}
	for _, group := range []map[string]string{snapshot.Changed, snapshot.Untracked} {
		for rel := range group {
			candidates[rel] = true
		}
	}

	var rels []string
	for rel := range candidates {
		if !exempt(rel) {
			rels = append(rels, rel)
		}
	}
	sort.Strings(rels)

	var changes []GuardChange
	for _, rel := range rels {
		path := filepath.Join(now.Root, filepath.FromSlash(rel))
		hash, err := contentHash(path)
		if err != nil {
			return nil, err
		}
		recorded, wasRecorded := snapshot.Changed[rel]
		if untracked, ok := snapshot.Untracked[rel]; ok {
			recorded, wasRecorded = untracked, true
		}
		switch {
		case wasRecorded && hash == recorded:
		case hash == absentHash:
			changes = append(changes, GuardChange{Text: fmt.Sprintf("%s: %s was deleted", label, rel)})
		case !wasRecorded && slices.Contains(now.Untracked, rel):
			changes = append(changes, GuardChange{Text: fmt.Sprintf("%s: %s was added", label, rel), Path: path})
		default:
			changes = append(changes, GuardChange{Text: fmt.Sprintf("%s: %s was changed", label, rel), Path: path})
		}
	}
	return changes, nil
}

// scratchChanges names each file of the scratch directory that was changed, added or deleted since the snapshot.
// It names none when the directory is gone.
func (s JudgeSnapshot) scratchChanges() ([]GuardChange, error) {
	if s.ScratchDir == "" {
		return nil, nil
	}
	if _, err := os.Stat(s.ScratchDir); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	now, err := hashDirectory(s.ScratchDir)
	if err != nil {
		return nil, err
	}
	var changes []GuardChange
	for rel, hash := range now {
		path := filepath.Join(s.ScratchDir, filepath.FromSlash(rel))
		recorded, ok := s.Scratch[rel]
		switch {
		case !ok:
			changes = append(changes, GuardChange{Text: fmt.Sprintf("scratch: %s was added", rel), Path: path})
		case recorded != hash:
			changes = append(changes, GuardChange{Text: fmt.Sprintf("scratch: %s was changed", rel), Path: path})
		}
	}
	for rel := range s.Scratch {
		if _, ok := now[rel]; !ok {
			changes = append(changes, GuardChange{Text: fmt.Sprintf("scratch: %s was deleted", rel)})
		}
	}
	slices.SortFunc(changes, func(a, b GuardChange) int { return strings.Compare(a.Text, b.Text) })
	return changes, nil
}
