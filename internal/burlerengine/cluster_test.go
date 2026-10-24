//go:build tier0

// cluster_test.go table-drives auditClusterRound over the full violation taxonomy (fork-count
// mismatch, Agent-call-in-fork, write-in-fork, git-mutation-in-fork, named spawns) plus the
// warning-only ReportReturned==false case, and separately matrixes commandRunsMutatingGit against a set
// of mutating and non-mutating Bash command strings.

package burlerengine

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// cleanFork returns a ForkReport that violates none of auditClusterRound's rules —
// tests mutate a copy of this base to trigger exactly one violation at a time.
func cleanFork(path string) shuttleengine.ForkReport {
	return shuttleengine.ForkReport{
		TranscriptPath: path,
		ReportReturned: true,
	}
}

func TestAuditClusterRound(t *testing.T) {
	tests := []struct {
		name         string
		audit        *shuttleengine.ForkAudit
		wantN        int
		wantErr      bool
		errSubstr    string
		wantErrIs    error
		wantWarnings []string
		// fix drives auditFixRound instead of auditClusterRound, and wantForks is the fork count it must report.
		fix       bool
		wantForks int
	}{
		{
			name:      "fix: exact count is clean",
			audit:     &shuttleengine.ForkAudit{Forks: []shuttleengine.ForkReport{cleanFork("a"), cleanFork("b")}},
			wantN:     2,
			fix:       true,
			wantForks: 2,
		},
		{
			name:         "fix: a nil audit is a count warning",
			audit:        nil,
			wantN:        2,
			wantWarnings: []string{"twill fixer spawned 0 fork(s) for 2 lens(es)"},
			fix:          true,
		},
		{
			name:         "fix: no forks is a count warning",
			audit:        &shuttleengine.ForkAudit{},
			wantN:        2,
			wantWarnings: []string{"twill fixer spawned 0 fork(s) for 2 lens(es)"},
			fix:          true,
		},
		{
			name:         "fix: fewer forks than lenses is a count warning",
			audit:        &shuttleengine.ForkAudit{Forks: []shuttleengine.ForkReport{cleanFork("a")}},
			wantN:        3,
			wantWarnings: []string{"twill fixer spawned 1 fork(s) for 3 lens(es)"},
			fix:          true,
			wantForks:    1,
		},
		{
			name:         "fix: more forks than lenses is a count warning",
			audit:        &shuttleengine.ForkAudit{Forks: []shuttleengine.ForkReport{cleanFork("a"), cleanFork("b"), cleanFork("c")}},
			wantN:        2,
			wantWarnings: []string{"twill fixer spawned 3 fork(s) for 2 lens(es)"},
			fix:          true,
			wantForks:    3,
		},
		{
			name: "fix: a report not returned is a warning",
			audit: &shuttleengine.ForkAudit{
				Forks: []shuttleengine.ForkReport{cleanFork("a"), {TranscriptPath: "b", ReportReturned: false}},
			},
			wantN:        2,
			wantWarnings: []string{`fork "b" never returned a final report`},
			fix:          true,
			wantForks:    2,
		},
		{
			name: "fix: a write call in a fork is the twill fixer violation",
			audit: &shuttleengine.ForkAudit{
				Forks: []shuttleengine.ForkReport{cleanFork("a"), {TranscriptPath: "b", ReportReturned: true, WriteCalls: 2}},
			},
			wantN:     2,
			wantErr:   true,
			errSubstr: `twill fixer fork "b" attempted 2 write/edit tool call(s)`,
			fix:       true,
		},
		{
			name: "exact N passes clean",
			audit: &shuttleengine.ForkAudit{
				Forks: []shuttleengine.ForkReport{cleanFork("a"), cleanFork("b")},
			},
			wantN:   2,
			wantErr: false,
		},
		{
			name:      "nil audit",
			audit:     nil,
			wantN:     3,
			wantErr:   true,
			errSubstr: "requested 3, spawned 0",
			wantErrIs: ErrClusterForksMissing,
		},
		{
			name:      "zero forks against a positive want",
			audit:     &shuttleengine.ForkAudit{Forks: nil},
			wantN:     2,
			wantErr:   true,
			errSubstr: "requested 2, spawned 0",
			wantErrIs: ErrClusterForksMissing,
		},
		{
			name: "shortfall",
			audit: &shuttleengine.ForkAudit{
				Forks: []shuttleengine.ForkReport{cleanFork("a")},
			},
			wantN:     3,
			wantErr:   true,
			errSubstr: "requested 3, spawned 1",
			wantErrIs: ErrClusterForksMissing,
		},
		{
			name: "overshoot",
			audit: &shuttleengine.ForkAudit{
				Forks: []shuttleengine.ForkReport{cleanFork("a"), cleanFork("b"), cleanFork("c")},
			},
			wantN:     2,
			wantErr:   true,
			errSubstr: "requested 2, spawned 3",
			wantErrIs: ErrClusterForksMissing,
		},
		{
			name: "agent call in fork is a hard error even when denied",
			audit: &shuttleengine.ForkAudit{
				Forks: []shuttleengine.ForkReport{
					cleanFork("a"),
					{TranscriptPath: "b", ReportReturned: true, AgentCalls: 1},
				},
			},
			wantN:     2,
			wantErr:   true,
			errSubstr: "Agent tool call",
		},
		{
			name: "write call in fork",
			audit: &shuttleengine.ForkAudit{
				Forks: []shuttleengine.ForkReport{
					cleanFork("a"),
					{TranscriptPath: "b", ReportReturned: true, WriteCalls: 2},
				},
			},
			wantN:     2,
			wantErr:   true,
			errSubstr: "write/edit tool call",
		},
		{
			name: "git-mutating bash command in fork",
			audit: &shuttleengine.ForkAudit{
				Forks: []shuttleengine.ForkReport{
					cleanFork("a"),
					{TranscriptPath: "b", ReportReturned: true, BashCommands: []string{"git status", "git commit -am wip"}},
				},
			},
			wantN:     2,
			wantErr:   true,
			errSubstr: "git-mutating command",
		},
		{
			name: "named spawns",
			audit: &shuttleengine.ForkAudit{
				Forks:       []shuttleengine.ForkReport{cleanFork("a"), cleanFork("b")},
				NamedSpawns: 1,
			},
			wantN:     2,
			wantErr:   true,
			errSubstr: "spawned with a name",
		},
		{
			name: "report not returned is a warning, not a failure",
			audit: &shuttleengine.ForkAudit{
				Forks: []shuttleengine.ForkReport{
					cleanFork("a"),
					{TranscriptPath: "b", ReportReturned: false},
				},
			},
			wantN:        2,
			wantErr:      false,
			wantWarnings: []string{`fork "b" never returned a final report`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var warnings []string
			var err error
			if tt.fix {
				var forks int
				warnings, forks, err = auditFixRound(tt.audit, tt.wantN)
				if !tt.wantErr && forks != tt.wantForks {
					t.Errorf("auditFixRound() forks = %d; want %d", forks, tt.wantForks)
				}
				if wayForward := "; way forward: inspect the target's working-tree changes, revert what the fork wrote, and resume the run"; tt.wantErr && (err == nil || !strings.Contains(err.Error(), wayForward)) {
					t.Errorf("auditFixRound() error = %v; want it to contain %q", err, wayForward)
				}
			} else {
				warnings, err = auditClusterRound(tt.audit, tt.wantN)
			}

			if tt.wantErr {
				if err == nil {
					t.Fatalf("auditClusterRound() = nil error; want error containing %q", tt.errSubstr)
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("auditClusterRound() error = %q; want substring %q", err.Error(), tt.errSubstr)
				}
				if !strings.HasPrefix(err.Error(), "burler: ") {
					t.Errorf("auditClusterRound() error = %q; want burler: -prefixed message", err.Error())
				}
				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Errorf("errors.Is(auditClusterRound() error, %v) = false; want true", tt.wantErrIs)
				}
				return
			}

			if err != nil {
				t.Fatalf("auditClusterRound() = %v; want nil error", err)
			}
			if len(warnings) != len(tt.wantWarnings) {
				t.Fatalf("auditClusterRound() warnings = %v; want %v", warnings, tt.wantWarnings)
			}
			for i := range warnings {
				if warnings[i] != tt.wantWarnings[i] {
					t.Errorf("auditClusterRound() warnings[%d] = %q; want %q", i, warnings[i], tt.wantWarnings[i])
				}
			}
		})
	}
}

