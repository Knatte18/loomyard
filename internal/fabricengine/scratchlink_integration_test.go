//go:build integration

// scratchlink_integration_test.go covers the scratch step of fabric wiring against real hubs.
// It pins the root `.scratch` becoming a link into the weft-side `.lyx`, the adoption of a real directory, and the shapes the step leaves as found.
// It also pins the step's reports when a seam refuses it after its classification.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

const scratchNoteName = "note.txt"

// scratchPaths returns the prime pair's root `.scratch` entry and the weft-side directory it links to.
func scratchPaths(l *lyxcwd.Location, slug string) (root, target string) {
	root = filepath.Join(fabricengine.WorktreePath(l, slug), ".scratch")
	target = filepath.Join(fabricengine.RecordsWorktreePath(l, slug), l.AnchorRel, lyxdirs.DotLyxDirName, "scratch")
	return root, target
}

// scratchHubRelative returns path relative to the hub, slash-separated, as a mutation record names it.
func scratchHubRelative(t *testing.T, l *lyxcwd.Location, path string) string {
	t.Helper()
	rel, err := filepath.Rel(l.HubPath, path)
	if err != nil {
		t.Fatalf("Rel(%s, %s): %v", l.HubPath, path, err)
	}
	return filepath.ToSlash(rel)
}

// wireWithRecord runs WireJunctionsWith over the repo-wide name-set and returns its record.
func wireWithRecord(t *testing.T, l *lyxcwd.Location, slug string) *fabricengine.Mutations {
	t.Helper()
	names, err := fabricengine.RepoWiredNames(l)
	if err != nil {
		t.Fatalf("RepoWiredNames: %v", err)
	}
	rec := fabricengine.NewMutations(l.HubPath)
	if err := fabricengine.WireJunctionsWith(rec, l, slug, names); err != nil {
		t.Fatalf("WireJunctionsWith = %v; want nil", err)
	}
	return rec
}

// hasMutation reports whether record holds an entry of kind at target, with detail when detail is non-empty.
func hasMutation(record fabricengine.Mutations, kind fabricengine.Kind, target, detail string) bool {
	for _, entry := range record.Entries() {
		if entry.Kind == kind && entry.Target == target && (detail == "" || entry.Detail == detail) {
			return true
		}
	}
	return false
}

// describeEntry names what stands at path: absent, a link with its raw target, a directory or a file.
func describeEntry(t *testing.T, path string) string {
	t.Helper()
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return "absent"
	}
	if err != nil {
		t.Fatalf("Lstat(%s): %v", path, err)
	}
	if isLink, linkErr := fslink.IsLink(path); linkErr == nil && isLink {
		raw, rawErr := fslink.RawTarget(path)
		if rawErr != nil {
			t.Fatalf("RawTarget(%s): %v", path, rawErr)
		}
		return "link:" + filepath.Clean(raw)
	}
	if info.IsDir() {
		return "dir"
	}
	return "file"
}

// replaceLinkWithRealDirectory removes the root link and leaves a real `.scratch` holding scratchNoteName.
func replaceLinkWithRealDirectory(t *testing.T, root string) {
	t.Helper()
	if err := fslink.Remove(root); err != nil {
		t.Fatalf("remove root link %s: %v", root, err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("create real %s: %v", root, err)
	}
	if err := os.WriteFile(filepath.Join(root, scratchNoteName), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
}

// removeRootLink removes the prime pair's root link, leaving the root absent.
func removeRootLink(t *testing.T, root string) {
	t.Helper()
	if err := fslink.Remove(root); err != nil {
		t.Fatalf("remove root link %s: %v", root, err)
	}
}

// removeTarget deletes the weft-side target directory.
func removeTarget(t *testing.T, target string) {
	t.Helper()
	if err := os.RemoveAll(target); err != nil {
		t.Fatalf("remove target %s: %v", target, err)
	}
}

// trackScratchFile force-adds and commits a file under `.scratch/` in the warp worktree at dir.
func trackScratchFile(t *testing.T, dir, root string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("create %s: %v", root, err)
	}
	if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("tracked"), 0o644); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	gitkit.Git(t, dir, "add", "-f", "--", ".scratch/tracked.txt")
	gitkit.Git(t, dir, "commit", "-m", "track scratch")
}

