// bashlookup_test.go pins which shell commands count as a read-only lookup and which print a line range of one file.

package bashlookup_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine/bashlookup"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestIsReadOnlyLookup pins which commands, chains and shell constructs make a read-only lookup and which do not.
func TestIsReadOnlyLookup(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name    string
		command string
		want    bool
	}{
		{"Grep", "grep -n foo internal/x.go", true},
		{"Ripgrep", "rg -l foo", true},
		{"Find", "find . -name '*.go'", true},
		{"Ls", "ls -la internal", true},
		{"Cat", "cat a.go b.go", true},
		{"CatWithBraceExpansion", "cat a.{go,md}", true},
		{"Head", "head -50 a.go", true},
		{"Tail", "tail -n 20 a.go", true},
		{"SedPrintingOnly", "sed -n '10,20p' a.go", true},
		{"SedQuietLongOption", "sed --quiet 5p a.go", true},
		{"Wc", "wc -l a.go", true},
		{"GitLog", "git log --oneline -5", true},
		{"GitShowInAnotherDirectory", "git -C /repo show HEAD", true},
		{"GitDiffWithoutPager", "git --no-pager diff main", true},
		{"GitStatus", "git status --short", true},
		{"GitBlame", "git blame a.go", true},
		{"GitGrep", "git grep foo", true},
		{"GitLsFiles", "git ls-files", true},
		{"GitLsTree", "git ls-tree HEAD", true},
		{"GitRevParse", "git rev-parse HEAD", true},
		{"GitCatFile", "git cat-file -p HEAD:a.go", true},
		{"OrChainOfLookups", "ls x || cat y", true},
		{"PipeWithStderrOfLookups", "ls x |& head", true},
		{"OutputDuplicatedToStderr", "grep foo x >&2", true},
		{"PipeOfLookups", "grep -rn foo . | head -20", true},
		{"QuotedPipeIsPartOfTheWord", "grep -n 'a|b' x.go", true},
		{"DoubleQuotedAlternationAndSemicolonChain", `grep -n "a\|b" x.go; wc -l x.go`, true},
		{"AndChainOfLookups", "ls x && cat y", true},
		{"NewlineChainOfLookups", "ls x\ncat y", true},
		{"StderrToDevNull", "grep foo x 2>/dev/null | head", true},
		{"StderrToStdout", "grep foo x 2>&1 | tail -3", true},
		{"AllOutputToDevNull", "ls x &>/dev/null", true},
		{"InputRedirect", "wc -l < a.go", true},
		{"EmptyCommand", "", false},
		{"OtherCommand", "go vet ./internal/x", false},
		{"LymVerb", "lym board list", false},
		{"ChangeDirectoryFirst", "cd x; ls", false},
		{"RedirectIntoAFile", "grep a x > out.txt", false},
		{"AppendIntoAFile", "grep a x >> out.txt", false},
		{"SedInPlace", "sed -i 's/a/b/' x", false},
		{"SedInPlaceBundledWithQuiet", "sed -ni 's/a/b/p' x", false},
		{"SedWithoutQuiet", "sed 's/a/b/' x", false},
		{"FindDelete", "find . -name x -delete", false},
		{"FindExec", `find . -exec rm {} \;`, false},
		{"FindExecdir", `find . -execdir rm {} \;`, false},
		{"FindOk", `find . -ok rm {} \;`, false},
		{"FindOkdir", `find . -okdir rm {} \;`, false},
		{"FindFprint", "find . -fprint out.txt", false},
		{"FindFprint0", "find . -fprint0 out.txt", false},
		{"FindFprintf", "find . -fprintf out.txt %p", false},
		{"FindFls", "find . -fls out.txt", false},
		{"SedInPlaceLongOption", "sed --quiet --in-place=.bak 5p x", false},
		{"AllOutputIntoAFile", "grep a x &> out.txt", false},
		{"GitDiffOutputOption", "git diff --output=x", false},
		{"GitLogOutputOptionWithSeparateFile", "git log --output x", false},
		{"GitGrepOpenFilesInPager", "git grep -O less foo", false},
		{"GitLogExternalDiff", "git log --ext-diff", false},
		{"GitDiffAbbreviatedOutputOption", "git diff --out=x", false},
		{"GitGrepAbbreviatedPagerOption", "git grep --open-files less foo", false},
		{"GitGrepPagerOptionBundledWithAnotherFlag", "git grep -nOless foo", false},
		{"GitLogAbbreviatedExternalDiff", "git log --ext-d", false},
		{"GitDirectoryOptionWithoutDirectory", "git -C", false},
		{"GitWrite", "git commit -m x", false},
		{"GitConfigOption", "git -c core.pager=x log", false},
		{"GitAlone", "git", false},
		{"CommandSubstitution", "cat $(ls)", false},
		{"BacktickSubstitution", "cat `ls`", false},
		{"SubstitutionInDoubleQuotes", `grep "$(rm x)" y`, false},
		{"ProcessSubstitution", "cat <(ls a)", false},
		{"Heredoc", "cat <<EOF\nx\nEOF", false},
		{"Background", "grep a x &", false},
		{"Subshell", "(cd x; ls)", false},
		{"PipeIntoAnotherCommand", "grep -l a x | xargs rm", false},
		{"UnterminatedSingleQuote", "grep 'a x", false},
		{"UnterminatedDoubleQuote", `grep "a x`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := bashlookup.IsReadOnlyLookup(tt.command); got != tt.want {
				t.Errorf("IsReadOnlyLookup(%q) = %t; want %t", tt.command, got, tt.want)
			}
		})
	}
}

// TestLineRangeFile pins which commands print a line range of one file, and the file each names as written.
func TestLineRangeFile(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name    string
		command string
		want    string
		wantOK  bool
	}{
		{"SedRange", "sed -n '10,20p' a.go", "a.go", true},
		{"SedOneLine", "sed -n 5p a.go", "a.go", true},
		{"SedRangeToTheEnd", "sed -n '10,$p' a.go", "a.go", true},
		{"SedQuietLongOption", "sed --quiet 1,2p dir/a.go", "dir/a.go", true},
		{"HeadDashCount", "head -50 a.go", "a.go", true},
		{"HeadCountOption", "head -n 50 a.go", "a.go", true},
		{"HeadBundledCount", "head -n50 a.go", "a.go", true},
		{"HeadLongCount", "head --lines=50 a.go", "a.go", true},
		{"HeadLongCountSeparate", "head --lines 50 a.go", "a.go", true},
		{"TailFromALine", "tail -n +20 a.go", "a.go", true},
		{"SedWithoutQuiet", "sed 1,2p a.go", "", false},
		{"SedSubstitution", "sed -n 's/a/b/p' a.go", "", false},
		{"SedTwoFiles", "sed -n 1,2p a.go b.go", "", false},
		{"HeadWithoutCount", "head a.go", "", false},
		{"HeadByteCount", "head -c 50 a.go", "", false},
		{"TailFollow", "tail -f -n 20 a.go", "", false},
		{"HeadOfTwoFiles", "head -n 5 a.go b.go", "", false},
		{"HeadOfStandardInput", "head -n 5", "", false},
		{"PipeIntoHead", "grep x a.go | head -n 5", "", false},
		{"ChainOfTwoRanges", "sed -n 1,2p a.go; sed -n 3,4p a.go", "", false},
		{"Cat", "cat a.go", "", false},
		{"EmptyCommand", "", "", false},
		{"UnterminatedQuote", "sed -n '1,2p a.go", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := bashlookup.LineRangeFile(tt.command)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("LineRangeFile(%q) = %q, %t; want %q, %t", tt.command, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
