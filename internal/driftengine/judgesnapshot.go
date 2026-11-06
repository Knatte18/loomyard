// judgesnapshot.go — the snapshot of both checkouts and the current capture's scratch directory taken before the judge runs, and the comparison that names what the judge session changed.

package driftengine

import (
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

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/state"
)

const (
	// judgeSnapshotFileName is the snapshot record's file name under the snapshot directory.
	judgeSnapshotFileName = "snapshot.json"
	// judgeSnapshotUntrackedDir holds the copies of the untracked files, one subdirectory per checkout.
	judgeSnapshotUntrackedDir = "untracked"
	// absentHash is the content hash recorded for a path that is not on disk.
	absentHash = "absent"
)

// CheckoutSnapshot is what one checkout held when the snapshot was taken.
type CheckoutSnapshot struct {
	// Root is the checkout's absolute directory.
	Root string `json:"root"`
	// Head is the checkout's HEAD commit.
	Head string `json:"head"`
	// Changed maps each changed tracked path to the hash of its content, absentHash for a deleted one.
	Changed map[string]string `json:"changed,omitempty"`
	// Untracked maps each untracked path to the hash of its content.
	Untracked map[string]string `json:"untracked,omitempty"`
}

// JudgeSnapshot is the state of both checkouts and of the current capture's scratch directory at the moment the judge was spawned.
type JudgeSnapshot struct {
	// Code is the code checkout.
	Code CheckoutSnapshot `json:"code"`
	// Records is the records checkout.
	Records CheckoutSnapshot `json:"records"`
	// ScratchDir is the current capture's scratch directory.
	ScratchDir string `json:"scratch_dir,omitempty"`
	// Scratch maps each file of the scratch directory, by slash-separated path, to the hash of its content.
	Scratch map[string]string `json:"scratch,omitempty"`
}

// contentHash returns the sha256 of the file at path, and absentHash when it does not exist.
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

// snapshotCheckout records the checkout's HEAD and the content of its changed and untracked paths, copying each untracked file under copyDir.
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

// TakeJudgeSnapshot records the state of both checkouts and of scratchDir under Deps.JudgeSnapshotDir, replacing an earlier snapshot.
// The record holds each checkout's HEAD, a content hash of every changed tracked path and of every untracked path, and the path and hash of every file under scratchDir;
// a copy of each untracked file lies beside the record.
// Only the copies are cleared first: the earlier record stays until one atomic write replaces it, so a reader never finds the snapshot gone while one is taken.
func TakeJudgeSnapshot(d Deps, scratchDir string) (JudgeSnapshot, error) {
	err := requireSeams("the judge snapshot",
		seam{"Checkouts", d.Checkouts != nil},
		seam{"JudgeSnapshotDir", d.JudgeSnapshotDir != ""},
	)
	if err != nil {
		return JudgeSnapshot{}, err
	}
	code, records, err := d.Checkouts()
	if err != nil {
		return JudgeSnapshot{}, fmt.Errorf("drift: read both checkouts for the judge snapshot: %w", err)
	}
	copies := filepath.Join(d.JudgeSnapshotDir, judgeSnapshotUntrackedDir)
	if err := os.RemoveAll(copies); err != nil {
		return JudgeSnapshot{}, fmt.Errorf("drift: clear the judge snapshot's copies %s: %w", copies, err)
	}

	snapshot := JudgeSnapshot{ScratchDir: scratchDir}
	if snapshot.Code, err = snapshotCheckout(code, filepath.Join(copies, "code")); err != nil {
		return JudgeSnapshot{}, err
	}
	if snapshot.Records, err = snapshotCheckout(records, filepath.Join(copies, "records")); err != nil {
		return JudgeSnapshot{}, err
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

// ReadJudgeSnapshot reads the snapshot TakeJudgeSnapshot recorded, answering found false when it is gone.
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

// Changes compares both checkouts and the scratch directory with the snapshot and names each change the judge session made, sorted within each part.
// It names a moved code HEAD, and each changed, added or deleted tracked or untracked path of either checkout outside the exempt ones, and each changed, added or deleted scratch file; a scratch directory that is gone is left out.
// The exempt paths are judgement.yaml, which the judge writes, its timestamped siblings, which keep an earlier judge's file when the next judge starts, and, on the records checkout, what lym writes itself around a session:
// every path under a Deps.LoomRecordsDirs directory or starting with one of its prefixes, the drift logs, and scope.json and judged.json while both hold what their newest Drift-Records commit wrote, so a hand edit of either is still a change.
// A path is judged by content: one the snapshot recorded is a change only when its content now differs from the recorded one, so a later commit of content that was already there, and a deletion that was already uncommitted and is still absent, are no change.
// A moved records HEAD is a change only through the paths Deps.RecordsChangedSince names since the snapshot's head, minus the exempt and the unchanged.
func (s JudgeSnapshot) Changes(d Deps) ([]string, error) {
	err := requireSeams("the judge snapshot",
		seam{"Checkouts", d.Checkouts != nil},
		seam{"RecordsChangedSince", d.RecordsChangedSince != nil},
		seam{"TrailerCommit", d.TrailerCommit != nil},
		seam{"FileAt", d.FileAt != nil},
	)
	if err != nil {
		return nil, err
	}
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

	var changes []string
	if code.Head != s.Code.Head {
		changes = append(changes, fmt.Sprintf("code: HEAD moved from %q to %q", s.Code.Head, code.Head))
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
		return isJudgementPath(judgementRel, rel) || isLoomRecordsPath(lymWritten, rel)
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

// lymWrittenRecords returns the records paths lym writes itself around a judge session, in the form of Deps.LoomRecordsDirs: those entries, the drift logs and, while both match their newest Drift-Records commit, the two guarded records.
// Every lym write of a guarded record lands in such a commit, so a guarded record that differs from it is no write of lym's and is left out.
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

// isJudgementPath reports whether rel is the judgement file at judgementRel or a timestamped sibling of it, in the same directory with the same extension.
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

// isLoomRecordsPath reports whether rel lies under a directory of loomRecordsDirs or starts with one of its entries that ends in `-`.
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
func checkoutChanges(label string, snapshot CheckoutSnapshot, now fabricengine.CheckoutState, committed []string, exempt func(rel string) bool) ([]string, error) {
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

	var changes []string
	for _, rel := range rels {
		hash, err := contentHash(filepath.Join(now.Root, filepath.FromSlash(rel)))
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
			changes = append(changes, fmt.Sprintf("%s: %s was deleted", label, rel))
		case !wasRecorded && slices.Contains(now.Untracked, rel):
			changes = append(changes, fmt.Sprintf("%s: %s was added", label, rel))
		default:
			changes = append(changes, fmt.Sprintf("%s: %s was changed", label, rel))
		}
	}
	return changes, nil
}

// scratchChanges names each file of the snapshot's scratch directory that was changed, added or deleted since, and none when the directory is gone.
func (s JudgeSnapshot) scratchChanges() ([]string, error) {
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
	var changes []string
	for rel, hash := range now {
		recorded, ok := s.Scratch[rel]
		switch {
		case !ok:
			changes = append(changes, fmt.Sprintf("scratch: %s was added", rel))
		case recorded != hash:
			changes = append(changes, fmt.Sprintf("scratch: %s was changed", rel))
		}
	}
	for rel := range s.Scratch {
		if _, ok := now[rel]; !ok {
			changes = append(changes, fmt.Sprintf("scratch: %s was deleted", rel))
		}
	}
	sort.Strings(changes)
	return changes, nil
}
