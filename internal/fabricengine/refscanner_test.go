//go:build tier0

// refscanner_test.go covers RefScanner.Matches against the three cases websterengine's
// audit_test.go exercises today for its own weftReferencePattern (a command containing the sibling
// worktree path, a command containing a -weft sibling name, and a lym fabric/weft/warp invocation),
// plus a clean command that must not match, and the command-position rule: an invocation after a
// shell separator still matches, while the same words inside a quoted search pattern do not. The *lymcwd.Location is built synthetically, the way
// websterengine/audit_test.go's fakeLayout helper does, so no git fixture is needed.

package fabricengine_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// fakeLayout returns a lymcwd.Location that resolves fabricengine.RecordsWorktree() without spawning
// git, the same shape websterengine/audit_test.go's own fakeLayout builds.
func fakeLayout() *lymcwd.Location {
	return &lymcwd.Location{HubPath: "/hub", WorktreeName: filepath.Base("/hub/master-builder")}
}

// TestRefScanner_Matches covers the behavioural contract that must not regress when the regex moves
// packages: the sibling worktree path, a -weft sibling name, a lym fabric/weft/warp invocation, and
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
		{"lym fabric invocation", "lym fabric sync", true},
		{"lym weft invocation", "lym weft sync", true},
		{"lym warp invocation", "lym warp checkout feature", true},
		{"lym.exe weft invocation", "lym.exe weft push", true},
		{"clean command does not match", "git commit -am wip", false},
		{"invocation after a separator", "cd /hub/master-builder && lym fabric sync", true},
		{"invocation in a pipeline", "echo y | lym fabric remove x", true},
		{"invocation in a command substitution", "out=$(lym fabric status)", true},
		{"invocation behind a directory prefix", "/usr/local/bin/lym fabric push", true},
		{"invocation behind an env assignment", "LYM_DEBUG=1 lym fabric commit", true},
		{"invocation behind a command wrapper", "env lym fabric sync", true},
		{"quoted search pattern is text", `grep -n "fabric remove\|lym fabric.*remove" docs/overview.md`, false},
		{"single-quoted search pattern is text", `rg 'lym warp checkout' docs`, false},
		{"echoed prose is text", `echo "run lym fabric sync later"`, false},
		{"invocation after a quoted argument", `grep -q "x" f.txt && lym fabric sync`, true},
		{"quoted weft path still matches", `cat "/hub/master-builder-weft/_lym/plan.md"`, true},
		{"quoted sibling weft path matches", `ls "/hub/other-weft/_lym"`, true},
		{"bare unquoted sibling name matches", "git -C other-weft status", true},
		{"weft-suffixed string literal is text", `echo "archive-happy-weft"`, false},
		{"heredoc body is content", "cat > x_test.go <<'EOF'\n\tconst branch = \"archive-happy-weft\"\n\t// lym fabric sync\n\tp := main-weft\nEOF\ngo test ./x", false},
		{"command after a heredoc is still checked", "cat > x.txt <<EOF\nhello\nEOF\nlym fabric sync", true},
		{"slug merely containing the suffix is not a weft path", "cd /hub/fabric-readd-weft-push/internal/gitrepo && grep -n x y.go", false},
		{"bare slug merely containing the suffix is text", "git -C fabric-readd-weft-push status", false},
		{"weft twin of a suffix-containing slug matches", "cd /hub/fabric-readd-weft-push-weft/_lym && ls", true},
		{"weft name ending the command matches", "ls /hub/other-weft", true},
		{"escaped alternation in a quoted grep is text", `grep -n "weftBranch\s*[:=]\|WeftSuffix\|-weft" x_test.go`, false},
		{"windows sibling weft path matches", `dir C:\hub\other-weft\_lym`, true},
		{"gate card ID ending in the suffix is text", "lym gate test --wait --gate card --card 6-clone-records-marker-on-prime-weft 2>&1 | tail -40", false},
		{"gate card ID after a commit with a quoted message is text", "git add -A internal && git commit -q -m \"7: anchor\n\nbody\" && lym gate test --wait --gate batch --card 6-prime-weft --card 7-anchor 2>&1 | grep -E \"^(--- FAIL|FAIL)\"", false},
		{"gate test with a separator-prefixed weft path matches", "lym gate test ../other-weft/internal/x", true},
		{"bare weft name after a gate segment matches", "lym gate test --card 6-prime-weft && git -C other-weft status", true},
		{"gate words outside command position are not blanked", "echo lym gate other-weft", true},
		{"bare weft name after a semicolon ends the gate segment", "lym gate test --card a-weft; ls other-weft", true},
		{"bare weft name after a pipe ends the gate segment", "lym gate test --card a-weft | grep other-weft", true},
		{"bare weft name after a double pipe ends the gate segment", "lym gate test --card a-weft || ls other-weft", true},
		{"bare weft name after a newline ends the gate segment", "lym gate test --card a-weft\nls other-weft", true},
		{"gate card ID after a stderr redirect is text", "lym gate test 2>&1 --card 6-prime-weft", false},
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
		{"quoted separator path keeps its quotes in view", `cat "/hub/other-weft/_lym/plan.md"`, "/other-weft", ""},
		{"bare word is read from the quote-blanked copy", "git -C other-weft status", "other-weft", ""},
		{"spelling is read from the quote-blanked copy", `grep -q "x" f.txt && lym fabric sync`, "", "lym fabric"},
		{"only the path rule matches", "ls /hub/other-weft", "/other-weft", ""},
		{"only the spelling rule matches", "echo y | lym warp checkout x", "", "lym warp"},
		{"heredoc body returns neither", "cat > x <<EOF\n/hub/a-weft\nlym fabric sync\nEOF", "", ""},
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

