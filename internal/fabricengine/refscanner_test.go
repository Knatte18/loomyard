// refscanner_test.go covers RefScanner.Matches against the three cases websterengine's
// audit_test.go exercises today for its own weftReferencePattern (a command containing the sibling
// worktree path, a command containing a -weft sibling name, and a lyx fabric/weft/warp invocation),
// plus a clean command that must not match, and the command-position rule: an invocation after a
// shell separator still matches, while the same words inside a quoted search pattern do not. The *lyxcwd.Location is built synthetically, the way
// websterengine/audit_test.go's fakeLayout helper does, so no git fixture is needed.

package fabricengine_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// fakeLayout returns a lyxcwd.Location that resolves fabricengine.RecordsWorktree() without spawning
// git, the same shape websterengine/audit_test.go's own fakeLayout builds.
func fakeLayout() *lyxcwd.Location {
	return &lyxcwd.Location{HubPath: "/hub", WorktreeName: filepath.Base("/hub/master-builder")}
}

// TestRefScanner_Matches covers the behavioural contract that must not regress when the regex moves
// packages: the sibling worktree path, a -weft sibling name, a lyx fabric/weft/warp invocation, and
// a clean command that must not match.
func TestRefScanner_Matches(t *testing.T) {
	layout := fakeLayout()
	scanner := fabricengine.NewRefScanner(layout)
	weftWorktree := fabricengine.RecordsWorktree(layout)

	tests := []struct {
		name string
		cmd  string
		want bool
	}{
		{"command containing the sibling worktree path", "git -C " + weftWorktree + " add -A", true},
		{"command containing a -weft sibling name", "cd /hub/master-builder-weft && git status", true},
		{"lyx fabric invocation", "lyx fabric sync", true},
		{"lyx weft invocation", "lyx weft sync", true},
		{"lyx warp invocation", "lyx warp checkout feature", true},
		{"lyx.exe weft invocation", "lyx.exe weft push", true},
		{"clean command does not match", "git commit -am wip", false},
		{"invocation after a separator", "cd /hub/master-builder && lyx fabric sync", true},
		{"invocation in a pipeline", "echo y | lyx fabric remove x", true},
		{"invocation in a command substitution", "out=$(lyx fabric status)", true},
		{"invocation behind a directory prefix", "/usr/local/bin/lyx fabric push", true},
		{"invocation behind an env assignment", "LYX_DEBUG=1 lyx fabric commit", true},
		{"invocation behind a command wrapper", "env lyx fabric sync", true},
		{"quoted search pattern is text", `grep -n "fabric remove\|lyx fabric.*remove" docs/overview.md`, false},
		{"single-quoted search pattern is text", `rg 'lyx warp checkout' docs`, false},
		{"echoed prose is text", `echo "run lyx fabric sync later"`, false},
		{"invocation after a quoted argument", `grep -q "x" f.txt && lyx fabric sync`, true},
		{"quoted weft path still matches", `cat "/hub/master-builder-weft/_lyx/plan.md"`, true},
		{"quoted sibling weft path matches", `ls "/hub/other-weft/_lyx"`, true},
		{"bare unquoted sibling name matches", "git -C other-weft status", true},
		{"weft-suffixed string literal is text", `echo "archive-happy-weft"`, false},
		{"heredoc body is content", "cat > x_test.go <<'EOF'\n\tconst branch = \"archive-happy-weft\"\n\t// lyx fabric sync\n\tp := main-weft\nEOF\ngo test ./x", false},
		{"command after a heredoc is still checked", "cat > x.txt <<EOF\nhello\nEOF\nlyx fabric sync", true},
		{"slug merely containing the suffix is not a weft path", "cd /hub/fabric-readd-weft-push/internal/gitrepo && grep -n x y.go", false},
		{"bare slug merely containing the suffix is text", "git -C fabric-readd-weft-push status", false},
		{"weft twin of a suffix-containing slug matches", "cd /hub/fabric-readd-weft-push-weft/_lyx && ls", true},
		{"weft name ending the command matches", "ls /hub/other-weft", true},
		{"escaped alternation in a quoted grep is text", `grep -n "weftBranch\s*[:=]\|WeftSuffix\|-weft" x_test.go`, false},
		{"windows sibling weft path matches", `dir C:\hub\other-weft\_lyx`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scanner.Matches(tt.cmd); got != tt.want {
				t.Errorf("Matches(%q) = %v; want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

// TestReferenceRule_Match covers what TestRefScanner_Matches cannot see.
// That is the matched text each rule returns, taken from the copy its pattern ran on, and the two rules being callable apart.
func TestReferenceRule_Match(t *testing.T) {
	t.Parallel()

	rule := fabricengine.NewReferenceRule()

	tests := []struct {
		name         string
		cmd          string
		wantPath     string
		wantSpelling string
	}{
		{"quoted separator path keeps its quotes in view", `cat "/hub/other-weft/_lyx/plan.md"`, "/other-weft", ""},
		{"bare word is read from the quote-blanked copy", "git -C other-weft status", "other-weft", ""},
		{"spelling is read from the quote-blanked copy", `grep -q "x" f.txt && lyx fabric sync`, "", "lyx fabric"},
		{"only the path rule matches", "ls /hub/other-weft", "/other-weft", ""},
		{"only the spelling rule matches", "echo y | lyx warp checkout x", "", "lyx warp"},
		{"heredoc body returns neither", "cat > x <<EOF\n/hub/a-weft\nlyx fabric sync\nEOF", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path, pathOK := rule.MatchPath(tt.cmd)
			if path != tt.wantPath || pathOK != (tt.wantPath != "") {
				t.Errorf("MatchPath(%q) = %q, %v; want %q", tt.cmd, path, pathOK, tt.wantPath)
			}
			spelling, spellingOK := rule.MatchSpelling(tt.cmd)
			if spelling != tt.wantSpelling || spellingOK != (tt.wantSpelling != "") {
				t.Errorf("MatchSpelling(%q) = %q, %v; want %q", tt.cmd, spelling, spellingOK, tt.wantSpelling)
			}
		})
	}
}

// TestIsReadOnlyCommand covers the allow-list of read forms (readers, lyx help, lyx fabric readers, git reads, sed prints) and the tokenised forms among them (quoted words, globs, tildes, the two stderr redirections),
// and every way a command stops being read-only: a redirection, a substitution, backgrounding, an expanding word and any other command.
func TestIsReadOnlyCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cmd  string
		want bool
	}{
		{"cat alone", "cat docs/overview.md", true},
		{"head alone", "head -n 5 x.txt", true},
		{"tail alone", "tail -n 5 x.txt", true},
		{"ls alone", "ls -la internal", true},
		{"wc alone", "wc -l x.txt", true},
		{"grep alone", "grep -rn needle internal", true},
		{"jq alone", "jq .name package.json", true},
		{"cut alone", "cut -d, -f1 x.csv", true},
		{"pipeline of readers", "cat x.txt | grep needle | wc -l", true},
		{"semicolon chain", "ls a; ls b", true},
		{"and chain", "ls a && cat b", true},
		{"separators inside quotes are text", `grep "a|b;c && d > e" x.txt`, true},
		{"redirection character inside quotes is text", `grep '>' x.txt`, true},
		{"substitution inside single quotes is text", `grep '$(x)' y.txt`, true},
		{"output redirection", "cat x > y", false},
		{"append redirection", "cat x >> y", false},
		{"stderr to /dev/null is dropped", "cat x 2>/dev/null", true},
		{"stderr to a file", "lyx fabric --help 2>x", false},
		{"stderr to /dev/null with a space", "lyx fabric --help 2> /dev/null", false},
		{"stdout to stderr", "lyx fabric --help 1>&2", false},
		{"stderr redirection opening its segment", "2>&1 cat x", false},
		{"stderr redirection glued to a quoted word", `cat 'x'2>&1`, false},
		{"stderr dup and /dev/null inside a compound", `sed -n 285,300p f.md | cut -c1-200; grep -n "x" -r contracts 2>/dev/null | head -5; lyx fabric --help 2>&1 | head -30`, true},
		{"lyx help piped after a stderr dup", "lyx fabric --help 2>&1 | head -30", true},
		{"loop over a lyx help", "for v in a b; do lyx fabric $v --help 2>&1 | head -5; done", false},
		{"sed prints a line range of stdin", "cat x | sed -n 1,5p", true},
		{"sed prints a line of a file", "sed -n 7p f.md", true},
		{"sed in place", "sed -i 1p f", false},
		{"sed flag after the script", "sed -n 1p f -i", false},
		{"sed file glob", "sed -n 1p *", false},
		{"sed write command", "sed -n 1w x f", false},
		{"sed without -n", "sed 1p f", false},
		{"git log with a quoted format", "git log --format='%h %s'", true},
		{"git log with a double-quoted format and a quoted pathspec glob", `git log --format="%h %s" -- '*.go'`, true},
		{"git log with an unquoted pathspec glob after --", "git log --oneline -- *.go", true},
		{"git log with a hash pathspec after --", "git log --oneline -- x#", true},
		{"git log with a hash before --", "git log x#--output=f", false},
		{"git -C with a home directory", "git -C ~/repo status", true},
		{"git -C with a glob directory", "git -C * status", false},
		{"git branch list with a quoted glob", "git branch --list 'feat/*'", true},
		{"git branch list with an unquoted glob", "git branch --list *", false},
		{"git log with a quoted revision exclusion", "git log '^main' feat", true},
		{"git log with an unquoted revision exclusion", "git log ^main", false},
		{"git log with a quoted writing flag", "git log '--output=x'", false},
		{"git log with a variable in double quotes", `git log "$X"`, false},
		{"git log with a variable in single quotes", `git log '$X'`, false},
		{"git log with a backslash in single quotes", `git log 'a\b'`, false},
		{"git log with a zsh glob qualifier", "git log -- *(.)", false},
		{"git log with a bash extglob", "git log -- @(a|b)", false},
		{"git ls-remote with a glob after --", "git ls-remote -- *", false},
		{"git remote show with a glob after --", "git remote show -n -- *", false},
		{"git cat-file with a glob", "git cat-file -p *", false},
		{"git config with a glob", "git config --get *", false},
		{"lyx fabric diff with a quoted glob", "lyx fabric diff '*'", false},
		{"bare command substitution", "cat $(ls)", false},
		{"command substitution inside double quotes", `cat "$(ls)"`, false},
		{"bare backtick", "cat `ls`", false},
		{"backtick inside double quotes", "cat \"`ls`\"", false},
		{"process substitution", "cat <(ls)", false},
		{"trailing background", "ls a &", false},
		{"sort", "sort x.txt", false},
		{"uniq", "cat x | uniq", false},
		{"tee", "cat x | tee y", false},
		{"xargs", "ls | xargs cat", false},
		{"find", "find . -name x", false},
		{"python", `python3 -c "print(1)"`, false},
		{"git status is a read", "git status", true},
		{"git push", "git push", false},
		{"go", "go test ./...", false},
		{"env", "env cat x", false},
		{"bash -c", "bash -c 'ls'", false},
		{"variable as the command", "$X a", false},
		{"or chain of readers", "ls a || cat b", true},
		{"or chain into a writer", "ls a || git push", false},
		{"reader pipe into lyx help", "cat x | lyx fabric add --help", true},
		{"lyx help", "lyx --help", true},
		{"lyx help with a nested command word", "lyx webster record-batch --help", true},
		{"lyx short help", "lyx fabric -h", true},
		{"lyx command word in capitals", "lyx Fabric --help", false},
		{"lyx help with a flag before the last word", "lyx fabric --apply --help", false},
		{"lyx bare json", "lyx --json", true},
		{"lyx command words then json", "lyx fabric add --json", true},
		{"lyx help then json", "lyx fabric --help --json", true},
		{"lyx short help then json", "lyx webster record-batch -h --json", true},
		{"lyx json help with a variable", "lyx fabric $v --help --json", false},
		{"loop over a lyx json help", "for v in a b; do lyx fabric $v --help --json; done", false},
		{"lyx json after a flag", "lyx fabric add x --apply --json", false},
		{"lyx json before a flag", "lyx fabric add --json --apply", false},
		{"lyx fabric list", "lyx fabric list", true},
		{"lyx fabric pairs", "lyx fabric pairs", true},
		{"lyx fabric status", "lyx fabric status", true},
		{"lyx fabric shortname", "lyx fabric shortname", true},
		{"lyx fabric prune", "lyx fabric prune", true},
		{"lyx fabric cleanup", "lyx fabric cleanup", true},
		{"lyx fabric diff with a revision", "lyx fabric diff 1a2b3c", true},
		{"lyx fabric diff alone", "lyx fabric diff", false},
		{"lyx fabric diff with a flag", "lyx fabric diff --stat", false},
		{"lyx fabric diff with two words", "lyx fabric diff a b", false},
		{"lyx fabric add", "lyx fabric add x", false},
		{"lyx fabric prune apply", "lyx fabric prune --apply", false},
		{"lyx fabric cleanup apply", "lyx fabric cleanup --apply", false},
		{"lyx fabric shortname with a word", "lyx fabric shortname x", false},
		{"lyx webster run", "lyx webster run", false},
		{"bare lyx", "lyx", false},
		{"cat of a fabric file", "cat _lyx/fabric/origin.json", true},
		{"git remote get-url", "git -C x remote get-url origin", true},
		{"git remote get-url push", "git remote get-url --push o", true},
		{"git remote get-url without a name", "git remote get-url", false},
		{"git remote add", "git remote add o url", false},
		{"git remote -v", "git remote -v", true},
		{"git remote show", "git remote show -n o", true},
		{"git ls-remote heads", "git ls-remote --heads", true},
		{"git ls-remote tags", "git ls-remote --tags o", true},
		{"git ls-remote unlisted flag", "git ls-remote --upload-pack=x o", false},
		{"git rev-parse abbrev-ref", "git rev-parse --abbrev-ref HEAD", true},
		{"git rev-parse unlisted flag", "git rev-parse --git-common-dir", false},
		{"git log count", "git log -n 5", true},
		{"git log count without digits", "git log -n x", false},
		{"git log digits flag and path", "git log -5 -- a.go", true},
		{"git log format", "git log --format=%H", true},
		{"git log with an unlisted flag", "git log --output=x", false},
		{"git log with a variable", "git log $X", false},
		{"git log with a glob star", "git log *", false},
		{"git log with a glob question mark", "git log a?", false},
		{"git log with a glob bracket", "git log [a]", false},
		{"git log with a home path", "git log ~/x", true},
		{"git log with a brace", "git log {a,b}", false},
		{"git log with a quoted positional", `git log "a"`, true},
		{"git show stat", "git show --stat HEAD", true},
		{"git diff cached", "git diff --cached -- a.go", true},
		{"git show with an output file", "git show --output=x", false},
		{"git diff with an output file", "git diff --output=x", false},
		{"git diff external", "git diff --ext-diff", false},
		{"git remote set-url", "git remote set-url o u", false},
		{"git branch rename", "git branch -m a b", false},
		{"git config get with a file", "git config --get --file f k", false},
		{"git status branch", "git status -b", true},
		{"git branch all", "git branch -a", true},
		{"git branch list pattern", "git branch --list pat", true},
		{"git branch create", "git branch x", false},
		{"git branch delete", "git branch -D x", false},
		{"git cat-file type", "git cat-file -t HEAD", true},
		{"git cat-file without a mode", "git cat-file HEAD", false},
		{"git config get-regexp", "git config --get-regexp remote", true},
		{"git config set", "git config user.name x", false},
		{"git config unset", "git config --unset a", false},
		{"git with a global option other than -C", "git --git-dir=x status", false},
		{"git -C with a flag as the directory", "git -C -c status", false},
		{"bare git", "git", false},
		{"git -C alone", "git -C x", false},
		{"git subcommand with no form", "git checkout x", false},
		{"reader after a non-reader", "ls a && git push", false},
		{"single-quoted non-reader before a reader name", "'tee' cat", false},
		{"double-quoted non-reader after a separator", `ls a && "rm" cat x`, false},
		{"quoted non-reader after a pipe", "cat a | 'tee' cat", false},
		{"reader name with a quoted suffix names another command", `cat"x" y`, false},
		{"backslash in the command word", `\cat x`, false},
		{"empty command", "  ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := fabricengine.IsReadOnlyCommand(tt.cmd); got != tt.want {
				t.Errorf("IsReadOnlyCommand(%q) = %v; want %v", tt.cmd, got, tt.want)
			}
		})
	}
}