// writeLegacyScratchExclude replaces the exclude file's `/.scratch` line with the legacy `/.scratch/`.
func writeLegacyScratchExclude(t *testing.T, l *lyxcwd.Location, slug string) {
	t.Helper()
	path := excludeFilePath(t, l, slug)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var kept []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) != "/.scratch" {
			kept = append(kept, line)
		}
	}
	content := strings.TrimRight(strings.Join(kept, "\n"), "\n") + "\n/.scratch/\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// skipUnlessChmodBlocksWrites skips the test where a read-only directory does not stop the writer.
func skipUnlessChmodBlocksWrites(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("a read-only directory does not block writes on this platform or as root")
	}
}

// makeReadOnly makes dir read-only and registers a cleanup restoring its mode so the temp directory can be removed.
func makeReadOnly(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("chmod %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
}

// scratchShapeCase is one shape the scratch step leaves as found.
type scratchShapeCase struct {
	name string
	// arrange shapes the prime pair's root `.scratch`, its target and its exclude.
	arrange func(t *testing.T, l *lyxcwd.Location, slug string)
	// leaves reports whether the step leaves both the root entry and the target as arranged.
	leaves bool
	// phrase is the part of the left report that names this shape.
	phrase string
}

var scratchShapeCases = []scratchShapeCase{
	{
		name: "LinkToAnotherTarget",
		arrange: func(t *testing.T, l *lyxcwd.Location, slug string) {
			root, _ := scratchPaths(l, slug)
			other := filepath.Join(t.TempDir(), "elsewhere")
			if err := os.MkdirAll(other, 0o755); err != nil {
				t.Fatalf("create %s: %v", other, err)
			}
			removeRootLink(t, root)
			if err := fslink.CreateDirLink(root, other); err != nil {
				t.Fatalf("link %s: %v", root, err)
			}
		},
		leaves: true,
		phrase: "a link to another target",
	},
	{
		name: "RealDirectoryWhileTargetExists",
		arrange: func(t *testing.T, l *lyxcwd.Location, slug string) {
			root, _ := scratchPaths(l, slug)
			replaceLinkWithRealDirectory(t, root)
		},
		leaves: true,
		phrase: "a real directory while the target",
	},
	{
		name: "RegularFile",
		arrange: func(t *testing.T, l *lyxcwd.Location, slug string) {
			root, _ := scratchPaths(l, slug)
			removeRootLink(t, root)
			if err := os.WriteFile(root, []byte("not a directory"), 0o644); err != nil {
				t.Fatalf("write %s: %v", root, err)
			}
		},
		leaves: true,
		phrase: "a regular file",
	},
	{
		name: "TrackedContentWhileTargetAbsent",
		arrange: func(t *testing.T, l *lyxcwd.Location, slug string) {
			root, target := scratchPaths(l, slug)
			removeRootLink(t, root)
			trackScratchFile(t, fabricengine.WorktreePath(l, slug), root)
			removeTarget(t, target)
		},
		leaves: true,
		phrase: "tracked by git",
	},
	{
		name: "TrackedContentWithRootDeletedByHand",
		arrange: func(t *testing.T, l *lyxcwd.Location, slug string) {
			root, _ := scratchPaths(l, slug)
			removeRootLink(t, root)
			trackScratchFile(t, fabricengine.WorktreePath(l, slug), root)
			if err := os.RemoveAll(root); err != nil {
				t.Fatalf("remove %s: %v", root, err)
			}
		},
		leaves: true,
		phrase: "tracked by git",
	},
	{
		name: "FailedRename",
		arrange: func(t *testing.T, l *lyxcwd.Location, slug string) {
			skipUnlessChmodBlocksWrites(t)
			root, target := scratchPaths(l, slug)
			replaceLinkWithRealDirectory(t, root)
			removeTarget(t, target)
			makeReadOnly(t, filepath.Dir(target))
		},
		leaves: true,
		phrase: "whose move to",
	},
	{
		name: "FailedExcludeWrite",
		arrange: func(t *testing.T, l *lyxcwd.Location, slug string) {
			skipUnlessChmodBlocksWrites(t)
			root, target := scratchPaths(l, slug)
			replaceLinkWithRealDirectory(t, root)
			removeTarget(t, target)
			writeLegacyScratchExclude(t, l, slug)
			infoDir := filepath.Dir(excludeFilePath(t, l, slug))
			if _, err := os.Stat(filepath.Join(infoDir, "exclude.lyx.lock")); err != nil {
				t.Fatalf("exclude lock file missing before the chmod: %v", err)
			}
			makeReadOnly(t, infoDir)
		},
		leaves: true,
		phrase: "scratch exclude not written",
	},
	{
		name: "FailedTargetCreation",
		arrange: func(t *testing.T, l *lyxcwd.Location, slug string) {
			root, target := scratchPaths(l, slug)
			removeRootLink(t, root)
			removeTarget(t, target)
			if err := os.WriteFile(target, []byte("not a directory"), 0o644); err != nil {
				t.Fatalf("write %s: %v", target, err)
			}
		},
		leaves: true,
		phrase: "scratch link not created",
	},
}

// TestWireJunctions_ScratchLink pins the scratch step through WireJunctionsWith.
// A link is created or kept, a real directory is adopted, and every leave shape is left as found without failing wiring.
func TestWireJunctions_ScratchLink(t *testing.T) {
	t.Parallel()

	for _, anchor := range []string{".", "backend"} {
		t.Run("AbsentRootLinked_Anchor_"+anchor, func(t *testing.T) {
			t.Parallel()

			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: anchor})
			l := h.Location
			slug := l.WorktreeName
			root, target := scratchPaths(l, slug)
			removeRootLink(t, root)
			removeTarget(t, target)

			rec := wireWithRecord(t, l, slug)

			if got, want := describeEntry(t, root), "link:"+target; got != want {
				t.Fatalf("root .scratch = %q; want %q", got, want)
			}
			record := rec.Snapshot()
			if !hasMutation(record, fabricengine.KindDirCreated, scratchHubRelative(t, l, target), "") {
				t.Errorf("record %+v lacks dir_created at the target", record.Entries())
			}
			if !hasMutation(record, fabricengine.KindLinkCreated, scratchHubRelative(t, l, root), "") {
				t.Errorf("record %+v lacks link_created at the root", record.Entries())
			}

			before, err := os.Lstat(root)
			if err != nil {
				t.Fatalf("Lstat(%s): %v", root, err)
			}
			if err := os.WriteFile(filepath.Join(root, scratchNoteName), []byte("written through the link"), 0o644); err != nil {
				t.Fatalf("write through the link: %v", err)
			}
			after, err := os.Lstat(root)
			if err != nil {
				t.Fatalf("Lstat(%s) after the write: %v", root, err)
			}
			if before.Mode() != after.Mode() || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
				t.Errorf("root .scratch Lstat changed after a write under it: before %v %d %v, after %v %d %v",
					before.Mode(), before.Size(), before.ModTime(), after.Mode(), after.Size(), after.ModTime())
			}
		})
	}

	t.Run("CorrectLinkRecordsNothing", func(t *testing.T) {
		t.Parallel()

		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		l := h.Location

		rec := wireWithRecord(t, l, l.WorktreeName)

		if record := rec.Snapshot(); record.Len() != 0 {
			t.Errorf("record = %+v; want empty for an already-correct link", record.Entries())
		}
	})

	t.Run("CorrectLinkWithRemovedTargetRestoresIt", func(t *testing.T) {
		t.Parallel()

		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		l := h.Location
		slug := l.WorktreeName
		root, target := scratchPaths(l, slug)
		removeTarget(t, target)

		rec := wireWithRecord(t, l, slug)

		if got, want := describeEntry(t, target), "dir"; got != want {
			t.Errorf("target = %q; want %q", got, want)
		}
		if got, want := describeEntry(t, root), "link:"+target; got != want {
			t.Errorf("root .scratch = %q; want %q", got, want)
		}
		if record := rec.Snapshot(); !hasMutation(record, fabricengine.KindDirCreated, scratchHubRelative(t, l, target), "") {
			t.Errorf("record %+v lacks dir_created at the target", record.Entries())
		}
	})

	t.Run("RealDirectoryAdoptedIntoTheTarget", func(t *testing.T) {
		t.Parallel()

		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		l := h.Location
		slug := l.WorktreeName
		warp := fabricengine.WorktreePath(l, slug)
		root, target := scratchPaths(l, slug)
		gitkit.CommitFile(t, warp, ".gitignore", "**/.scratch/\n", "ignore scratch directories")
		replaceLinkWithRealDirectory(t, root)
		removeTarget(t, target)
		writeLegacyScratchExclude(t, l, slug)

		before := CaptureManifest(t, l.HubPath)
		rec := wireWithRecord(t, l, slug)
		after := CaptureManifest(t, l.HubPath)

		if got, want := describeEntry(t, root), "link:"+target; got != want {
			t.Fatalf("root .scratch = %q; want %q", got, want)
		}
		content, err := os.ReadFile(filepath.Join(root, scratchNoteName))
		if err != nil || string(content) != "hello" {
			t.Errorf("note read through .scratch = %q, %v; want %q", content, err, "hello")
		}

		record := rec.Snapshot()
		if !hasMutation(record, fabricengine.KindPathRenamed, scratchHubRelative(t, l, target), scratchHubRelative(t, l, root)) {
			t.Errorf("record %+v lacks path_renamed from the root to the target", record.Entries())
		}
		if !hasMutation(record, fabricengine.KindLinkCreated, scratchHubRelative(t, l, root), "") {
			t.Errorf("record %+v lacks link_created at the root", record.Entries())
		}
		if !hasMutation(record, fabricengine.KindFileWritten, scratchHubRelative(t, l, excludeFilePath(t, l, slug)), "") {
			t.Errorf("record %+v lacks file_written at the exclude file", record.Entries())
		}
		AssertRecordMatchesDiff(t, record, DiffManifest(before, after, nil))

		lines := gitkit.ExcludeLines(t, warp)
		if !containsLine(lines, "/.scratch") || containsLine(lines, "/.scratch/") {
			t.Errorf("exclude lines = %v; want /.scratch and no /.scratch/", lines)
		}
		if status := gitkit.GitStatusPorcelain(t, warp); strings.Contains(status, ".scratch") {
			t.Errorf("git status lists .scratch: %q", status)
		}
	})

	for _, tt := range scratchShapeCases {
		t.Run("Leaves_"+tt.name, func(t *testing.T) {
			t.Parallel()

			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			l := h.Location
			slug := l.WorktreeName
			root, target := scratchPaths(l, slug)
			tt.arrange(t, l, slug)
			rootBefore, targetBefore := describeEntry(t, root), describeEntry(t, target)

			names, err := fabricengine.RepoWiredNames(l)
			if err != nil {
				t.Fatalf("RepoWiredNames: %v", err)
			}
			rec := fabricengine.NewMutations(l.HubPath)
			if err := fabricengine.WireJunctionsWith(rec, l, slug, names); err != nil {
				t.Fatalf("WireJunctionsWith = %v; want nil", err)
			}

			if tt.leaves {
				if got := describeEntry(t, root); got != rootBefore {
					t.Errorf("root .scratch = %q; want it left as %q", got, rootBefore)
				}
				if got := describeEntry(t, target); got != targetBefore {
					t.Errorf("target = %q; want it left as %q", got, targetBefore)
				}
			}
			record := rec.Snapshot()
			for _, kind := range []fabricengine.Kind{fabricengine.KindPathRenamed, fabricengine.KindLinkCreated} {
				if hasMutation(record, kind, scratchHubRelative(t, l, target), "") || hasMutation(record, kind, scratchHubRelative(t, l, root), "") {
					t.Errorf("record %+v holds a %s entry for a left shape", record.Entries(), kind)
				}
			}
		})
	}
}

