// Package verifytree runs a verify command against a worktree and records the pass.
// A later site on the same tree then skips the run.
//
// Verify checks that the worktree is clean.
// It skips a run when the verified-tree record holds an entry of the same command naming HEAD's tree.
// Otherwise it runs the command and records the pass; only Verify writes the record, and only after a pass.
// A record in an older format, or a malformed one, reads as no record.
// A caller's own cancel is a returned error and no record.
// Every site of one worktree shares one directory, Dir(anchorRoot), so a pass at one site lets the next site skip.
//
// Verify takes no gate slot and applies no timeout.
// Each command is one `lym gate test --wait` call, or such calls joined by `&&`, bounded by the gate.
// A failed command whose log holds a tier timeout line is a failed result with TimedOut set.
// One whose log holds the runner crash line is a failed result with RunnerCrash set.
// One whose log holds the memory pressure line is a failed result with MemoryPressure set.
// Either judged no code, which Result.Unjudged reports.
// A site stops on an unjudged result rather than hand it to a fixer.
// The command's environment drops the strand-name variable and gateslot.PrebuiltLymEnv.
// Bound: a test is code the fork writes, which could clear the variable itself.
//
// A site may tell Verify a PauseHold, built by NewPauseHold; a nil hold runs as above.
// Under a covering pause Verify starts no command until the resume, then starts over from the dirty check.
// A command running past a covering entry's grace end is stopped by signal on its process group.
// The stopped run is neither a pass nor a failure and reruns after the resume.
// The hold reads an unreadable record as uncovered, so a corrupt file never stops a verify.
//
// DirtyPaths is the dirty check every caller shares.
// UncommittedFindings is the one finding a producing session gets when its turn ends with dirty paths.
//
// Each spawned run writes its own `verify-<n>.log` in the verify directory, named on Result.Log and never overwritten.
//
// The landing failure record is the never-tracked file `landing-failure.yaml` in the verify directory.
// Parent-Align and Finalize leave it when a verify of their own gate key fails.
// It names the failing gate, tier, tests, log, HEAD and merge-in commit.
// It adds an optional trace of the failure to the plan.
// A later passing gate at either row removes it through RemoveLandingFailure.
//
// The package imports no resolver: the worktree and the directory are told through Paths.
package verifytree
