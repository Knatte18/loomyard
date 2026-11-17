// instructionfiles_test.go covers the instruction files a Codex launch reads and the CLAUDE.md files it names as unread.

package codexengine

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/heddleengine"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/permkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestPrepare_InstructionFiles pins the project instruction cap and the pointers to CLAUDE.md files Codex does not read.
// The engine is anchored in src/app of the worktree.
// The cap is the size of the files Codex reads from the root down to the anchor, and at least Codex's default.
// A CLAUDE.md is named when Codex does not read it itself.
// A directory the walk cannot read is skipped with a warning.
// The pointers sit between the user's instruction files and the skill listing.
func TestPrepare_InstructionFiles(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	filler := func(size int) string { return strings.Repeat("a", size) }
	tests := []struct {
		name string
		// files maps a path under the worktree root to its text.
		files map[string]string
		// unreadableDir is a directory under the root that is made unreadable.
		unreadableDir string
		// withUserSetup gives the engine a source returning one instruction file and a skill listing.
		withUserSetup bool
		// wantCap is the project_doc_max_bytes the launch line carries.
		wantCap int
		// wantNamed are the CLAUDE.md files, under the root, the pointers name.
		wantNamed []string
		// wantWarn is a text the warnings must hold.
		wantWarn string
		// anchorRel is the folder of the worktree the engine is anchored at; empty means src/app.
		anchorRel string
		// selfLinks are paths under the root made as links to themselves, which cannot be examined.
		selfLinks []string
		// wantErr is a text the refusal must hold; empty means the launch goes ahead.
		wantErr string
	}{
		{
			name:    "SmallSetGivesTheDefaultCap",
			files:   map[string]string{"AGENTS.md": "root rules", "src/app/AGENTS.md": "app rules"},
			wantCap: defaultProjectDocMaxBytes,
		},
		{
			name:    "FilesOverTheDefaultGiveTheirSum",
			files:   map[string]string{"AGENTS.md": filler(20000), "src/app/AGENTS.md": filler(20000)},
			wantCap: 40000,
		},
		{
			name: "OverrideCountsInsteadOfAgents",
			files: map[string]string{
				"AGENTS.md": filler(10000), "src/app/AGENTS.override.md": filler(25000), "src/app/AGENTS.md": filler(5000),
			},
			wantCap: 35000,
		},
		{
			name:      "ClaudeFileBelowTheAnchorIsNamed",
			files:     map[string]string{"src/app/deep/CLAUDE.md": "deep rules"},
			wantCap:   defaultProjectDocMaxBytes,
			wantNamed: []string{"src/app/deep/CLAUDE.md"},
		},
		{
			name: "ClaudeFilesBesideAgentsFilesAreNamed",
			files: map[string]string{
				"AGENTS.md": "root", "CLAUDE.md": "root rules", "src/lib/AGENTS.md": "lib", "src/lib/CLAUDE.md": "lib rules",
			},
			wantCap:   defaultProjectDocMaxBytes,
			wantNamed: []string{"CLAUDE.md", "src/lib/CLAUDE.md"},
		},
		{
			name: "ClaudeFileBesideAHeddleRenderedOverrideIsNotNamed",
			files: map[string]string{
				"src/app/AGENTS.override.md": heddleengine.Marker + "\nrendered", "src/app/AGENTS.md": "app", "src/app/CLAUDE.md": "app rules",
			},
			wantCap: defaultProjectDocMaxBytes,
		},
		{
			name:    "LoneClaudeFileWithNoImportIsNotNamed",
			files:   map[string]string{"CLAUDE.md": "root rules"},
			wantCap: defaultProjectDocMaxBytes,
		},
		{
			name: "UnmarkedOverrideLeavesTheClaudeFileNamed",
			files: map[string]string{
				"src/app/AGENTS.override.md": "own rules", "src/app/AGENTS.md": "app", "src/app/CLAUDE.md": "app rules",
			},
			wantCap:   defaultProjectDocMaxBytes,
			wantNamed: []string{"src/app/CLAUDE.md"},
		},
		{
			name:      "ClaudeFileWithAnImportIsNamed",
			files:     map[string]string{"CLAUDE.md": "see @docs/rules.md", "docs/rules.md": "imported rules"},
			wantCap:   defaultProjectDocMaxBytes,
			wantNamed: []string{"CLAUDE.md"},
		},
		{
			name: "ClaudeFileWithAnImportBesideAHeddleRenderedOverrideIsNotNamed",
			files: map[string]string{
				"src/app/AGENTS.override.md": heddleengine.Marker + "\nrendered", "src/app/CLAUDE.md": "see @docs/rules.md", "docs/rules.md": "imported rules",
			},
			wantCap: defaultProjectDocMaxBytes,
		},
		{
			name:    "ClaudeFileInADependencyDirectoryIsNotNamed",
			files:   map[string]string{"src/app/node_modules/dep/CLAUDE.md": "dependency rules"},
			wantCap: defaultProjectDocMaxBytes,
		},
		{
			name:      "ClaudeFileBesideAgentsIsNamedWhenTheAnchorIsInADependencyDirectoryName",
			anchorRel: "vendor/tool",
			files:     map[string]string{"vendor/tool/AGENTS.md": "tool", "vendor/tool/CLAUDE.md": "tool rules"},
			wantCap:   defaultProjectDocMaxBytes,
			wantNamed: []string{"vendor/tool/CLAUDE.md"},
		},
		{
			name:      "InstructionFileThatCannotBeExaminedRefuses",
			selfLinks: []string{"src/app/AGENTS.override.md"},
			wantErr:   "examine the instruction file",
		},
		{
			name:          "UnreadableDirectoryIsSkippedWithAWarning",
			files:         map[string]string{"src/locked/inner.txt": "x"},
			unreadableDir: "src/locked",
			wantCap:       defaultProjectDocMaxBytes,
			wantWarn:      filepath.Join("src", "locked"),
		},
		{
			name:          "PointersSitBetweenTheFilesAndTheListing",
			files:         map[string]string{"AGENTS.md": "root", "CLAUDE.md": "root rules"},
			withUserSetup: true,
			wantCap:       defaultProjectDocMaxBytes,
			wantNamed:     []string{"CLAUDE.md"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.unreadableDir != "" && os.Geteuid() == 0 {
				t.Skip("root reads every directory, so no directory is unreadable to it")
			}
			repo := newRepoFixture(t)
			root, _ := repo.addWorktree(t, "wt")
			anchorRel := filepath.Join("src", "app")
			if tt.anchorRel != "" {
				anchorRel = tt.anchorRel
			}
			anchor := filepath.Join(root, anchorRel)
			mkdirAll(t, anchor)
			repo.writeCodexLayer(t, anchor, "")
			writeWritableRoots(t, anchor, "")
			for rel, text := range tt.files {
				mkdirAll(t, filepath.Dir(filepath.Join(root, rel)))
				writeFile(t, filepath.Join(root, rel), text)
			}
			for _, rel := range tt.selfLinks {
				if err := os.Symlink(filepath.Base(rel), filepath.Join(root, rel)); err != nil {
					t.Fatalf("symlink: %v", err)
				}
			}
			if tt.unreadableDir != "" {
				permkit.Chmod(t, filepath.Join(root, tt.unreadableDir), 0)
			}
			engine, warnings := newEngine(anchor)
			if tt.withUserSetup {
				engine.userSetup = &fakeUserSetupSource{setup: shuttleengine.UserSetup{
					InstructionFiles: []shuttleengine.InstructionItem{{Name: "/u/CLAUDE.md", Text: "user file text", WayForward: "shorten it"}},
					SkillListing:     "skill listing text",
				}}
			}

			runDir := t.TempDir()
			launch, err := engine.Prepare(runDir, shuttleengine.Spec{Prompt: "p", PromptEngine: engineName}, shuttleengine.Config{})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), "way forward") {
					t.Fatalf("Prepare() error = %v; want a refusal holding %q and a way forward", err, tt.wantErr)
				}
				if _, statErr := os.Stat(filepath.Join(runDir, "prompt.md")); !errors.Is(statErr, os.ErrNotExist) {
					t.Errorf("prompt.md stat error = %v; want no prompt file", statErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Prepare() error: %v", err)
			}

			if want := shell.ForGOOS().Quote("project_doc_max_bytes=" + strconv.Itoa(tt.wantCap)); !strings.Contains(launch.Cmd, want) {
				t.Errorf("Prepare() launch line = %q; want it to hold %s", launch.Cmd, want)
			}
			named := map[string]bool{}
			for _, rel := range tt.wantNamed {
				named[rel] = true
			}
			for rel := range tt.files {
				if filepath.Base(rel) != instructionFallbackFile {
					continue
				}
				if got := strings.Contains(launch.Cmd, filepath.Join(root, rel)); got != named[rel] {
					t.Errorf("launch line naming %s = %v; want %v", rel, got, named[rel])
				}
			}
			if !strings.Contains(warnings.String(), tt.wantWarn) {
				t.Errorf("warnings = %q; want them to hold %q", warnings, tt.wantWarn)
			}
			if tt.wantWarn == "" && warnings.String() != "" {
				t.Errorf("warnings = %q; want none", warnings)
			}
			if tt.withUserSetup {
				file := strings.Index(launch.Cmd, "user file text")
				pointer := strings.Index(launch.Cmd, filepath.Join(root, "CLAUDE.md"))
				listing := strings.Index(launch.Cmd, "skill listing text")
				if file < 0 || file >= pointer || pointer >= listing {
					t.Errorf("positions of the file, the pointer and the listing = %d, %d, %d; want them in that order", file, pointer, listing)
				}
			}
		})
	}
}