// TestWireJunctions_ScratchLinkRefusedAfterClassify pins the step's reports when the link, the rename or a check is refused after the root was classified.
// Wiring still returns nil and the warn names the way forward.
// It is serial: it sets package seams and captures the logger.
func TestWireJunctions_ScratchLinkRefusedAfterClassify(t *testing.T) {
	// Each row builds and arranges its hub before it sets a seam, so the template build never fires one.
	t.Run("RealDirectoryRecreatedBeforeTheLink", func(t *testing.T) {
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		l := h.Location
		slug := l.WorktreeName
		root, target := scratchPaths(l, slug)
		replaceLinkWithRealDirectory(t, root)
		removeTarget(t, target)
		fabricengine.SetScratchBeforeLinkHookForTest(t, func() {
			if err := os.Mkdir(root, 0o755); err != nil {
				t.Errorf("recreate %s: %v", root, err)
			}
		})
		logs := logcapture.Capture(t)

		wireWithRecord(t, l, slug)

		if got := describeEntry(t, root); got != "dir" {
			t.Errorf("recreated root .scratch = %q; want an untouched real directory", got)
		}
		if _, err := os.Stat(filepath.Join(target, scratchNoteName)); err != nil {
			t.Errorf("moved note missing at the target: %v", err)
		}
		assertWarnNames(t, logs.String(), target)
	})

	t.Run("RegularFileWrittenBeforeTheLink", func(t *testing.T) {
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		l := h.Location
		slug := l.WorktreeName
		root, target := scratchPaths(l, slug)
		removeRootLink(t, root)
		fabricengine.SetScratchBeforeLinkHookForTest(t, func() {
			if err := os.WriteFile(root, []byte("in the way"), 0o644); err != nil {
				t.Errorf("write %s: %v", root, err)
			}
		})
		logs := logcapture.Capture(t)

		wireWithRecord(t, l, slug)

		if got := describeEntry(t, root); got != "file" {
			t.Errorf("root .scratch = %q; want the regular file left in place", got)
		}
		entries, err := os.ReadDir(target)
		if err != nil || len(entries) != 0 {
			t.Errorf("target entries = %v, %v; want an empty target", entries, err)
		}
		assertWarnNames(t, logs.String(), root)
	})

	t.Run("TrackedCheckFails", func(t *testing.T) {
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		l := h.Location
		slug := l.WorktreeName
		root, target := scratchPaths(l, slug)
		replaceLinkWithRealDirectory(t, root)
		removeTarget(t, target)
		fabricengine.SetScratchPathTrackedForTest(t, func(string, string) (bool, error) {
			return false, os.ErrPermission
		})
		logs := logcapture.Capture(t)

		wireWithRecord(t, l, slug)

		if got := describeEntry(t, root); got != "dir" {
			t.Errorf("root .scratch = %q; want the real directory unrenamed", got)
		}
		if got := describeEntry(t, target); got != "absent" {
			t.Errorf("target = %q; want it absent", got)
		}
		assertWarnNames(t, logs.String(), root)
	})

	t.Run("WeftWorktreeGoneDuringTheStep", func(t *testing.T) {
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		l := h.Location
		slug := l.WorktreeName
		root, target := scratchPaths(l, slug)
		removeRootLink(t, root)
		removeTarget(t, target)
		fabricengine.SetScratchPathTrackedForTest(t, func(string, string) (bool, error) {
			if err := os.RemoveAll(fabricengine.RecordsWorktreePath(l, slug)); err != nil {
				t.Errorf("remove the weft worktree: %v", err)
			}
			return false, nil
		})

		wireWithRecord(t, l, slug)

		if got := describeEntry(t, target); got != "absent" {
			t.Errorf("target = %q; want nothing created once the weft worktree is gone", got)
		}
		if got := describeEntry(t, root); got != "absent" {
			t.Errorf("root .scratch = %q; want no link", got)
		}
	})

	t.Run("ReconcileRealDirectoryRecreatedBeforeTheLink", func(t *testing.T) {
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		l := h.Location
		slug := l.WorktreeName
		root, target := scratchPaths(l, slug)
		replaceLinkWithRealDirectory(t, root)
		removeTarget(t, target)
		fabricengine.SetScratchBeforeLinkHookForTest(t, func() {
			if err := os.Mkdir(root, 0o755); err != nil {
				t.Errorf("recreate %s: %v", root, err)
			}
		})

		_, pair := reconcilePrimePair(t, h)

		if !strings.Contains(pair.Detail, target) || !strings.Contains(pair.Detail, "lyx fabric reconcile") {
			t.Errorf("Detail = %q; want it to name %s and `lyx fabric reconcile`", pair.Detail, target)
		}
	})

	t.Run("ReconcileRegularFileWrittenBeforeTheLink", func(t *testing.T) {
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		l := h.Location
		slug := l.WorktreeName
		root, _ := scratchPaths(l, slug)
		removeRootLink(t, root)
		fabricengine.SetScratchBeforeLinkHookForTest(t, func() {
			if err := os.WriteFile(root, []byte("in the way"), 0o644); err != nil {
				t.Errorf("write %s: %v", root, err)
			}
		})

		_, pair := reconcilePrimePair(t, h)

		if !strings.Contains(pair.Detail, root) || !strings.Contains(pair.Detail, "lyx fabric reconcile") {
			t.Errorf("Detail = %q; want it to name %s and `lyx fabric reconcile`", pair.Detail, root)
		}

		fabricengine.SetScratchBeforeLinkHookForTest(t, nil)
		_, pair = reconcilePrimePair(t, h)

		if !strings.Contains(pair.Detail, "a regular file") {
			t.Errorf("second Detail = %q; want the regular file reported as left as found", pair.Detail)
		}
	})

	t.Run("ReconcileTrackedCheckFails", func(t *testing.T) {
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		l := h.Location
		slug := l.WorktreeName
		root, target := scratchPaths(l, slug)
		replaceLinkWithRealDirectory(t, root)
		removeTarget(t, target)
		fabricengine.SetScratchPathTrackedForTest(t, func(string, string) (bool, error) {
			return false, os.ErrPermission
		})

		_, pair := reconcilePrimePair(t, h)

		if !strings.Contains(pair.Detail, root) || !strings.Contains(pair.Detail, "lyx fabric reconcile") {
			t.Errorf("Detail = %q; want it to name %s and `lyx fabric reconcile`", pair.Detail, root)
		}
	})
}

