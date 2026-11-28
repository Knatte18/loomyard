// cluster_test.go tests auditClusterRound against each violation kind:
// fork-count mismatch, Agent call in a fork, write in a fork, git mutation in a fork, and named spawns.
// It also covers the warning-only case where ReportReturned is false,
// and a twill fixer's fresh subagent, which the fix-round audit neither checks nor counts.
// It matrixes mutatingGitCommands against mutating and non-mutating Bash command strings.
// It also runs it on fork command sequences that create their own repositories and push between them.
// It pins the rule each counted command names.

package burlerengine

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// cleanFork returns a ForkReport that breaks no cluster-audit rule.
// Tests mutate a copy of this base to trigger exactly one violation at a time.
func cleanFork(path string) shuttleengine.ForkReport {
	return shuttleengine.ForkReport{
		TranscriptPath: path,
		ReportReturned: true,
	}
}

// revertWayForward is a fork-audit halt's way forward when a fork may have changed the target.
const revertWayForward = "way forward: inspect the target's working tree, HEAD, branches, tags and remotes for what the fork changed, undo it, and resume the run"

// untouchedPushWayForward is a fork-audit halt's way forward when no fork changed the target and one pushed from /tmp/r.
const untouchedPushWayForward = "no target change was found: the audit found no write or edit call and no state-changing git command outside a repository a fork created, and the push from /tmp/r went to a destination the audit could not place; way forward: check that destination is neither the target's repository nor one of its remotes, and resume the run"

