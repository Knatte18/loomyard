// cluster.go implements the fail-loud policy Engine.Run enforces over a cluster round's
// shuttleengine.ForkAudit: the fork-count contract and the per-fork violation classes a disobedient
// fork reviewer can trigger.
// auditClusterRound is the single place that turns the raw audit facts (shuttleengine's own
// knowledge — never this package's) into burler's hard-error/warning split, per the
// fail-loud-posture Shared Decision.

package burlerengine

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ErrClusterForksMissing is the sentinel for when a cluster round's ForkAudit does not carry
// exactly the requested number of fork reviewers.
var ErrClusterForksMissing = errors.New("the cluster round did not produce exactly the requested number of fork reviewers")

// mutatingGitPattern matches a Bash command string that invokes git with a
// state-mutating subcommand. The subcommand must be a whole token.
var mutatingGitPattern = regexp.MustCompile(
	`(?:^|[;&|]\s*)(?:\S*[\\/])?git(?:\s+[^\s;&|]+)*?\s+(?:add|commit|push|pull|fetch|merge|rebase|reset|restore|rm|mv|checkout|switch|stash|apply|cherry-pick|tag|branch)(?:\s|$)`,
)

// auditClusterRound enforces the cluster-round fail-loud policy. Checks run in
// a fixed order; the first violation is returned as a hard error.
func auditClusterRound(audit *shuttleengine.ForkAudit, wantN int) ([]string, error) {
	gotN := 0
	if audit != nil {
		gotN = len(audit.Forks)
	}
	if audit == nil || gotN != wantN {
		return nil, fmt.Errorf("burler: %w (requested %d, spawned %d)", ErrClusterForksMissing, wantN, gotN)
	}

	for _, fork := range audit.Forks {
		if fork.AgentCalls > 0 {
			return nil, fmt.Errorf("burler: fork %q attempted %d Agent tool call(s) — forks cannot nest and must never call the Agent tool, even when the attempt was denied", fork.TranscriptPath, fork.AgentCalls)
		}
	}
	for _, fork := range audit.Forks {
		if fork.WriteCalls > 0 {
			return nil, fmt.Errorf("burler: fork %q attempted %d write/edit tool call(s) — a fork reviewer must never mutate a file", fork.TranscriptPath, fork.WriteCalls)
		}
	}
	for _, fork := range audit.Forks {
		for _, cmd := range fork.BashCommands {
			if mutatingGitPattern.MatchString(cmd) {
				return nil, fmt.Errorf("burler: fork %q ran a git-mutating command (%q) — a fork reviewer must never run a state-mutating git command", fork.TranscriptPath, cmd)
			}
		}
	}
	if audit.NamedSpawns > 0 {
		return nil, fmt.Errorf("burler: %d fork(s) were spawned with a name — named forks silently lose inherited context, which is a silent quality-degradation defect, not an advisory", audit.NamedSpawns)
	}

	// A fork that never returned a report is surfaced as a warning, not a failure.
	var warnings []string
	for _, fork := range audit.Forks {
		if !fork.ReportReturned {
			warnings = append(warnings, fmt.Sprintf("fork %q never returned a final report", fork.TranscriptPath))
		}
	}
	return warnings, nil
}