// TestCommandRunsMutatingGit matrixes commandRunsMutatingGit against every Bash command shape the audit must classify correctly.
// Every listed mutating subcommand matches, tolerant of a leading path, a chain prefix, a later line and global options.
// Read-only subcommands whose arguments name a mutating verb, help requests and git tag's list forms never match.
func TestCommandRunsMutatingGit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		cmd  string
		want bool
	}{
		{"add", "git add .", true},
		{"commit", "git commit -am wip", true},
		{"push", "git push origin main", true},
		{"pull", "git pull --rebase", true},
		{"fetch", "git fetch origin", true},
		{"merge", "git merge feature", true},
		{"rebase", "git rebase main", true},
		{"reset", "git reset --hard HEAD~1", true},
		{"restore", "git restore file.go", true},
		{"rm", "git rm file.go", true},
		{"mv", "git mv a b", true},
		{"checkout", "git checkout main", true},
		{"switch", "git switch main", true},
		{"stash", "git stash pop", true},
		{"apply", "git apply patch.diff", true},
		{"cherry-pick", "git cherry-pick abc123", true},
		{"tag", "git tag v1.0", true},
		{"tag delete", "git tag -d v1", true},
		{"annotated tag", "git tag -a v1 -m msg", true},
		{"signed tag", "git tag -s v1", true},
		{"forced tag", "git tag -f v1 HEAD", true},
		{"tag message in the = form", "git tag --message=msg v1", true},
		{"tag delete beside a list option", "git tag -l -d v1", true},
		{"tag -a beside a list option", "git tag -l -a v1", true},
		{"tag -s beside a list option", "git tag -l -s v1", true},
		{"tag -f beside a list option", "git tag -l -f v1", true},
		{"tag --message in the = form beside a list option", "git tag -l --message=msg v1", true},
		{"tag --delete beside a list option", "git tag -l --delete v1", true},
		{"tag --annotate beside a list option", "git tag -l --annotate v1", true},
		{"tag --sign beside a list option", "git tag -l --sign v1", true},
		{"tag --force beside a list option", "git tag -l --force v1", true},
		{"tag -u beside a list option", "git tag -l -u key v1", true},
		{"tag --local-user beside a list option", "git tag -l --local-user key v1", true},
		{"tag -F beside a list option", "git tag -l -F msg.txt v1", true},
		{"tag --file beside a list option", "git tag -l --file msg.txt v1", true},
		{"tag -e beside a list option", "git tag -l -e v1", true},
		{"tag --edit beside a list option", "git tag -l --edit v1", true},
		{"tag --list after a double dash is an operand", "git tag v1 -- --list", true},
		{"tag --list as a message value", "git tag -m --list v1", true},
		{"tag short-option cluster", "git tag -ln v1", true},
		{"branch", "git branch -d old", true},
		{"leading path", "/usr/bin/git commit -m x", true},
		{"chained with &&", "cd internal/burler && git commit -am fix", true},
		{"chained with ;", "cd internal/burler; git push", true},
		{"flag before subcommand", "git -C /worktree commit -m x", true},
		{"config option before subcommand", "git -c user.name=x commit -m x", true},
		{"mutating command on a later line", "git status\ngit commit -m x", true},
		{"-h as a commit message is not a help request", "git commit -m -h", true},
		{"--help as a tag message is not a help request", "git tag -m --help v1", true},
		{"-h after a double dash is not a help request", "git checkout -- -h", true},
		{"quoted -c value with a space", `git -c "user.name=A B" commit`, true},
		{"quoted -C value with a space", `git -C "/path with space" commit`, true},
		{"single-quoted -C value with a space", "git -C '/path with space' commit", true},
		{"backslash line continuation before the subcommand", "git \\\ncommit -m x", true},
		{"escaped quote inside a double-quoted value", `git -c "a\"b" commit`, true},
		{"escaped apostrophe before a later mutating call", `echo it\'s; git commit -m x`, true},
		{"apostrophe in a comment line opens no quote", "# don't do this\ngit commit -m x", true},
		{"backslash-newline inside a word joins it", "git com\\\nmit -m x", true},
		{"--attr-source value is skipped", "git --attr-source HEAD commit", true},
		{"--git-dir in the = form", "git --git-dir=x commit -m x", true},
		{"--work-tree as a separate word", "git --work-tree x commit -m x", true},
		{"--git-dir as a separate word", "git --git-dir x commit -m x", true},
		{"--namespace value is skipped", "git --namespace ns commit -m x", true},
		{"--config-env value is skipped", "git --config-env name=VAR commit -m x", true},
		{"git help on a mutating verb is not mutating", `git help -m cherry-pick 2>/dev/null | col -b | grep -n -A14 "^ *--empty" | head -30`, false},
		{"git --help on a mutating verb is not mutating", "git --help commit", false},
		{"mutating verb with --help is not mutating", "git commit --help", false},
		{"mutating verb with -h is not mutating", "git rebase -h", false},
		{"mutating verb as a log argument is not mutating", "git log --grep merge", false},
		{"mutating verb as a show argument is not mutating", "git show tag", false},
		{"flag before a read-only subcommand is not mutating", "git -C commit log", false},
		{"tag --points-at after read-only commands is not mutating", "sed -n '1,40p' internal/x.go; git -C /worktree log -1 --format='%h %d' ; git -C /worktree tag --points-at HEAD", false},
		{"bare git tag is not mutating", "git tag", false},
		{"tag -l with a pattern is not mutating", "git tag -l 'v1.*'", false},
		{"tag --list is not mutating", "git tag --list", false},
		{"tag --contains is not mutating", "git tag --contains abc123", false},
		{"tag --contains in the = form is not mutating", "git tag --contains=abc123", false},
		{"tag --no-contains is not mutating", "git tag --no-contains abc123", false},
		{"tag --merged is not mutating", "git tag --merged main", false},
		{"tag --no-merged is not mutating", "git tag --no-merged main", false},
		{"tag -n is not mutating", "git tag -n", false},
		{"tag -n with an attached count is not mutating", "git tag -n5", false},
		{"tag --sort value starting with a dash is not mutating", "git tag -l --sort -version:refname", false},
		{"tag --format value starting with a dash is not mutating", "git tag -l --format -version:refname", false},
		{"tag list option before a double dash is not mutating", "git tag -l -- v1", false},
		{"git log is not mutating", "git log --oneline", false},
		{"git diff is not mutating", "git diff HEAD~1", false},
		{"git status is not mutating", "git status", false},
		{"subcommand word inside a filename is not mutating", "git diff branch-notes.txt", false},
		{"chained non-git command after a clean git call", "git log && rm -rf tmp", false},
		{"no git at all", "cat notes.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := commandRunsMutatingGit(tt.cmd); got != tt.want {
				t.Errorf("commandRunsMutatingGit(%q) = %v; want %v", tt.cmd, got, tt.want)
			}
		})
	}
}