// mutationIndexes returns the positions in record of every entry of kind at target.
func mutationIndexes(record fabricengine.Mutations, kind fabricengine.Kind, target string) []int {
	var indexes []int
	for i, entry := range record.Entries() {
		if entry.Kind == kind && entry.Target == target {
			indexes = append(indexes, i)
		}
	}
	return indexes
}

// TestRemove_UnlinksScratchLinkBeforeWorktreeRemoval pins Remove unlinking the root `.scratch` link through the gate ahead of the warp worktree removal.
// It claims only a link into the pair's weft worktree and leaves every other root entry to the worktree removal.
func TestRemove_UnlinksScratchLinkBeforeWorktreeRemoval(t *testing.T) {
	t.Parallel()

	t.Run("WiredLink", func(t *testing.T) {
		t.Parallel()

		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		l := h.Location
		const slug = "unlink-scratch-wired"
		hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
		root, target := scratchPaths(l, slug)
		if err := os.WriteFile(filepath.Join(root, scratchNoteName), []byte("written through the link"), 0o644); err != nil {
			t.Fatalf("write through the link: %v", err)
		}

		result, err := h.Topology.Remove(l, slug, true, false)
		if err != nil {
			t.Fatalf("Remove = %v; want nil", err)
		}

		record := result.Mutations
		unlinked := mutationIndexes(record, fabricengine.KindLinkRemoved, scratchHubRelative(t, l, root))
		warpRemoved := mutationIndexes(record, fabricengine.KindWorktreeRemoved, scratchHubRelative(t, l, fabricengine.WorktreePath(l, slug)))
		weftRemoved := mutationIndexes(record, fabricengine.KindWorktreeRemoved, scratchHubRelative(t, l, fabricengine.RecordsWorktreePath(l, slug)))
		if len(unlinked) != 1 || len(warpRemoved) != 1 || len(weftRemoved) != 1 {
			t.Fatalf("record %+v; want one link_removed at the root and one worktree_removed per worktree", record.Entries())
		}
		if unlinked[0] > warpRemoved[0] {
			t.Errorf("link_removed at index %d follows the warp worktree_removed at %d; want it ahead", unlinked[0], warpRemoved[0])
		}
		targetRel := scratchHubRelative(t, l, target)
		for i, entry := range record.Entries() {
			if i < weftRemoved[0] && (entry.Target == targetRel || strings.HasPrefix(entry.Target, targetRel+"/")) {
				t.Errorf("entry %+v at index %d touches the scratch target ahead of the weft worktree_removed", entry, i)
			}
		}
	})

	t.Run("RealDirectoryIsNotClaimed", func(t *testing.T) {
		t.Parallel()

		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		l := h.Location
		const slug = "unlink-scratch-real"
		hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
		root, _ := scratchPaths(l, slug)
		replaceLinkWithRealDirectory(t, root)

		result, err := h.Topology.Remove(l, slug, true, false)
		if err != nil {
			t.Fatalf("Remove = %v; want nil", err)
		}

		if hasMutation(result.Mutations, fabricengine.KindLinkRemoved, scratchHubRelative(t, l, root), "") {
			t.Errorf("record %+v holds link_removed at a real root .scratch", result.Mutations.Entries())
		}
	})

	t.Run("LinkOutsideTheWeftWorktreeIsNotClaimed", func(t *testing.T) {
		t.Parallel()

		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		l := h.Location
		const slug = "unlink-scratch-outside"
		hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
		root, _ := scratchPaths(l, slug)
		other := filepath.Join(t.TempDir(), "elsewhere")
		if err := os.MkdirAll(other, 0o755); err != nil {
			t.Fatalf("create %s: %v", other, err)
		}
		if err := os.WriteFile(filepath.Join(other, scratchNoteName), []byte("outside"), 0o644); err != nil {
			t.Fatalf("write outside note: %v", err)
		}
		removeRootLink(t, root)
		if err := fslink.CreateDirLink(root, other); err != nil {
			t.Fatalf("link %s: %v", root, err)
		}

		result, err := h.Topology.Remove(l, slug, true, false)
		if err != nil {
			t.Fatalf("Remove = %v; want nil", err)
		}

		if hasMutation(result.Mutations, fabricengine.KindLinkRemoved, scratchHubRelative(t, l, root), "") {
			t.Errorf("record %+v holds link_removed at a link outside the weft worktree", result.Mutations.Entries())
		}
		if content, err := os.ReadFile(filepath.Join(other, scratchNoteName)); err != nil || string(content) != "outside" {
			t.Errorf("outside note = %q, %v; want it to survive Remove", content, err)
		}
	})
}

