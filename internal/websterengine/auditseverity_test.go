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
		// A fabric reference with no recorded command is not provably read-only, so it fails closed.
		{"fabric-reference", AuditViolation{Class: ClassFabricReference}, geom, AuditSeverityCorrectness},
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
		{"cd " + dir + "\nrm webster/state.json", AuditSeverityCorrectness},
		{"sleep 1 & rm " + dir + "/webster/state.json", AuditSeverityCorrectness},
		{"echo $(rm " + dir + "/webster/state.json)", AuditSeverityCorrectness},
		{"echo `rm " + dir + "/webster/state.json`", AuditSeverityCorrectness},
		{`bash -c "git -C ` + dir + ` reset --hard"`, AuditSeverityCorrectness},
		{"ls " + dir + "/webster/reports | xargs rm", AuditSeverityCorrectness},
		{"find " + dir + "/webster -name '*.yaml' -delete", AuditSeverityCorrectness},
		{"find " + dir + ` -name x -exec rm {} \;`, AuditSeverityCorrectness},
		{"git --git-dir=" + dir + "/.git checkout HEAD~1 -- webster/state.json", AuditSeverityCorrectness},
		{"perl -pi -e 's/a/b/' " + dir + "/webster/state.json", AuditSeverityCorrectness},
		{"sed -Ei 's/a/b/' " + dir + "/webster/state.json", AuditSeverityCorrectness},
		{"git -C " + dir + " log -1 >> " + dir + "/log.txt", AuditSeverityCorrectness},
		{"cat " + dir + "/webster/state.json | head", AuditSeverityPolicy},
		{"git -C " + dir + " log -1", AuditSeverityPolicy},
		{"lyx fabric status", AuditSeverityPolicy},
		{`grep -n ">" ` + dir + "/webster/state.json", AuditSeverityPolicy},
		{"git -C " + dir + " log --format='%h > %s; x && y' -1", AuditSeverityPolicy},
		{"cat " + dir + "/webster/state.json 2>/dev/null", AuditSeverityPolicy},
		{"cat " + dir + "/webster/state.json > /dev/null 2>&1", AuditSeverityPolicy},
		{"cat " + dir + "/webster/state.json &>/dev/null", AuditSeverityPolicy},
		{"git --git-dir " + dir + "/.git log -1", AuditSeverityPolicy},
		{"git --work-tree " + dir + " status", AuditSeverityPolicy},
		{"perl -MList::Util=sum -ne 'print' " + dir + "/webster/state.json", AuditSeverityCorrectness},
		{"sed -n '1,5p' " + dir + "/webster/state.json", AuditSeverityCorrectness},
		{`python3 -c "open('` + dir + `/webster/state.json','w').write('x')"`, AuditSeverityCorrectness},
		{"dd if=/dev/zero of=" + dir + "/webster/state.json", AuditSeverityCorrectness},
		{"rsync -a x " + dir + "/webster/", AuditSeverityCorrectness},
		{"install x " + dir + "/webster/state.json", AuditSeverityCorrectness},
		{"patch " + dir + "/webster/state.json p.diff", AuditSeverityCorrectness},
		{"chmod 000 " + dir + "/webster/state.json", AuditSeverityCorrectness},
		{"mkdir " + dir + "/webster/x", AuditSeverityCorrectness},
		{"tar -xf a.tar -C " + dir, AuditSeverityCorrectness},
		{"unzip a.zip -d " + dir, AuditSeverityCorrectness},
		{`awk '{print > "f"}' ` + dir + "/webster/state.json", AuditSeverityCorrectness},
		{"bash " + dir + "/script.sh", AuditSeverityCorrectness},
		{`eval "rm ` + dir + `/webster/state.json"`, AuditSeverityCorrectness},
		{"source " + dir + "/env.sh", AuditSeverityCorrectness},
		{"lyx " + "webster rebaseline", AuditSeverityCorrectness},
		{"$TOOL " + dir + "/webster/state.json", AuditSeverityCorrectness},
		{"git -C " + dir + " log --output=" + dir + "/x -1", AuditSeverityCorrectness},
		{"find " + dir + " -fprint " + dir + "/x", AuditSeverityCorrectness},
		{"wc -l " + dir + "/webster/state.json", AuditSeverityPolicy},
		{"stat " + dir + "/webster/state.json", AuditSeverityPolicy},
		{"tail -n 5 " + dir + "/webster/state.json", AuditSeverityPolicy},
		{"find " + dir + ` -name '*.yaml' -exec cat {} \;`, AuditSeverityPolicy},
		{"ls " + dir + " | xargs", AuditSeverityPolicy},
		{"env LC_ALL=C grep x " + dir + "/webster/state.json", AuditSeverityPolicy},
		{`bash -c "cat ` + dir + `/webster/state.json"`, AuditSeverityPolicy},
		{"lyx fabric prune", AuditSeverityPolicy},
		{"./cat " + dir + "/webster/state.json", AuditSeverityCorrectness},
		{"/tmp/tools/grep x " + dir + "/webster/state.json", AuditSeverityCorrectness},
		{"/usr/bin/cat " + dir + "/webster/state.json", AuditSeverityPolicy},
		{"/bin/bash -c 'cat " + dir + "/webster/state.json'", AuditSeverityPolicy},
		{"git -c core.fsmonitor='rm x' -C " + dir + " status", AuditSeverityCorrectness},
		{"git -c diff.external=./x -C " + dir + " diff", AuditSeverityCorrectness},
		{"git --config-env=diff.external=X -C " + dir + " diff", AuditSeverityCorrectness},
		{"git -C " + dir + " grep -Ovim state", AuditSeverityCorrectness},
		{"git -C " + dir + " grep -iO state", AuditSeverityCorrectness},
		{"git -C " + dir + " grep --open-files-in-pager=vim state", AuditSeverityCorrectness},
		{"git -C " + dir + " grep --op=./pg.sh state", AuditSeverityCorrectness},
		{"git -C " + dir + " grep --ope=./pg.sh state", AuditSeverityCorrectness},
		{"git -C " + dir + " grep --op state", AuditSeverityCorrectness},
		{"git -C " + dir + " grep --or -e a -e b", AuditSeverityPolicy},
		{"git -C " + dir + " grep --only-matching state", AuditSeverityPolicy},
		{"git -C " + dir + " grep -n state -- webster", AuditSeverityPolicy},
		{"git -C " + dir + " log -O order.txt -1", AuditSeverityPolicy},
		{"for f in " + dir + `/webster/*; do cat "$f"; done`, AuditSeverityPolicy},
		{"if test -f " + dir + "/x; then cat " + dir + "/x; fi", AuditSeverityPolicy},
		{"while read l; do echo $l; done < " + dir + "/webster/state.json", AuditSeverityPolicy},
		{"[[ -f " + dir + "/x ]] && cat " + dir + "/x", AuditSeverityPolicy},
		{"for f in " + dir + `/webster/*; do rm "$f"; done`, AuditSeverityCorrectness},
		{"for f in $(rm " + dir + "/x); do cat $f; done", AuditSeverityCorrectness},
		{"GIT_EXTERNAL_DIFF=./x git -C " + dir + " diff", AuditSeverityCorrectness},
		{"GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=core.fsmonitor GIT_CONFIG_VALUE_0=./x git -C " + dir + " status", AuditSeverityCorrectness},
		{"env GIT_EXTERNAL_DIFF=./x git -C " + dir + " diff", AuditSeverityCorrectness},
		{"HOME=./h git -C " + dir + " status", AuditSeverityCorrectness},
		{"PATH=./bin cat " + dir + "/webster/state.json", AuditSeverityCorrectness},
		{"PATH=./bin:$PATH; cat " + dir + "/webster/state.json", AuditSeverityCorrectness},
		{"PATH+=:./bin; cat " + dir + "/webster/state.json", AuditSeverityCorrectness},
		{"LD_PRELOAD=./x.so cat " + dir + "/webster/state.json", AuditSeverityCorrectness},
		{"GIT_PAGER=cat git -C " + dir + " log -1", AuditSeverityPolicy},
		{"f=" + dir + "/webster/state.json; cat $f", AuditSeverityPolicy},
		{"sudo -u cat rm " + dir + "/webster/state.json", AuditSeverityCorrectness},
		{"env -u cat rm " + dir + "/webster/state.json", AuditSeverityCorrectness},
		{"xargs --arg-file cat rm " + dir + "/webster/state.json", AuditSeverityCorrectness},
		{"bash " + dir + "/script.sh -c true", AuditSeverityCorrectness},
		{"time -o " + dir + "/t.txt cat " + dir + "/webster/state.json", AuditSeverityCorrectness},
		{"timeout -k 5 10 rm " + dir + "/webster/state.json", AuditSeverityCorrectness},
		{"sudo -u root cat " + dir + "/webster/state.json", AuditSeverityPolicy},
		{"nice -n 5 cat " + dir + "/webster/state.json", AuditSeverityPolicy},
		{"timeout -s KILL 10 cat " + dir + "/webster/state.json", AuditSeverityPolicy},
		{"bash -o pipefail -c 'cat " + dir + "/webster/state.json | head'", AuditSeverityPolicy},
		{"bash -eo pipefail -c 'cat " + dir + "/webster/state.json'", AuditSeverityPolicy},
		{"cat <<EOF | grep x\nrm " + dir + " > y\nEOF\nls " + dir, AuditSeverityPolicy},
		{"ls " + dir + " # > not a redirect", AuditSeverityPolicy},
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
