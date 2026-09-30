//go:build integration

// auditseverity_test.go exercises ClassifyViolation over a real scratch repo, and the stability of the finding Keys CheckParent assigns.

package websterengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

func TestClassifyViolation(t *testing.T) {
	root := gitwrapNewScratchRepo(t)
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored.log\n_lyx/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitwrapCommitFile(t, root, "tracked.txt", "x", "add tracked")
	gitwrapMustGit(t, root, "add", ".gitignore")
	gitwrapMustGit(t, root, "commit", "-m", "ignore")

	geom := Geometry{
		AnchorRoot:   root,
		WorktreeRoot: root,
		WebsterDir:   filepath.Join(root, "_lyx", "webster"),
		ReportsDir:   filepath.Join(root, "_lyx", "webster", "reports"),
		PlanDir:      filepath.Join(root, "_lyx", "plan"),
	}
	outside := t.TempDir()

	// A second fixture whose _lyx is a link to a sibling directory, written through the link's target.
	linkRoot := gitwrapNewScratchRepo(t)
	weft := t.TempDir()
	if err := os.MkdirAll(filepath.Join(weft, "webster"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fslink.CreateDirLink(filepath.Join(linkRoot, "_lyx"), weft); err != nil {
		t.Fatal(err)
	}
	linkGeom := Geometry{
		AnchorRoot:   linkRoot,
		WorktreeRoot: linkRoot,
		WebsterDir:   filepath.Join(linkRoot, "_lyx", "webster"),
		ReportsDir:   filepath.Join(linkRoot, "_lyx", "webster", "reports"),
		PlanDir:      filepath.Join(linkRoot, "_lyx", "plan"),
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
		{"fabric-reference", AuditViolation{Class: ClassFabricReference}, geom, AuditSeverityPolicy},
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
		{"through link target", AuditViolation{Class: ClassParentWrite, Path: filepath.Join(weft, "webster", "state.json")}, linkGeom, AuditSeverityCorrectness},
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

func TestClassifyViolation_FabricReferenceByCommand(t *testing.T) {
	const dir = "/fabric/sibling"
	tests := []struct {
		command string
		want    AuditSeverity
	}{
		{"git -C " + dir + " checkout HEAD~1 -- webster/state.json", AuditSeverityCorrectness},
		{"git -C " + dir + " reset --hard", AuditSeverityCorrectness},
		{"git -C " + dir + " stash", AuditSeverityCorrectness},
		{"rm " + dir + "/webster/reports/01-a.yaml", AuditSeverityCorrectness},
		{"echo x > " + dir + "/webster/state.json", AuditSeverityCorrectness},
		{"lyx fabric commit", AuditSeverityCorrectness},
		{"cat " + dir + "/webster/state.json | head", AuditSeverityPolicy},
		{"git -C " + dir + " log -1", AuditSeverityPolicy},
		{"lyx fabric status", AuditSeverityPolicy},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			v := AuditViolation{Class: ClassFabricReference, Command: tt.command}
			got, err := ClassifyViolation(v, Geometry{})
			if err != nil {
				t.Fatalf("ClassifyViolation: %v", err)
			}
			if got != tt.want {
				t.Errorf("ClassifyViolation(%q) = %q; want %q", tt.command, got, tt.want)
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
