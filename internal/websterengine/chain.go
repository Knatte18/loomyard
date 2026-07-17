// chain.go implements webster's own chain-rollback act, RestartChain: the
// `begin-batch --restart-chain` variant of builder's spawn-batch chain
// rollback. Webster re-expresses this against its own State type rather
// than reusing builder's RestartChain directly (builder's own RestartChain
// couples to builder's own *builderengine.State), while still reusing
// builder's mechanism-agnostic chain-membership and git helpers
// (ChainMembers, ChainEndFor, ResetHard, BatchReportFileName) by import —
// per the reuse-by-import-never-copy decision, no shared contract is ever
// parsed twice. Per the discussion's correctness-by-tool-design decision,
// the recorded state.json SHA (State.ChainStartSHAs) is the ONLY reset
// target this file ever uses — there is no caller-supplied SHA parameter
// anywhere here.

package websterengine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// RestartChain performs webster's chain-rollback act against member — any
// batch number belonging to the deferred-verify chain, not necessarily the
// chain's end — resolving the chain's end via builderengine.ChainEndFor and
// its full membership via builderengine.ChainMembers. It verifies
// st.ChainStartSHAs[chainEnd] is recorded (a hard error when absent — that
// recorded SHA is the only reset target, so an unrecorded chain can never be
// rolled back to a hallucinated one) and that member actually names a chain
// at all (a hard error when it does not), resets worktree's host repo to the
// recorded anchor via builderengine.ResetHard, deletes every chain member's
// batch-report file from reportsDir (builderengine.BatchReportFileName),
// clears each member's st.Batches entry (its persisted digest included), and
// resets st.CurrentBatch to 0. Before the hard reset it stops every chain
// member's recorded strand the mux still reports live (builder's own restart
// step 1) — a kept-alive recovery strand for a chain member, left running,
// would commit on top of the rolled-back tree and be unreclaimable once its
// BatchState record is deleted below. It returns the chain's LOWEST member
// number, per builder's own re-point rule: begin-batch re-points its own
// batchNumber at this return value before continuing, since the chain always
// restarts from its lowest member regardless of which member the caller
// named. The caller is responsible for persisting st via SaveState
// afterward.
func RestartChain(mux shuttleengine.MuxOps, worktree string, st *State, plan *builderengine.Plan, member int, reportsDir string) (int, error) {
	chainEnd := builderengine.ChainEndFor(plan, member)
	if chainEnd == 0 {
		return 0, fmt.Errorf("webster: batch %d names no deferred-verify chain", member)
	}

	members := builderengine.ChainMembers(plan, chainEnd)
	if len(members) == 0 {
		return 0, fmt.Errorf("webster: batch %d names no deferred-verify chain", member)
	}

	startSHA, ok := st.ChainStartSHAs[chainEnd]
	if !ok || startSHA == "" {
		return 0, fmt.Errorf("webster: no chain-start SHA recorded for chain-end batch %d", chainEnd)
	}

	// Stop every member's recorded strand the mux still reports live BEFORE
	// the destructive reset: only a recovery batch carries a StrandGUID (a
	// plain fork batch's is empty and RemoveStrandIfLive no-ops on it), but a
	// chain member that went to recovery and classified dead keeps its strand
	// alive by design, and once its BatchState is deleted below the strand is
	// unreclaimable — so it must be stopped here, or it commits on top of the
	// tree this reset is about to roll back.
	for _, n := range members {
		if bs, ok := st.Batches[n]; ok && bs != nil && bs.StrandGUID != "" {
			if err := builderengine.RemoveStrandIfLive(mux, bs.StrandGUID); err != nil {
				return 0, err
			}
		}
	}

	if err := builderengine.ResetHard(worktree, startSHA); err != nil {
		return 0, err
	}

	byNumber := make(map[int]builderengine.PlanBatch, len(plan.Batches))
	for _, b := range plan.Batches {
		byNumber[b.Number] = b
	}

	for _, n := range members {
		b, ok := byNumber[n]
		if !ok {
			continue
		}

		reportPath := filepath.Join(reportsDir, builderengine.BatchReportFileName(b.Number, b.Slug))
		if err := os.Remove(reportPath); err != nil && !os.IsNotExist(err) {
			return 0, fmt.Errorf("webster: remove stale chain report %s: %w", reportPath, err)
		}

		delete(st.Batches, n)
	}

	// The chain restarts from its lowest member, so nothing stays in-flight
	// across the reset regardless of which member CurrentBatch previously
	// pointed at.
	st.CurrentBatch = 0

	// members is sorted ascending and non-empty whenever chainEnd is
	// non-zero (the chain always contains at least its declaring member and
	// its end), so [0] is the lowest.
	return members[0], nil
}
