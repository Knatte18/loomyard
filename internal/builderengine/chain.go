// chain.go implements deferred-verify chain membership (ChainMembers, ChainEndFor) and the Go-owned
// rollback act RestartChain performs behind `spawn-batch --restart-chain`: reset the repo to
// the chain's recorded start SHA, clear the chain members' stale reports, and reset their in-memory
// run state.
// Per the discussion's correctness-by-tool- design decision, the recorded state.json SHA is the
// ONLY reset target — there is no caller-supplied SHA parameter anywhere in this file.

package builderengine

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// ChainMembers returns every batch number belonging to the deferred- verify chain whose terminal
// batch is chainEnd: every batch whose own ChainEnd equals chainEnd, plus chainEnd itself, sorted
// ascending.
// Returns nil when chainEnd names no chain at all — no batch's ChainEnd points at it.
func ChainMembers(plan *Plan, chainEnd int) []int {
	var members []int
	for _, b := range plan.Batches {
		if b.ChainEnd == chainEnd {
			members = append(members, b.Number)
		}
	}
	if len(members) == 0 {
		return nil
	}

	members = append(members, chainEnd)
	sort.Ints(members)
	return members
}

// ChainEndFor returns the chain-end batch number that batch belongs to: batch's own ChainEnd when
// it declares one,
// or batch itself when some other batch's ChainEnd names it.
// Returns 0 when batch is chainless.
func ChainEndFor(plan *Plan, batch int) int {
	for _, b := range plan.Batches {
		if b.Number == batch && b.ChainEnd != 0 {
			return b.ChainEnd
		}
	}
	for _, b := range plan.Batches {
		if b.ChainEnd == batch {
			return batch
		}
	}
	return 0
}

// FabricResetter is the hard-reset surface RestartChain drives on the fabric repo: a single
// ResetHard(sha) verb, structurally satisfied by both *gitrepo.Repo (tests, driving their scratch
// worktree directly) and *fabricengine.Fabric (production, which forwards to its fabric repo).
// It exists so RestartChain never depends on a concrete git-handle type.
type FabricResetter interface {
	ResetHard(sha string) error
}

// RestartChain performs the chain-rollback act: it verifies st.ChainStartSHAs[chainEnd] is recorded
// (error if absent — that recorded SHA is the only reset target, so an unrecorded chain can never
// be rolled back to a hallucinated one), resets resetter's repo to it via
// FabricResetter.ResetHard, deletes every chain member's batch-report file from reportsDir (named
// NN-<slug>.yaml, per plan-format.md's batch-report filename contract), and resets each member's
// BatchState entry plus st.CurrentBatch.
// The caller is responsible for persisting st via SaveState afterward.
func RestartChain(resetter FabricResetter, st *State, plan *Plan, chainEnd int, reportsDir string) error {
	startSHA, ok := st.ChainStartSHAs[chainEnd]
	if !ok || startSHA == "" {
		return fmt.Errorf("builder: no chain-start SHA recorded for chain-end batch %d", chainEnd)
	}

	members := ChainMembers(plan, chainEnd)
	if len(members) == 0 {
		return fmt.Errorf("builder: batch %d names no deferred-verify chain", chainEnd)
	}

	if err := resetter.ResetHard(startSHA); err != nil {
		return err
	}

	byNumber := make(map[int]PlanBatch, len(plan.Batches))
	for _, b := range plan.Batches {
		byNumber[b.Number] = b
	}

	for _, n := range members {
		b, ok := byNumber[n]
		if !ok {
			continue
		}

		reportPath := filepath.Join(reportsDir, BatchReportFileName(b.Number, b.Slug))
		if err := os.Remove(reportPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("builder: remove stale chain report %s: %w", reportPath, err)
		}

		delete(st.Batches, n)
	}

	// Chain restarts from its lowest member; reset CurrentBatch.
	st.CurrentBatch = 0

	return nil
}
