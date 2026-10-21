// subjects.go builds the identities a Bouncer re-entry compares: the document, code and merge-in subjects of a loom or darn run's review segments, the record head and merge-in-only reads that go with them, and the darn reject file.

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

// darnRejectReviewFile is the file in the landing directory that holds a darn PR-Gate reject's findings and its time.
const darnRejectReviewFile = "reject-review.md"

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

// codeSubject returns the identity of the task's diff against its merge base with its recorded parent branch.
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
	handle, err := fabricengine.Open(location)
	if err != nil {
		return shedadapters.Subject{}, false, err
	}
	base, err := handle.MergeBase(origin.ParentBranch)
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

// mergeSubject returns the identity of the merge-in the merge record names: its merged tip and merge commit.
// No merge record answers not found.
func mergeSubject(location *lyxcwd.Location) (shedadapters.Subject, bool, error) {
	record, found, err := landingshed.ReadMergeRecord(landingshed.MergeRecordPath(loomengine.LoomScratchDir(location)))
	if err != nil || !found {
		return shedadapters.Subject{}, false, err
	}
	digest := sha256.New()
	writeSubjectPart(digest, "merged_tip", "value", []byte(record.MergedTip))
	writeSubjectPart(digest, "merge_commit", "value", []byte(record.MergeCommit))
	return shedadapters.Subject{Kind: shedadapters.SubjectMerge, Hash: hex.EncodeToString(digest.Sum(nil))}, true, nil
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

// recordDarnRejection writes a darn PR-Gate reject's findings and time into the landing directory's reject file, replacing an earlier one.
// It is the body of the RecordRejection seam; the Commit seam then commits the landing directory.
func recordDarnRejection(location *lyxcwd.Location, rejection loomshed.PendingRejection) error {
	path := filepath.Join(loomengine.LandingDir(location), darnRejectReviewFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("loom: create the landing directory for the reject file: %w", err)
	}
	body := "```yaml\nrejected_at: " + rejection.RejectedAt + "\n```\n\n" + strings.TrimSpace(rejection.Findings) + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return fmt.Errorf("loom: write the reject file: %w", err)
	}
	return nil
}

// darnRejectReview reads the reject file recordDarnRejection wrote: its path and the rejection time.
// An absent file answers not found; a file without the time header or with a time that is not RFC 3339 is an error.
func darnRejectReview(location *lyxcwd.Location) (string, time.Time, bool, error) {
	path := filepath.Join(loomengine.LandingDir(location), darnRejectReviewFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", time.Time{}, false, nil
	}
	if err != nil {
		return "", time.Time{}, false, fmt.Errorf("loom: read the reject file: %w", err)
	}
	lines := strings.SplitN(string(data), "\n", 4)
	if len(lines) < 3 || lines[0] != "```yaml" || !strings.HasPrefix(lines[1], "rejected_at: ") || lines[2] != "```" {
		return "", time.Time{}, false, fmt.Errorf("loom: the reject file %s has no rejected_at header", path)
	}
	rejectedAt, err := time.Parse(time.RFC3339, strings.TrimPrefix(lines[1], "rejected_at: "))
	if err != nil {
		return "", time.Time{}, false, fmt.Errorf("loom: the reject file %s rejected_at: %w", path, err)
	}
	return path, rejectedAt, true, nil
}
