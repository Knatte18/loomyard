//go:build integration

// auditseverity_test.go exercises ClassifyViolation over a real scratch repo, and the stability of the finding Keys CheckParent assigns.

package websterengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

func TestClassifyViolation(t *testing.T) {
	root := gitwrapNewScratchRepo(t)
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored.log\n_lyx/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitkit.CommitFile(t, root, "tracked.txt", "x", "add tracked")
	gitkit.Git(t, root, "add", ".gitignore")
	gitkit.Git(t, root, "commit", "-m", "ignore")

	geom := Geometry{
		AnchorRoot:   root,
		WorktreeRoot: root,
		WebsterDir:   filepath.Join(root, "_lyx", "webster"),
		ReportsDir:   filepath.Join(root, "_lyx", "webster", "reports"),
		PlanDir:      filepath.Join(root, "_lyx", "plan"),
		ScratchDir:   filepath.Join(root, ".lyx", "scratch"),
	}
	outside := t.TempDir()

	// A second fixture whose _lyx is a link to a sibling directory, written through the link's target.
	linkRoot := gitwrapNewScratchRepo(t)
	lyxTarget := t.TempDir()
	if err := os.MkdirAll(filepath.Join(lyxTarget, "webster"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fslink.CreateDirLink(filepath.Join(linkRoot, "_lyx"), lyxTarget); err != nil {
		t.Fatal(err)
	}
	linkGeom := Geometry{
		AnchorRoot:   linkRoot,
		WorktreeRoot: linkRoot,
		WebsterDir:   filepath.Join(linkRoot, "_lyx", "webster"),
		ReportsDir:   filepath.Join(linkRoot, "_lyx", "webster", "reports"),
		PlanDir:      filepath.Join(linkRoot, "_lyx", "plan"),
		ScratchDir:   filepath.Join(linkRoot, ".lyx", "scratch"),
	}

	// A second worktree of root, with a _lyx link pointing to a temp directory.
	second := filepath.Join(t.TempDir(), "second")
	gitkit.Git(t, root, "worktree", "add", second)
	secondLyx := t.TempDir()
	if err := fslink.CreateDirLink(filepath.Join(second, "_lyx"), secondLyx); err != nil {
		t.Fatal(err)
	}

	// A third worktree of root, nested inside root's own checkout, with its own geometry.
	nested := filepath.Join(root, "wts", "nested")
	gitkit.Git(t, root, "worktree", "add", nested)
	nestedGeom := Geometry{
		AnchorRoot:   nested,
		WorktreeRoot: nested,
		WebsterDir:   filepath.Join(nested, "_lyx", "webster"),
		ReportsDir:   filepath.Join(nested, "_lyx", "webster", "reports"),
		PlanDir:      filepath.Join(nested, "_lyx", "plan"),
		ScratchDir:   filepath.Join(nested, ".lyx", "scratch"),
	}
	if err := os.WriteFile(filepath.Join(nested, "ignored.log"), []byte("i"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, "untracked.txt"), []byte("u"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ignored.log"), []byte("i"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		v    AuditViolation
		geom Geometry
		want AuditSeverity
	}{
		{"fork-contract-write", AuditViolation{Class: ClassForkContractWrite, Path: "x"}, geom, AuditSeverityCorrectness},
		{"fork-plan-write", AuditViolation{Class: ClassForkPlanWrite, Path: "x"}, geom, AuditSeverityCorrectness},
		{"fork-state-write", AuditViolation{Class: ClassForkStateWrite, Path: "x"}, geom, AuditSeverityCorrectness},
		{"fabric-reference", AuditViolation{Class: ClassFabricReference}, geom, AuditSeverityCorrectness},
		{"fabric-reference read-only command", AuditViolation{Class: ClassFabricReference, Command: "cat <dir>/webster/state.json"}, geom, AuditSeverityCorrectness},
		{"named-spawn", AuditViolation{Class: ClassNamedSpawn}, geom, AuditSeverityPolicy},
		{"nested-agent", AuditViolation{Class: ClassNestedAgent}, geom, AuditSeverityPolicy},
		{"tracked file", AuditViolation{Class: ClassParentWrite, Path: filepath.Join(root, "tracked.txt")}, geom, AuditSeverityCorrectness},
		{"untracked non-ignored", AuditViolation{Class: ClassParentWrite, Path: filepath.Join(root, "untracked.txt")}, geom, AuditSeverityCorrectness},
		{"relative untracked", AuditViolation{Class: ClassParentWrite, Path: "untracked.txt"}, geom, AuditSeverityCorrectness},
		{"ignored outside _lyx", AuditViolation{Class: ClassParentWrite, Path: filepath.Join(root, "ignored.log")}, geom, AuditSeverityPolicy},
		{"state.json", AuditViolation{Class: ClassParentWrite, Path: filepath.Join(root, "_lyx", "webster", "state.json")}, geom, AuditSeverityCorrectness},
		{"report", AuditViolation{Class: ClassParentWrite, Path: filepath.Join(root, "_lyx", "webster", "reports", "01-a.yaml")}, geom, AuditSeverityCorrectness},
		{"plan file", AuditViolation{Class: ClassParentWrite, Path: filepath.Join(root, "_lyx", "plan", "00-overview.md")}, geom, AuditSeverityCorrectness},
		{"outside worktree", AuditViolation{Class: ClassParentWrite, Path: filepath.Join(outside, "f.txt")}, geom, AuditSeverityPolicy},
		{"pause flag", AuditViolation{Class: ClassParentWrite, Path: filepath.Join(geom.ScratchDir, "pause")}, geom, AuditSeverityCorrectness},
		{"lock file", AuditViolation{Class: ClassParentWrite, Path: filepath.Join(geom.ScratchDir, "mutate.lock")}, geom, AuditSeverityCorrectness},
		{"reed launch script", AuditViolation{Class: ClassParentWrite, Path: filepath.Join(root, ".lyx", "reed", "launch", "guid.sh")}, geom, AuditSeverityCorrectness},
		{"other module lock", AuditViolation{Class: ClassParentWrite, Path: filepath.Join(root, ".lyx", "shed", "run.lock")}, geom, AuditSeverityCorrectness},
		{"ignored build artifact outside state dirs", AuditViolation{Class: ClassParentWrite, Path: filepath.Join(root, "ignored.log")}, geom, AuditSeverityPolicy},
		{"other worktree tracked file", AuditViolation{Class: ClassParentWrite, Path: filepath.Join(second, "tracked.txt")}, geom, AuditSeverityCorrectness},
		{"other worktree _lyx link", AuditViolation{Class: ClassParentWrite, Path: filepath.Join(secondLyx, "x.md")}, geom, AuditSeverityCorrectness},
		{"nested worktree tracked file", AuditViolation{Class: ClassParentWrite, Path: filepath.Join(nested, "tracked.txt")}, geom, AuditSeverityCorrectness},
		{"inside nested worktree, ignored", AuditViolation{Class: ClassParentWrite, Path: filepath.Join(nested, "ignored.log")}, nestedGeom, AuditSeverityPolicy},
		{"inside nested worktree, tracked", AuditViolation{Class: ClassParentWrite, Path: filepath.Join(nested, "tracked.txt")}, nestedGeom, AuditSeverityCorrectness},
		{"through link target", AuditViolation{Class: ClassParentWrite, Path: filepath.Join(lyxTarget, "webster", "state.json")}, linkGeom, AuditSeverityCorrectness},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ClassifyViolation(tt.v, tt.geom)
			if err != nil {
				t.Fatalf("ClassifyViolation: %v", err)
			}
			if got != tt.want {
				t.Errorf("ClassifyViolation = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestCheckParent_KeysDistinctAndStable(t *testing.T) {
	audit := shuttleengine.ForkAudit{
		NamedSpawns:  2,
		ParentWrites: []string{"/w/a.go", "/w/b.go"},
	}
	first := CheckParent(audit, "/w/o.yaml", "/w/s.md", "/w", NeverMatches{})
	audit.NamedSpawns = 3
	audit.ParentWrites = append(audit.ParentWrites, "/w/c.go")
	grown := CheckParent(audit, "/w/o.yaml", "/w/s.md", "/w", NeverMatches{})

	seen := map[string]bool{}
	for _, v := range first {
		if v.Key == "" || seen[v.Key] {
			t.Fatalf("key %q empty or repeated in %v", v.Key, first)
		}
		seen[v.Key] = true
	}
	if len(first) != 4 || len(grown) != 6 {
		t.Fatalf("violations = %d then %d; want 4 then 6", len(first), len(grown))
	}
	grownKeys := map[string]bool{}
	for _, v := range grown {
		grownKeys[v.Key] = true
	}
	for k := range seen {
		if !grownKeys[k] {
			t.Errorf("key %q did not survive the audit growing", k)
		}
	}
}
