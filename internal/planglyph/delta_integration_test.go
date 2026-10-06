//go:build integration

// delta_integration_test.go covers Delta, and DetectDrift's gate one, against a real git fixture
// repository, spawning git through (*quarry.Repo).DeltaGit — the reason this file carries the
// integration build tag rather than running in the untagged tier.

package planglyph

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/quarry"
)

// deltaFixtureRepo is a throwaway git repository built fresh for one test under t.TempDir(), with
// a fixed identity and a fixed default branch name so no machine's global git configuration can
// change the fixture's behaviour.
type deltaFixtureRepo struct {
	t    *testing.T
	root string
}

// newDeltaFixtureRepo initializes a fresh git repository, skipping the whole test when no git
// binary is available on this machine.
func newDeltaFixtureRepo(t *testing.T) *deltaFixtureRepo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not found on this machine")
	}

	f := &deltaFixtureRepo{t: t, root: t.TempDir()}
	f.git("init", "--quiet", "--initial-branch=main")
	f.git("config", "user.name", "planglyph-delta-fixture")
	f.git("config", "user.email", "planglyph-delta-fixture@example.com")
	return f
}

// git runs one git invocation against the fixture's root, failing the test immediately on error.
func (f *deltaFixtureRepo) git(args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", f.root}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// writeAndCommit writes content to path, repository-relative, and commits it, returning the
// resulting commit's SHA.
func (f *deltaFixtureRepo) writeAndCommit(path, content, message string) string {
	f.t.Helper()
	full := filepath.Join(f.root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		f.t.Fatalf("mkdir %q: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		f.t.Fatalf("write %q: %v", full, err)
	}
	f.git("add", "-A")
	f.git("commit", "--quiet", "-m", message)
	return f.git("rev-parse", "HEAD")
}

// TestRealGitDelta is one scenario over one real three-commit repository: the first commit is an
// empty package, the second adds Old and Kept, and the third renames Old to New.
// It covers Delta's revision echo, created symbols and bad-revision error, and it drives the
// repository's real rename answer into DetectDrift's gate one from both sides.
//
// Gate one asks "is this rename some declared Rename card's own expected outcome?" by comparing
// quarry's RenamedPair.To.ID against resolveKeyFor(pair.New) — the plan format requires a symbol
// Rename's New side to be a plan: handle, while quarry reports the new symbol under its bare glyph,
// so the two spellings must be brought together or every declared rename is misread as drift and
// auto-"repaired": a plan-wide RewriteRefs plus an amendment recording work the plan had asked for.
// Every hand-built delta in the untagged tests supplies both sides of that comparison itself, so
// only a real quarry answer can show that quarry's own Symbol.ID spelling still matches.
//
// The rename step FAILS rather than skips when the answer carries no exact-tier pair, because a
// silent skip would restore exactly that blind spot.
func TestRealGitDelta(t *testing.T) {
	// The steps share one git repository, and the rename steps read the delta the "rename delta"
	// step produced, so no step runs in parallel.
	t.Parallel()

	f := newDeltaFixtureRepo(t)
	empty := f.writeAndCommit("sub/a.go", "package sub\n", "first commit")
	added := f.writeAndCommit("sub/a.go", "package sub\n\nfunc Old() {}\n\nfunc Kept() {}\n", "add Old and Kept")
	renamed := f.writeAndCommit("sub/a.go", "package sub\n\nfunc New() {}\n\nfunc Kept() {}\n", "rename Old to New")

	if !t.Run("delta echoes the revisions and lists created symbols", func(t *testing.T) {
		answer, err := Delta(f.root, empty, added)
		if err != nil {
			t.Fatalf("Delta(%q, %q, %q) returned error: %v", f.root, empty, added, err)
		}
		if answer.From != empty {
			t.Errorf("Delta(...).From = %q; want %q", answer.From, empty)
		}
		if answer.To == nil || *answer.To != added {
			t.Errorf("Delta(...).To = %v; want a pointer to %q", answer.To, added)
		}

		found := false
		for _, s := range answer.Created {
			if s.ID == "sub#Old" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Delta(...).Created = %+v; want %q present", answer.Created, "sub#Old")
		}
	}) {
		return
	}

	if !t.Run("a bad revision is quarry unavailable", func(t *testing.T) {
		_, err := Delta(f.root, "does-not-exist-rev", empty)
		if !errors.Is(err, ErrQuarryUnavailable) {
			t.Errorf("Delta(%q, %q, %q) error = %v; want errors.Is(err, ErrQuarryUnavailable)", f.root, "does-not-exist-rev", empty, err)
		}
	}) {
		return
	}

	var delta quarry.GitDeltaAnswer
	if !t.Run("rename delta carries the exact-tier pair", func(t *testing.T) {
		var err error
		delta, err = Delta(f.root, added, renamed)
		if err != nil {
			t.Fatalf("Delta(%q, %q, %q) returned error: %v", f.root, added, renamed, err)
		}

		var matched bool
		for _, pair := range delta.Renamed {
			if pair.From.ID == "sub#Old" && pair.To.ID == "sub#New" {
				matched = true
				break
			}
		}
		if !matched {
			t.Fatalf("real Delta answer carries no exact-tier rename sub#Old -> sub#New; Renamed = %+v. Either quarry no longer classifies this edit as an exact rename, or its Symbol.ID spelling has moved — both break gate one, which compares these IDs against resolveKeyFor's output", delta.Renamed)
		}
	}) {
		return
	}

	// Gate one must leave a declared rename alone: card 1's own Rename pair is the delta's outcome, not drift.
	// This step relies on the delta the previous step produced.
	if !t.Run("gate one recognizes a declared rename", func(t *testing.T) {
		dir, plan := writePlanFixture(t, map[int]string{
			1: "**Rename:**\n- `sub#Old` -> `plan:sub#New`\n\n**Intent:** one\n\n## Rename mechanic\n",
			2: "**Edit:**\n- `sub#Kept`\n\n**Uses:**\n- `plan:sub#New`\n\n**Intent:** two\n",
		})
		before := readCardFile(t, dir, 2, "card2")

		findings, err := DetectDrift(plan, plan, dir, f.root, delta, "deadbeef", driftTestTimestamp)
		if err != nil {
			t.Fatalf("DetectDrift(...) returned error: %v", err)
		}
		if len(findings) != 0 {
			t.Errorf("findings = %+v; want none — the delta's rename is card 1's own declared outcome, not drift", findings)
		}

		if got := readCardFile(t, dir, 2, "card2"); got != before {
			t.Errorf("card 2 was rewritten:\n got %q\nwant %q (unchanged) — gate one must suppress the exact-tier repair for a declared rename", got, before)
		}
		if _, err := os.Stat(filepath.Join(dir, planparser.AmendmentsFileName)); !os.IsNotExist(err) {
			t.Errorf("an amendments file exists after a DECLARED rename; gate one must record no repair (stat err = %v)", err)
		}
	}) {
		return
	}

	// The companion: the same real answer against a plan declaring NO Rename card must auto-repair.
	// Together with the step above it pins gate one from both sides, so a change to quarry's
	// Symbol.ID spelling or to resolveKeyFor breaks exactly one of the two.
	t.Run("gate one repairs an undeclared rename", func(t *testing.T) {
		dir, plan := writePlanFixture(t, map[int]string{
			1: "**Edit:**\n- `sub#Kept`\n\n**Uses:**\n- `sub#Old`\n\n**Intent:** one\n",
		})

		findings, err := DetectDrift(plan, plan, dir, f.root, delta, "deadbeef", driftTestTimestamp)
		if err != nil {
			t.Fatalf("DetectDrift(...) returned error: %v", err)
		}
		if len(findings) != 0 {
			t.Fatalf("findings = %+v; want none for an auto-repaired exact-tier rename", findings)
		}

		requireExactTierRepair(t, dir, []string{"1-card1", "sub#Old", "sub#New", "exact", "deadbeef"})
	})
}
