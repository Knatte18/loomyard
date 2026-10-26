// start.go is the "lyx beam start" sequence over a list of resolved beams: it brings each one up through a seam and gathers one entry per beam, a failure in one never stopping the others.

package beamcli

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/orchcli"
)

// startBeams brings up each target through up, in order, and gathers one entry per target.
// Only the first target is prepared, so the hub is reconciled once.
// A target whose up fails records the failure on its entry, and the others still start; the returned error joins every failure, each naming its beam.
func startBeams(targets []beamTarget, up func(target beamTarget, prepare bool) (orchcli.BeamUpReport, error)) (startResult, error) {
	result := startResult{Beams: make([]startEntry, 0, len(targets))}
	var failures []error
	for index, target := range targets {
		prepare := index == 0
		report, err := up(target, prepare)
		entry := startEntry{
			Worktree:         target.Label,
			Outcome:          report.Dobby,
			Detail:           report.DobbyDetail,
			Action:           report.Action,
			PromptSource:     report.PromptSource,
			AbandonedSession: report.AbandonedSession,
			Warnings:         report.Warnings,
		}
		if prepare {
			entry.Reconcile = &report.Reconcile
			entry.FailedPushes = report.FailedPushes
			entry.OwnerPauses = report.OwnerPauses
			entry.Mutations = report.Mutations
		}
		if err != nil {
			entry.Error = err.Error()
			failures = append(failures, fmt.Errorf("%s: %w", target.Label, err))
		}
		result.Beams = append(result.Beams, entry)
	}
	return result, errors.Join(failures...)
}

// ownedTargets lists the targets `--all` starts: the prime and each local beam this machine owns, the prime first when it is owned.
func ownedTargets(prime *lyxcwd.Location, owned []string) ([]beamTarget, error) {
	targets := make([]beamTarget, 0, len(owned))
	for _, name := range owned {
		if name == prime.WorktreeName {
			targets = append(targets, primeTarget(prime))
			continue
		}
		pair, err := lyxcwd.ResolveWorktree(fabricengine.WorktreePath(prime, name))
		if err != nil {
			return nil, fmt.Errorf("resolve the worktree of %s: %w", name, err)
		}
		targets = append(targets, beamTarget{Location: pair, Slug: name, Label: name})
	}
	return targets, nil
}
