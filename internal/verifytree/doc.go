// Package verifytree is the one function every plan-verify site calls.
//
// Verify checks that the worktree is clean, skips a run when the verified-tree record holds an entry of the same command naming HEAD's tree, and otherwise runs the command and records the pass.
// The record holds one entry per command, each with the command, HEAD's tree, HEAD's commit and the time of its pass.
// A pass replaces its own command's entry and drops every other command's entry naming a different tree, except the entry of the base command the caller names on Site.
// LatestPass returns the entry of one command, which a caller uses as a diff base.
// A record in an older format, or a malformed one, reads as no record.
// The record is written only by Verify and only after a pass, so a crash before that write leaves a mismatch and the next call runs again.
// A caller's own cancel is a returned error and no record.
// While a command runs, Verify keeps a running marker that loom status reads through ReadMarker.
//
// Verify takes no gate slot and applies no timeout: every command it runs is one `lyx gate test --wait` call, or such calls joined by `&&`, which take their own slots and are bounded by the gate's pre-tier ceiling and the tiers' timeouts.
// A failed command whose log holds the gate's tier timeout line (gateslot.FindTierTimeout) is read as timed out:
// the result is a failed one with the command's own exit code, TimedOut set and the tier and its timeout in Detail.
// The marker is written as running from the start, and a waiting gate shows through the gate's own wait record.
// A marker an older binary wrote in the waiting state, or without a state, stays readable.
// The command's environment drops the strand-name variable (agentname.StrandNameEnv), so a test that drives `lyx fabric` never meets the sandboxed-role guard.
// Bound: a test is code the fork writes, which could clear the variable itself.
// A verify command never sees gateslot.PrebuiltLyxEnv: Verify strips an inherited value, so the command's own `go test` builds lyx once per test binary.
//
// A site tells Verify a PauseHold, built by NewPauseHold over the pause record, and a nil hold runs exactly as above.
// The hold is two functions and two timing seams: PastGrace reports whether a covering live entry has passed its grace end, Wait returns once no live entry covers the scope, Poll is how often Verify asks PastGrace while the command runs, and Sleep spaces Wait's record reads.
// A Verify entered under a covering pause writes a marker in the `paused` state and starts no command until the pause is resumed, then starts over from the dirty check, so a tree dirtied during the pause is reported dirty and a record written during it skips.
// A command running when a covering entry passes its grace end gets SIGTERM on its process group and SIGKILL after the wait delay; the stopped run is neither a pass nor a failure, keeps its log, and reruns from the dirty check after the resume.
// A pause resumed before its grace end lets the running command finish and record its result.
// A held Verify removes its marker on every return, its dirty and skipped returns included.
// The hold reads an unreadable record as uncovered, so a corrupt file never stops a verify, and while the site's slug-entry exemption reports true, as it does for a run holding its parent lock, only a hub entry holds it.
//
// Each run that spawns its command writes its own log, `verify-<n>.log` in the verify directory with n one above the highest existing number, and names it on Result.Log, so a later verify never overwrites an earlier run's evidence.
// After each such run the directory keeps the newest verifyLogKeep logs plus the one the Publish failure record names, and removes the rest.
//
// The Publish failure record is the one never-tracked file Publish leaves when a verify of its `landing` or `publish` gate fails.
// It names the failing gate as a FailureKind (`landing` or `publish`) and the failing tier of that gate, the failing tests, the failing run's own log, HEAD and the merge-in commit Publish made.
// A record written before the gates carries the retired kinds `plan_verify` and `publish_verify` and no tier, and a reader maps them onto the two gates.
// Publish writes it through WritePublishFailure, the round gate reads it through ReadPublishFailure and checks its log path with IsLogPath, and a later passing Publish removes it through RemovePublishFailure, which leaves the log to pruning.
// The record sits in the verify directory beside the logs.
//
// Every site of one worktree shares one directory, Dir(anchorRoot), so a pass at one site lets the next site skip.
// The package imports no resolver: the worktree and the directory are told through Paths.
package verifytree
