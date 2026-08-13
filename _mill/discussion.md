# Discussion: gitexec: add the checked entry point and migrate the call sites

```yaml
task: 'gitexec: add the checked entry point and migrate the call sites'
slug: gitexec-checked-entry-point
status: discussing
parent: main
```

## Problem

`internal/gitexec` exposes exactly one function, `RunGit(args []string, cwd string) (stdout, stderr string, exitCode int, err error)`.
Every caller that treats a non-zero exit as a *failure* — the large majority — has to write two separate guards with two separate messages, one for `err != nil` (git could not be executed) and one for `exitCode != 0` (git ran and rejected the command).
Because the two are separate, the exit-path guard routinely renders `git exited %d` and discards the stderr git actually produced, and the exec-path guard renders a message that the returned error's own text would have supplied.
The shape produces the discard, not the authors: 34 production error messages in the tree embed a bare exit code today.

The verdict — recorded in `manifest/designs/gitexec-error-shape.md`, filed as GitHub issue #145, now closed pointing at that file — is to add a second, **must-succeed** entry point, `gitexec.Run(args []string, cwd string) (string, error)`, returning a typed `*GitError`.
`RunGit` keeps its name, its four-value shape, and its semantics: it stays permanently correct for sites where a non-zero exit is an *answer*, not a failure.
`internal/gitrepo` gains the same pair: `run` stays raw, `runChecked` is added.
The remaining raw sites are pinned by a guard test requiring a written justification comment, so a raw site becomes a deliberate, reviewed act rather than a silent default.

**Why now.** This task's `depends_on` was `fabric-corrindex-record-race`, the tail of the serialised fabric chain (`fabric-destructive-chokepoint` → `fabric-live-state-harness` → `fabric-mutation-record-envelope` → `fabric-corrindex-record-race`).
That chain has landed, so `internal/fabricengine` — which holds 57 of the 61 production call sites — is free to be rewritten.
Two further forcing functions: `internal/gitexec` is shared by every module that touches git, so each new consumer restarts the discard count from zero; and if `gitexec` is ever backed by `go-git` instead of shelling out (the retired feasibility spike in `manifest/roadmap.md`), callers consuming an `error` rather than a synthesised `(stderr, exitCode)` pair is what makes that backend swap possible at all.

## Scope

**In:**

- `internal/gitexec`: add `GitError` and `Run`; keep `RunGit` byte-for-byte unchanged; rewrite the package header comment to carry the durable rationale the deleted design doc held.
- `internal/gitrepo`: add `runChecked`; migrate 18 of 21 `r.run` call sites to it; leave 3 raw with marker comments.
- `internal/fabricengine`: migrate the 57 production `gitexec.RunGit` call sites per the classification below, including re-signaturing `wrapProbeError` and the three `destroy.go` gate executors.
- `internal/websterengine/gitwrap.go`: migrate its one site to the checked form.
- `internal/lyxcwd` and `internal/fabriccli`: migrate their one site each to the checked form with `errors.As` recovery, preserving both pinned surfaces exactly.
- `internal/fabricengine/export_test.go`: `DeleteBranchForTest` returns `deleteBranch(...)` directly, so the executor re-signature changes this test seam's own signature and every `fabricengine_test` caller of it. **This is an in-scope consequence** — "test files are exempt from the Checked-Call Invariant" is about the marker requirement, not a claim that no test file changes.
- `CONSTRAINTS.md`: add the **gitexec Checked-Call Invariant**; amend the **gitrepo Client Boundary Invariant**; cross-reference each from the other.
- New guard test `cmd/lyx/checkedcall_test.go` enforcing the new invariant.
- Repair `cmd/lyx/gitrepoboundary_test.go`, whose three assertions all break once `runChecked` exists.
- Teach `cmd/lyx/tierpurity_test.go`, `cmd/lyx/hermeticenv_test.go`, and `cmd/lyx/rawgitmutation_test.go` about the new entry point, and add the new guard file to `tierpurity_test.go`'s `allowedSpawners` (and `hermeticenv_test.go`'s equivalent, if it needs one).
- Correct the prose in `internal/gitrepo/doc.go` that the change falsifies.
- Delete `manifest/designs/gitexec-error-shape.md` and move `manifest/roadmap.md`'s entry for this task from `## Planned` to `## Done`, dropping its link to the deleted doc.

**Out:**

- Renaming, deprecating, or changing `RunGit` in any way. It is not legacy.
- Re-reviewing the wording quality of the error messages the fabric v2 crucible campaign already fixed site by site. That judgement was made per site; re-litigating it is a second campaign.
- The six paths in `internal/fabricengine/add.go` that report a fixed, wrong cause (`"cwd is not a valid git worktree"`) for failures unrelated to the cwd. A shape change does not remove a wrong cause; these keep their existing (wrong) strings and are migrated mechanically like any other site.
- Redacting or stripping credentials from `GitError.Args`. The contract is stated in godoc instead — see the `args-verbatim` decision.
- Adding a `Stdout` field to `GitError`. Stdout arrives in `Run`'s first return value in every case, which is what makes the omission coherent.
- Pinning the git subprocess locale. `internal/gitrepo/doc.go` names this as a deliberately-untaken gitexec-level decision; it stays untaken.
- Any change to `internal/gitkit`, `internal/hubforge`, or the ~50 `*_test.go` sites that use `RunGit` for fixture setup. Test files are exempt from the new invariant.

## Decisions

### the-two-shape-split

- Decision: `gitexec.Run(args []string, cwd string) (string, error)` is added beside `gitexec.RunGit`, which is unchanged. `internal/gitrepo` gains `runChecked` beside `run`. Neither raw form is deprecated.
- Rationale: the distinction maps onto one the code genuinely has — at a real class of sites a non-zero exit is a legitimate answer. Giving the short, obvious name to the form that should be reached for by default makes the path of least resistance the safe one; that is the entire mechanism. Because the raw form stays permanently correct, "incremental migration leaves the footgun loaded" is answered by the guard test rather than by a big-bang rewrite.
- Rejected: making `RunGit` itself return `*GitError` (degrades every predicate site and forces `errors.As` into code that is a clean `if exitCode == 0` today); a guard test with no shape change (a grep-shaped test cannot tell a predicate site from a failure site); "not worth it, recorded" (the discard count is what the API shape produces, and the blast radius outside fabric is 4 production sites, not a scale matching "shared infrastructure"); the name `RunGitE` (a Go-stdlib-ism this repo uses nowhere, reading as "variant of the real one"); renaming the raw form to `RunGitRaw` (best final naming, but a breaking change at every site, collapsing into the rejected breaking-signature option); scoping the verdict to `gitexec` only and filing `gitrepo` as a follow-up (two decisions about one shape, taken at different times by different sessions).

### giterror-shape

- Decision:

  ```go
  type GitError struct {
      Args     []string
      Dir      string
      ExitCode int
      Stderr   string
  }
  func (e *GitError) Error() string
  ```

  Returned as `*GitError`. `Error()` renders `git <args>: exit <code>: <trimmed stderr>`, omitting the trailing `: <stderr>` segment entirely when stderr is empty, yielding `git <args>: exit <code>`.
- Rationale: the four fields are what every merged message needs and no more.
- Rejected: a minimal `{ExitCode, Stderr}` struct (every caller then re-adds the command and directory to its own wrapper); adding a `Stdout` field (speculative — no site in the tree reads stdout on a failure path); a literal `(no stderr)` marker (explicit, but noisy in what is the common case for commands failing on exit status alone).

### arg-joining

- Decision: space-separated, `%q`-quoted only when an arg needs it — an arg containing whitespace, or an empty arg.
- Rationale: `git status --porcelain` stays readable; `git commit -m "fix the thing"` stays unambiguous and copy-pasteable.
- Rejected: quoting every arg unconditionally (noise on the common case: `git "status" "--porcelain"`); a bare join (commit messages, `--filter=` values, and paths with spaces all occur in this tree and would render ambiguously).

### args-verbatim

- Decision: `Error()` renders `Args` verbatim, with no redaction, and the godoc says so explicitly: *callers must not pass credentials in args.*
- Rationale: arg vectors reach error strings, which reach logs and the board. No path in this repo constructs a URL with embedded `userinfo` today, so this is not a live leak — but `GitError` is shared infrastructure, and the rule needs to exist before someone adds token auth.
- Rejected: implementing `userinfo` stripping (a redaction rule invites callers to rely on it, and it only covers the URL-shaped case).

### exec-failures-unwrapped

- Decision: when git cannot be executed at all (binary missing, `cwd` does not exist), `Run` returns the raw underlying error **not** wrapped in a `GitError`. `*GitError` is produced only when git ran and exited non-zero.
- Rationale: this makes `errors.As(err, &gitErr)` mean precisely "git ran and rejected this" — the distinction every predicate-recovery site depends on. A caller asking "does this branch exist?" must be able to tell "git said no" from "git never ran".
- Rejected: wrapping exec-level failures as `GitError{ExitCode: -1}` (one error type to handle, but it destroys the `errors.As` distinction and makes `-1` a sentinel every caller must know about, silently turning an infrastructure failure into a negative answer).

### stdout-on-error

- Decision: `Run` returns whatever git wrote to stdout in **every** case, including when it returns a `*GitError`. The first return value is never blanked on failure. This is stated in the function's godoc, not left to the reader.
- Rationale: it is what `RunGit` already does, so the two forms stay consistent, and it is what makes rejecting a `Stdout` field on `GitError` coherent.
- Rejected: returning `""` on error (reads as tidier and prevents a caller consuming partial output, but throws away data the process already captured and would force a `Stdout` field back onto `GitError`).

