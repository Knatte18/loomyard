//go:build !windows

// strayfiles_test.go covers the stray-file line, the check that fails a tier result for new untracked files,
// and a tier run through the green-aware path that leaves one.
// The file shares its helpers with the integration test, so it is left out of the Windows build as that one is.

package gatecli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestStrayFilesLine(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name  string
		stray []string
		// wantNamed are the quoted names the line holds, joined as the line joins them.
		wantNamed string
		wantRest  string
	}{
		{name: "OneFile", stray: []string{"a.lock"}, wantNamed: `"a.lock"`},
		{name: "ANameWithACommaAndANewlineIsEscaped", stray: []string{"a,b", "c\nd"}, wantNamed: `"a,b", "c\nd"`},
		{
			name:      "FilesPastTheCapAreCounted",
			stray:     []string{"f00", "f01", "f02", "f03", "f04", "f05", "f06", "f07", "f08", "f09", "f10", "f11"},
			wantNamed: `"f00", "f01", "f02", "f03", "f04", "f05", "f06", "f07", "f08", "f09"`,
			wantRest:  " and 2 more",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			want := "gate test: tier1 left new untracked files in the worktree: " + tt.wantNamed + tt.wantRest + "; "

			got := strayFilesLine("tier1", tt.stray)

			if !strings.HasPrefix(got, want) || !strings.Contains(got, "; way forward: ") || strings.Contains(got, "\n") {
				t.Errorf("strayFilesLine() = %q; want one line opening %q with a way forward", got, want)
			}
		})
	}
}

func TestCheckStrayFiles(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		// noRepo checks a directory outside every git repository.
		noRepo   bool
		exitCode int
		// newFiles are written after the first listing.
		newFiles []string
		// wantExit is the checked result's exit code.
		wantExit int
		// wantLine is whether the check names the new files.
		wantLine bool
	}{
		{name: "ANewFileFailsAPassingResult", newFiles: []string{"s.lock"}, wantExit: 1, wantLine: true},
		{name: "ANewFileKeepsAFailingExitCode", exitCode: 3, newFiles: []string{"s.lock"}, wantExit: 3, wantLine: true},
		{name: "NoNewFileLeavesTheResultAlone", wantExit: 0},
		{name: "ADirectoryOutsideEveryRepositoryIsNotChecked", noRepo: true, newFiles: []string{"s.lock"}, wantExit: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			if !tt.noRepo {
				root = newStrayRepo(t)
			}
			before, _ := untrackedWorktreeFiles(root)
			for _, name := range tt.newFiles {
				if err := os.WriteFile(filepath.Join(root, name), nil, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			var stderr bytes.Buffer
			result := tierResult{record: gateslot.RunRecord{ExitCode: tt.exitCode, Outcome: "passed"}}

			got := checkStrayFiles(result, before, "tier1", strayRequest(root, &stderr))

			if got.record.ExitCode != tt.wantExit {
				t.Errorf("checkStrayFiles() exit = %d; want %d", got.record.ExitCode, tt.wantExit)
			}
			if !tt.wantLine {
				if got.strayLine != "" || stderr.Len() != 0 || len(got.causes) != 0 || len(got.log) != 0 {
					t.Errorf("checkStrayFiles() = line %q, stderr %q, causes %q, log %q; want none", got.strayLine, stderr.String(), got.causes, got.log)
				}
				return
			}
			want := "gate test: tier1 left new untracked files in the worktree: " + strconv.Quote(tt.newFiles[0]) + "; "
			if !strings.HasPrefix(got.strayLine, want) || stderr.String() != got.strayLine+"\n" || !slices.Equal(got.causes, []string{got.strayLine}) || string(got.log) != got.strayLine+"\n" {
				t.Errorf("checkStrayFiles() = line %q, stderr %q, causes %q, log %q; want the line opening %q in each", got.strayLine, stderr.String(), got.causes, got.log, want)
			}
			if tt.exitCode == 0 && got.record.Outcome != "failed" {
				t.Errorf("checkStrayFiles() outcome = %q; want %q", got.record.Outcome, "failed")
			}
		})
	}
}

func TestRunGreenAwareFailsATierThatLeavesAStrayFile(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	root := newStrayRepo(t)
	var out, stderr bytes.Buffer
	exe := &execution{
		ctx:       context.Background(),
		out:       &out,
		request:   strayRequest(root, &stderr),
		env:       []string{"PATH=" + os.Getenv("PATH")},
		runLogDir: t.TempDir(),
		callID:    "stray",
	}
	run := strayRun("touch s.lock", root)

	end := exe.runGreenAware(run)

	if end.code != 1 || end.halt || !strings.Contains(stderr.String(), `"s.lock"`) {
		t.Fatalf("runGreenAware() = %+v, stderr %q; want exit 1 and no halt, naming s.lock", end, stderr.String())
	}
	if len(exe.failed) != 1 || !slices.ContainsFunc(exe.failed[0].causes, func(cause string) bool { return strings.Contains(cause, `"s.lock"`) }) {
		t.Errorf("runGreenAware() failed = %+v; want one failed run holding the stray-file line", exe.failed)
	}
}
