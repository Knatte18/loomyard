//go:build integration

// commitrecords_integration_test.go drives commitRecordsVerb with the real loomCommitStatusDeps over the real-pair fixture wiring_commitstatus_integration_test.go declares.

package loomcli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestCommitRecordsVerb_Real_CommitsAndPushesLateDriveReport asserts a drive report written after the last transition lands on the records tip and the records origin's branch carries it.
func TestCommitRecordsVerb_Real_CommitsAndPushesLateDriveReport(t *testing.T) {
	_, location, recordsSibling := realSeamFixture(t)
	writeRecordFile(t, filepath.Join(shedrun.DriveReportsDir(location, shedrun.SelfRunID), "report.md"), "stop report\n")

	var out bytes.Buffer
	if code := commitRecordsVerb(&out, loomCommitStatusDeps(location, shedrun.SelfRunID)); code != 0 {
		t.Fatalf("exit = %d; want 0; output %s", code, out.String())
	}

	want := filepath.ToSlash(filepath.Join(shedrun.DriveReportsRel(location, shedrun.SelfRunID), "report.md"))
	if got := gitkit.Git(t, recordsSibling, "show", "--name-only", "--format=", "HEAD"); !strings.Contains(got, want) {
		t.Errorf("records HEAD touched %q; want it to include %q", got, want)
	}
	if got := gitkit.Git(t, recordsSibling, "log", "--oneline", "@{u}..HEAD"); got != "" {
		t.Errorf("unpushed records commits = %q; want none", got)
	}
	branch := gitkit.CurrentBranch(t, recordsSibling)
	remote := gitkit.Git(t, recordsSibling, "ls-remote", "origin", "refs/heads/"+branch)
	head := gitkit.RevParse(t, recordsSibling, "HEAD")
	if !strings.HasPrefix(remote, head) {
		t.Errorf("origin %s = %q; want it at the records HEAD %s", branch, remote, head)
	}
}

// TestCommitRecordsVerb_Real_CleanTreeIsNoOpSuccess asserts a second call over the now-clean tree succeeds and leaves the records HEAD unchanged.
func TestCommitRecordsVerb_Real_CleanTreeIsNoOpSuccess(t *testing.T) {
	_, location, recordsSibling := realSeamFixture(t)
	writeRecordFile(t, filepath.Join(shedrun.DriveReportsDir(location, shedrun.SelfRunID), "report.md"), "stop report\n")
	deps := loomCommitStatusDeps(location, shedrun.SelfRunID)

	var first bytes.Buffer
	if code := commitRecordsVerb(&first, deps); code != 0 {
		t.Fatalf("first exit = %d; want 0; output %s", code, first.String())
	}
	before := gitkit.RevParse(t, recordsSibling, "HEAD")

	var second bytes.Buffer
	if code := commitRecordsVerb(&second, deps); code != 0 {
		t.Fatalf("second exit = %d; want 0; output %s", code, second.String())
	}
	if got := gitkit.RevParse(t, recordsSibling, "HEAD"); got != before {
		t.Errorf("records HEAD = %q; want unchanged %q — a clean tree adds no commit", got, before)
	}
}
