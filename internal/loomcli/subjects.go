// subjects.go builds the identities a Bouncer re-entry compares: the document, code and merge-in subjects of a loom or darn run's review segments, the record head and merge-in-only reads that go with them, the review input seams, and the merge base reader of the task's diff.

package loomcli

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// harnessOwnFiles are the worktree-relative files the harness itself writes into a task worktree.
// A code subject leaves each out of the task's patch while it is untracked.
var harnessOwnFiles = []string{".scratch_task.json"}

// planOverviewFile is the plan directory's overview file, whose identity text leaves Approve's flag out.
const planOverviewFile = "00-overview.md"

// planCardFilePattern matches a plan directory's top-level card file.
var planCardFilePattern = regexp.MustCompile(`^\d+-\S+\.md$`)

// documentSubject returns the identity of the artifacts paths name, in order.
// The decision record is hashed through its identity text, so a carry-over block or an entry added after the discussion never changes it.
// A directory is hashed as the plan it holds: its overview through its identity text, so Approve's flag never changes it, then each top-level card file in name order; archived generations, the lens review and every other file are left out.
// An absent path hashes as absent, so creating it changes the subject.
// The answer is always found; Base is left to the caller, which reads the record head.
func documentSubject(location *lyxcwd.Location, paths []string) (shedadapters.Subject, bool, error) {
	decisionRecord := filepath.Clean(loomengine.DiscussionDecisionRecord(location))
	digest := sha256.New()
	for _, path := range paths {
		info, err := os.Stat(path)
		if errors.Is(err, fs.ErrNotExist) {
			writeSubjectPart(digest, path, "absent", nil)
			continue
		}
		if err != nil {
			return shedadapters.Subject{}, false, fmt.Errorf("loom: read review artifact %s: %w", path, err)
		}
		if info.IsDir() {
			if err := hashPlanDirectory(digest, path); err != nil {
				return shedadapters.Subject{}, false, err
			}
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return shedadapters.Subject{}, false, fmt.Errorf("loom: read review artifact %s: %w", path, err)
		}
		if filepath.Clean(path) == decisionRecord {
			content = discussionparser.IdentityText(content)
		}
		writeSubjectPart(digest, path, "file", content)
	}
	return shedadapters.Subject{Kind: shedadapters.SubjectDocument, Hash: hex.EncodeToString(digest.Sum(nil))}, true, nil
}

// hashPlanDirectory adds the plan generation in dir to digest: the overview's identity text, then each top-level card file in name order.
func hashPlanDirectory(digest hash.Hash, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("loom: read review artifact directory %s: %w", dir, err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if name := entry.Name(); name == planOverviewFile || planCardFilePattern.MatchString(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("loom: read review artifact %s: %w", filepath.Join(dir, name), err)
		}
		if name == planOverviewFile {
			content = planparser.IdentityText(content)
		}
		writeSubjectPart(digest, filepath.Join(dir, name), "file", content)
	}
	return nil
}

// writeSubjectPart adds one named, length-framed part to digest, so no two part sequences share bytes.
func writeSubjectPart(digest hash.Hash, name, kind string, content []byte) {
	fmt.Fprintf(digest, "%s\x00%s\x00%d\x00", name, kind, len(content))
	digest.Write(content)
}

// taskMergeBase returns the reader of the commit the task's diff is taken from: the merge base of the task pair's HEAD with the parent tip the run's merge record names.
// The local parent branch can lag a tip the task merged, so with no merge record it falls back to the merge base with that branch.
// An unreadable merge record is logged at Warn and read as absent, so no reader errors on it and the next Merge-In or Finalize rewrites it.
// The record and the fabric are read on each call, so a merge made after bootstrap is read as it stands.
func taskMergeBase(l *lyxcwd.Location) func() (string, error) {
	fallback := pairMergeBase(l)
	recordPath := landingshed.MergeRecordPath(loomengine.LoomScratchDir(l))
	return func() (string, error) {
		record, found, err := landingshed.ReadMergeRecord(recordPath)
		if err != nil {
			logger.Warn("loom: read the merge record for the task's merge base; falling back to the parent branch", "path", recordPath, "error", err)
			return fallback()
		}
		if !found || record.MergedTip == "" {
			return fallback()
		}
		handle, err := fabricengine.Open(l)
		if err != nil {
			return "", err
		}
		return handle.MergeBase(record.MergedTip)
	}
}