// TestIsReadOnlyCommand covers the allow-list of read forms (readers, lym help, lym fabric readers, git reads, sed prints) and the tokenised forms among them (quoted words, globs, tildes, the two stderr redirections),
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
		{"stderr to a file", "lym fabric --help 2>x", false},
		{"stderr to /dev/null with a space", "lym fabric --help 2> /dev/null", false},
		{"stdout to stderr", "lym fabric --help 1>&2", false},
		{"stderr redirection opening its segment", "2>&1 cat x", false},
		{"stderr redirection glued to a quoted word", `cat 'x'2>&1`, false},
		{"stderr dup and /dev/null inside a compound", `sed -n 285,300p f.md | cut -c1-200; grep -n "x" -r contracts 2>/dev/null | head -5; lym fabric --help 2>&1 | head -30`, true},
		{"lym help piped after a stderr dup", "lym fabric --help 2>&1 | head -30", true},
		{"loop over a lym help", "for v in a b; do lym fabric $v --help 2>&1 | head -5; done", false},
		{"sed prints a line range of stdin", "cat x | sed -n 1,5p", true},
		{"sed prints a line of a file", "sed -n 7p f.md", true},
		{"sed prints a regex range", `sed -n '/^func X/,/^}/p' f`, true},
		{"sed long quiet flag", "sed --quiet 3p f", true},
		{"sed silent flag", "sed --silent 3p f", true},
		{"sed joined print ranges", `sed -n '1,5p;20,30p' f`, true},
		{"sed prints a regex address", `sed -n '/a/p' f`, true},
		{"sed write after an address", `sed -n '1w out' f`, false},
		{"sed substitute with write flag", `sed -n 's/a/b/w f' f`, false},
		{"sed last line address", `sed -n '$p' f`, false},
		{"sed semicolon inside a regex", `sed -n '/a;b/p' f`, false},
		{"sed trailing semicolon", `sed -n '1p;' f`, false},
		{"sed empty regex", `sed -n '//p' f`, false},
		{"sed regex flag", `sed -n '/a/Ip' f`, false},
		{"sed in place after the script", "sed -n 3p -i f", false},
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
		{"lym fabric diff with a quoted glob", "lym fabric diff '*'", false},
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
		{"reader pipe into lym help", "cat x | lym fabric add --help", true},
		{"lym help", "lym --help", true},
		{"lym help with a nested command word", "lym webster record-batch --help", true},
		{"lym short help", "lym fabric -h", true},
		{"lym command word in capitals", "lym Fabric --help", false},
		{"lym help with a flag before the last word", "lym fabric --apply --help", false},
		{"lym bare json", "lym --json", true},
		{"lym command words then json", "lym fabric add --json", true},
		{"lym help then json", "lym fabric --help --json", true},
		{"lym short help then json", "lym webster record-batch -h --json", true},
		{"lym json help with a variable", "lym fabric $v --help --json", false},
		{"loop over a lym json help", "for v in a b; do lym fabric $v --help --json; done", false},
		{"lym json after a flag", "lym fabric add x --apply --json", false},
		{"lym json before a flag", "lym fabric add --json --apply", false},
		{"lym fabric list", "lym fabric list", true},
		{"lym fabric pairs", "lym fabric pairs", true},
		{"lym fabric status", "lym fabric status", true},
		{"lym fabric shortname", "lym fabric shortname", true},
		{"lym fabric prune", "lym fabric prune", true},
		{"lym fabric cleanup", "lym fabric cleanup", true},
		{"lym fabric diff with a revision", "lym fabric diff 1a2b3c", true},
		{"lym fabric diff alone", "lym fabric diff", false},
		{"lym fabric diff with a flag", "lym fabric diff --stat", false},
		{"lym fabric diff with two words", "lym fabric diff a b", false},
		{"lym fabric add", "lym fabric add x", false},
		{"lym fabric prune apply", "lym fabric prune --apply", false},
		{"lym fabric cleanup apply", "lym fabric cleanup --apply", false},
		{"lym fabric shortname with a word", "lym fabric shortname x", false},
		{"lym webster run", "lym webster run", false},
		{"bare lym", "lym", false},
		{"cat of a fabric file", "cat _lym/fabric/origin.json", true},
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
		{"git config get with a file", "git config --get --file=f k", false},
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
		{"reader with a zsh glob qualifier running code", "cat *(e:'touch x':)", false},
		{"reader with a zsh glob qualifier function", "ls *(+fn)", false},
		{"reader with an unquoted parenthesis", "grep x (a)", false},
		{"reader with a parameter expansion modifier", "cat ${x@P}", false},
		{"reader with an indexed expansion", "head -n ${a[y]} f", false},
		{"reader with a variable", "cat $x", false},
		{"reader with a tilde expansion", "cat $~x", false},
		{"reader with a positional parameter", "cat $1", false},
		{"reader with the argument list", "cat $@", false},
		{"reader with an expansion inside double quotes", `cat "${x@P}"`, false},
		{"reader with an ANSI-C quote hiding a substitution", `cat $'\'' $(touch x) f`, false},
		{"git diff with an unquoted glob before the pathspec marker", "git diff *.go", false},
		{"reader with a bare glob", "cat *", true},
		{"reader with a glob in a flag value", "grep -rn x --include=*.go internal", true},
		{"jq filter with parentheses in single quotes", "jq '.a | map(.b)' f.json", true},
		{"grep with an unquoted end anchor", "grep a$ f", true},
		{"grep with a single-quoted end anchor", "grep 'a$' f", true},
		{"grep with a double-quoted end anchor", `grep "^a$" f`, true},
		{"git diff with a glob after the pathspec marker", "git diff -- *.go", true},
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
