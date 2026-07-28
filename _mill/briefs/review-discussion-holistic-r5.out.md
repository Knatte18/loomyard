MILL_REVIEW_BEGIN
# Review: native clients: migrate gitrepo to go-git + selfreport gh-CLI to go-github

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-28
```

## Findings

### [GAP] No timeout on the GitHub HTTP client itself
**Section:** `go-github-version-and-auth-construction` / `never-block-on-credentials`
**Issue:** The never-block property is specified only for the `gh auth token` shell-out (5s); the built client is `github.NewClient(&http.Client{Transport: authRT})` with no `Timeout`, `http.DefaultTransport` sets no response deadline, and `CreateIssue(title, body, labels)` keeps a context-free signature — so a stalled connection hangs an autonomous run forever, and the 401 replay doubles the wait.
**Fix:** State the request deadline (client `Timeout` value and/or a `context.WithTimeout` inside `CreateIssue`), and whether the deadline covers the replay attempt as a whole or per attempt.

### [GAP] `hasUnpushed` has two conflicting failure rules
**Section:** `lazy-cached-repo-handle` vs. Testing (`hasUnpushed` parity cases)
**Issue:** `lazy-cached-repo-handle` names only `remoteName`/`isStrictDescendant`/`SHAExists` as unable to return the open error, implying `hasUnpushed` (`push.go:156`, returns `(bool, error)`) returns it; Testing mandates the go-git side "swallow into `true` to match" the CLI, which today returns `(true, nil)` on any non-zero `rev-list` exit — including exit 128 in a non-repo, i.e. exactly the failed-open case.
**Fix:** Classify the open error for `hasUnpushed` explicitly, and state how go-git failures map onto the CLI's two branches (`(false, err)` spawn failure vs. `(true, nil)` non-zero exit).

### [GAP] gitrepo boundary invariant: "mechanically checkable" but no mechanism
**Section:** `gogit-boundary-local-vs-remote` / `constraints-gitrepo-boundary-invariant`
**Issue:** The boundary decision claims it "is mechanically checkable (see `constraints-gitrepo-boundary-invariant`)", but that decision specifies only a written rule plus a reviewer obligation — no guard test, no `Enforced by` line, unlike the GitHub Auth Invariant, whose guard is fully specified down to allowlists and vacuous-scan protection. Every existing CONSTRAINTS entry names its enforcement.
**Fix:** Decide whether a `gitexec`-call guard test ships in this task or the entry is explicitly `Enforced by review obligation`, and drop the contradictory "mechanically checkable" claim.

### [NOTE] `ChangedFilesSince` ordering is a recommendation, not a decision
**Section:** Testing → three uncovered parity cases
**Issue:** "Decide and record whether order is part of the contract. Recommended: it is **not**" leaves the one open choice in the testing section to the plan writer, while the lifted harness sorts both sides (`harness_test.go:187-190`) and would hide a divergence either way.
**Fix:** Record the decision (order is not contractual; godoc says so; comparison stays sorted) as a Decision line rather than a recommendation.

### [NOTE] `authRT` mutates the caller's request
**Section:** `go-github-version-and-auth-construction` (transport nesting)
**Issue:** "Outbound it **sets** `Authorization`" mutates the request go-github hands the transport; Go's `RoundTripper` contract forbids modifying the request, and the same object is then replayed after a 401 — the standard fix (clone the request, set the header on the clone) is not stated even though the sibling hazard (`req.GetBody` rewind) is.
**Fix:** State that the transport clones the request before setting the header, on both the initial pass and the replay.

### [NOTE] New guard's walk has no skip-dir set
**Section:** `constraints-github-auth-invariant` (guard specification)
**Issue:** The guard is specified as a module-root walk over non-test `.go` files with two allowlists, but the guards it models on carry a skip-dir set (`cmd/lyx/tierpurity_test.go`'s `tierPuritySkipDirs`: `.git`, `_lyx`, `_mill`, `.scratch`, `.wiki`, `_raddle`) — without it the walk descends into gitignored trees such as `.scratch/gogitprobe*/`, making a local `go test` result depend on untracked files.
**Fix:** Name the skip-dir set (or state that the walk deliberately scans nested/gitignored trees) alongside the allowlists.

## Verdict

GAPS_FOUND
Three gaps: unbounded GitHub request, conflicting `hasUnpushed` failure rules, unspecified boundary-invariant enforcement.
MILL_REVIEW_END
