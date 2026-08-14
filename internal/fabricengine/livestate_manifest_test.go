//go:build integration

// livestate_manifest_test.go captures a whole-hub filesystem manifest before and after a verb runs, and diffs two
// manifests against a set of permitted removal roots so a cell can assert that a verb touched nothing
// outside what it declared it would.
// The manifest is the survival-assertion mechanism batches 5-7 build cells against: CaptureManifest,
// DiffManifest, and AssertNoUnpermittedChange are the whole of the external interface those batches
// consume.

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

// gitDirName is the base name a manifest walk treats specially: every ".git" entry, file or directory,
// gets the narrow allowlisted treatment gitDirRule describes, rather than the ordinary walk rules.
// It is not a fabric geometry token, so it is spelled here directly rather than routed through an
// accessor — the geometry-through-accessors rule governs `_board`, `-weft`, `_portals`, `_launchers`,
// `_lyx` and `.lyx`, not git's own directory name.
const gitDirName = ".git"

// Kind enumerates the closed set of filesystem entry shapes a Manifest records: a regular file, a
// directory, or a link (a symlink on POSIX, a junction on Windows).
type Kind int

const (
	// KindFile is a regular file, recorded with a content hash.
	KindFile Kind = iota
	// KindDir is a directory, recorded at existence granularity only.
	KindDir
	// KindLink is a link — a symlink on POSIX, a junction on Windows — recorded as a leaf, never
	// descended into, with its raw one-hop target.
	KindLink
)

// String reports kind's name in prose, so a diff failure message names the entry shape it is
// complaining about.
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
// Deliberately not recorded, each for its own reason: permission bits (meaningless on Windows, where
// every file looks alike to a POSIX-shaped mode check); mtime (touched by operations — a git checkout,
// a copy — that are not the destruction this harness exists to catch); and size-without-hash (two
// files of equal size can differ in content, which is exactly the case a survival assertion must not
// miss).
type Entry struct {
	// Kind is the entry's filesystem shape.
	Kind Kind
	// LinkTarget is the raw one-hop target of a KindLink entry, from fslink.RawTarget — not
	// fslink.PointsTo, for the same reason destroy.go's ownedWiredJunction check uses the raw target:
	// a fully-resolved chain fails outright when a later segment is gone, and silently walks past a
	// target that is itself a further link.
	// Empty for KindFile and KindDir.
	LinkTarget string
	// ContentHash is the SHA-256 hash, hex-encoded, of a KindFile entry's bytes.
	// Empty for KindDir and KindLink, and empty for a ".git" file entry, which the git allowlist
	// records at existence granularity only.
	ContentHash string
}

// Manifest is a whole-hub filesystem capture: every key is a filepath.ToSlash-normalised path relative
// to the captured hub root, mapped to the Entry recorded for it.
type Manifest map[string]Entry

// CaptureManifest walks hubRoot and returns a Manifest of everything found there.
//
// A link is always recorded as a leaf and never descended into, on every platform: detection goes
// through fslink.IsLink, never a raw os.Lstat mode-bit check, because that is the only test that
// answers the question uniformly for a POSIX symlink and a Windows junction alike.
// This rule is load-bearing, not an optimisation.
// Fabric's wired junctions carry absolute targets that point back inside the same hub — warp/_lyx to
// the hub's weft-side _lyx, warp/.lyx likewise — so a walk that descended through a junction would
// record the weft sibling's contents twice: once under its own path, and again under every warp
// junction chaining into it.
// Every legitimate weft-side change would then surface as an unpermitted mutation under a warp key,
// while a permit root written against a warp path would silently license weft destruction.
// The link's identity is fully captured by its kind plus its raw one-hop target; whatever the target
// contains is recorded once, under the target's own path.
//
// Windows junctions and POSIX symlinks differ in how a plain directory walk traverses them — a walk
// that happens not to descend on one platform may descend on the other — which is exactly why this
// function checks fslink.IsLink itself before deciding whether to recurse, rather than trusting the
// walk's own default behaviour.
//
// Inside any ".git" directory, or at a ".git" file (what a linked worktree carries), only the ".git"
// entry itself and each ".git/worktrees/<name>" directory are recorded, both at existence granularity
// with no content hash; everything else under ".git" is excluded.
// A blanket ".git" exclusion would blind the harness to linked-worktree deregistration — the shape of
// R3's defect exactly — while hashing everything under ".git" would drown every cell in index, logs,
// refs, objects, FETCH_HEAD, ORIG_HEAD and packed-refs churn until the permit lists permitted
// everything.
// Branch existence is deliberately not carried by the manifest at all; it is asserted through git
// itself in the per-verb effect assertions, where the answer is authoritative rather than inferred
// from a ref file.
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
			// A junction can present as a directory entry to the walk on Windows; SkipDir on a
			// directory-shaped entry is what keeps this function from ever descending through a link
			// regardless of platform. SkipDir on a non-directory entry would instead skip the rest of
			// the containing directory, which is why the two cases are not folded together.
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

