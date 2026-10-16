// merriambase_test.go covers the start base the batchifier prices Merriam's session from: the line sum of the texts it loads, the fixed system-prompt context, and how the base follows the worktree's CLAUDE.md and the plan's overview.
// Tier 1: the base is read from a temp worktree with the shipped stencils seeded; no git, no process.

package websterengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// instructionFileEngine is a shuttlefake.Engine that also offers the InstructionFileLister capability.
type instructionFileEngine struct {
	*shuttlefake.Engine
	files []string
}

// InstructionFiles returns the file names the engine was built with.
func (e instructionFileEngine) InstructionFiles() []string { return e.files }

// TestMerriamBase pins MerriamBaseOf's line sum and fixed context, and that MerriamBase grows with the plan's 00-overview.md and with the worktree's CLAUDE.md, adds nothing for an absent CLAUDE.md or CLAUDE.local.md and refuses an absent overview.
// The steps share one temp worktree and each rewrites its own files, so none relies on another's result.
// The scenario calls t.Parallel as a whole and no step does, since the steps share the one worktree.
func TestMerriamBase(t *testing.T) {
	t.Parallel()

	// wantFixed is 20600, the measured Merriam session start, minus the computed size of the texts the base covers at the commit the constant was taken at.
	const wantFixed = 15896

	t.Run("MerriamBaseOf sums the lines of its texts and carries the fixed context", func(t *testing.T) {
		got := websterengine.MerriamBaseOf("one\ntwo\n", "three", "", "four\nfive\nsix\n")

		want := batcher.StartBase{Lines: 6, Fixed: wantFixed}
		if got != want {
			t.Errorf("MerriamBaseOf() = %+v; want %+v", got, want)
		}
		if none := websterengine.MerriamBaseOf(); none != (batcher.StartBase{Fixed: wantFixed}) {
			t.Errorf("MerriamBaseOf() with no text = %+v; want only the fixed context", none)
		}
	})

	t.Run("MasterInstructionFiles lists the names Master's engine offers", func(t *testing.T) {
		withFiles := shuttlefake.EngineSet(instructionFileEngine{Engine: &shuttlefake.Engine{}, files: []string{"AGENTS.md"}})
		plain := shuttlefake.EngineSet(&shuttlefake.Engine{})

		got, err := websterengine.MasterInstructionFiles(withFiles, shuttleengine.ModelChoice{})()
		if err != nil || !reflect.DeepEqual(got, []string{"AGENTS.md"}) {
			t.Errorf("MasterInstructionFiles() over an engine with the capability = %v, %v; want [AGENTS.md], nil", got, err)
		}

		got, err = websterengine.MasterInstructionFiles(plain, shuttleengine.ModelChoice{})()
		if err != nil || len(got) != 0 {
			t.Errorf("MasterInstructionFiles() over an engine without the capability = %v, %v; want none, nil", got, err)
		}

		_, err = websterengine.MasterInstructionFiles(plain, shuttleengine.ModelChoice{Engine: "gone"})()
		if err == nil {
			t.Fatal("MasterInstructionFiles() error = nil; want a refusal for an engine the set lacks")
		}
		if !errors.Is(err, shuttleengine.ErrEngineNotRegistered) {
			t.Errorf("MasterInstructionFiles() error = %v; want it to wrap ErrEngineNotRegistered", err)
		}
		for _, want := range []string{"master", "set master in webster.yaml"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("MasterInstructionFiles() error = %q; want it to contain %q", err.Error(), want)
			}
		}
	})

	worktree := t.TempDir()
	planDir := filepath.Join(worktree, "plan")
	stencilsDir := stencilkit.Seed(t)
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}
	geom := websterengine.Geometry{WorktreeRoot: worktree, PlanDir: planDir, StencilsDir: stencilsDir}
	write := func(t *testing.T, path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	overview := filepath.Join(planDir, "00-overview.md")
	claude := filepath.Join(worktree, "CLAUDE.md")
	claudeLocal := filepath.Join(worktree, "CLAUDE.local.md")
	claudeFiles := []string{"CLAUDE.md", "CLAUDE.local.md"}

	baseLines := 0
	t.Run("an absent CLAUDE.md and CLAUDE.local.md add nothing", func(t *testing.T) {
		write(t, overview, "# plan\n")

		got, err := websterengine.MerriamBase(geom, claudeFiles)
		if err != nil {
			t.Fatalf("MerriamBase() error = %v; want nil", err)
		}
		if got.Fixed != wantFixed || got.Lines == 0 {
			t.Errorf("MerriamBase() = %+v; want the fixed context and the stencil and overview lines", got)
		}
		baseLines = got.Lines
	})

	t.Run("the base grows with the overview", func(t *testing.T) {
		write(t, overview, "# plan\nsecond line\nthird line\n")

		got, err := websterengine.MerriamBase(geom, claudeFiles)
		if err != nil {
			t.Fatalf("MerriamBase() error = %v; want nil", err)
		}
		if want := baseLines + 2; got.Lines != want {
			t.Errorf("MerriamBase().Lines = %d; want %d, two overview lines more", got.Lines, want)
		}
		baseLines = got.Lines
	})

	t.Run("the base grows with CLAUDE.md and CLAUDE.local.md", func(t *testing.T) {
		write(t, claude, "a\nb\nc\n")
		write(t, claudeLocal, "d\n")

		got, err := websterengine.MerriamBase(geom, claudeFiles)
		if err != nil {
			t.Fatalf("MerriamBase() error = %v; want nil", err)
		}
		if want := baseLines + 4; got.Lines != want {
			t.Errorf("MerriamBase().Lines = %d; want %d, four CLAUDE lines more", got.Lines, want)
		}
	})

	t.Run("the base counts only the files it is told", func(t *testing.T) {
		write(t, filepath.Join(worktree, "AGENTS.md"), "x\ny\n")

		agents, err := websterengine.MerriamBase(geom, []string{"AGENTS.md"})
		if err != nil {
			t.Fatalf("MerriamBase() with AGENTS.md error = %v; want nil", err)
		}
		if want := baseLines + 2; agents.Lines != want {
			t.Errorf("MerriamBase().Lines with AGENTS.md alone = %d; want %d, its two lines and not the present CLAUDE.md", agents.Lines, want)
		}

		none, err := websterengine.MerriamBase(geom, nil)
		if err != nil {
			t.Fatalf("MerriamBase() with no files error = %v; want nil", err)
		}
		if none.Lines != baseLines {
			t.Errorf("MerriamBase().Lines with no files = %d; want %d, nothing over a present CLAUDE.md", none.Lines, baseLines)
		}
	})

	t.Run("a named file that is a directory is refused naming it", func(t *testing.T) {
		if err := os.Mkdir(filepath.Join(worktree, "DIRECTORY.md"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		_, err := websterengine.MerriamBase(geom, []string{"DIRECTORY.md"})
		if err == nil {
			t.Fatal("MerriamBase() error = nil; want a refusal for the directory")
		}
		if !strings.Contains(err.Error(), "DIRECTORY.md") {
			t.Errorf("MerriamBase() error = %q; want it to contain %q", err.Error(), "DIRECTORY.md")
		}
	})

	t.Run("the base grows by the declared changes of the decision record and ignores a malformed record", func(t *testing.T) {
		write(t, overview, "# plan\n")
		repoRoot := t.TempDir()
		write(t, filepath.Join(repoRoot, "PATTERN.md"), "# PATTERN\n\nsome constraints\n")
		record := filepath.Join(worktree, "decision-record.md")
		patternGeom := geom
		patternGeom.RepoRoot = repoRoot
		patternGeom.DecisionRecordPath = record

		noRecord, err := websterengine.MerriamBase(patternGeom, nil)
		if err != nil {
			t.Fatalf("MerriamBase() with no record error = %v; want nil", err)
		}

		write(t, record, "## PATTERN changes\n\n- `PATTERN-x` — removes\n")
		bullets, err := discussionparser.DeclaredPatternChanges(record)
		if err != nil || len(bullets) != 1 {
			t.Fatalf("DeclaredPatternChanges() = %v, %v; want one bullet, nil", bullets, err)
		}
		with, err := pattern.Directive(repoRoot, stencilsDir, pattern.RoleOrchestrator, bullets)
		if err != nil {
			t.Fatalf("pattern.Directive() with the bullet error = %v; want nil", err)
		}
		without, err := pattern.Directive(repoRoot, stencilsDir, pattern.RoleOrchestrator, nil)
		if err != nil {
			t.Fatalf("pattern.Directive() with no bullet error = %v; want nil", err)
		}
		got, err := websterengine.MerriamBase(patternGeom, nil)
		if err != nil {
			t.Fatalf("MerriamBase() with a record error = %v; want nil", err)
		}
		if want := noRecord.Lines + websterengine.MerriamBaseOf(with).Lines - websterengine.MerriamBaseOf(without).Lines; got.Lines != want || got.Lines <= noRecord.Lines {
			t.Errorf("MerriamBase().Lines = %d; want %d, the rendered fragment's lines over the no-record base %d", got.Lines, want, noRecord.Lines)
		}

		write(t, record, "## PATTERN changes\n\n- `PATTERN-a` — renames: x\n")
		malformed, err := websterengine.MerriamBase(patternGeom, nil)
		if err != nil {
			t.Fatalf("MerriamBase() with a malformed record error = %v; want nil", err)
		}
		if malformed != noRecord {
			t.Errorf("MerriamBase() with a malformed record = %+v; want the no-record base %+v", malformed, noRecord)
		}
	})

	t.Run("an absent overview is refused naming the file and no way forward", func(t *testing.T) {
		if err := os.Remove(overview); err != nil {
			t.Fatalf("remove overview: %v", err)
		}

		_, err := websterengine.MerriamBase(geom, claudeFiles)
		if err == nil {
			t.Fatal("MerriamBase() error = nil; want a refusal for the absent overview")
		}
		if !strings.Contains(err.Error(), "00-overview.md") {
			t.Errorf("MerriamBase() error = %q; want it to contain %q", err.Error(), "00-overview.md")
		}
		if strings.Contains(err.Error(), "way forward") {
			t.Errorf("MerriamBase() error = %q; want no way forward", err.Error())
		}
	})
}
