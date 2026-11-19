// strayfiles.go fails a tier run that left new untracked, non-ignored files in the worktree it ran in.

package gatecli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/logger"
)

// strayFilesListed caps how many new untracked files the stray-file line names.
const strayFilesListed = 10

// untrackedWorktreeFiles returns the root-relative paths of the untracked, non-ignored files under root.
// It reports false when root is in no git worktree or git cannot list the files.
func untrackedWorktreeFiles(root string) (map[string]bool, bool) {
	out, err := gitexec.Run([]string{"ls-files", "--others", "--exclude-standard", "-z"}, root)
	if err != nil {
		logger.Info("gate test: cannot list the untracked files, so the run is not checked for files it leaves", "root", root, "cause", err)
		return nil, false
	}
	files := map[string]bool{}
	for _, path := range strings.Split(out, "\x00") {
		if path != "" {
			files[path] = true
		}
	}
	return files, true
}

// untrackedBeforeRun returns the listing a tier run is checked against, taken before its first attempt.
// A run with no invocation runs no command, so it takes no listing and reports false.
func untrackedBeforeRun(run tierRun, root string) (map[string]bool, bool) {
	if len(run.invocations) == 0 {
		return nil, false
	}
	return untrackedWorktreeFiles(root)
}

// checkStrayFiles fails result when the tier run left untracked, non-ignored files under the request's root.
// A stray file is one the listing taken before the run, before, did not hold.
// It writes the line naming them to stderr and the log, adds it to the causes, and keeps it in strayLine.
// A passing run's exit code turns to 1; a failing run keeps its own code.
// A listing that cannot be taken after the run leaves result as it is.
func checkStrayFiles(result tierResult, before map[string]bool, tier string, request testRequest) tierResult {
	after, listed := untrackedWorktreeFiles(request.root)
	if !listed {
		return result
	}
	var stray []string
	for path := range after {
		if !before[path] {
			stray = append(stray, path)
		}
	}
	if len(stray) == 0 {
		return result
	}
	sort.Strings(stray)
	line := strayFilesLine(tier, stray)
	result.log = append(result.log, line+"\n"...)
	printLine(request.stderr, line)
	result.causes = append(result.causes, line)
	result.strayLine = line
	if result.record.ExitCode == 0 {
		result.record.ExitCode = 1
		result.record.Outcome = "failed"
	}
	return result
}

// strayFilesLine returns the line naming the new untracked files a tier run left, sorted, and its way forward.
// It names at most strayFilesListed files and counts the rest.
// Each name is a quoted string, so a name with a newline or comma stays one item.
func strayFilesLine(tier string, stray []string) string {
	named := stray
	rest := ""
	if len(stray) > strayFilesListed {
		named = stray[:strayFilesListed]
		rest = fmt.Sprintf(" and %d more", len(stray)-strayFilesListed)
	}
	quoted := make([]string, len(named))
	for i, path := range named {
		quoted[i] = strconv.Quote(path)
	}
	return fmt.Sprintf("gate test: %s left new untracked files in the worktree: %s%s; a test wrote outside its own temp directory, often through a path built from an empty or relative directory; way forward: make the test write under its temp directory, remove the files named here, then re-run the same command", tier, strings.Join(quoted, ", "), rest)
}
