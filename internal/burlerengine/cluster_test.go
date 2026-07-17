// cluster_test.go table-drives auditClusterRound over the full violation taxonomy
// (fork-count mismatch, Agent-call-in-fork, write-in-fork, git-mutation-in-fork, named
// spawns) plus the warning-only ReportReturned==false case, and separately matrixes
// mutatingGitPattern against a set of mutating and non-mutating Bash command strings.

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
	}{
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
			warnings, err := auditClusterRound(tt.audit, tt.wantN)

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

// TestMutatingGitPattern matrixes mutatingGitPattern against every Bash command shape
// the audit must classify correctly: every listed mutating subcommand, tolerant of a
// leading path/&&/; chain prefix, and — the false-positive guards — a subcommand word
// merely appearing inside a longer argument token, plus non-mutating git commands
// (log, diff, status) that must never match.
func TestMutatingGitPattern(t *testing.T) {
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
		{"branch", "git branch -d old", true},
		{"leading path", "/usr/bin/git commit -m x", true},
		{"chained with &&", "cd internal/burler && git commit -am fix", true},
		{"chained with ;", "cd internal/burler; git push", true},
		{"flag before subcommand", "git -C /worktree commit -m x", true},
		{"git log is not mutating", "git log --oneline", false},
		{"git diff is not mutating", "git diff HEAD~1", false},
		{"git status is not mutating", "git status", false},
		{"subcommand word inside a filename is not mutating", "git diff branch-notes.txt", false},
		{"chained non-git command after a clean git call", "git log && rm -rf tmp", false},
		{"no git at all", "cat notes.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mutatingGitPattern.MatchString(tt.cmd); got != tt.want {
				t.Errorf("mutatingGitPattern.MatchString(%q) = %v; want %v", tt.cmd, got, tt.want)
			}
		})
	}
}
