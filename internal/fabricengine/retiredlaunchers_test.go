// retiredlaunchers_test.go covers SweepRetiredLaunchers over a seeded `_launchers` tree in a bare temporary hub.
// It needs no git, so it runs in the offline tier.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestSweepRetiredLaunchers pins what the sweep removes, keeps and records for each shape of retired container.
// Every path in a row is hub-relative and slash-separated.
func TestSweepRetiredLaunchers(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	fabricWrittenTree := []string{
		"_launchers/hub.code-workspace",
		"_launchers/y/beam/fabric-checkout.cmd",
		"_launchers/y/beam/fabric-checkout.sh",
		"_launchers/y/beam/ide.cmd",
		"_launchers/y/beam/ide.sh",
		"_launchers/y/beam/run.cmd",
		"_launchers/y/beam/run.sh",
		"_launchers/y/ide-menu.cmd",
		"_launchers/y/ide-menu.sh",
	}
	fabricWrittenRemoved := []string{
		"_launchers/hub.code-workspace",
		"_launchers/y/beam/fabric-checkout.cmd",
		"_launchers/y/beam/fabric-checkout.sh",
		"_launchers/y/beam/ide.cmd",
		"_launchers/y/beam/ide.sh",
		"_launchers/y/beam/run.cmd",
		"_launchers/y/beam/run.sh",
		"_launchers/y/beam",
		"_launchers/y/ide-menu.cmd",
		"_launchers/y/ide-menu.sh",
		"_launchers/y",
		"_launchers",
	}

	tests := []struct {
		name  string
		apply bool
		// files are the regular files seeded in the hub, each with its parent directories.
		files []string
		// linkRel, when set, is a symlink seeded at that path pointing at a directory outside the hub.
		linkRel     string
		wantRemoved []string
		wantKept    []string
		// wantStay lists the paths that still exist afterwards.
		wantStay []string
		// wantGone lists the paths that no longer exist afterwards.
		wantGone []string
	}{
		{
			name:        "NoContainer",
			apply:       true,
			files:       []string{"notes.txt"},
			wantRemoved: []string{},
			wantKept:    []string{},
			wantStay:    []string{"notes.txt"},
		},
		{
			name:        "ContainerIsAFile",
			apply:       true,
			files:       []string{"_launchers"},
			wantRemoved: []string{},
			wantKept:    []string{"kept _launchers: not a directory"},
			wantStay:    []string{"_launchers"},
		},
		{
			name:        "OnlyFabricWrittenFilesUnderNestedAnchors",
			apply:       true,
			files:       fabricWrittenTree,
			wantRemoved: fabricWrittenRemoved,
			wantKept:    []string{},
			wantGone:    []string{"_launchers"},
		},
		{
			name:  "ForeignFileKeepsItsAncestors",
			apply: true,
			files: []string{
				"_launchers/y/beam/ide.sh",
				"_launchers/y/beam/notes.txt",
				"_launchers/z/run.sh",
			},
			wantRemoved: []string{
				"_launchers/y/beam/ide.sh",
				"_launchers/z/run.sh",
				"_launchers/z",
			},
			wantKept: []string{"kept _launchers/y/beam/notes.txt: not a file fabric wrote there"},
			wantStay: []string{
				"_launchers/y/beam/notes.txt",
				"_launchers/y/beam",
				"_launchers/y",
				"_launchers",
			},
			wantGone: []string{"_launchers/y/beam/ide.sh", "_launchers/z"},
		},
		{
			name:        "SymlinkIsKeptAndNotFollowed",
			apply:       true,
			files:       []string{"_launchers/y/ide.sh"},
			linkRel:     "_launchers/y/outside-link",
			wantRemoved: []string{"_launchers/y/ide.sh"},
			wantKept:    []string{"kept _launchers/y/outside-link: not a file fabric wrote there"},
			wantStay:    []string{"_launchers/y/outside-link", "_launchers/y", "_launchers"},
			wantGone:    []string{"_launchers/y/ide.sh"},
		},
		{
			name:        "SymlinkAtTheContainerIsKeptAndNotFollowed",
			apply:       true,
			linkRel:     "_launchers",
			wantRemoved: []string{},
			wantKept:    []string{"kept _launchers: not a directory"},
			wantStay:    []string{"_launchers"},
		},
		{
			name:        "DryRunListsTheRemovalsAndRemovesNothing",
			apply:       false,
			files:       fabricWrittenTree,
			wantRemoved: fabricWrittenRemoved,
			wantKept:    []string{},
			wantStay:    fabricWrittenRemoved,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hub := t.TempDir()
			for _, rel := range tt.files {
				path := filepath.Join(hub, filepath.FromSlash(rel))
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("create the parent of %s: %v", rel, err)
				}
				if err := os.WriteFile(path, []byte("seeded\n"), 0o644); err != nil {
					t.Fatalf("seed %s: %v", rel, err)
				}
			}
			var outsideFiles []string
			if tt.linkRel != "" {
				outside := t.TempDir()
				for _, name := range []string{"sentinel.txt", "ide.sh"} {
					path := filepath.Join(outside, name)
					if err := os.WriteFile(path, []byte("outside\n"), 0o644); err != nil {
						t.Fatalf("seed %s outside the hub: %v", name, err)
					}
					outsideFiles = append(outsideFiles, path)
				}
				if err := os.Symlink(outside, filepath.Join(hub, filepath.FromSlash(tt.linkRel))); err != nil {
					if errors.Is(err, os.ErrPermission) {
						t.Skipf("symlinks need a privilege this host does not grant: %v", err)
					}
					t.Fatalf("seed the symlink %s: %v", tt.linkRel, err)
				}
			}

			rec := fabricengine.NewMutations(hub)
			sweep, err := fabricengine.SweepRetiredLaunchers(rec, hub, tt.apply)
			if err != nil {
				t.Fatalf("SweepRetiredLaunchers(apply %v) error = %v; want nil", tt.apply, err)
			}

			wantRemoved := make([]string, len(tt.wantRemoved))
			for i, rel := range tt.wantRemoved {
				wantRemoved[i] = filepath.Join(hub, filepath.FromSlash(rel))
			}
			if !reflect.DeepEqual(sweep.Removed, wantRemoved) {
				t.Errorf("SweepRetiredLaunchers().Removed = %q; want %q", sweep.Removed, wantRemoved)
			}
			if !reflect.DeepEqual(sweep.Kept, tt.wantKept) {
				t.Errorf("SweepRetiredLaunchers().Kept = %q; want %q", sweep.Kept, tt.wantKept)
			}

			wantEntries := []fabricengine.Mutation{}
			if tt.apply {
				for _, rel := range tt.wantRemoved {
					wantEntries = append(wantEntries, fabricengine.Mutation{Kind: fabricengine.KindPathRemoved, Target: rel, Detail: "single"})
				}
			}
			if got := rec.Entries(); !reflect.DeepEqual(got, wantEntries) {
				t.Errorf("mutation record = %+v; want %+v", got, wantEntries)
			}

			for _, rel := range tt.wantStay {
				if _, err := os.Lstat(filepath.Join(hub, filepath.FromSlash(rel))); err != nil {
					t.Errorf("%s after the sweep: %v; want it kept", rel, err)
				}
			}
			for _, rel := range tt.wantGone {
				if _, err := os.Lstat(filepath.Join(hub, filepath.FromSlash(rel))); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("%s after the sweep: Lstat error = %v; want it removed", rel, err)
				}
			}
			for _, path := range outsideFiles {
				if _, err := os.Stat(path); err != nil {
					t.Errorf("file outside the hub after the sweep: %v; want it untouched", err)
				}
			}
		})
	}
}
