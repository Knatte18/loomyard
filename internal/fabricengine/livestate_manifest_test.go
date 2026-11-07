// livestate_manifest_test.go captures a whole-hub filesystem manifest before and after a verb runs.
// It diffs two manifests against a set of permitted removal roots.
// A cell uses it to assert that a verb touched nothing outside what it declared it would.
// CaptureManifest, DiffManifest and AssertNoUnpermittedChange form its external interface.

package fabricengine_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fslink"
)

// gitDirName is the base name of git's own metadata entry.
// A manifest walk gives every ".git" entry, file or directory, a narrow allowlisted treatment.
// It does not apply the ordinary walk rules to them.
// The name is not a fabric geometry token, so it is spelled directly rather than routed through an accessor.
// Fabric geometry names go through accessors: the Bolt checkout, `-weft`, `_portals`, `_launchers`, `_lym`, `.lym`.
const gitDirName = ".git"

// Kind enumerates the closed set of filesystem entry shapes a Manifest records:
// a regular file, a directory, or a link (a symlink on POSIX, a junction on Windows).
type Kind int

const (
	// KindFile is a regular file, recorded with a content hash.
	KindFile Kind = iota
	// KindDir is a directory, recorded at existence granularity only.
	KindDir
	// KindLink is a link, a symlink on POSIX or a junction on Windows.
	// It is recorded as a leaf, never descended into, with its raw one-hop target.
	KindLink
)

// String reports kind's name in prose, so a diff failure message names the entry shape it complains about.
func (k Kind) String() string {
	switch k {
	case KindFile:
		return "file"
	case KindDir:
		return "dir"
	case KindLink:
		return "link"
	default:
		return "unknown"
	}
}

// Entry is what a Manifest records for one hub-relative path.
//
// Permission bits are not recorded, since they are meaningless on Windows.
// Modification time is not recorded, since a git checkout or a copy touches it without destroying anything.
// Size is not recorded alone, since two files of equal size can differ in content,
// and a survival assertion must catch that.
type Entry struct {
	// Kind is the entry's filesystem shape.
	Kind Kind
	// LinkTarget is the raw one-hop target of a KindLink entry.
	// It is kept unresolved.
	// A fully resolved chain fails outright when a later segment is gone,
	// and it walks past a target that is itself a further link.
	// Empty for KindFile and KindDir.
	LinkTarget string
	// ContentHash is the SHA-256 hash, hex-encoded, of a KindFile entry's bytes.
	// Empty for KindDir and KindLink.
	// A ".git" file entry is empty too, since it is recorded at existence granularity only.
	ContentHash string
}

// Manifest is a whole-hub filesystem capture.
// Each key is a filepath.ToSlash-normalised path relative to the captured hub root.
// Each value is the Entry recorded for that path.
type Manifest map[string]Entry

// CaptureManifest walks hubRoot and returns a Manifest of everything found there.
//
// Detection goes through fslink.IsLink, not a raw os.Lstat mode-bit check.
// That is the only test that answers uniformly for a POSIX symlink and a Windows junction.
// CaptureManifest records every entry under hubRoot, keyed by its slash-separated path relative to hubRoot.
// A symbolic link or junction is recorded once, with its raw one-hop target, and the walk never descends into it.
// Descending would record the target's contents a second time under the link's path,
// and a permit written against the link's path would then license changes to the target.
// Links are detected explicitly rather than left to the walk.
// Windows junctions and POSIX symlinks differ in whether a directory walk descends into them.
//
// Inside a ".git" directory, or at a ".git" file, only two kinds of entry are recorded:
// the ".git" entry itself, and each ".git/worktrees/<name>" directory.
// Both are recorded at existence granularity, with no content hash.
// Everything else under ".git" is excluded.
// Excluding ".git" wholesale would hide linked-worktree deregistration.
// Hashing everything under it would drown the manifest in index, log, ref and object churn.
//
// Branch existence is not carried by the manifest; callers assert it through git directly.
//
// It calls tb.Fatalf on any error.
func CaptureManifest(tb testing.TB, hubRoot string) Manifest {
	tb.Helper()

	manifest := make(Manifest)
	err := filepath.WalkDir(hubRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == hubRoot {
			return nil
		}

		rel, relErr := filepath.Rel(hubRoot, path)
		if relErr != nil {
			return relErr
		}
		relSlash := filepath.ToSlash(rel)

		if d.Name() == gitDirName {
			return captureGitEntry(manifest, hubRoot, path, relSlash, d)
		}

		isLink, linkErr := fslink.IsLink(path)
		if linkErr != nil {
			return linkErr
		}
		if isLink {
			target, targetErr := fslink.RawTarget(path)
			if targetErr != nil {
				return targetErr
			}
			manifest[relSlash] = Entry{Kind: KindLink, LinkTarget: target}
			// On Windows a junction can present as a directory entry to the walk.
			// SkipDir on a directory-shaped entry keeps the walk from descending through a link on any platform.
			// SkipDir on a non-directory entry would skip the rest of the containing directory.
			// The two cases are therefore kept apart.
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			manifest[relSlash] = Entry{Kind: KindDir}
			return nil
		}

		hash, hashErr := hashFileContent(path)
		if hashErr != nil {
			return hashErr
		}
		manifest[relSlash] = Entry{Kind: KindFile, ContentHash: hash}
		return nil
	})
	if err != nil {
		tb.Fatalf("CaptureManifest(%s): %v", hubRoot, err)
	}
	return manifest
}