// codeSubject returns the identity of the task's diff against its merge base from taskMergeBase.
// A run with no recorded parent branch, a standalone run, answers not found, so its code segments write no records.
// The diff leaves out the run's own state directories and harnessOwnFiles while untracked; the run's records live on the records side of the fabric, so no diff of the task's code holds them.
// Bound: a change confined to harnessOwnFiles reads as an unchanged subject; the verify tiers at Publish, darn's Merge-In and loom's Merge-Review bound it.
func codeSubject(location *lyxcwd.Location, runID string) (shedadapters.Subject, bool, error) {
	origin, found, err := fabricengine.ReadOrigin(location)
	if err != nil {
		return shedadapters.Subject{}, false, err
	}
	if !found || origin.ParentBranch == "" {
		return shedadapters.Subject{}, false, nil
	}
	base, err := taskMergeBase(location)()
	if err != nil {
		return shedadapters.Subject{}, false, err
	}
	handle, err := fabricengine.Open(location)
	if err != nil {
		return shedadapters.Subject{}, false, err
	}
	patch, err := handle.ReadTaskPatch(base, patchSkipEntries(location, runID))
	if err != nil {
		return shedadapters.Subject{}, false, err
	}
	return shedadapters.Subject{
		Kind:      shedadapters.SubjectCode,
		MergeBase: base,
		Head:      patch.Head,
		FullHash:  patch.FullHash,
		PatchID:   patch.PatchID,
		Base:      base,
		Patch:     patch.Full,
	}, true, nil
}

// patchSkipEntries returns the skip entries of a code subject's patch: each of the run's own state directories under the worktree as a `/`-ended entry, then harnessOwnFiles.
// A standalone run keeps those directories inside the worktree, which is where they would show as untracked.
func patchSkipEntries(location *lyxcwd.Location, runID string) []string {
	geometry := hubgeom.WebsterGeometry(location)
	var skip []string
	for _, dir := range []string{geometry.WebsterDir, geometry.PlanDir, geometry.ScratchDir, shedrun.ScratchDir(location, runID)} {
		rel, err := filepath.Rel(location.WorktreePath(), dir)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		skip = append(skip, filepath.ToSlash(rel)+"/")
	}
	return append(skip, harnessOwnFiles...)
}

// recordHead returns the reader of the commit the run records' checkout stands at.
// The fabric is opened on each call, so a head that moved is read as it stands.
func recordHead(location *lyxcwd.Location) func() (string, error) {
	return func() (string, error) {
		handle, err := fabricengine.Open(location)
		if err != nil {
			return "", err
		}
		return handle.RecordsHeadSHA()
	}
}

// parentMergeInOnly returns the seam that reports whether the task's history since a commit holds only merge-ins of the recorded parent branch, read against that branch's tip as it stands.
func parentMergeInOnly(location *lyxcwd.Location) func(since string) (bool, error) {
	return func(since string) (bool, error) {
		parent, err := readRecordedParentBranch(location)
		if err != nil {
			return false, err
		}
		handle, err := fabricengine.Open(location)
		if err != nil {
			return false, err
		}
		tips, err := handle.ReadParentTips(parent, false)
		if err != nil {
			return false, err
		}
		return handle.MergeInOnly(since, tips.Local)
	}
}

// documentSubjectWithHead returns the document subject seam: documentSubject with Base set to the record head read after the artifacts.
func documentSubjectWithHead(location *lyxcwd.Location, head func() (string, error)) func(paths []string) (shedadapters.Subject, bool, error) {
	return func(paths []string) (shedadapters.Subject, bool, error) {
		subject, found, err := documentSubject(location, paths)
		if err != nil || !found {
			return subject, found, err
		}
		subject.Base, err = head()
		if err != nil {
			return shedadapters.Subject{}, false, err
		}
		return subject, true, nil
	}
}

// fixerReportInput returns the review input seam of a loom run: the merge fixer's latest Merge-In report under scratchDir, with the time it was written.
// A session that never ran leaves no report, which answers not found.
func fixerReportInput(scratchDir string) func() (string, time.Time, bool, error) {
	return reviewInputFile(filepath.Join(scratchDir, loomshed.NameMergeIn+"-fixer-report.md"))
}

// overlapNoteInput returns the review input seam of a darn run: the overlap note Finalize writes under scratchDir on a large-merge reroute, with the time it was written.
// A run that never rerouted leaves no note, which answers not found.
func overlapNoteInput(scratchDir string) func() (string, time.Time, bool, error) {
	return reviewInputFile(landingshed.OverlapNotePath(scratchDir))
}

// reviewInputFile returns the seam that reports the file at path and its modification time, and not found while the file is absent.
func reviewInputFile(path string) func() (string, time.Time, bool, error) {
	return func() (string, time.Time, bool, error) {
		info, err := os.Stat(path)
		if errors.Is(err, fs.ErrNotExist) {
			return "", time.Time{}, false, nil
		}
		if err != nil {
			return "", time.Time{}, false, fmt.Errorf("loom: read the review input %s: %w", path, err)
		}
		return path, info.ModTime(), true, nil
	}
}
