// cluster.go implements the fail-loud policy Engine.Run enforces over a cluster round's
// shuttleengine.ForkAudit: the fork-count contract and the per-fork violation classes a
// disobedient fork reviewer can trigger. auditClusterRound is the single place that
// turns the raw audit facts (shuttleengine's own knowledge — never this package's) into
// burler's hard-error/warning split, per the fail-loud-posture Shared Decision.

package burlerengine

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ErrClusterForksMissing is the sentinel wrapped into the error returned when a cluster
// round's ForkAudit does not carry exactly the requested number of fork reviewers (a nil
// audit and a fork-count shortfall or overshoot are both this same defect). It carries no
// "burler: " prefix of its own — every call site that surfaces it (auditClusterRound,
// below) already wraps it inside its own burler-prefixed message, and prefixing it here
// too would double that prefix in the final error text (the same rationale the deleted
// ErrClusterUnsupported sentinel followed).
var ErrClusterForksMissing = errors.New("the cluster round did not produce exactly the requested number of fork reviewers")

// mutatingGitPattern matches a Bash command string that invokes git with a
// state-mutating subcommand (add, commit, push, pull, fetch, merge, rebase, reset,
// restore, rm, mv, checkout, switch, stash, apply, cherry-pick, tag, branch). It
// tolerates a path-qualified git binary (`/usr/bin/git`), any number of intervening
// flag/argument tokens between `git` and the subcommand (`git -C dir commit`), and a
// leading `&&`/`;`/`|` chain prefix (`cd x && git commit`) — but the subcommand itself
// must be a whole token (bounded by whitespace or the command's end), so an argument
// that merely CONTAINS a subcommand word (`git diff branch-notes.txt`) does not
// false-positive, and the intervening-token scan never crosses a chain operator, so a
// git-free command chained after a harmless git invocation (`git log && rm -rf x`)
// is not mistaken for a git subcommand either.
var mutatingGitPattern = regexp.MustCompile(
	`(?:^|[;&|]\s*)(?:\S*[\\/])?git(?:\s+[^\s;&|]+)*?\s+(?:add|commit|push|pull|fetch|merge|rebase|reset|restore|rm|mv|checkout|switch|stash|apply|cherry-pick|tag|branch)(?:\s|$)`,
)

// auditClusterRound enforces the cluster-round fail-loud policy over audit, the raw
// fork-spawning facts shuttleengine observed for a run whose Spec authorized
// ForkSubagents. wantN is the fan's resolved entry count (len(Profile.clusterLenses)) —
// the exact number of fork reviewers the round was supposed to produce.
//
// Checks run in a fixed order and the first violation is returned as a burler-prefixed
// hard error (per the fail-loud-posture Shared Decision — a cluster round that cannot
// deliver exactly what the profile demanded is an infrastructure defect, not an
// advisory):
//
//  1. audit is nil, or len(audit.Forks) != wantN: wraps ErrClusterForksMissing with %w,
//     naming the requested and actual counts.
//  2. Any fork with AgentCalls > 0: an attempted nested Agent-tool call, even one the
//     hook denied, is itself the violation — forks cannot nest.
//  3. Any fork with WriteCalls > 0: a fork reviewer must never mutate a file.
//  4. Any fork whose BashCommands contains an entry matching mutatingGitPattern: a fork
//     reviewer must never run any git command.
//  5. audit.NamedSpawns > 0: a named fork silently loses inherited context — silent
//     quality degradation is the rejected class, same severity as the mechanical
//     violations above it.
//
// A fork with ReportReturned == false is sloppiness no mechanism can prevent (the fork
// ran clean but never delivered its findings) — it is collected into warnings and
// returned alongside a nil error, never failing the round.
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
				return nil, fmt.Errorf("burler: fork %q ran a git-mutating command (%q) — a fork reviewer must never run any git command", fork.TranscriptPath, cmd)
			}
		}
	}
	if audit.NamedSpawns > 0 {
		return nil, fmt.Errorf("burler: %d fork(s) were spawned with a name — named forks silently lose inherited context, which is a silent quality-degradation defect, not an advisory", audit.NamedSpawns)
	}

	// A fork that never returned a report is sloppiness no mechanism prevents in
	// advance — surface it as a warning on the Result rather than failing a round
	// whose forks otherwise obeyed every hard rule above.
	var warnings []string
	for _, fork := range audit.Forks {
		if !fork.ReportReturned {
			warnings = append(warnings, fmt.Sprintf("fork %q never returned a final report", fork.TranscriptPath))
		}
	}
	return warnings, nil
}