// captureGitEntry applies the ".git" allowlist to one walk entry named ".git".
// It records the entry itself and, for a directory, each ".git/worktrees/<name>" directory.
// It then returns filepath.SkipDir so the walk never descends into anything else ".git" carries.
func captureGitEntry(manifest Manifest, hubRoot, path, relSlash string, d fs.DirEntry) error {
	if !d.IsDir() {
		// A linked worktree's ".git" is a file, not a directory.
		// It is recorded at existence granularity only, the same as a ".git" directory.
		manifest[relSlash] = Entry{Kind: KindFile}
		return nil
	}

	manifest[relSlash] = Entry{Kind: KindDir}

	worktreesDir := filepath.Join(path, "worktrees")
	entries, readErr := os.ReadDir(worktreesDir)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return filepath.SkipDir
		}
		return readErr
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		worktreeRel, relErr := filepath.Rel(hubRoot, filepath.Join(worktreesDir, entry.Name()))
		if relErr != nil {
			return relErr
		}
		manifest[filepath.ToSlash(worktreeRel)] = Entry{Kind: KindDir}
	}
	return filepath.SkipDir
}

// hashFileContent returns the hex-encoded SHA-256 hash of the file at path.
func hashFileContent(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// ChangeKind enumerates the closed set of ways a path can differ between two manifests.
type ChangeKind int

const (
	// ChangeRemoved reports a path present in before and absent from after.
	ChangeRemoved ChangeKind = iota
	// ChangeKindChanged reports a path whose Entry.Kind differs between before and after.
	// For example, a file where a link was.
	ChangeKindChanged
	// ChangeLinkTargetChanged reports a KindLink path whose raw one-hop target differs.
	// Only the raw one-hop target is compared, not the content it points at.
	ChangeLinkTargetChanged
	// ChangeContentChanged reports a KindFile path whose content hash differs.
	// The hashes are compared between before and after.
	ChangeContentChanged
	// ChangeAdded reports a path present in after and absent from before.
	ChangeAdded
)

// String returns the name of k in prose, so a failure message names the kind of change.
func (k ChangeKind) String() string {
	switch k {
	case ChangeRemoved:
		return "removed"
	case ChangeKindChanged:
		return "kind changed"
	case ChangeLinkTargetChanged:
		return "link target changed"
	case ChangeContentChanged:
		return "content changed"
	case ChangeAdded:
		return "added"
	default:
		return "unknown"
	}
}

// Change is one path-level difference between two manifests.
// Before is nil for a ChangeAdded entry; After is nil for a ChangeRemoved entry; both are set for
// every other kind.
type Change struct {
	// Path is the manifest key the change was found at.
	Path string
	// Kind reports which way the path differed.
	Kind ChangeKind
	// Before is the entry before, or nil if the path did not exist before.
	Before *Entry
	// After is the entry after, or nil if the path does not exist after.
	After *Entry
}

// DiffManifest returns every change to a path that is not at or below any permitted root.
// A change is a path that disappeared, changed kind, changed link target, or changed content hash.
// It also returns every path added between the two captures, reported with ChangeAdded,
// so a cell can assert on destruction without treating creation as destruction.
//
// A permitted root is a filepath.ToSlash hub-relative path prefix.
// Matching is on path segments, not on raw string prefix, so a root of "_portals/x" never permits "_portalsfoo/y".
// On Windows the segment comparison folds case; elsewhere it is byte-exact.
//
// The returned slice is sorted by Path, so a caller's failure output is stable across runs.
func DiffManifest(before, after Manifest, permitted []string) []Change {
	var changes []Change

	for path, beforeEntry := range before {
		afterEntry, stillPresent := after[path]
		if !stillPresent {
			if pathPermitted(path, permitted) {
				continue
			}
			entry := beforeEntry
			changes = append(changes, Change{Path: path, Kind: ChangeRemoved, Before: &entry})
			continue
		}

		if afterEntry.Kind != beforeEntry.Kind {
			if pathPermitted(path, permitted) {
				continue
			}
			before, after := beforeEntry, afterEntry
			changes = append(changes, Change{Path: path, Kind: ChangeKindChanged, Before: &before, After: &after})
			continue
		}

		switch beforeEntry.Kind {
		case KindLink:
			if afterEntry.LinkTarget != beforeEntry.LinkTarget {
				if pathPermitted(path, permitted) {
					continue
				}
				before, after := beforeEntry, afterEntry
				changes = append(changes, Change{Path: path, Kind: ChangeLinkTargetChanged, Before: &before, After: &after})
			}
		case KindFile:
			if afterEntry.ContentHash != beforeEntry.ContentHash {
				if pathPermitted(path, permitted) {
					continue
				}
				before, after := beforeEntry, afterEntry
				changes = append(changes, Change{Path: path, Kind: ChangeContentChanged, Before: &before, After: &after})
			}
		}
	}

	for path, afterEntry := range after {
		if _, existedBefore := before[path]; existedBefore {
			continue
		}
		entry := afterEntry
		changes = append(changes, Change{Path: path, Kind: ChangeAdded, After: &entry})
	}

	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes
}

// pathPermitted reports whether path is at or below any of permitted's roots.
func pathPermitted(path string, permitted []string) bool {
	for _, root := range permitted {
		if pathAtOrBelowRoot(path, root) {
			return true
		}
	}
	return false
}

// pathAtOrBelowRoot reports whether path, split on "/", carries root's segments as a prefix.
// The comparison runs segment by segment, not as a raw string prefix,
// so a root of "_portals/x" does not match "_portalsfoo/y".
func pathAtOrBelowRoot(path, root string) bool {
	pathSegments := strings.Split(path, "/")
	rootSegments := strings.Split(root, "/")
	if len(rootSegments) > len(pathSegments) {
		return false
	}
	for i, rootSegment := range rootSegments {
		if !segmentsEqual(pathSegments[i], rootSegment) {
			return false
		}
	}
	return true
}

// segmentsEqual reports whether two path segments name the same thing.
// The comparison is byte-exact, except on Windows, where it folds case.
func segmentsEqual(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// nearestPermittedRoot returns the permitted root that shares the longest path-segment prefix with path.
// A failure message names this root even when it does not cover path.
// It returns "(none declared)" if permitted is empty, or "(none)" if no permitted root shares path's first segment.
func nearestPermittedRoot(path string, permitted []string) string {
	if len(permitted) == 0 {
		return "(none declared)"
	}

	pathSegments := strings.Split(path, "/")
	best := ""
	bestShared := 0
	for _, root := range permitted {
		rootSegments := strings.Split(root, "/")
		shared := 0
		for i := 0; i < len(rootSegments) && i < len(pathSegments); i++ {
			if !segmentsEqual(pathSegments[i], rootSegments[i]) {
				break
			}
			shared++
		}
		if shared > bestShared {
			bestShared = shared
			best = root
		}
	}
	if bestShared == 0 {
		return "(none)"
	}
	return best
}

// describe renders one Change in prose for a failure message:
// the path, the kind of change, and enough of the before/after entries to make it legible without a debugger.
func (c Change) describe() string {
	switch c.Kind {
	case ChangeRemoved:
		return fmt.Sprintf("%s: removed (was %s)", c.Path, c.Before.Kind)
	case ChangeKindChanged:
		return fmt.Sprintf("%s: kind changed (%s -> %s)", c.Path, c.Before.Kind, c.After.Kind)
	case ChangeLinkTargetChanged:
		return fmt.Sprintf("%s: link target changed (%s -> %s)", c.Path, c.Before.LinkTarget, c.After.LinkTarget)
	case ChangeContentChanged:
		return fmt.Sprintf("%s: content changed (%s)", c.Path, c.Before.Kind)
	case ChangeAdded:
		return fmt.Sprintf("%s: added (%s)", c.Path, c.After.Kind)
	default:
		return fmt.Sprintf("%s: unknown change", c.Path)
	}
}

// AssertNoUnpermittedChange fails tb if before and after differ anywhere outside permitted's roots.
// Additions are never a failure, so a cell can inspect creation without treating it as destruction.
// Only removal, kind change, link target change and content change can fail this assertion.
// The failure message names each offending path, its kind of change, and the nearest permitted root.
func AssertNoUnpermittedChange(tb testing.TB, before, after Manifest, permitted []string) {
	tb.Helper()

	var offending []Change
	for _, change := range DiffManifest(before, after, permitted) {
		if change.Kind == ChangeAdded {
			continue
		}
		offending = append(offending, change)
	}
	if len(offending) == 0 {
		return
	}

	var message strings.Builder
	message.WriteString("unpermitted change(s) outside permitted roots:\n")
	for _, change := range offending {
		fmt.Fprintf(&message, "  %s; nearest permitted root: %s\n", change.describe(), nearestPermittedRoot(change.Path, permitted))
	}
	tb.Fatalf("%s", message.String())
}
