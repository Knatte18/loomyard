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

// fakeLayout returns a lyxcwd.Location that resolves fabricengine.WeftWorktree() without spawning
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
	weftWorktree := fabricengine.WeftWorktree(layout)

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

// TestIsReadOnlyCommand covers the closed reader list and every way a command stops being read-only: a redirection, a substitution, backgrounding and any other command.
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
		{"descriptor redirection", "cat x 2>/dev/null", false},
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
		{"git", "git status", false},
		{"go", "go test ./...", false},
		{"reader after a non-reader", "ls a && git push", false},
		{"single-quoted non-reader before a reader name", "'tee' cat", false},
		{"double-quoted non-reader after a separator", `ls a && "rm" cat x`, false},
		{"quoted non-reader after a pipe", "cat a | 'tee' cat", false},
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
