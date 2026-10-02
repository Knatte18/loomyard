//go:build integration

// reviewrange_integration_test.go settles, against a real repository, that the rework-branch review-range command the Webster-Review rubric states lists the live generation's own commits and nothing a mid-run parent merge brought in.

package loomcli

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
)

func TestReviewRange_ReworkBranchListsOnlyOwnCommitsAfterHead(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}
	commit := func(file, msg string) string {
		t.Helper()
		git("add", file)
		git("-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-m", msg)
		return git("rev-parse", "HEAD")
	}
	write := func(file string) {
		t.Helper()
		git("config", "user.name", "t")
		cmd := exec.Command("sh", "-c", "echo "+file+" > "+file)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("write %s: %v\n%s", file, err, out)
		}
	}

	git("init", "-b", "parent")
	write("base.txt")
	commit("base.txt", "base")

	git("checkout", "-b", "task")
	write("a.txt")
	headA := commit("a.txt", "A")

	git("checkout", "parent")
	write("p.txt")
	commit("p.txt", "P")

	git("checkout", "task")
	git("-c", "user.name=t", "-c", "user.email=t@example.com", "merge", "--no-ff", "-m", "M", "parent")
	write("b.txt")
	commit("b.txt", "B")

	var line string
	for _, l := range strings.Split(string(stencils.LoomRubricWebsterReview), "\n") {
		if t := strings.TrimSpace(l); strings.HasPrefix(t, "git log --first-parent") {
			line = t
			break
		}
	}
	if line == "" {
		t.Fatal("rubric has no line beginning `git log --first-parent`")
	}
	line = strings.ReplaceAll(line, "<head_sha>", headA)
	args := strings.Fields(line)[1:]
	args = append([]string{"--no-pager", "log", "--format=%s"}, args[1:]...)

	got := git(args...)
	if !strings.Contains(got, "B") {
		t.Errorf("range output lacks B:\n%s", got)
	}
	for _, unwanted := range []string{"\nP\n", "\nM\n"} {
		if strings.Contains("\n"+got+"\n", unwanted) {
			t.Errorf("range output lists %q:\n%s", strings.TrimSpace(unwanted), got)
		}
	}
}