// captureGitEntry applies the ".git" allowlist to one walk entry named ".git": it records the entry
// itself and, for a directory, each ".git/worktrees/<name>" directory, then returns filepath.SkipDir
// so the walk never descends into anything else ".git" carries.
func captureGitEntry(manifest Manifest, hubRoot, path, relSlash string, d fs.DirEntry) error {
	if !d.IsDir() {
		// A linked worktree's ".git" is a file, not a directory; it is recorded at existence
		// granularity only, matching the directory case below.
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
	// ChangeKindChanged reports a path whose Entry.Kind differs between before and after — a file
	// where a link was, or the like.
	ChangeKindChanged
	// ChangeLinkTargetChanged reports a KindLink path whose raw one-hop target differs between before
	// and after.
	ChangeLinkTargetChanged
	// ChangeContentChanged reports a KindFile path whose content hash differs between before and
	// after.
	ChangeContentChanged
	// ChangeAdded reports a path present in after and absent from before.
	ChangeAdded
)

// String reports k's name in prose, so a diff failure message names the kind of change it found.
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

// Change is one path-level difference DiffManifest found between two manifests.
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

// DiffManifest compares before and after and returns every path that disappeared, changed kind,
// changed link target, or changed content hash and that is not at or below any permitted root, plus
// every path added between the two captures, reported with ChangeAdded so a cell can assert on
// destruction without being noisy about creation.
//
// A permitted root is a filepath.ToSlash hub-relative path prefix.
// Matching is on path segments, not on raw string prefix, so a root of "_portals/x" never permits
// "_portalsfoo/y".
// Comparison of a path segment against a root segment folds case on Windows only, via this file's own
// segmentsEqual — lyxcwd.samePath does exactly this but is unexported, so this file carries its own
// equivalent rather than reaching for it.
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
// Segment-wise comparison, not raw string-prefix comparison, is what keeps a root of "_portals/x" from
// matching "_portalsfoo/y".
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

// segmentsEqual reports whether two path segments name the same thing: byte-exact everywhere except
// Windows, where the comparison folds case — the same posture lyxcwd.samePath takes for a full path,
// carried here at segment granularity because that unexported function is not reachable from this
// package.
func segmentsEqual(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// nearestPermittedRoot returns whichever of permitted shares the longest path-segment prefix with
// path, for a failure message to name even when that root did not end up covering path — the message
// this supports is "here is the permitted root that came closest, and it still didn't reach you".
// Returns "(none declared)" if permitted is empty, or "(none)" if no permitted root shares even path's
// first segment.
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

// describe renders one Change in prose for a failure message: the path, what kind of change it was,
// and enough of the before/after entries to make the change legible without a debugger.
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
// Additions are never a failure here — DiffManifest's ChangeAdded entries exist so a cell can inspect
// creation without AssertNoUnpermittedChange treating it as destruction — only ChangeRemoved,
// ChangeKindChanged, ChangeLinkTargetChanged and ChangeContentChanged can fail this assertion.
// The failure message names each offending path, its kind of change, and the nearest permitted root
// that did not cover it, because triaging which permit root a survival assertion needs widened is the
// reason this manifest exists alongside sentinels rather than instead of them.
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