func TestAuditClusterRound(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name         string
		audit        *shuttleengine.ForkAudit
		wantN        int
		wantErr      bool
		errSubstr    string
		wantErrIs    error
		wantWarnings []string
		// fix runs the case through the fix-round audit, and wantForks is the fork count that audit must report.
		fix       bool
		wantForks int
		// wantWayForward is the text a violation's error ends with.
		wantWayForward string
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
			wantN:          2,
			wantErr:        true,
			errSubstr:      `twill fixer fork "b" attempted 2 write/edit tool call(s)`,
			fix:            true,
			wantWayForward: revertWayForward,
		},
		{
			name: "fix: a push from a fork-made repository to a destination it did not make finds no target change",
			audit: &shuttleengine.ForkAudit{
				Forks: []shuttleengine.ForkReport{cleanFork("a"), {TranscriptPath: "b", ReportReturned: true, BashCommands: []string{"git init /tmp/r", "git -C /tmp/r push /srv/target.git main"}}},
			},
			wantN:          2,
			wantErr:        true,
			errSubstr:      `twill fixer fork "b" ran a git-mutating command ("git -C /tmp/r push /srv/target.git main")`,
			fix:            true,
			wantWayForward: untouchedPushWayForward,
		},
		{
			name: "fix: a nested spawn beside a push from a fork-made repository still says where the push came from",
			audit: &shuttleengine.ForkAudit{
				Forks: []shuttleengine.ForkReport{{TranscriptPath: "a", ReportReturned: true, AgentCalls: 1}, {TranscriptPath: "b", ReportReturned: true, BashCommands: []string{"git init /tmp/r && git -C /tmp/r push origin main"}}},
			},
			wantN:          2,
			wantErr:        true,
			errSubstr:      `twill fixer fork "a" attempted 1 nested spawn call(s)`,
			fix:            true,
			wantWayForward: untouchedPushWayForward,
		},
		{
			name: "fix: a command that pushes from a fork-made repository and commits in the target keeps the revert way forward",
			audit: &shuttleengine.ForkAudit{
				Forks: []shuttleengine.ForkReport{cleanFork("a"), {TranscriptPath: "b", ReportReturned: true, BashCommands: []string{"git init /tmp/r && git -C /tmp/r push /srv/target.git main && git commit -am x"}}},
			},
			wantN:          2,
			wantErr:        true,
			errSubstr:      "git-mutating command",
			fix:            true,
			wantWayForward: revertWayForward,
		},
		{
			name: "fix: a nested spawn beside a fork's commit in the target keeps the revert way forward",
			audit: &shuttleengine.ForkAudit{
				Forks: []shuttleengine.ForkReport{{TranscriptPath: "a", ReportReturned: true, BashCommands: []string{"git commit -am x"}}, {TranscriptPath: "b", ReportReturned: true, AgentCalls: 1}},
			},
			wantN:          2,
			wantErr:        true,
			errSubstr:      `twill fixer fork "b" attempted 1 nested spawn call(s)`,
			fix:            true,
			wantWayForward: revertWayForward,
		},
		{
			name: "fix: a fresh subagent that wrote is neither a violation nor a fork",
			audit: &shuttleengine.ForkAudit{
				Forks: []shuttleengine.ForkReport{cleanFork("a"), {TranscriptPath: "fresh", WriteCalls: 5, FreshSubagent: true}, cleanFork("b")},
			},
			wantN:     2,
			fix:       true,
			wantForks: 2,
		},
		{
			name: "fix: the forks under a fresh subagent are neither violations nor self-check forks",
			audit: &shuttleengine.ForkAudit{
				Forks: []shuttleengine.ForkReport{
					cleanFork("a"),
					{TranscriptPath: "deeper", WriteCalls: 1, SpawnerTranscriptPath: "nested"},
					{TranscriptPath: "fresh", AgentCalls: 1, FreshSubagent: true},
					{TranscriptPath: "nested", AgentCalls: 1, WriteCalls: 1, BashCommands: []string{"git commit -am x"}, SpawnerTranscriptPath: "fresh"},
					cleanFork("b"),
				},
			},
			wantN:     2,
			fix:       true,
			wantForks: 2,
		},
		{
			name: "fix: a write by a fork a self-check fork spawned keeps the revert way forward",
			audit: &shuttleengine.ForkAudit{
				Forks: []shuttleengine.ForkReport{
					{TranscriptPath: "a", ReportReturned: true, AgentCalls: 1},
					{TranscriptPath: "nested", WriteCalls: 1, SpawnerTranscriptPath: "a"},
				},
			},
			wantN:          1,
			wantErr:        true,
			errSubstr:      `twill fixer fork "a" attempted 1 nested spawn call(s)`,
			fix:            true,
			wantForks:      2,
			wantWayForward: revertWayForward,
		},
		{
			name: "fix: a commit by a fresh subagent a self-check fork spawned keeps the revert way forward",
			audit: &shuttleengine.ForkAudit{
				Forks: []shuttleengine.ForkReport{
					{TranscriptPath: "a", ReportReturned: true, AgentCalls: 1},
					{TranscriptPath: "fresh", BashCommands: []string{"git commit -am x"}, FreshSubagent: true, SpawnerTranscriptPath: "a"},
				},
			},
			wantN:          1,
			wantErr:        true,
			errSubstr:      `twill fixer fork "a" attempted 1 nested spawn call(s)`,
			fix:            true,
			wantForks:      2,
			wantWayForward: revertWayForward,
		},
		{
			name: "fix: a named fresh delegate is no named-fork violation",
			audit: &shuttleengine.ForkAudit{
				Forks:           []shuttleengine.ForkReport{cleanFork("a"), cleanFork("b"), {TranscriptPath: "fresh", FreshSubagent: true}},
				NamedSpawns:     1,
				NamedForkSpawns: 0,
			},
			wantN:     2,
			fix:       true,
			wantForks: 2,
		},
		{
			name: "fix: a named fork spawn is the twill fixer violation",
			audit: &shuttleengine.ForkAudit{
				Forks:           []shuttleengine.ForkReport{cleanFork("a"), cleanFork("b")},
				NamedSpawns:     1,
				NamedForkSpawns: 1,
			},
			wantN:          2,
			wantErr:        true,
			errSubstr:      "twill fixer 1 fork(s) were spawned with a name",
			fix:            true,
			wantWayForward: "no target change was found: the audit found no write or edit call and no state-changing git command outside a repository a fork created; way forward: resume the run",
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
			errSubstr: "nested spawn call",
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
			wantN:          2,
			wantErr:        true,
			errSubstr:      `ran a git-mutating command ("git commit -am wip"), counted as a state-changing git command outside every repository the fork created`,
			wantWayForward: revertWayForward,
		},
		{
			name: "a push after ln from a fork-made repository names the ln rule and finds no target change",
			audit: &shuttleengine.ForkAudit{
				Forks: []shuttleengine.ForkReport{
					cleanFork("a"),
					{TranscriptPath: "b", ReportReturned: true, BashCommands: []string{
						"S=/wt/.scratch/exp2; mkdir -p $S; git init -q $S/r; git -C $S/r commit -q --allow-empty -m c; ln -s /nonexistent-mount/x.git $S/link.git; git -C $S/r remote add origin $S/link.git; git -C $S/r push origin HEAD 2>&1 | head -1",
					}},
				},
			},
			wantN:          2,
			wantErr:        true,
			errSubstr:      "counted as a push after ln, since a link may lead out of every repository the fork created",
			wantWayForward: "no target change was found: the audit found no write or edit call and no state-changing git command outside a repository a fork created, and the push from $S/r went to a destination the audit could not place; way forward: check that destination is neither the target's repository nor one of its remotes, and resume the run",
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
			} else {
				warnings, err = auditClusterRound(tt.audit, tt.wantN)
			}
			if tt.wantWayForward != "" && (err == nil || !strings.HasSuffix(err.Error(), "; "+tt.wantWayForward)) {
				t.Errorf("audit error = %v; want it to end with %q", err, tt.wantWayForward)
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

// TestFirstMutatingGitCommand checks mutatingGitCommands against one Bash command per case.
// Every mutating subcommand matches, whether it has a leading path, a chain prefix, a later line or global options.
// Read-only subcommands never match, even when their arguments name a mutating verb.
// Help requests, and the list forms of git tag and git branch, never match either.
func TestFirstMutatingGitCommand(t *testing.T) {
	tiergate.Require(t, 0)
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
		{"branch create", "git branch new", true},
		{"branch force delete", "git branch -D old", true},
		{"branch rename", "git branch -m a b", true},
		{"branch copy", "git branch -c a b", true},
		{"branch set upstream", "git branch -u origin/main", true},
		{"branch set upstream in the = form", "git branch --set-upstream-to=origin/main", true},
		{"branch unset upstream", "git branch --unset-upstream", true},
		{"branch force reset", "git branch -f main HEAD", true},
		{"branch -a with a branch name creates it", "git branch -a foo", true},
		{"branch -v with a branch name creates it", "git branch -v foo", true},
		{"branch --verbose with a branch name creates it", "git branch --verbose foo", true},
		{"branch -vv with a branch name creates it", "git branch -vv foo", true},
		{"branch name before a trailing double dash", "git branch foo --", true},
		{"branch --format with a branch name creates it", "git branch --format %(refname) foo", true},
		{"branch delete beside a list option", "git branch -l -d old", true},
		{"branch delete in a read-only cluster", "git branch -aD old", true},
		{"branch name after a double dash", "git branch -- new", true},
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
		{"bare git branch is not mutating", "git branch", false},
		{"branch -a is not mutating", "git branch -a", false},
		{"branch -r is not mutating", "git branch -r", false},
		{"branch -avv cluster is not mutating", "git branch -avv", false},
		{"branch -a --contains is not mutating", "git branch -a --contains 71a46b11", false},
		{"branch --contains in the = form is not mutating", "git branch --contains=71a46b11", false},
		{"branch -l with a pattern is not mutating", "git branch -l 'feat*'", false},
		{"branch --list with a pattern is not mutating", "git branch --list feat*", false},
		{"branch --merged is not mutating", "git branch --merged main", false},
		{"branch --no-merged is not mutating", "git branch --no-merged main", false},
		{"branch --points-at is not mutating", "git branch --points-at HEAD", false},
		{"branch --show-current is not mutating", "git branch --show-current", false},
		{"branch --sort value starting with a dash is not mutating", "git branch --sort -committerdate", false},
		{"branch --format value starting with a dash is not mutating", "git branch --format -x", false},
		{"branch --points-at value starting with a dash is not mutating", "git branch --points-at -x", false},
		{"branch bare double dash is not mutating", "git branch --", false},
		{"branch -v is not mutating", "git branch -v", false},
		{"branch -v beside -l with a pattern is not mutating", "git branch -v -l feat*", false},
		{"branch list option before a pattern after a double dash is not mutating", "git branch -l -- feat*", false},
		{"branch -a with stderr redirected is not mutating", "git branch -a 2>/dev/null", false},
		{"branch with a 2>&1 redirect into a pipe is not mutating", "git branch 2>&1 | head", false},
		{"branch with a spaced output redirect is not mutating", "git branch -r > branches.txt", false},
		{"redirect before the subcommand hides no mutating verb", "git 2>/dev/null commit -m x", true},
		{"fd duplication before the subcommand hides no mutating verb", "git 2>&1 commit -m x", true},
		{"quoted redirect character is an operand", `git branch ">"`, true},
		{"quoted digit before a redirect is an operand", `git branch "2">x`, true},
		{"redirect with no target before a separator drops no later word", "git branch -a >; git commit -m x", true},
		{"digit word detached from the redirect is an operand", "git branch 2 > x", true},
		{"escaped redirect character is an operand", `git branch \>`, true},
		{"read-only git chain from a burler halt is not mutating", `git branch -a --contains 71a46b11 2>/dev/null | head -20 && git merge-base --is-ancestor 71a46b11 HEAD 2>/dev/null && echo ancestor || echo not-ancestor; git show --stat 71a46b11 2>/dev/null | head -40 | grep -v "^$"`, false},
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
			if got := len(mutatingGitCommands([]string{tt.cmd})) > 0; got != tt.want {
				t.Errorf("mutatingGitCommands([%q]) found = %v; want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

// TestFirstMutatingGitCommandScopesToForkRepositories pins that a mutating git call in a fork-made repository passes.
// This holds whether the call reaches that repository by -C, by cd, or as a worktree or clone made from it.
// A push passes when its destination leads into a fork-made repository, by a remote the fork pointed there or by a path.
// A call that may reach any other repository is the first one returned.
func TestFirstMutatingGitCommandScopesToForkRepositories(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name     string
		commands []string
		want     string
	}{
		{
			"scratch repository experiment in one command",
			[]string{`S=/tmp/scratch/wt && git init "$S/repo" && git -C "$S/repo" commit --allow-empty -m init && git -C "$S/repo" worktree add "$S/wt2" && git -C "$S/repo" worktree repair`},
			"",
		},
		{
			"absolute scratch repository in one command",
			[]string{"git init /tmp/scratch/repo && git -C /tmp/scratch/repo commit --allow-empty -m init"},
			"",
		},
		{"repository created by an earlier command", []string{"git init -b main .scratch/repo", "git -C .scratch/repo commit --allow-empty -m x"}, ""},
		{"cd into the created repository", []string{"git init /tmp/r && cd /tmp/r && git commit --allow-empty -m x"}, ""},
		{"git init inside a cd", []string{"cd /tmp/r && git init && git add . && git commit -m x"}, ""},
		{"subdirectory of the created repository", []string{"git init /tmp/r", "git -C /tmp/r/sub add f"}, ""},
		{"relative -C under an anchored cd", []string{"cd $S && git init repo && git -C repo/../repo commit -m x"}, ""},
		{"worktree added from the created repository", []string{"git init /tmp/r", "git -C /tmp/r worktree add /tmp/w", "git -C /tmp/w commit -m x"}, ""},
		{"commit outside every created repository", []string{"git init /tmp/r", "git -C /tmp/other commit -m x"}, "git -C /tmp/other commit -m x"},
		{"sibling sharing the created name prefix", []string{"git init /tmp/r", "git -C /tmp/r2 commit -m x"}, "git -C /tmp/r2 commit -m x"},
		{"commit in the starting directory", []string{"git init /tmp/r && git commit -m x"}, "git init /tmp/r && git commit -m x"},
		{"push from the created repository", []string{"git init /tmp/r", "git -C /tmp/r push ../target HEAD:main"}, "git -C /tmp/r push ../target HEAD:main"},
		{"--git-dir beside the created repository", []string{"git init /tmp/r", "git -C /tmp/r --git-dir=/target/.git commit -m x"}, "git -C /tmp/r --git-dir=/target/.git commit -m x"},
		{"--work-tree beside the created repository", []string{"git init /tmp/r", "git -C /tmp/r --work-tree /target add f"}, "git -C /tmp/r --work-tree /target add f"},
		{"bare cd after creating a repository", []string{"git init r && cd && git -C r commit -m x"}, "git init r && cd && git -C r commit -m x"},
		{"cd - after creating a repository", []string{"git init r && cd - && git -C r commit -m x"}, "git init r && cd - && git -C r commit -m x"},
		{"git init of the starting directory", []string{"git init", "git commit -m x"}, "git commit -m x"},
		{"git init of an ancestor", []string{"git init ..", "git -C .. commit -m x"}, "git -C .. commit -m x"},
		{"relative path escaping a created ~ repository", []string{"git init ~/r", "git -C ~/r/../.. commit -m x"}, "git -C ~/r/../.. commit -m x"},
		{"git -C above an anchored created repository", []string{`git init "$S" && git -C "$S/../target" commit -m x`}, `git init "$S" && git -C "$S/../target" commit -m x`},
		{"git init of a path leaving the starting directory", []string{"git init ../x", "git -C ../x commit -m x"}, "git -C ../x commit -m x"},
		{"git init -- terminator", []string{"git init -- -r", "git -C -r commit -m x"}, ""},
		{"git worktree add valued options", []string{"git init /tmp/r", "git -C /tmp/r worktree add -b br /tmp/w", "git -C /tmp/w commit -m x"}, ""},
		{"git init valued options", []string{"git init --template /tmp/tpl /tmp/r", "git -C /tmp/r commit -m x"}, ""},
		{"created path with a trailing slash", []string{"git init /tmp/r/", "git -C /tmp/r commit -m x"}, ""},
		{"anchored cd after a bare cd", []string{"git init /tmp/r && cd && cd /tmp/r && git commit -m x"}, ""},
		{"cd in an earlier command does not carry over", []string{"git init /tmp/r && cd /tmp/r", "git commit -m x"}, "git commit -m x"},
		{"worktree added from an uncreated repository", []string{"git worktree add /tmp/w", "git -C /tmp/w commit -m x"}, "git -C /tmp/w commit -m x"},
		{
			"scratch push probe between a bare and a work repository",
			[]string{"cd .scratch/probe && git init --bare remote.git && git init work && cd work && git commit --allow-empty -m init && git remote add origin ../remote.git && git push --force-with-lease origin HEAD:main"},
			"",
		},
		{"push to a path inside a created repository", []string{"git init --bare /tmp/remote.git", "git init /tmp/w", "git -C /tmp/w push /tmp/remote.git HEAD:main"}, ""},
		{"push to the origin of a clone of a created repository", []string{"git init --bare /tmp/remote.git && git clone /tmp/remote.git /tmp/w && git -C /tmp/w commit --allow-empty -m x && git -C /tmp/w push origin HEAD"}, ""},
		{"push from a clone in its derived directory", []string{"cd /tmp/p && git init --bare src.git && git clone src.git && git -C src commit --allow-empty -m x && git -C src push origin HEAD"}, ""},
		{"push to a clone's remote named by --origin", []string{"git init --bare /tmp/b.git && git clone --origin up /tmp/b.git /tmp/w && git -C /tmp/w push up HEAD"}, ""},
		{"commit in a clone of an uncreated repository", []string{"git clone /srv/target /tmp/c && git -C /tmp/c commit --allow-empty -m x"}, ""},
		{"push to the origin of a clone of an uncreated repository", []string{"git clone /srv/target /tmp/c", "git -C /tmp/c push origin HEAD"}, "git -C /tmp/c push origin HEAD"},
		{"push to a remote pointed outside", []string{"git init /tmp/r && git -C /tmp/r remote add origin /srv/target.git && git -C /tmp/r push origin main"}, "git init /tmp/r && git -C /tmp/r remote add origin /srv/target.git && git -C /tmp/r push origin main"},
		{"push to a remote whose URL was set outside", []string{"git init --bare /tmp/b.git && git init /tmp/r && git -C /tmp/r remote add origin /tmp/b.git", "git -C /tmp/r remote set-url origin /srv/target.git", "git -C /tmp/r push origin main"}, "git -C /tmp/r push origin main"},
		{"push to a remote given a second URL outside", []string{"git init --bare /tmp/b.git && git init /tmp/r && git -C /tmp/r remote add origin /tmp/b.git", "git -C /tmp/r remote set-url --add --push origin /srv/target.git", "git -C /tmp/r push origin main"}, "git -C /tmp/r push origin main"},
		{"push to a renamed remote", []string{"git init --bare /tmp/b.git && git init /tmp/r && git -C /tmp/r remote add origin /tmp/b.git && git -C /tmp/r remote rename origin up", "git -C /tmp/r push up main"}, "git -C /tmp/r push up main"},
		{"push to a remote name the fork never pointed", []string{"git init /tmp/r", "git -C /tmp/r push origin main"}, "git -C /tmp/r push origin main"},
		{"push with no destination", []string{"git init /tmp/r", "git -C /tmp/r push"}, "git -C /tmp/r push"},
		{"push with --repo", []string{"git init --bare /tmp/b.git && git init /tmp/r", "git -C /tmp/r push --repo=/tmp/b.git"}, "git -C /tmp/r push --repo=/tmp/b.git"},
		{"push to a URL", []string{"git init /tmp/r", "git -C /tmp/r push file:///tmp/r main"}, "git -C /tmp/r push file:///tmp/r main"},
		{"push after a config key that may redirect it", []string{"git init --bare /tmp/b.git && git init /tmp/r", "git -C /tmp/r config url./srv/target.git.insteadOf /tmp/b.git", "git -C /tmp/r push /tmp/b.git main"}, "git -C /tmp/r push /tmp/b.git main"},
		{"push under a -c key that may redirect it", []string{"git init --bare /tmp/b.git && git init /tmp/r", "git -C /tmp/r -c remote.pushDefault=x push /tmp/b.git"}, "git -C /tmp/r -c remote.pushDefault=x push /tmp/b.git"},
		{
			"push after a push URL set outside and a plain URL set inside",
			[]string{"git init --bare /tmp/b.git && git init /tmp/r && git -C /tmp/r remote add origin /tmp/b.git", "git -C /tmp/r remote set-url --push origin /srv/target.git", "git -C /tmp/r remote set-url origin /tmp/b.git", "git -C /tmp/r push origin main"},
			"git -C /tmp/r push origin main",
		},
		{
			"push after a URL set outside from a worktree of the pushing repository",
			[]string{"git init --bare /tmp/b.git && git init /tmp/r && git -C /tmp/r remote add origin /tmp/b.git && git -C /tmp/r worktree add /tmp/w", "git -C /tmp/w remote set-url origin /srv/target.git", "git -C /tmp/r push origin main"},
			"git -C /tmp/r push origin main",
		},
		{
			"push after a URL set outside under --git-dir",
			[]string{"git init --bare /tmp/b.git && git init /tmp/r && git -C /tmp/r remote add origin /tmp/b.git", "git --git-dir=/tmp/r/.git remote set-url origin /srv/target.git", "git -C /tmp/r push origin main"},
			"git -C /tmp/r push origin main",
		},
		{
			"push under a --config-env key that may redirect it",
			[]string{"git init --bare /tmp/b.git && git init /tmp/r && git -C /tmp/r remote add origin /tmp/b.git", "git -C /tmp/r --config-env=remote.origin.pushurl=E push origin main"},
			"git -C /tmp/r --config-env=remote.origin.pushurl=E push origin main",
		},
		{
			"push under a -c include that may redirect it",
			[]string{"git init --bare /tmp/b.git && git init /tmp/r && git -C /tmp/r remote add origin /tmp/b.git", "git -C /tmp/r -c include.path=/tmp/x push origin main"},
			"git -C /tmp/r -c include.path=/tmp/x push origin main",
		},
		{
			"push to the directory above the repository from a subdirectory",
			[]string{"git init r && mkdir r/x && git -C r/x push .. HEAD:refs/heads/x"},
			"git init r && mkdir r/x && git -C r/x push .. HEAD:refs/heads/x",
		},
		{
			"push to a remote added from a subdirectory with a relative URL above the repository",
			[]string{"git init r && git -C r/x remote add origin .. && git -C r push origin HEAD"},
			"git init r && git -C r/x remote add origin .. && git -C r push origin HEAD",
		},
		{"push to a relative path inside a created repository", []string{"git init --bare remote.git && git init r && git -C r push ../remote.git HEAD"}, ""},
		{
			"clone of a repository named remote.git sets no redirect",
			[]string{"cd /tmp/p && git init --bare remote.git && git clone remote.git work && git -C work commit --allow-empty -m x && git -C work push origin HEAD"},
			"",
		},
		{
			"clone with a config key that may redirect pushes",
			[]string{"cd /tmp/p && git init --bare remote.git && git clone -c remote.origin.pushurl=/srv/t.git remote.git work && git -C work push origin HEAD"},
			"cd /tmp/p && git init --bare remote.git && git clone -c remote.origin.pushurl=/srv/t.git remote.git work && git -C work push origin HEAD",
		},
		{
			"clone with a --config key that may redirect pushes",
			[]string{"git init --bare /tmp/b.git && git clone --config=url.x.insteadOf=y /tmp/b.git /tmp/w", "git -C /tmp/w push /tmp/b.git HEAD"},
			"git -C /tmp/w push /tmp/b.git HEAD",
		},
		{"push to a clone's remote named by an attached --origin", []string{"git init --bare /tmp/b.git && git clone --origin=up /tmp/b.git /tmp/w && git -C /tmp/w push up HEAD"}, ""},
		{"push from a clone inside a created repository", []string{"git init /tmp/r && git init --bare /tmp/b.git && git clone /tmp/b.git /tmp/r/sub && git -C /tmp/r/sub push origin HEAD"}, ""},
		{
			"push after a branch key that may redirect it",
			[]string{"git init --bare /tmp/b.git && git init /tmp/r", "git -C /tmp/r config branch.main.remote x", "git -C /tmp/r push /tmp/b.git main"},
			"git -C /tmp/r push /tmp/b.git main",
		},
		{
			"push to a remote removed after it was added",
			[]string{"git init --bare /tmp/b.git && git init /tmp/r && git -C /tmp/r remote add origin /tmp/b.git && git -C /tmp/r remote remove origin", "git -C /tmp/r push origin main"},
			"git -C /tmp/r push origin main",
		},
		{
			"push to a remote removed by rm after it was added",
			[]string{"git init --bare /tmp/b.git && git init /tmp/r && git -C /tmp/r remote add origin /tmp/b.git && git -C /tmp/r remote rm origin", "git -C /tmp/r push origin main"},
			"git -C /tmp/r push origin main",
		},
		{
			"push to a remote whose URL was deleted",
			[]string{"git init --bare /tmp/b.git && git init /tmp/r && git -C /tmp/r remote add origin /tmp/b.git", "git -C /tmp/r remote set-url --delete origin /tmp/b.git", "git -C /tmp/r push origin main"},
			"git -C /tmp/r push origin main",
		},
		{
			"push through a symlink inside a created repository",
			[]string{"git init --bare /tmp/b.git && ln -s /srv/target.git /tmp/b.git/t && git init /tmp/r && git -C /tmp/r push /tmp/b.git/t main"},
			"git init --bare /tmp/b.git && ln -s /srv/target.git /tmp/b.git/t && git init /tmp/r && git -C /tmp/r push /tmp/b.git/t main",
		},
		{
			"git init of an ancestor of a directory an earlier call ran in",
			[]string{"git -C /srv/target log && git init /srv && git -C /srv/target commit -m x"},
			"git -C /srv/target log && git init /srv && git -C /srv/target commit -m x",
		},
		{
			"git clone into a directory an earlier call ran in",
			[]string{"git -C /srv/target status && git clone /tmp/b.git /srv/target && git -C /srv/target commit -m x"},
			"git -C /srv/target status && git clone /tmp/b.git /srv/target && git -C /srv/target commit -m x",
		},
		{
			"push with -u from outside every created repository",
			[]string{"git init --bare /tmp/b.git", "git push -u /tmp/b.git main"},
			"git push -u /tmp/b.git main",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			found := mutatingGitCommands(tt.commands)
			got := ""
			if len(found) > 0 {
				got = found[0].command
			}
			if got != tt.want {
				t.Errorf("mutatingGitCommands(%q) first command = %q; want %q", tt.commands, got, tt.want)
			}
		})
	}
}

// TestMutatingGitCommandsNameTheBrokenRule pins the rule each counted command names, with a row for every rule.
// A push after a call that makes every later push count names that call's rule, the first such call winning.
// A command with several counted invocations names the first one's rule.
func TestMutatingGitCommandsNameTheBrokenRule(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name     string
		commands []string
		want     string
	}{
		{"commit outside every created repository", []string{"git commit -am x"}, ruleOutsideCreated},
		{"push after ln", []string{"git init /tmp/r", "ln -s /srv/t.git /tmp/l.git", "git -C /tmp/r push /tmp/r main"}, rulePushAfterLink},
		{"push after a config key and a later ln", []string{"git init /tmp/r && git -C /tmp/r config url.x.insteadOf y && ln -s a b && git -C /tmp/r push /tmp/r main"}, rulePushAfterRedirectKey},
		{"push after a remote change in an unknown directory", []string{"git init /tmp/r", "git --git-dir=/tmp/x remote add o /x", "git -C /tmp/r push /tmp/r main"}, rulePushAfterUnknownRemoteChange},
		{"push with --repo", []string{"git init /tmp/r", "git -C /tmp/r push --repo=/tmp/r"}, rulePushWithRepo},
		{"push with -u from outside", []string{"git init --bare /tmp/b.git", "git push -u /tmp/b.git main"}, rulePushSetsUpstreamOutside},
		{"push with no destination", []string{"git init /tmp/r", "git -C /tmp/r push"}, rulePushNoDestination},
		{"push to a remote never pointed", []string{"git init /tmp/r", "git -C /tmp/r push origin main"}, rulePushToRemoteOutside},
		{"push to a remote pointed outside", []string{"git init /tmp/r && git -C /tmp/r remote add origin /srv/t.git && git -C /tmp/r push origin main"}, rulePushToRemoteOutside},
		{"push to a path outside", []string{"git init /tmp/r", "git -C /tmp/r push /srv/t.git main"}, rulePushToPathOutside},
		{"push to a URL", []string{"git init /tmp/r", "git -C /tmp/r push file:///tmp/r main"}, rulePushToPathOutside},
		{"push to an scp-style location", []string{"git init /tmp/r", "git -C /tmp/r push host:repo main"}, rulePushToPathOutside},
		{"the first counted invocation names the rule", []string{"git commit -am x && git init /tmp/r && git -C /tmp/r push"}, ruleOutsideCreated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			found := mutatingGitCommands(tt.commands)
			if len(found) == 0 {
				t.Fatalf("mutatingGitCommands(%q) found none; want one naming %q", tt.commands, tt.want)
			}
			if got := found[0].rule; got != tt.want {
				t.Errorf("mutatingGitCommands(%q) first rule = %q; want %q", tt.commands, got, tt.want)
			}
		})
	}
}
