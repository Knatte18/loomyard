// doc.go carries gitnativepoc's package-level godoc: the durable write-up for
// the git-native-library feasibility spike (recommendation, per-operation
// verdicts, evidence, and the pinned go-git version). It is the single
// package-doc comment for gitnativepoc — every other file in the package
// carries only a plain file comment, per golang-comments convention.

// Package gitnativepoc is a throwaway-but-kept feasibility-spike prototype
// evaluating whether go-git (github.com/go-git/go-git/v5) can replace
// internal/gitrepo's internal/gitexec-shelled-out CLI plumbing with a typed,
// pure-Go git implementation. It reimplements gitrepo's git surface — reads
// (CurrentSHA, SHAExists, ChangedFilesSince, SnapshotSHA, remoteName,
// hasUnpushed, isStrictDescendant) and writes (StageAndCommit,
// StageAllAndCommit, Push including the rebase-retry recovery path,
// SetSnapshotSHA) — over go-git, and read_test.go/write_test.go run each
// go-git-backed method side-by-side against the real gitrepo.Repo method (or,
// for gitrepo's unexported helpers, a direct git-fixture assertion) as a
// differential parity oracle. gitnativepoc is not wired into cmd/lyx, is not
// a registered cobra module, and does not change internal/gitrepo or
// internal/gitexec in any way — see internal/gitrepo/doc.go for the package
// this spike evaluates replacing.
//
// # Recommendation: ADOPT-PARTIAL
//
// The entire read surface, both commit methods, and SetSnapshotSHA move
// cleanly to go-git with full behavioural parity (MIGRATE). The one
// exception is gitrepo.Push's rebase-retry recovery: go-git v5.19.1 ships no
// rebase implementation at all, so a plain push is MIGRATE but the
// pull-rebase-then-retry recovery on a non-fast-forward rejection is
// CLI-BOUND. A real migration would therefore keep the CLI (or a
// hand-rolled, from-scratch rebase-replay) for that one recovery path while
// moving everything else onto go-git — hence ADOPT-PARTIAL rather than a
// clean ADOPT or a DECLINE. No operation examined here forced a DECLINE-level
// blocker (a hard failure on gates (a) typed-result or (b) behavioural
// parity affecting the operation as a whole).
//
// # Per-operation verdicts
//
// Rubric (see the plan's migrate-vs-cli-bound-rubric Shared Decision): hard
// gates are (a) typed result/error, (b) behavioural parity with git on the
// documented "must match" cases, and (c) works on Windows (Win11); failing
// any one forces CLI-BOUND. This task verified (a) and (b) on Linux only;
// every MIGRATE verdict below is therefore provisional on (c), marked
// Win11-pending, until a later pass runs the same harness on a Win11
// machine — see the windows-portable-now-verify-later Shared Decision.
//
//   - CurrentSHA — MIGRATE (a: yes, go-git's plumbing.ErrReferenceNotFound on
//     an unborn HEAD is translated to the typed ErrNoCommits sentinel; b:
//     yes, matches gitrepo.CurrentSHA on both a committed repo and an unborn
//     HEAD; c: Win11-pending). Evidence: read_test.go's
//     TestCurrentSHA_CommittedRepo, TestCurrentSHA_UnbornHEAD.
//   - SHAExists — MIGRATE (a: yes, bool-only contract, no error to type; b:
//     yes, a real SHA, a missing-but-well-formed SHA, and a non-hex string
//     all fold into the same result on both sides; c: Win11-pending).
//     Evidence: read_test.go's TestSHAExists_CommittedSHA,
//     TestSHAExists_MissingAndNonHexSHA.
//   - ChangedFilesSince — MIGRATE (a: yes, typed ErrInvalidSHA on a non-hex
//     argument; b: yes, non-ASCII paths come back verbatim with no CLI-only
//     C-quoting layer to strip, and object.DiffTree reports a rename as a
//     delete-plus-add pair matching --no-renames; c: Win11-pending).
//     Evidence: read_test.go's TestChangedFilesSince_NonASCIIPath,
//     TestChangedFilesSince_Rename, TestChangedFilesSince_NonHexSHA.
//   - SnapshotSHA — MIGRATE (a: yes, typed ErrInvalidSnapshotKey on a
//     rejected key; b: yes, a set ref, a never-set (absent) ref, and an
//     invalid key all match gitrepo.SnapshotSHA's contract, including the
//     custom refs/loomyard/snapshot/* refspec fetch; c: Win11-pending).
//     Evidence: read_test.go's TestSnapshotSHA_SetRef,
//     TestSnapshotSHA_AbsentRef, TestSnapshotSHA_InvalidKey.
//   - remoteName — MIGRATE (a: yes, plain string return, no error to type;
//     b: yes, the "origin" fallback and a configured branch.<b>.remote both
//     match; c: Win11-pending). Evidence: read_test.go's
//     TestRemoteName_OriginFallback (asserted directly against a git fixture
//     since remoteName is unexported on both sides).
//   - hasUnpushed — MIGRATE (a: yes, bool-only contract; b: yes — go-git's
//     revision parser recognizes "@{u}" syntax but ResolveRevision never
//     implements the resulting AtUpstream case (confirmed empirically: it
//     always fails, even against a real upstream-tracking clone), so this is
//     CLI-BOUND for that literal syntax, but the same behavioural result is
//     reached MIGRATE by resolving the upstream ref from branch config and
//     comparing commit reachability directly via
//     object.NewCommitPreorderIter's ignore list, which holds for a
//     diverged/rebased history too, not just a fast-forward one; c:
//     Win11-pending). Evidence: read_test.go's TestHasUnpushed (asserted
//     directly against fixture-derived expectations since hasUnpushed is
//     unexported on both sides).
//   - isStrictDescendant — MIGRATE (a: yes, bool-only contract; b: yes, an
//     ancestor, the same commit compared to itself, and an unrelated
//     orphan-branch commit all match gitrepo's merge-base --is-ancestor
//     truth table via a go-git commit walk; c: Win11-pending). Evidence:
//     read_test.go's TestIsStrictDescendant (asserted directly against
//     fixture SHAs since isStrictDescendant is unexported on both sides).
//   - StageAndCommit — MIGRATE (a: yes, plain (sha, committed, err) return;
//     b: yes — go-git's CommitOptions has no pathspec field, so
//     Worktree.Commit alone cannot produce a scoped commit; parity is
//     achieved instead by staging via Worktree.Add then swapping in a
//     synthetic index (HEAD's tree flattened, with only the listed files
//     overlaid) before calling Worktree.Commit, and restoring the real index
//     afterward, which reproduces the CLI's pathspec-scoped tree exactly,
//     including leaving an already-staged-but-out-of-scope file staged and
//     uncommitted; c: Win11-pending). Evidence: write_test.go's
//     TestStageAndCommit_EmptyFileList, TestStageAndCommit_NothingToCommit,
//     TestStageAndCommit_RealCommit,
//     TestStageAndCommit_ScopedCommitLeavesOtherStagedEntries.
//   - StageAllAndCommit — MIGRATE (a: yes; b: yes, AddOptions{All: true} plus
//     an ordinary Worktree.Commit reproduce `git add -A && git commit`
//     exactly, no synthetic index needed since the whole-index tree build is
//     what the wildcard variant wants; c: Win11-pending). Evidence:
//     write_test.go's TestStageAllAndCommit_RealCommit,
//     TestStageAllAndCommit_NothingToCommit.
//   - Push (plain push, no rejection) — MIGRATE (a: yes, though see the note
//     below — go-git's own rejection is a plain fmt.Errorf with no typed
//     sentinel, no more strongly typed than the CLI's stderr, which is
//     itself a MIGRATE data point rather than a divergence, since detection
//     still works correctly via the same string-match posture; b: yes,
//     go-git's Repository.Push computes fast-forward-ness against the live
//     remote refs fetched during the same push handshake, exactly as
//     accurate as the CLI's; c: Win11-pending).
//   - Push (rebase-retry recovery on non-fast-forward rejection) — CLI-BOUND
//     (b: no — go-git v5.19.1 ships no rebase implementation whatsoever,
//     confirmed by grepping the entire module source for any Rebase type or
//     function; there is no API this poc could call to replay a local-only
//     commit on top of newly-fetched remote history the way `git pull
//     --rebase` does, so the poc returns the documented ErrPushCLIBound
//     sentinel instead of recovering; this is a go-git feature gap, not an
//     OS-specific behaviour, so it carries no Win11-pending marker — a
//     Win11 run cannot change this verdict). This is the single most
//     important divergence the write-surface spike found, and the deciding
//     reason the overall recommendation is ADOPT-PARTIAL rather than ADOPT.
//     Evidence: write_test.go's TestPush_RebaseRetryOnNonFastForward, whose
//     comment documents the CLI oracle recovering successfully while the poc
//     side hits the identical rejection and reports CLI-BOUND.
//   - SetSnapshotSHA — MIGRATE (a: yes for the package's own validation
//     (ErrInvalidSnapshotKey, ErrInvalidSHA); go-git offers no typed
//     rejection sentinel for the push-rejection detection itself, matching
//     Push's plain-push note above — a MIGRATE data point, not a divergence,
//     since detection still works; b: yes, both the fast-forward-only first
//     attempt and the adopt-on-conflict retry (fetch-and-adopt the remote's
//     value, then re-advance and retry when the caller's sha strictly
//     descends from what was adopted) land the same final remote value as
//     gitrepo.SetSnapshotSHA; c: Win11-pending). Evidence: write_test.go's
//     TestSetSnapshotSHA_FastForwardPush, TestSetSnapshotSHA_AdoptOnConflict.
//
// # go-git version
//
// go.mod pins github.com/go-git/go-git/v5 at v5.19.1 (the maintained fork,
// module path carrying the required /v5 major-version suffix per the
// go-git-version-pin Shared Decision). Every verdict above was produced
// against this exact resolved version; a later Win11 pass should record
// whether go.mod still pins the same version before treating any
// Win11-pending verdict as settled.
//
// # Supersedes manifest/designs/git-native-library.md
//
// This write-up supersedes manifest/designs/git-native-library.md's
// "read-only subset" framing (that draft scoped the spike to rev-parse,
// diff --name-only, and ref reads only). The spike that actually ran widened
// to the full surface gitrepo uses, including the write path and the
// rebase-retry recovery specifically — the design draft's own "Known costs"
// section flagged go-git's weak rebase support as the open question, and
// this write-up is what answers it (CLI-BOUND, see above) rather than
// leaving it as a documented risk. Per the documentation lifecycle and the
// writeup-home-and-lifecycle Shared Decision, the design draft is deleted
// once this write-up lands; this doc comment is its durable replacement.
//
// # Windows
//
// All poc code and the parity harness are written OS-portable (no
// Linux-only path or permission assumptions) but were verified on Linux
// only in this task, per the windows-portable-now-verify-later Shared
// Decision. Every MIGRATE verdict above is marked Win11-pending: gate (c) is
// a hard gate for the final recommendation but has not yet been exercised.
// The one verdict that is not Win11-pending is Push's rebase-retry
// CLI-BOUND finding, since go-git's lack of any rebase implementation is not
// an OS-specific behaviour a Windows run could change.
package gitnativepoc
