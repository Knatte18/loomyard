// cluster.go implements the fail-loud policy Engine.Review enforces over a cluster round's
// shuttleengine.ForkAudit: the fork-count contract and the per-fork violation classes a disobedient
// fork reviewer can trigger.
// auditClusterRound is the single place that turns the raw audit facts (shuttleengine's own
// knowledge — never this package's) into burler's hard-error/warning split, per the
// fail-loud-posture Shared Decision.
// auditFixRound applies the same per-fork checks to a twill fixer's self-check forks, where a wrong fork count is a warning.

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

	if violation := forkViolation(audit); violation != nil {
		return nil, fmt.Errorf("burler: %w", violation)
	}
	return unreturnedForkWarnings(audit), nil
}

// forkViolation returns the first Agent-call, write, mutating-git or named-spawn violation of audit, checked in that order, or nil when the forks are clean.
// The error carries no burler prefix, so each caller adds its own.
func forkViolation(audit *shuttleengine.ForkAudit) error {
	for _, fork := range audit.Forks {
		if fork.AgentCalls > 0 {
			return fmt.Errorf("fork %q attempted %d Agent tool call(s) — forks cannot nest and must never call the Agent tool, even when the attempt was denied", fork.TranscriptPath, fork.AgentCalls)
		}
	}
	for _, fork := range audit.Forks {
		if fork.WriteCalls > 0 {
			return fmt.Errorf("fork %q attempted %d write/edit tool call(s) — a fork must never mutate a file", fork.TranscriptPath, fork.WriteCalls)
		}
	}
	for _, fork := range audit.Forks {
		for _, cmd := range fork.BashCommands {
			if mutatingGitPattern.MatchString(cmd) {
				return fmt.Errorf("fork %q ran a git-mutating command (%q) — a fork must never run a state-mutating git command", fork.TranscriptPath, cmd)
			}
		}
	}
	if audit.NamedSpawns > 0 {
		return fmt.Errorf("%d fork(s) were spawned with a name — named forks silently lose inherited context, which is a silent quality-degradation defect, not an advisory", audit.NamedSpawns)
	}
	return nil
}

// unreturnedForkWarnings returns one warning per fork of audit that never returned a final report; such a fork is sloppiness, not a failure.
func unreturnedForkWarnings(audit *shuttleengine.ForkAudit) []string {
	var warnings []string
	for _, fork := range audit.Forks {
		if !fork.ReportReturned {
			warnings = append(warnings, fmt.Sprintf("fork %q never returned a final report", fork.TranscriptPath))
		}
	}
	return warnings
}

// auditFixRound enforces the twill fixer's fork policy over audit, the audit of a fixer that spawned self-check forks for wantN lenses.
// A fork violation is a hard error, wrapped with its way forward; a fork count other than wantN, none included, is a warning, since the fix is on disk and failing the half would redo a finished fix.
// It returns the warnings, which also include one per fork that never returned a report, and the audited fork count, 0 for a nil audit.
func auditFixRound(audit *shuttleengine.ForkAudit, wantN int) (warnings []string, forks int, err error) {
	if audit == nil {
		return []string{fmt.Sprintf("twill fixer spawned 0 fork(s) for %d lens(es)", wantN)}, 0, nil
	}
	if violation := forkViolation(audit); violation != nil {
		return nil, len(audit.Forks), fmt.Errorf("burler: twill fixer %w; way forward: inspect the target's working-tree changes, revert what the fork wrote, and resume the run", violation)
	}
	forks = len(audit.Forks)
	if forks != wantN {
		warnings = append(warnings, fmt.Sprintf("twill fixer spawned %d fork(s) for %d lens(es)", forks, wantN))
	}
	warnings = append(warnings, unreturnedForkWarnings(audit)...)
	return warnings, forks, nil
}