### checked-signature-drops-exit-code

- Decision: `Run` returns `(string, error)` — stdout and error only. The exit code is reachable on `*GitError.ExitCode`.
- Rationale: every exit-code comparison in `internal/fabricengine` is against zero, so on the checked form the value is dead weight in the signature — and it is exactly the redundant return that produced the `if exitCode != 0` habit.
- Rejected: `(stdout string, exitCode int, err error)` (keeps the code at hand without `errors.As`, at the cost of reintroducing the habit the change exists to break).

### gitrepo-runchecked-calls-gitexec-directly

- Decision:

  ```go
  func (r *Repo) run(args ...string) (stdout, stderr string, code int, err error) { return gitexec.RunGit(args, r.path) }
  func (r *Repo) runChecked(args ...string) (string, error)                       { return gitexec.Run(args, r.path) }
  ```

  `runChecked` is a second chokepoint beside `run`, not a wrapper around it.
- Rationale: the Client Boundary Invariant's real point is that git-CLI access is funnelled through named helpers; two named helpers satisfy that as well as one.
- Rejected: implementing `runChecked` on top of `run` (forces `gitrepo` to construct `*gitexec.GitError` itself, duplicating logic that belongs in one place and requiring `gitexec` to export a constructor); deleting `run` in favour of calling `gitexec.Run` directly at all 21 sites (removes the duplicate shape, but `run` binds `r.path`, and its removal churns the Client Boundary Invariant's pinned method list for no gain the paired form does not already deliver).

### default-merge-rule

- Decision: at a site where `err != nil` and `exitCode != 0` are separate guards with separate messages, **the exit-path message wins**. `%s`-of-stderr becomes `%w`-of-error; the exec-path message is dropped.

  ```go
  // before
  _, stderr, exitCode, err := gitexec.RunGit(a, d)
  if err != nil    { return fmt.Errorf("<exec-path message>: %w", err) }
  if exitCode != 0 { return fmt.Errorf("<exit-path message>: %s", stderr) }

  // after
  out, err := gitexec.Run(a, d)
  if err != nil { return fmt.Errorf("<exit-path message>: %w", err) }
  ```

  Sites where the exec-path message carries information the exit-path one lacks are read individually and the deviation noted in the commit.
- **This rule presumes the exit branch is a *message*. Four decisions carve out the sites where it is not, and `/mill-go` must check each site against them before applying the rule:** `destroy-executors-are-re-signatured` shape (D) (the exit branch is control flow gating a destructive fallback), `merge-rule-at-non-error-string-sinks` (the sink is a `string`, so `%w` is unavailable), `prior-call-diagnostic-exception` (the message cites an earlier call's exit code or stderr), and `sentinel-sites-keep-their-sentinel` / `pushrebasefree-is-a-sniff-plus-sentinel-hybrid` (the branch returns a sentinel whose identity `errors.Is` consumers depend on).
- Rationale: the exit-path message is the one written for the failure operators actually hit, and the returned error's own text now supplies what the exec-path message used to say. Stating the rule once stops the implementer re-deciding it at every site.
- Rejected: leaving it to per-site judgement (roughly 50 identical decisions); an AST rewrite (`gofmt -r` rewrites expressions, not two statements with divergent bodies, and a tool that merged them would be making the editorial choice silently).

### merge-rule-at-non-error-string-sinks

- Decision: at a site whose failure sink is a plain `string` field rather than a returned `error`, the merged form is `%v` of the error, and the `(git exit %d)` fragment is dropped exactly as elsewhere. `%w` is not available at these sites and must not be reached for.
- Sites: `internal/fabricengine/prune.go` (two assignments to `pe.Error`) and `internal/fabricengine/cleanup.go` (two assignments to `entry.Error`), all four built with `fmt.Sprintf`.
- Rationale: `default-merge-rule` is written in terms of `fmt.Errorf` and `%w`, and read literally it does not say what to do where neither exists. `%w` in a `fmt.Sprintf` is not a compile error — it renders as `%!w(…)` — so an implementer following the rule mechanically produces a corrupted operator-visible string rather than a build failure.
- These are report-entry fields consumed for display, not for `errors.Is`/`errors.As`, so nothing downstream needs the wrapped error's identity — which is why `%v` is sufficient rather than a signature change.
- Rejected: widening the fields to `error` so `%w` applies (a change to two report structs and their consumers, for no gain to a display-only value); leaving the fragment in place at these sites since `%d` still works in a `Sprintf` (it would be the one place in the tree still rendering a bare exit code beside a message that already carries it, and the `prune.go` fallback-failure message that legitimately cites the exit code is covered by `prior-call-diagnostic-exception` instead).

### drop-the-exit-code-fragment

- Decision: the `(git exit %d)` fragment is deleted together with its `exitCode` argument. `fmt.Errorf("warp switch to branch %q failed (git exit %d): %s", branch, exitCode, strings.TrimSpace(switchStderr))` becomes `fmt.Errorf("warp switch to branch %q failed: %w", branch, err)`.
- Rationale: `Run` deletes the binding the `%d` consumes, so leaving the fragment is both unfillable and a duplicate of what `GitError.Error()` renders. 34 production messages in the tree carry this fragment today; it is not covered by "`%s`-of-stderr becomes `%w`-of-error" and must be called out separately.
- Rejected: keeping the fragment by reading `gitErr.ExitCode` (duplicates the error's own rendering at every site).

### prior-call-diagnostic-exception

- Decision: a message that cites a **prior** call's exit code **or its stderr** is not a duplicate of the current error's own rendering, and is not dropped. Where the default rule would discard it, the prior call's `*GitError` is kept in scope and the fragment is filled from it.
- **Two live instances, and the rule covers stderr as well as exit codes — the earlier draft named only the exit-code half:**
  - `internal/gitrepo/push.go`, inside `pushWithRebaseRetry`: two messages about the **`rebase --abort`** outcome embed the **prior `pull --rebase` stderr** (`"gitrepo: git pull --rebase: %s (and rebase --abort could not run …: %v)"` and the `%s`/`%s` variant). The default rule applied to the abort call would drop `rebaseStderr` entirely, leaving an operator told only that an abort failed and nothing about the rebase that made an abort necessary. The migrated form keeps the `pull --rebase` call's own `*GitError` bound and renders `gitErr.Stderr` — or the error itself — in those two messages, exactly as today.
  - `internal/fabricengine`'s `readBranch`: **two** of its downstream messages render the earlier `rev-parse --abbrev-ref HEAD` exit code — one inside the error belonging to the later `branch --show-current` call, and one inside the no-current-branch-set error. Both must keep the earlier code. Both calls migrate to the checked form; the first call's `*GitError` is recovered with `errors.As` and its `ExitCode` cited explicitly in the combined message.
- Rationale: deleting that fragment would discard a second call's diagnostic rather than a duplicate of `GitError.Error()`. Migrating both keeps the combined diagnostic and leaves no raw site in a pure failure path.
- Rejected: keeping the first call raw with a marker citing the combined message (simpler, but leaves a raw site in a path where every exit is a failure, which the marker's own justification wording cannot honestly claim).

### sentinel-sites-keep-their-sentinel

- Decision: where an exit path returns a sentinel error rather than a message, the sentinel stays the `%w` verb and the `GitError` goes in as `%v`: `fmt.Errorf("%w: %v", ErrNotAGitRepo, err)`. A bare `return "", Sentinel` may also stay bare.
- Rationale: `%w`-wrapping the `GitError` over the top would break `errors.Is` at its consumers — `internal/loomengine/preflight.go` does `errors.Is(err, lyxcwd.ErrNotAGitRepo)`, and exact-string assertions in `internal/lyxcwd`, `internal/configcli`, `internal/reedcli`, and `internal/idecli` tests pin the bare-sentinel surface. The `%w: %v` shape is already used in this tree.
- Rejected: `%w`-wrapping the GitError (breaks `errors.Is`); introducing a joined error (`errors.Join`) here (changes the rendered string that four test suites pin).

### deliberate-suppression-stays-raw

- Decision: a site that withholds git's stderr on purpose, as a contract with a **test behind it**, stays raw with a marker comment. Two sites qualify: `internal/gitrepo`'s `Pull` and `Fetch`.
- Rationale: `internal/gitrepo/pull_test.go:87` and `:119` and `internal/gitrepo/fetch_integration_test.go:110` fail if `err.Error()` contains `"fatal:"`, and separately require the `git -C … pull --ff-only` reproduction pointer. `%w`-wrapping a `GitError` would embed the raw stderr and break them. This is the live counter-example to "every discard is an accident of the shape" — here the discard is a considered decision with a test enforcing it.
- Rejected: migrating them and updating the tests (the tests encode the contract; changing both together would make the contract unfalsifiable).

### resethard-is-not-suppression

- Decision: `internal/gitrepo`'s `ResetHard` migrates to the checked form, and the "no-stderr-leak style" clause in `internal/gitrepo/doc.go`'s ResetHard section is deleted in the same commit.
- Rationale: **the code, not the prose, is the evidence.** `reset.go` binds `_, _, code, err` and renders `git exited %d`; `reset_test.go` asserts on the resulting SHA, the restored file content, and `ErrInvalidSHA` — it never touches `err.Error()`. Nothing pins the suppression. That is precisely the difference from `Pull`/`Fetch`, whose suppression *is* pinned, and a failing hard reset on fabric's history-recovery path currently reports a bare number. `ErrInvalidSHA` is returned before any git spawn and is unaffected.
- Rejected: treating `doc.go`'s clause as binding (it is a prose claim with nothing behind it — the wrong artefact to take a design decision from); migrating but formatting `gitErr.ExitCode` instead of `%w` (a checked call whose error is then discarded is its own smell).

### rev-parse-probes-are-mixed-not-pure-predicates

- Decision: the seven `rev-parse`-style probes whose **exec** path returns a real error take the checked form with `errors.As` recovery, not the raw form: `add.go` (warp-branch existence), `boardweft.go` (local weft branch), `clone.go` (remote weft primary ref, and the unborn-branch verify in `bornWeftPrimaryBranch`), `warpprobe.go` (unborn-HEAD check), `checkout.go` (best-effort weft branch capture), `pull.go` (`weftHasUpstream`).
- Rationale: **this corrects the design doc, from the code.** The doc's classifier looked only at `exitCode == 0` and filed all eight `rev-parse` sites as pure predicates bound for the raw form. The code shows otherwise: **six of the seven** already separate `if err != nil { return …, fmt.Errorf(…) }` from `if exitCode == 0 { /* the answer */ }`. That is the mixed shape the doc itself sends to checked + `errors.As`. Taking the raw form there would require each marker comment to claim "every exit code is an answer", which is false on the exec path.
- **`checkout.go`'s weft-branch capture is the seventh and does not have that shape** — it is deliberately best-effort, written as a compound condition with no error return at all (`if out, _, code, werr := gitexec.RunGit(…); werr == nil && code == 0 {`), because a detached or unborn weft HEAD has no branch name to roll back to and `rollbackSwitch` simply skips the weft side. It migrates to `if out, err := gitexec.Run(…); err == nil {`, which collapses both conditions into one and is a plain simplification. It is listed here because it migrates to checked alongside the other six, not because it shares their two-branch shape.
- Rejected: raw per the doc (fewer sites touched, but the required justification would be untrue); deciding per site with no rule (seven independent judgement calls in `/mill-go` with nothing to deviate *from*).

### the-raw-vs-checked-discriminator

- Decision: **a site takes the raw form only when it cannot truthfully carry a marker saying so.** The single discriminator, applied before any other classification:

  > **Does the site report the exec path separately from the exit path?**
  > If **yes** — it takes the checked form, with `errors.As` recovery wherever the exit path is an answer.
  > If **no** — it may be raw, and only if it falls in one of the three raw classes below.

- Rationale: this is the rule that was implicit in `rev-parse-probes-are-mixed-not-pure-predicates` and needed stating, because without it structurally identical sites were being filed on both sides. A site that already returns `fmt.Errorf(…)` (or a distinct sentinel, or a distinct output) when git could not be executed has, by construction, an exec path that is a failure — so a `//gitexec:raw` marker claiming "every exit code here is an answer" would be false, and the invariant's whole mechanism is that the justification is *true*.
- **The three raw classes, each truthfully markable:**
  1. **No error channel in the signature** — the function returns a plain `bool` (or equivalent) and must collapse every outcome, exec failure included, into it. `internal/fabricengine/weftwiring.go`'s `isWeftWorktree` and `weftBranchExists`.
  2. **Test-pinned deliberate suppression** — non-zero *is* a failure, but folding stderr into the message would break a documented, test-enforced surface. `internal/gitrepo`'s `Pull` and `Fetch`.
  3. **The raw half of the `gitrepo` pair itself** — `run`'s own body, which is the chokepoint, not a decision about any one call.
- **This re-files three sites an earlier draft had raw.** Each separately reports its exec path, so each moves to checked + `errors.As`, and each is strictly better afterwards:

  | site | exec path today | exit path today | migrated form |
  |---|---|---|---|
  | `internal/gitrepo/push.go` `HasUnpushed` | `return false, err` | `return true, nil` | `errors.As` → `true, nil`; anything else → `false, err`. The godoc's "a spawn failure returns `(false, err)`; rev-list errors fold into `(true, nil)`" becomes *enforced by the type* rather than by the four-value shape |
  | `internal/lyxcwd/lyxcwd.go` `gitWorktreeRoot` | `fmt.Errorf("%w: %v", ErrNotAGitRepo, err)` | bare `ErrNotAGitRepo` | `errors.As` → bare `ErrNotAGitRepo`; anything else → the existing `%w: %v` form. **Both pinned surfaces survive byte-for-byte** — `internal/loomengine/preflight.go`'s `errors.Is`, and the exact-string assertions in `internal/lyxcwd`, `internal/configcli`, `internal/reedcli`, `internal/idecli` |
  | `internal/fabriccli/fabric.go` | `output.Err(out, runErr.Error())` | `output.Err(out, "usage: …")` | `errors.As` → the usage string; anything else → `output.Err(out, err.Error())` |

- Rejected: keeping the three raw and widening the marker wording to cover them (the wording would have to assert something the code contradicts, which turns the justification requirement into a formality — the exact failure mode `no-guard-test-at-all` and `pinning-without-a-justification-comment` were rejected to avoid); keeping them raw because each is "obviously a predicate at a glance" (that is the classifier error this discussion already corrected once, at the seven `rev-parse` probes).
- **Consequence for the per-package pinned counts:** `internal/gitrepo` pins **3** (`run`'s body, `Pull`, `Fetch`), `internal/fabricengine` pins **2** (the two `weftwiring.go` predicates), and `internal/lyxcwd`, `internal/fabriccli`, `internal/websterengine` pin **0** each. The `internal/gitrepo` disposition table's call-site figure becomes **2 raw, 19 checked** of the twenty-one `r.run` sites.

### bool-returning-predicates-stay-raw

- Decision: `internal/fabricengine/weftwiring.go`'s `isWeftWorktree` and `weftBranchExists` stay raw with markers reading, in substance, "bool-returning predicate: the signature has no error channel, so every outcome must collapse to a bool".
- Rationale: both return plain `bool` and already `return false` on the exec path as well as the non-zero path. They are the permanently-correct raw class the invariant exists to name.
- Rejected: migrating with `err != nil → return false` (collapses exec failure into "the branch does not exist", the exact conflation the two-shape split exists to prevent, now written at a site that has no way to report it).

### content-sniff-sites-take-the-checked-form

- Decision: sites where **stderr content**, not the exit code, decides answer-versus-failure migrate to the checked form with the sniff moved onto the recovered error: `errors.As(err, &gitErr) && strings.Contains(gitErr.Stderr, "…")`.
- Rationale: these sites get *better* under the change — the string they inspect and the diagnostic they fall through to become the same value, instead of a string that has to stay in scope alongside an exit code.
- Sites, **re-derived from the code — the design doc's query (`Contains(stderr`) found only the first two**:
  - `internal/fabricengine/index.go` — `"does not have any commits yet"` on `git log --format`, where an unborn HEAD is an empty history, not a scan failure.
  - `internal/gitrepo/push.go` — the `"no rebase in progress"` abort check.
  - `internal/gitrepo/push.go` — `containsAny(stderr, rebaseRetryTriggers)` on the push result inside `pushWithRebaseRetry`, the trigger for the whole rebase-retry recovery. Missed by the doc's grep because it goes through the `containsAny` helper rather than `strings.Contains` directly.
  - `internal/gitrepo/push.go` — **the same `containsAny(stderr, rebaseRetryTriggers)` pattern inside `PushRebaseFree`, a separate function.** See the hybrid decision immediately below; this site must not be read as a plain two-message merge.
- Rejected: leaving `pushWithRebaseRetry` raw as a unit (smaller blast radius on the one gitrepo path that recovers rather than fails, but it is four sites and two sniffs of exactly the class this decision covers).

### pushrebasefree-is-a-sniff-plus-sentinel-hybrid

- Decision: `internal/gitrepo`'s `PushRebaseFree` needs **both** the content-sniff treatment and the sentinel treatment, and neither decision read alone says so. Its migrated shape:

  ```go
  _, err := r.runChecked("-c", "push.autoSetupRemote=true", "push")
  if err == nil { return nil }
  var gitErr *gitexec.GitError
  if errors.As(err, &gitErr) && containsAny(gitErr.Stderr, rebaseRetryTriggers) {
      return ErrPushRejected           // bare, unwrapped — no %w, no %v
  }
  return fmt.Errorf("gitrepo: git push: %w", err)
  ```

- Rationale: applying the `default-merge-rule` boilerplate here — `if err != nil { return fmt.Errorf(…) }` — would drop the `containsAny` sniff entirely and return a generic wrapped error on every divergence. That breaks `errors.Is(err, gitrepo.ErrPushRejected)` at `internal/fabricengine/coalesce.go`, which is production consumption, not a test convenience, and fails `internal/gitrepo/push_test.go`'s `TestPushRebaseFree_DivergedRemote_ReturnsErrPushRejected`.
- The sentinel is returned **bare**, not as `fmt.Errorf("%w: %v", ErrPushRejected, err)`. Unlike the `lyxcwd` shape the `sentinel-sites-keep-their-sentinel` decision documents, the current code embeds no stderr in this path at all, and widening it here would be an unasked-for behaviour change to a surface `coalesce.go` reads.
- Rejected: treating it as a plain checked call per its row in the disposition table (the shorthand that made this gap possible — the row is now annotated); wrapping the sentinel with `%w: %v` for consistency with `lyxcwd` (changes a sentinel surface nothing asked to change).

### mixed-tri-states-take-the-checked-form

- Decision: sites where some exit codes are answers and the rest are failures take the checked form with `errors.As` recovery:

  ```go
  _, err := r.runChecked("merge-base", "--is-ancestor", sha, ref)
  if err == nil { return true, nil }
  var gitErr *gitexec.GitError
  if errors.As(err, &gitErr) && gitErr.ExitCode == 1 { return false, nil }
  return false, fmt.Errorf("gitrepo: merge-base --is-ancestor %s %s in %s: %w", sha, ref, r.path, err)
  ```

- Rationale: strictly better than today — the answer codes still answer, and the `default:` branch gains the stderr it currently throws away. This is exactly the bare-exit-code class the change exists to close.
- **Answer codes are inverted between the two `diff --cached --quiet` sites; transcribe the recovery per site, not once:**

  | site | exit 0 | exit 1 | default |
  |---|---|---|---|
  | `gitrepo.go` `CommitEmpty` | falls through, proceeds to commit | `return "", ErrIndexNotEmpty` | error (already binds stderr) |
  | `gitrepo.go` `StageAllAndCommit` | `return "", false, nil` — nothing to commit | falls through, proceeds to commit | error (already binds stderr) |

  `StageAndCommit`'s own `diff --cached --quiet` gate carries a third variant of the same shape and is transcribed the same way.
- Rejected: folding mixed tri-states into "raw, permanently correct" (an earlier draft did this, and it silently preserved a bare-exit-code failure path inside a site labelled as needing no diagnostic); deferring them as debt to sweep later (they are not debt, and sweeping them would be a regression).

### destroy-executors-are-re-signatured

- Decision: `internal/fabricengine/destroy.go`'s three git-spawning gate executors — `removeGitWorktree`, `deleteBranch`, `createGitWorktree` — drop their `(exitCode int, stderr string, err error)` return shape in favour of `error` (with `createGitWorktree` keeping its `createdToken` first return). Their recording predicate `if err == nil && exitCode == 0` becomes `if err == nil`. There are **nine** production call sites, and they do **not** all merge — see the shape table below; only shapes (A) and (B) take the default merge rule. The re-signature also changes `export_test.go`'s `DeleteBranchForTest` seam and its `fabricengine_test` callers.
- Rationale: **this class did not exist when the design doc was written** — the destructive-chokepoint slice landed it afterwards. The executors deliberately propagate git's exit code and stderr so call sites build their own messages, which is the two-message split one level down from the call site. It is the same situation as `wrapProbeError`, and takes the same treatment. Leaving it would put the habit the change exists to break inside the chokepoint every destructive primitive now routes through.
- Rejected: keeping the 3-tuple and re-deriving `exitCode`/`stderr` from `*GitError` inside each executor (zero call-site churn, but reintroduces the exact shape one level down); leaving the three raw with markers (their failure paths are genuine failures, so this is the "raw becomes legacy debt" outcome the verdict rejects).
- Note for `/mill-plan`: the gate's own pipeline errors (`checkPathRequest` / `checkBranchRequest`, which return `*destructiveRefusal`) are returned **before** any git spawn and are unaffected. `errors.As(err, &refusal)` at call sites such as `rollbackSwitch` keeps working unchanged, because a refusal is still not a `*GitError`.

**The nine executor call sites are NOT all plain merges. `default-merge-rule` does not apply to two of them, and applying it there would be a destructive-gate bypass.** The four shapes, classified from the code:

| shape | sites | treatment |
|---|---|---|
| **(D) exit branch is control flow, not a message** | `remove.go` (`removeGitWorktree`), `prune.go` (`removeGitWorktree`) | **`errors.As`, never a merge** — see below |
| (A) already unified as `err != nil \|\| exitCode != 0` | `weftwiring.go` ×2, `add.go` rollback ×2 (each behind `surfaceRefusal`) | collapses to `if err != nil`; the synthesised `fmt.Errorf("… failed with exit code %d", exitCode)` fallback disappears, since the branch now always has a real error to carry |
| (B) plain two-message merge | `add.go` (`createGitWorktree`) | `default-merge-rule` as written |
| (C) non-error string sink | `cleanup.go` (`deleteBranch` → `entry.Error`) | see `merge-rule-at-non-error-string-sinks` |
| — | `checkout.go` (`deleteBranch` in `rollbackSwitch`) | already `if _, _, err := …; err != nil` with an `errors.As(&refusal)` handler; becomes `if err := …; err != nil`, otherwise unchanged |

**Shape (D), stated as the rule:** at these two sites the old `exitCode != 0` branch is not a second message — it re-probes `isRegisteredLinkedWorktree` / `isRegisteredLinkedWorktreeIn` and, when the worktree is *still registered*, performs a **fallback destructive `removePath`** and returns success. The old `err != nil` branch bails without destroying anything. The migrated form must preserve that split exactly:

```go
if err := removeGitWorktree(rec, req, dir); err != nil {
    var gitErr *gitexec.GitError
    if !errors.As(err, &gitErr) {
        // git never ran, or the gate refused before it could: bail, destroy nothing.
        // (remove.go's existing *destructiveRefusal branch stays ahead of this, unchanged.)
        return fmt.Errorf("run git worktree remove for %s: %w", target, err)
    }
    // git ran and refused — the fallback path, exactly as today.
    if !isRegisteredLinkedWorktree(l, target) { … }
    …removePath(rec, fallbackReq)…
}
```

- Rationale: collapsing the two branches under the default rule has one of two outcomes, both wrong. Either the fallback removal is dropped, so `lyx fabric remove` and `prune` stop cleaning up a worktree git itself declined to remove; or the fallback runs on the exec-failure path too, and then an exec-level failure — or a `*destructiveRefusal` the gate raised *before git ran* — reaches a destructive primitive. The second is a Fabric Destruction Chokepoint Invariant violation reached by a message-merging rule, which is exactly the kind of silent widening the invariant exists to prevent.
- `errors.As(err, &gitErr)` is the correct discriminator here precisely because of the `exec-failures-unwrapped` decision: it means "git ran and rejected this" and nothing else. This is the load-bearing consumer of that decision outside the predicate sites.
- Rejected: keeping `removeGitWorktree` raw at these two sites so `exitCode` stays available (preserves the branch trivially, but leaves the destructive chokepoint on the raw form, which is where a bare-exit-code failure path is least acceptable); having the executor return a distinguished sentinel for "git refused" (a second discriminator beside `*GitError` doing the same job).

### error-constructing-helpers-are-re-signatured

- Decision: `internal/fabricengine/warpprobe.go`'s `wrapProbeError(weftURL, op, stderr string, cause error) error` becomes `wrapProbeError(weftURL, op string, cause error) error`, and its internal stderr-vs-cause selection branch is deleted. Its seven call paths (four exec-path with an empty stderr and an error, three exit-path with stderr and a nil error) collapse pairwise into one call each.
- Rationale: the helper's signature encodes the two-value split the change removes, and `GitError.Error()` already renders the stderr it was choosing between.
- Rejected: feeding the old helper `err.Error()` as the `stderr` argument (keeps a parameter whose reason for existing is gone, and stringifies an error callers may want to `errors.As`).
- **`/mill-go` must check every error-constructing helper the merge touches for the same split.** As of this discussion the two known instances are `wrapProbeError` and the three `destroy.go` executors; assume there is another the classifier did not account for.

### deliberate-discards-migrate-as-discards

- Decision: the four full-discard sites — two bare `gitexec.RunGit([]string{"worktree", "prune"}, …)` calls in `prune.go` carrying `//nolint:errcheck`, plus `_, _, _, _ =` forms in `reconcile.go` and `remove.go` — migrate to `_, _ = gitexec.Run(…)`. The `//nolint:errcheck` comments are deleted and each site gains its own comment stating why discarding is correct here.
- Rationale: all four are best-effort `worktree prune` bookkeeping; `remove.go`'s own comment states it must not turn a completed removal into an error. `//nolint:errcheck` enforces nothing — this repo has no `golangci-lint`, so the comment is documentation either way, and the guard test is the mechanism for making a deliberate discard visible.
- Rejected: marking them `//gitexec:raw` (they are not raw sites — they use the checked form and discard its error, so the marker would be a lie and would inflate the raw-site counts); keeping `//nolint:errcheck` beside the checked form (retains a comment that does nothing).
- Note: the design doc described "seven full-discard sites" including three `checkout.go` rollback discards. **Re-derived from the code: those no longer exist in that form.** `rollbackSwitch` now binds `warpExitCode`/`warpErr` and `weftExitCode`/`weftErr` to gate `rec.Append` (so they are not discards), and its `branch -D` already routes through `deleteBranch` with an `errors.As(&refusal)` handler that logs via `logger.Warn`. Under the executor re-signature above, `rollbackSwitch`'s deletion call becomes `if err := deleteBranch(rec, req); err != nil { … }`; its two switch calls become `if _, err := gitexec.Run(…); err == nil { rec.Append(…) }`.

### the-checked-call-invariant

- Decision: add a **gitexec Checked-Call Invariant** to `CONSTRAINTS.md`, enforced by a new `cmd/lyx/checkedcall_test.go`.
  - Every remaining raw `gitexec.RunGit` / `r.run` call site in non-test source carries an adjacent marker comment `//gitexec:raw — <why the raw form is correct here>`.
  - The guard asserts the marker is present at every raw site, and separately pins a **per-package count** of raw sites in a literal map as a drift tripwire.
  - Test files are exempt.
- **`gitrepo.run`'s own body is a marked raw site.** The invariant's wording — "every remaining raw `gitexec.RunGit` … call site" — covers the `gitexec.RunGit(args, r.path)` inside `run`'s body literally, and it is left that way deliberately: no carve-out in the guard. Its marker reads, in substance, `//gitexec:raw — the raw half of the gitrepo checked/raw pair, by design; each of its callers carries its own justification`. The three pinned `internal/gitrepo` raw sites are therefore `run`'s body, `Pull`, and `Fetch`; the "2 raw" figure in the disposition table counts `r.run(` **call sites among the twenty-one**, which is a different unit from the pinned count and is labelled as such.
- **The full per-package pinned map, derived from `the-raw-vs-checked-discriminator`:** `internal/gitrepo` 3, `internal/fabricengine` 2, `internal/lyxcwd` 0, `internal/fabriccli` 0, `internal/websterengine` 0. A package with a pinned zero is listed explicitly rather than omitted, so that "this package has no raw sites, deliberately" is a statement the map makes rather than an absence a reader has to infer.
  - Rejected: exempting the `run`/`runChecked` helper bodies as mere declarations and pinning 3 (keeps every marker a genuine per-call-site decision, but buys that with a special case inside the guard, and special cases inside guards are how guards rot); marking `run`'s body but not counting it (keeps both properties separately, but then the count measures a different set from the markers and the two can diverge unseen).
- Rationale: the justification is "why the raw form is correct here", not the narrower "why non-zero is not a failure" — the narrower wording is unfillable at the deliberate-suppression sites, where non-zero *is* a failure whose stderr is deliberately withheld. The wording must cover both raw classes: **pure predicate** (every exit code is an answer) and **pinned deliberate-suppression contract** (non-zero is a failure, but folding stderr in would break a documented, test-enforced surface). Keying on the marker makes the justification requirement *be* the enforcement rather than a convention standing beside it; the per-package count keeps "a new raw site appeared" a visible diff.
- Rejected: a file:line list or an enclosing-function list (location keys rot on every unrelated edit above the line; function keys churn on renames while still not enforcing the justification); the marker with no count (a new raw site becomes invisible so long as it carries a comment); one global total instead of per-package (movement between packages becomes invisible); requiring markers in test files (~50 fixture-setup sites where exit status is legitimately irrelevant — ceremony with no design weight; the three token guards cover the test side).
- The same `//gitexec:raw` token is used at both `gitexec.RunGit` and `r.run(` sites — one invariant, one searchable token. A separate `//gitrepo:raw` token was rejected as splitting the invariant into two vocabularies.
- **The `gitrepo` raw-call token is pinned as `r.run(`, parenthesis included, and the paren is load-bearing.** `r.runChecked(` contains the substring `r.run`, so an unparenthesised token would demand a `//gitexec:raw` marker at all nineteen *migrated* sites — the precise inverse of the invariant. This is the opposite choice from `gitexec.Run`, where the paren is deliberately omitted so the shorter prefix covers both `Run(` and `RunGit(` in one token. The two are not inconsistent: in the `gitexec` case the prefix's extra matches are exactly the sites the guard wants, and in the `gitrepo` case they are exactly the sites it must not flag. `/mill-go` must state this reasoning in the guard's header comment, since a later reader will otherwise "fix" one of the two spellings to match the other.
- Guard location and scope: `cmd/lyx/checkedcall_test.go`, walking all non-test `.go` files under **`internal/` and `cmd/`**, matching where the other cross-package token guards live. Rejected: a `gitrepo`-local guard plus a `cmd/lyx` one (splits one invariant across two files); an explicit package list (a second thing to keep current); walking `internal/` alone (harmless today — zero production sites live under `cmd/` — but a future raw call in `cmd/` would escape the marker requirement silently, and widening the walk costs nothing now while a later widening would be a change nobody is prompted to make).
- **The guard is its own scan data, so it must be allowlisted by the guards it teaches, in the same commit.** `cmd/lyx/checkedcall_test.go` is untagged and will contain the literal `gitexec.RunGit` token as scan data plus `exec.Command` for its `go env GOMOD` root resolution — exactly the shape every sibling guard already carries. It needs an `allowedSpawners` entry in `cmd/lyx/tierpurity_test.go` with a one-line reason, alongside the existing entries for `gitrepoboundary_test.go`, `rawgitmutation_test.go`, `destructiveguard_test.go`, `ghguard_test.go`, and `boardguard_test.go`. `/mill-go` must check `cmd/lyx/hermeticenv_test.go`'s own allowlist for the same requirement rather than assuming only `tierpurity_test.go` needs it.
- **Known blind spot, to be stated honestly in both the test's header comment and the `CONSTRAINTS.md` entry:** a raw call slipped into an already-marked region, or an alternative spelling the substring match misses. This is the same class of blind spot `cmd/lyx/destructiveguard_test.go` and `cmd/lyx/gitrepoboundary_test.go` already name; issue #135's shared static-analysis framework would close it repo-wide, and this invariant does not resolve that question.

### client-boundary-guard-keys-on-both-helpers

- Decision: `TestGitrepoBoundary_PinnedRunCallSites` keys on `r.run(` **OR** `r.runChecked(` as one combined set, and `gitrepoPinnedRunBoundMethods` keeps all twelve current entries. The `gitexec.` occurrence assertion becomes an AST call-expression count of exactly two — one inside `run`, one inside `runChecked` — replacing today's whole-file substring count (see the clause below).
- Rationale: after migration only `Pull`, `Fetch`, and `HasUnpushed` still call `r.run`. A guard keyed on `r.run` alone would go blind to CLI access from the other nine methods — which is the invariant's entire purpose. The invariant answers "which `gitrepo` methods reach the git CLI at all", and both helpers reach it.
- **The `gitexec.` occurrence assertion must stop being a substring count.** Today it is `gitexecTotal += strings.Count(rendered.String(), "gitexec.")` over the whole rendered file, and `!= 1` fails. After migration roughly six methods (`StageAndCommit`, `CommitEmpty`, `StageAllAndCommit`, `IsAncestor`, `pushWithRebaseRetry`, `PushRebaseFree`) each declare `var gitErr *gitexec.GitError` for their `errors.As` recovery, so the substring count lands near eight — **`!= 2` would be just as false as `!= 1`.** The replacement assertion counts `gitexec.Run` / `gitexec.RunGit` **call expressions** via the AST walk the test already performs (`ast.Inspect` for a `*ast.CallExpr` whose `Fun` is a `*ast.SelectorExpr` on package ident `gitexec`), and asserts exactly two, one inside `run`'s body and one inside `runChecked`'s. Type references to `*gitexec.GitError` are then correctly invisible to it.
  - Rejected: declaring a package-local alias for the error type so the substring count stays usable (hides the type's provenance at every recovery site to keep a fragile assertion working); raising the substring threshold to a hand-counted number (it would drift on every new `errors.As` site, which is exactly the churn the AST form removes).
- Rejected: two separate pinned sets, `runBound` and `runCheckedBound` (also records which form each method uses, but that makes the Client Boundary Invariant encode raw-vs-checked, which is the *other* invariant's job, and lets the two diverge); keeping the `r.run` key and letting the Checked-Call Invariant cover `runChecked` sites (that invariant is keyed on call sites and marker comments, not method names, so neither would answer the question any more).
- **Composition, to be written into both `CONSTRAINTS.md` entries as a one-line cross-reference each way:** the Client Boundary Invariant answers *which methods may reach the git CLI at all* and is keyed by **method name** — update it when a method gains or loses a CLI call. The Checked-Call Invariant answers *which call sites may use the raw form* and is keyed by **call site** — update it only when a site moves between raw and checked. A new CLI call inside an already-pinned method trips the second and not the first; a new method reaching the CLI trips both.

### token-guards-key-on-the-shorter-prefix

- Decision: in all three token guards, the entry `"gitexec.RunGit"` (`"gitexec.RunGit("` in `rawgitmutation_test.go`) is replaced by the shorter prefix `"gitexec.Run"`.
- Rationale: all three match by **raw substring**, and their own header comments already justify prefix matching in exactly these terms (`tierpurity_test.go`: *"Matching is deliberately raw-substring, not whole-token or AST: exec.Command also matches exec.CommandContext"*). One token covers both entry points, so no set can go half-updated later.
- Affected: `cmd/lyx/tierpurity_test.go` (`bannedTokens`, Test Tier Purity Invariant — without it an untagged test can spawn git through the new entry point); `cmd/lyx/hermeticenv_test.go` (`gitSpawnTokens`, Hermetic Git Test Environment Invariant — without it a non-hermetic package escapes the check); `cmd/lyx/rawgitmutation_test.go` (`rawGitMutationBannedTokens`, Fabric Git Invariant raw-git-mutation guard — without it a fabric-bypassing raw call goes undetected).
- `rawgitmutation_test.go`'s allowlist entry for `internal/websterengine/gitwrap.go` names `gitexec.RunGit` in its reason string and must be reworded, since that file migrates to `gitexec.Run` and remains allowlisted.
- Rejected: adding `"gitexec.Run("` as a second entry in each of the three sets (more explicit at a glance, three sets to keep in sync forever).

### prose-corrections

- Decision: correct, in the same commit, the prose the change falsifies:
  - `internal/gitrepo/doc.go`: *"internal/gitexec is deliberately minimal: **one function**, RunGit(args []string, cwd string) (stdout, stderr string, exitCode int, err error)"* — now two functions; the paragraph must describe the pair and which side each `gitrepo` method sits on.
  - `internal/gitrepo/doc.go`: *"it has roughly **eighty** call-sites across packages"* — the code has 61 production sites plus 21 behind `r.run`. Re-derive the figure at implementation time rather than transcribing this one.
  - `internal/gitrepo/doc.go`: the ResetHard *"no-stderr-leak style"* clause — deleted per the `resethard-is-not-suppression` decision.
  - `internal/gitrepo/doc.go`: the method list in the two-backend-boundary section, which enumerates the CLI-bound methods.
  - `CONSTRAINTS.md`'s gitrepo Client Boundary Invariant: the method list, the "exactly one `gitexec.` occurrence" claim, and the new cross-reference.
- Rationale: the doc comments are load-bearing for the next reader, and a stale one is worse than none. `internal/fabricengine/doc.go` must also be checked for any statement of the old shape.
- **Working rule for `/mill-go`, carried from this discussion: do not take a design decision from a doc file. Read the code, and read the tests that pin it.** That rule is what produced two of the corrections above — the doc doc claimed ResetHard suppressed stderr deliberately, and the code showed nothing enforced it.

### package-header-carries-the-durable-rationale

- Decision: `manifest/designs/gitexec-error-shape.md` is deleted and `manifest/roadmap.md`'s entry for this task **moves from `## Planned` to `## Done`**, with its `See [designs/gitexec-error-shape.md](…)` line removed, **in the same commit**, per the Documentation Lifecycle rule and CLAUDE.md's "`manifest/roadmap.md` moves only on completing or adding a planned item".
- The roadmap entry is not deleted: this is a completed planned item, which is exactly the case the roadmap's `## Done` section exists for. Its Done wording states what shipped rather than what was decided — the two entry points and which is correct where, `gitrepo`'s matching pair, and the Checked-Call Invariant as the mechanism that keeps raw sites deliberate — and replaces the design-doc link with a pointer to `internal/gitexec`'s package documentation, since that is where the durable rationale now lives. It must also drop the entry's stale "roughly 70 call sites" figure rather than carry it into Done.
- Rejected: deleting the entry outright (loses the record that a planned item completed, which is what the Done section is for); leaving it under Planned with the link removed (a shipped item sitting under Planned is the drift the roadmap rule exists to prevent). `internal/gitexec`'s package header comment inherits a tight version: the two-shape contract and when to reach for each, why exec-level failures stay unwrapped, stdout-on-error, args-rendered-verbatim with the no-credentials rule, and a pointer to the Checked-Call Invariant.
- Rationale: those are the decisions a future caller must not re-litigate. The rejected-alternatives list dies with the design doc, which is what the Documentation Lifecycle intends.
- Rejected: porting the rejected-alternatives list into the header (protects against re-litigation, but grows a leaf package's header into a design essay).
- **Markdown Link Integrity fails on a dangling relative link, so the deletion and the roadmap edit cannot be split across commits.**

## Technical context

### Re-derived inventory — the design doc's figures are stale

The design doc's site inventories were measured on 2026-08-10/11 against `main` at `c52faee4`, before the serialised fabric chain landed. The doc's own stated acceptance bar is that the section be **re-derivable, not executable**. Figures below were re-derived for this discussion; `/mill-go` re-derives once more at implementation time.

| measure | design doc | re-derived now |
|---|---|---|
| production `gitexec.RunGit` sites | 74 | **61** |
| — `internal/fabricengine` | 70 | **57** |
| — `internal/gitrepo` / `websterengine` / `lyxcwd` / `fabriccli` | 1 each | 1 each (unchanged) |
| `r.run` sites in `internal/gitrepo` | 21 | 21 (unchanged) |
| full-discard sites | 7 | **4** |
| content-sniff sites | 2 | **4** |
| `wrapProbeError` call paths | 7 | 7 (unchanged) |
| messages embedding a bare exit code | ~30 | **34** |

New since the doc was written: `internal/fabricengine/destroy.go` holds three git-spawning gate executors that did not exist then.

### Regeneration queries (the durable half)

- All production sites: `grep -rn 'gitexec\.RunGit(' --include='*.go' internal/ cmd/ | grep -v _test`
- `gitrepo` fan-out: `grep -rn 'r\.run(' --include='*.go' internal/gitrepo | grep -v _test`
- Full discards: `grep -rn "_, _, _, _ *= *gitexec.RunGit\|^\s*gitexec.RunGit(" --include="*.go" internal/ | grep -v _test`
- Exit-code fragments: `grep -rn 'git exit %d\|exited %d' --include='*.go' internal/ | grep -v _test`
- Content sniffs: `grep -rn 'Contains(stderr\|Contains(.*[Ss]tderr\|containsAny(' --include='*.go' internal/ | grep -v _test` — the `containsAny(` alternative is load-bearing: without it **both** `pushWithRebaseRetry`'s and `PushRebaseFree`'s trigger sniffs are missed, and the second of those guards a production-consumed sentinel.
- Sentinels that must survive the migration: `grep -rn 'errors\.Is(' --include='*.go' internal/ | grep -v _test` — run this before merging any message, to find every `errors.Is` consumer whose sentinel a `%w`-wrapped `GitError` would displace.
- Error-constructing helpers: `grep -rn 'stderr string' --include='*.go' internal/fabricengine internal/gitrepo | grep -v _test` — the query that surfaces both `wrapProbeError` and the `destroy.go` executors.
- Two-message merge sites: for each `gitexec.RunGit(` in a non-test `fabricengine` file, inspect the window to the next call or function end and count it when the window contains **both** an `err != nil` guard and an exit-code comparison. A coarse fixed-line window over-counts; respect block boundaries.

### `internal/gitrepo` — the full 21-site disposition, read from the code

**2 raw, 19 checked** — counting the twenty-one `r.run` **call sites**. The Checked-Call Invariant's per-package pinned count for `internal/gitrepo` is **3**, a different unit: it adds `run`'s own `gitexec.RunGit` body, which is not one of the twenty-one. See `the-checked-call-invariant` and `the-raw-vs-checked-discriminator`.

| method | sites | disposition |
|---|---|---|
| `Pull` (`pull.go`), `Fetch` (`pull.go`) | 2 | **raw** — suppression pinned by `pull_test.go` and `fetch_integration_test.go` (raw class 2) |
| `HasUnpushed` (`push.go`) | 1 | **checked + `errors.As`** — `errors.As` → `(true, nil)`, anything else → `(false, err)`. Re-filed from raw in review r3: it reports its exec path separately, so it fails `the-raw-vs-checked-discriminator` |
| `StageAndCommit` (`gitrepo.go`) | 3 | checked; the `diff --cached --quiet` gate is a tri-state → `errors.As` |
| `CommitEmpty` (`gitrepo.go`) | 3 | `diff --cached --quiet` mixed tri-state → checked + `errors.As` (exit 1 → `ErrIndexNotEmpty`); `ls-files --cached` and `commit --allow-empty` plain checked |
| `StageAllAndCommit` (`gitrepo.go`) | 3 | `diff --cached --quiet` mixed tri-state, **answer codes inverted** (exit 0 → `("", false, nil)`) → checked + `errors.As`; `add -A` and `commit` plain checked |
| `CheckoutDetached`, `RestoreBranch` (`gitrepo.go`) | 2 | checked |
| `pushWithRebaseRetry` (`push.go`) | 4 | checked + `errors.As`; both stderr sniffs move onto `gitErr.Stderr` |
| `PushRebaseFree` (`push.go`) | 1 | checked + `errors.As` + **stderr sniff + bare sentinel** — see `pushrebasefree-is-a-sniff-plus-sentinel-hybrid`. **Not** a plain two-message merge: applying the default rule here breaks `errors.Is(err, ErrPushRejected)` at `fabricengine/coalesce.go` |
| `IsAncestor` (`ancestry.go`) | 1 | checked + `errors.As` (exit 1 → `false, nil`) |
| `ResetHard` (`reset.go`) | 1 | checked |

### The four sites outside `internal/fabricengine`

| site | disposition | why |
|---|---|---|
| `internal/gitrepo/gitrepo.go` — the `run` helper | **both** | gains the `runChecked` sibling; see the table above for the fan-out |
| `internal/lyxcwd/lyxcwd.go` — `rev-parse --show-toplevel` | **checked + `errors.As`** | the exit path returns the bare `ErrNotAGitRepo` sentinel and the exec path returns `%w: %v` — two separately reported paths, so `the-raw-vs-checked-discriminator` sends it checked. `errors.As` reproduces both surfaces exactly, preserving `preflight.go`'s `errors.Is` and the four CLI suites' exact-string assertions. **No marker.** |
| `internal/fabriccli/fabric.go` — `branch --show-current` | **checked + `errors.As`** | the exit path prints a usage string, the exec path prints `runErr.Error()` — again two separately reported paths. `errors.As` → the usage string; anything else → the error's own text. **No marker.** |
| `internal/websterengine/gitwrap.go` — `status --porcelain` | **checked** | both branches are genuine failures with real messages: a clean two-message merge. The `rawgitmutation_test.go` allowlist reason for this file must be reworded in the same commit. |

### `internal/fabricengine` — the classes, by shape

- **Two-message merge (the dominant class).** Both branches carry a message; the exit-path message wins under the default merge rule. Present throughout `boardweft.go`, `clone.go`, `checkout.go`, `weftwiring.go`, `add.go`, `cleanup.go`, `reconcile.go`, `status.go`, `hook.go`, `worktreelist.go`, `gitexclude.go`, `weftgit.go`, `dirtiness.go`, `index.go`, `pull.go`, `prune.go`, `remove.go`.
- **Mixed `rev-parse` probes** — checked + `errors.As`, per `rev-parse-probes-are-mixed-not-pure-predicates`.
- **Bool-returning pure predicates** — raw with markers, per `bool-returning-predicates-stay-raw`.
- **Content sniff** — `index.go`'s unborn-HEAD check.
- **Prior-call diagnostic composed into a later call's message** — `reconcile.go`'s `readBranch` (prior exit code, two messages). The sibling instance lives in `internal/gitrepo/push.go` (prior stderr); see `prior-call-diagnostic-exception`.
- **Error-constructing helper** — `warpprobe.go`'s `wrapProbeError`, 7 call paths collapsing to 4.
- **Gate executors** — `destroy.go`'s `removeGitWorktree`, `deleteBranch`, `createGitWorktree`, with **9** production call sites in `weftwiring.go` (×2), `add.go` (×3), `cleanup.go`, `checkout.go`, `remove.go`, `prune.go`, plus the `export_test.go` seam. They split into four shapes; two of them must not take the default merge rule.
- **Best-effort discards** — the four `worktree prune` calls.
- **Hand-read exception (migrated mechanically, strings unchanged)** — the six `add.go` paths reporting `"cwd is not a valid git worktree"` for failures unrelated to the cwd.

### Guard tests that must change

| file | what breaks / what changes |
|---|---|
| `cmd/lyx/gitrepoboundary_test.go` | all three assertions — see `client-boundary-guard-keys-on-both-helpers` |
| `cmd/lyx/tierpurity_test.go` | `bannedTokens` → `"gitexec.Run"`; **new `allowedSpawners` entry for `cmd/lyx/checkedcall_test.go`** |
| `cmd/lyx/hermeticenv_test.go` | `gitSpawnTokens` → `"gitexec.Run"`; check whether its own allowlist needs the new guard file too |
| `cmd/lyx/rawgitmutation_test.go` | `rawGitMutationBannedTokens` → `"gitexec.Run"`; allowlist reason for `gitwrap.go` reworded |
| `cmd/lyx/checkedcall_test.go` | **new** — the Checked-Call Invariant guard; walks `internal/` **and** `cmd/` |

Existing patterns to mirror, both already in the repo, so this needs no new machinery: `cmd/lyx/gitrepoboundary_test.go` (AST walk, pinned set, vacuous-scan floor) and `internal/gitrepo/noforceadd_test.go` (untagged substring scan, vacuous-scan floor). The new guard should carry a `checkedCallMinScannedFiles`-style vacuous-scan floor for the same reason both of those do.

`cmd/lyx/destructiveguard_test.go` bans `"branch", "-D"` and `"worktree", "remove"` outside `destroy.go`; the executor re-signature keeps those argument slices inside `destroy.go`, so that guard is unaffected — but `/mill-go` must confirm it, not assume it.

## Constraints

From `CONSTRAINTS.md`, in force for this task:

- **Fabric Destruction Chokepoint Invariant** — `destroy.go` is the only file permitted to perform a destructive primitive; the banned bypass tokens include `"worktree", "remove"`, `"branch", "-D"`, and `createdToken{`. The executor re-signature must not move any banned argument slice out of `destroy.go`. The clause *"a gate refusal is never discarded on a best-effort path"* must survive the migration — `rollbackSwitch`'s `logger.Warn` path and every `surfaceRefusal` call site.
- **Mutation Record Invariant** — every destructive executor takes `rec *Mutations` and appends **after** the primitive observably changed state, never on a no-op or a refusal. The recording predicate changes from `err == nil && exitCode == 0` to `err == nil`; it must not become unconditional.
- **gitrepo Client Boundary Invariant** — amended by this task; see the decision above.
- **Never Force-Add Invariant** — `internal/gitrepo/noforceadd_test.go` bans the literal `"-f"` in `internal/gitrepo` non-test source. Any migrated call must not introduce that token.
- **Test Tier Purity Invariant** and **Hermetic Git Test Environment Invariant** — new tests that spawn git must be tagged and must run under `gitkit.HermeticGitEnv()`; the new `GitError.Error()` rendering test spawns nothing and stays untagged Tier 1.
- **Markdown Link Integrity** — the design-doc deletion and the roadmap-link removal are one commit.
- **Documentation Lifecycle** — governs the design doc's deletion and the migration of its durable rationale into the package header.
- **CLI / Cobra Invariant** — untouched: no command surface changes.
- **Cwd Resolution Invariant** — untouched: `Run` takes an explicit `cwd` exactly as `RunGit` does, and resolves nothing.

Discovered during discussion:

- `internal/gitexec` is a **zero-dependency leaf** (`bytes`, `os/exec`, `internal/proc`). `GitError` must not pull in anything new; `Error()` needs only `fmt`/`strings`.
- **`Run` and `RunGit` share one exec body via an unexported helper; neither calls the other.** The two exported functions become thin wrappers over a single unexported core that runs the command and returns the captured stdout, stderr, exit code, and error. `RunGit` keeps its current behaviour byte-for-byte, including blanking stdout and returning `-1` on an exec-level failure — that is its published shape and nothing may change it. `Run` translates the core's result: nil error and stdout on success, `*GitError` plus stdout on a non-zero exit, and the raw error unwrapped on an exec-level failure.
  - This matters because `RunGit`'s exec path returns `("", "", -1, err)` — it blanks stdout. Had `Run` been implemented by delegating to `RunGit`, the stated stdout-on-error contract would have been quietly unsatisfiable on that path, and the `-1` sentinel the `exec-failures-unwrapped` decision rejects would have had to be unpicked from a value that already discarded the evidence.
  - **The stdout contract, stated precisely:** `Run` returns whatever git wrote to stdout in every case where git actually ran — success and non-zero exit alike. On an exec-level failure git never ran, so stdout is empty; that is a fact about the failure, not a blanking rule. The `stdout-on-error` decision above is about the `*GitError` path, which is the one callers can observe.
  - Rejected: implementing `Run` as a wrapper over `RunGit` (smallest diff, but makes the stdout contract unsatisfiable on the exec path and forces `Run` to reverse-engineer `-1` back into "git never ran"); duplicating the exec body in both functions (two places for `proc.HideWindow`, buffer wiring, and `*exec.ExitError` handling to drift apart).

## Testing

**Tier 1 (untagged, no git spawn):**

- `internal/gitexec` — new test file for `GitError.Error()` rendering. **This is the TDD candidate**: write it before `Error()` exists. Scenarios: stderr present → `git <args>: exit <code>: <stderr>`; stderr empty or whitespace-only → the trailing segment omitted entirely; stderr trimmed; an arg containing a space → `%q`-quoted; an empty arg → `%q`-quoted; an ordinary arg → unquoted; a mixed vector rendering both forms in one string.
- `cmd/lyx/checkedcall_test.go` — the new guard. Assert the invariant holds on the tree as migrated, and assert its own vacuous-scan floor. Cover both raw call spellings, with their deliberately different paren treatment: `gitexec.RunGit` (unparenthesised, so the shorter `gitexec.Run` prefix logic stays coherent) and `r.run(` (parenthesised, so `r.runChecked(` is not swept in). A test that asserts the token spellings themselves — that `r.run(` does not match a `runChecked` call — is worth having, since that is the one place a plausible-looking edit silently inverts the guard.
- `cmd/lyx/gitrepoboundary_test.go` — must pass with the twelve-method pinned set against the OR-ed `r.run`/`r.runChecked` key, and with `gitexecTotal == 2`.
- `cmd/lyx/tierpurity_test.go`, `hermeticenv_test.go`, `rawgitmutation_test.go` — must pass with the shortened token, which is a strictly wider ban than today.

**Tier 2 (`-tags integration`, real git):**

- `internal/gitexec` — extend the existing integration file for `Run`: success returns stdout with a nil error; a non-zero exit returns a non-nil error that `errors.As` recovers as `*GitError` with the right `ExitCode`, `Args`, `Dir`, and non-empty `Stderr`; **stdout is still returned on the error path** (the explicitly-stated contract, so it needs an explicit test); an exec-level failure (non-existent `cwd`) returns an error that `errors.As` does **not** match as `*GitError` — the single most important assertion in the file, since the whole predicate-recovery design rests on it.
- `internal/gitrepo` — the existing suites are the regression net for the 18 migrated sites. `pull_test.go` and `fetch_integration_test.go` must keep passing **unchanged** — they are what proves the two raw suppression sites stayed raw. `reset_test.go` must keep passing; it does not assert on error text, which is why `ResetHard` could migrate.
- `internal/gitrepo` — `IsAncestor`, `CommitEmpty` (`ErrIndexNotEmpty` on a dirty index, both born and unborn HEAD), and `StageAllAndCommit`'s nothing-to-commit signal are the three `errors.As` tri-states; each needs its answer branch exercised, since a mis-transcribed exit-code comparison there converts an answer into a failure silently. `StageAllAndCommit`'s inverted codes make it the likeliest to be got wrong.
- `internal/gitrepo` — `Push`'s rebase-retry recovery, whose trigger sniff moves onto `gitErr.Stderr`. If no existing test drives the non-fast-forward path, `/mill-plan` should add one; a silently-broken trigger degrades to a hard push error rather than a recovery, which is a real regression.
- `internal/gitrepo` — `push_test.go`'s `TestPushRebaseFree_DivergedRemote_ReturnsErrPushRejected` must keep passing **unchanged**. It is the one test that catches the `PushRebaseFree` hybrid being flattened into a plain merge, and its production consumer is `internal/fabricengine/coalesce.go`'s `errors.Is`.
- `internal/fabricengine` — the existing integration suite is the regression net for the 57 migrated sites, and specifically for the `destroy.go` executor re-signature: `warpforward_integration_test.go` asserts a dirtiness-gate *refusal* is distinguishable from a failure, which is exactly what the `err == nil` recording predicate and the surviving `errors.As(&refusal)` paths must preserve.
- `internal/websterengine` — its suite covers the one migrated `gitwrap.go` site.

**Verification per batch and at the end:** `go build ./...`, `go vet ./...`, `go test ./...` (Tier 1), `go test -tags integration ./...` (Tier 2). Tier 2 per batch is deliberate, not only at the end — `fabricengine` and `gitrepo`'s real coverage lives behind the tag, and a mis-merged message found at the end of an 80-site migration is expensive to bisect.

**Not tested, stated honestly:** the merged error *wording* at each site. No test asserts on most of these strings, so the merge rule is enforced by review, not by the suite. That is why the default merge rule is written down here rather than left to per-site judgement.

## Q&A log

- **Q:** Should `destroy.go`'s three gate executors (`removeGitWorktree`, `deleteBranch`, `createGitWorktree`), which return `(exitCode, stderr, err)` so their nine call sites build their own messages, be re-signatured? **A:** Yes — re-signature to return `error`, merge at the call sites. This class did not exist when the design doc was written; it is `wrapProbeError` one level down and takes the same treatment.
- **Q:** How do the three token guards learn about `gitexec.Run`? **A:** Replace the token with the shorter prefix `"gitexec.Run"` in all three. All three match by raw substring and their own header comments justify prefix matching; one token covers both entry points so no set can go half-updated.
- **Q:** Where does the Checked-Call Invariant guard live and what does it scan? **A:** New `cmd/lyx/checkedcall_test.go`, walking all non-test `.go` under `internal/` **and `cmd/`**, matching where the other cross-package token guards live. See `the-checked-call-invariant` for why the walk covers `cmd/` even though zero production sites live there today.
- **Q:** How detailed should the site inventory in this file be? **A:** Shapes, dispositions, and the regeneration queries — not a coordinate list. The implementer re-derives file:line at implementation time; the design doc's own acceptance bar is re-derivability, and 82 coordinates would be partly stale before `/mill-go` starts.
- **Q:** One commit, or a batched DAG? **A:** One logical change, batched by `/mill-plan`. The branch squash-merges, so the design doc's "same commit" requirements are satisfied at merge.
- **Q:** `ResetHard` — the design doc says checked, but `gitrepo/doc.go` documents deliberate stderr suppression. **A:** Checked, and delete the `doc.go` clause. **Operator's standing instruction: do not trust doc files; trust the code.** The code discards stderr and renders `git exited %d`; `reset_test.go` never asserts on `err.Error()`. Nothing pins the suppression — unlike `Pull`/`Fetch`, which three test assertions do pin.
- **Q:** `pushWithRebaseRetry` has 4 `r.run` sites and two stderr sniffs the design doc's grep missed (`containsAny` rather than `strings.Contains`). Raw or checked? **A:** Checked throughout, sniffs moved onto `gitErr.Stderr` via `errors.As`.
- **Q:** Which prose docs get corrected in the same commit? **A:** All the ones the change falsifies — the operator's instruction was explicit: *fix the doc files that are wrong.* `internal/gitrepo/doc.go` (three separate false claims), `CONSTRAINTS.md` (both invariants), and `internal/fabricengine/doc.go` if it states the old shape.
- **Q:** What survives the design doc's deletion, and where? **A:** A tight `internal/gitexec` package header — the two-shape contract, exec-failures-unwrapped, stdout-on-error, args-verbatim/no-credentials, and a pointer to the Checked-Call Invariant. The rejected-alternatives list dies with the doc.
- **Q:** Test coverage for `Run` / `GitError`? **A:** New untagged Tier 1 test for `Error()` rendering (TDD candidate), plus integration tests for `Run` itself. The rendering rules need no git spawn and are what regresses.
- **Q:** The design doc files 8 `rev-parse` probes as pure predicates bound for the raw form. Correct? **A:** No — corrected from the code. Seven of them already separate an error-returning exec path from an answer-returning exit path, which is the mixed shape. They take checked + `errors.As`. The doc's classifier only ever looked at `exitCode == 0`.
- **Q:** The two bool-returning predicates (`isWeftWorktree`, `weftBranchExists`)? **A:** Raw, with markers citing "no error channel in the signature". Migrating them would collapse an exec failure into "the branch does not exist".
- **Q:** How does the guard pin raw-site counts? **A:** Per-package counts in a literal map, plus the marker requirement per site. A global total would hide movement between packages; markers alone would hide a new raw site.
- **Q:** `readBranch`'s prior-exit-code message? **A:** Both calls migrate to checked; the first call's `*GitError` is recovered with `errors.As` and its `ExitCode` cited explicitly, so the combined diagnostic survives and no raw site is left in a pure failure path.
- **Q:** The four full-discard `worktree prune` sites? **A:** Checked, written `_, _ = gitexec.Run(…)`, `//nolint:errcheck` deleted, each with its own comment. `//nolint:errcheck` enforces nothing — this repo has no `golangci-lint`.
- **Q:** After migration only `Pull`, `Fetch`, and `HasUnpushed` still call `r.run`. What does the Client Boundary guard key on? **A:** `r.run(` OR `r.runChecked(` as one combined set, pinned list unchanged at twelve. Keying on `r.run` alone would go blind to nine of the twelve methods, defeating the invariant's purpose.
- **Q:** Marker token at `gitrepo`'s raw `r.run` sites? **A:** The same `//gitexec:raw — <why>` token. One invariant, one searchable token.
- **Q:** Verification scope? **A:** `go build`, `go vet`, Tier 1 and Tier 2 per batch and again at the end. `fabricengine` and `gitrepo`'s real coverage is behind the `integration` tag.
- **Q:** Adopt the derived 21-site `gitrepo` disposition table (3 raw, 18 checked)? **A:** Yes, as written — every row read from the code rather than from the design doc's inventory.
- **Q:** (Discussion review r3, BLOCKING) `HasUnpushed`, `lyxcwd`, and `fabriccli` were filed raw while structurally identical sites went checked, with no stated discriminator — and the rationale for sending the `rev-parse` probes to checked applies verbatim to all three. **A:** Added `the-raw-vs-checked-discriminator` as the single rule: **does the site report the exec path separately?** Yes → checked. No → raw, and only within three truthfully-markable classes (no error channel in the signature; test-pinned deliberate suppression; `run`'s own body). All three sites re-filed to checked + `errors.As`, each preserving its pinned surface exactly. Raw counts drop to `gitrepo` 3, `fabricengine` 2, everything else 0.
- **Q:** (Discussion review r2, BLOCKING) At `remove.go` and `prune.go` the executors' `exitCode != 0` branch is control flow, not a second message — it re-probes registration and may perform a fallback destructive `removePath` — so the default merge rule would either drop the fallback or route an exec failure / `*destructiveRefusal` into a destructive primitive. **A:** Stated as its own rule: at an executor call site the old exit branch becomes `errors.As(err, &gitErr)`, with everything that is not a `*GitError` keeping the bail path. All nine executor call sites are now classified into four shapes in `destroy-executors-are-re-signatured`, and `default-merge-rule` carries an explicit list of the four decisions that carve out non-message exit branches.
- **Q:** (Discussion review r1, BLOCKING) The invariant's wording covers the `gitexec.RunGit` call inside `gitrepo.run`'s own body, but the disposition table's "3 raw" does not — is `run`'s body a marked raw site, and what does the per-package map pin for `internal/gitrepo`? **A:** Yes, it is a marked raw site, and the map pins **4**. No carve-out in the guard — special cases inside guards are how guards rot. The "3 raw" figure counts `r.run` call sites among the twenty-one, a different unit, and is now labelled as such in both places.
- **Q:** (Orchestrator review, `_mill/orch-review.md`) `PushRebaseFree` carries the same `containsAny(stderr, rebaseRetryTriggers)` sniff as `pushWithRebaseRetry`, but the disposition table listed it as a plain `checked`. **A:** Confirmed in the code and fixed. It is a fourth content-sniff site and a **hybrid** — sniff plus bare `ErrPushRejected`, which `internal/fabricengine/coalesce.go` consumes via `errors.Is` and `push_test.go` pins. The default merge rule applied here would have silently broken both. Added as its own decision, annotated in the table, and the sniff regeneration query now names why `containsAny(` must be in it. The same review's `readBranch` nuance — two downstream messages cite the earlier exit code, not one — is folded into that decision.