// reconcilePrimePair runs Reconcile over h's hub and returns the result with the prime pair's entry.
func reconcilePrimePair(t *testing.T, h *hubforge.Hub) (fabricengine.ReconcileResult, *fabricengine.ReconcilePairResult) {
	t.Helper()
	l := h.Location
	result, err := h.Topology.Reconcile(l)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	return result, findReconcilePair(t, result.Pairs, fabricengine.RecordsWorktreePath(l, l.WorktreeName))
}

// addRawWarpWorktree adds a warp worktree named slug straight through git, with no weft branch, and returns its path.
// With realLyxDir it also puts a real `_lyx` directory at the worktree's anchor.
func addRawWarpWorktree(t *testing.T, h *hubforge.Hub, slug string, realLyxDir bool) string {
	t.Helper()
	l := h.Location
	path := fabricengine.WorktreePath(l, slug)
	gitkit.Git(t, h.PrimeWorktree(), "worktree", "add", "-b", slug, path)
	if realLyxDir {
		if err := os.MkdirAll(filepath.Join(path, l.AnchorRel, lyxdirs.LyxDirName), 0o755); err != nil {
			t.Fatalf("create real _lyx directory: %v", err)
		}
	}
	return path
}

// TestReconcile_ScratchLink pins the scratch step under Reconcile: it converges every managed pair, reports in the pair's Detail without changing Action or Error, and creates nothing for a pair that never reaches the repair.
func TestReconcile_ScratchLink(t *testing.T) {
	t.Parallel()

	t.Run("RealDirectoryAdoptedOnAHealthyPair", func(t *testing.T) {
		t.Parallel()

		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		l := h.Location
		slug := l.WorktreeName
		root, target := scratchPaths(l, slug)
		replaceLinkWithRealDirectory(t, root)
		removeTarget(t, target)
		writeLegacyScratchExclude(t, l, slug)

		result, pair := reconcilePrimePair(t, h)

		if pair.Action != fabricengine.ReconcileActionAlreadyHealthy || pair.Error != "" {
			t.Errorf("pair = {Action %q, Error %q}; want already_healthy with no error", pair.Action, pair.Error)
		}
		if !strings.Contains(pair.Detail, target) {
			t.Errorf("Detail = %q; want it to name the target %s", pair.Detail, target)
		}
		if !hasMutation(result.Mutations, fabricengine.KindPathRenamed, scratchHubRelative(t, l, target), scratchHubRelative(t, l, root)) {
			t.Errorf("record %+v lacks path_renamed from the root to the target", result.Mutations.Entries())
		}
		lines := gitkit.ExcludeLines(t, fabricengine.WorktreePath(l, slug))
		if !containsLine(lines, "/.scratch") || containsLine(lines, "/.scratch/") {
			t.Errorf("exclude lines = %v; want /.scratch and no /.scratch/", lines)
		}
	})

	t.Run("StepRunsWhenTheJunctionRepairFails", func(t *testing.T) {
		t.Parallel()

		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		l := h.Location
		slug := l.WorktreeName
		root, target := scratchPaths(l, slug)
		replaceLinkWithRealDirectory(t, root)
		removeTarget(t, target)
		lyxLink := fabricengine.WarpLyxLinkHere(l)
		if err := fslink.Remove(lyxLink); err != nil {
			t.Fatalf("remove %s: %v", lyxLink, err)
		}
		if err := os.MkdirAll(lyxLink, 0o755); err != nil {
			t.Fatalf("create real %s: %v", lyxLink, err)
		}
		if err := os.WriteFile(filepath.Join(lyxLink, "held.txt"), []byte("held"), 0o644); err != nil {
			t.Fatalf("write into real %s: %v", lyxLink, err)
		}

		_, pair := reconcilePrimePair(t, h)

		if !strings.Contains(pair.Error, "re-point junction") {
			t.Errorf("Error = %q; want the re-point failure", pair.Error)
		}
		if got, want := describeEntry(t, root), "link:"+target; got != want {
			t.Errorf("root .scratch = %q; want %q", got, want)
		}
		content, err := os.ReadFile(filepath.Join(root, scratchNoteName))
		if err != nil || string(content) != "hello" {
			t.Errorf("note read through .scratch = %q, %v; want %q", content, err, "hello")
		}
	})

	t.Run("FailedTargetParentIsReportedInTheDetail", func(t *testing.T) {
		t.Parallel()

		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		l := h.Location
		slug := l.WorktreeName
		root, _ := scratchPaths(l, slug)
		replaceLinkWithRealDirectory(t, root)
		weftLyx := filepath.Join(fabricengine.RecordsWorktreePath(l, slug), l.AnchorRel, lyxdirs.DotLyxDirName)
		if err := os.RemoveAll(weftLyx); err != nil {
			t.Fatalf("remove %s: %v", weftLyx, err)
		}
		if err := os.WriteFile(weftLyx, []byte("not a directory"), 0o644); err != nil {
			t.Fatalf("write %s: %v", weftLyx, err)
		}

		_, pair := reconcilePrimePair(t, h)

		if !strings.Contains(pair.Detail, root) || !strings.Contains(pair.Detail, "lyx fabric reconcile") {
			t.Errorf("Detail = %q; want it to name %s and `lyx fabric reconcile`", pair.Detail, root)
		}
		if got := describeEntry(t, root); got != "dir" {
			t.Errorf("root .scratch = %q; want the real directory left", got)
		}
		if _, err := os.Stat(filepath.Join(root, scratchNoteName)); err != nil {
			t.Errorf("note missing from the left directory: %v", err)
		}
	})

	for _, tt := range []struct {
		name       string
		realLyxDir bool
		action     fabricengine.ReconcileAction
	}{
		{"RawAdoptedPairGetsNothing", false, fabricengine.ReconcileActionRawAdopted},
		{"UnmanagedPairGetsNothing", true, fabricengine.ReconcileActionUnmanagedReported},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			l := h.Location
			const slug = "scratch-raw"
			addRawWarpWorktree(t, h, slug, tt.realLyxDir)

			result, err := h.Topology.Reconcile(l)
			if err != nil {
				t.Fatalf("Reconcile: %v", err)
			}

			pair := findReconcilePair(t, result.Pairs, fabricengine.RecordsWorktreePath(l, slug))
			if pair.Action != tt.action {
				t.Errorf("Action = %q; want %q", pair.Action, tt.action)
			}
			root, target := scratchPaths(l, slug)
			if got := describeEntry(t, root); got != "absent" {
				t.Errorf("root .scratch = %q; want absent", got)
			}
			if got := describeEntry(t, target); got != "absent" {
				t.Errorf("target = %q; want absent", got)
			}
		})
	}

	for _, tt := range scratchShapeCases {
		t.Run("Leaves_"+tt.name, func(t *testing.T) {
			t.Parallel()

			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			l := h.Location
			slug := l.WorktreeName
			root, _ := scratchPaths(l, slug)
			tt.arrange(t, l, slug)
			named := root
			if tt.name == "FailedExcludeWrite" {
				named = excludeFilePath(t, l, slug)
			}

			_, pair := reconcilePrimePair(t, h)

			if pair.Error != "" {
				t.Errorf("Error = %q; want none", pair.Error)
			}
			for _, want := range []string{tt.phrase, named, "lyx fabric reconcile"} {
				if !strings.Contains(pair.Detail, want) {
					t.Errorf("Detail = %q; want it to contain %q", pair.Detail, want)
				}
			}
		})
	}
}

// assertWarnNames checks that the captured log names path and the reconcile way forward.
func assertWarnNames(t *testing.T, logs, path string) {
	t.Helper()
	if !strings.Contains(logs, path) {
		t.Errorf("warn %q does not name %q", logs, path)
	}
	if !strings.Contains(logs, "lyx fabric reconcile") {
		t.Errorf("warn %q does not name `lyx fabric reconcile`", logs)
	}
}
