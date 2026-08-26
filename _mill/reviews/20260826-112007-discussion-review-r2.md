MILL_REVIEW_BEGIN
# Review: reed: attach doesn't reconcile session geometry with the terminal

```yaml
duration_s: 163.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: claude-opus-5 (runtime-reported; no independent means of verification)
reviewed_file: _mill/discussion.md
date: 2026-08-26
```

## Findings

### [NIT:consistency] x/term is not an indirect require in go.mod
**Demoted-from:** BLOCKING
**Section:** Decisions § terminal-size-from-x-term
**Issue:** `go.mod` has no `golang.org/x/term` line at all (neither require block; only `golang.org/x/sys v0.45.0` is direct), and no Go source in the repo imports it — `go.sum` alone carries `v0.42.0`, so there is nothing to "promote from indirect to direct" and this is a genuinely new direct dependency, not a graph-neutral edit.
**Fix:** Restate the decision as adding a new direct module requirement and justify it on that footing (or re-justify against `golang.org/x/sys`, which IS already direct).

### [NIT:consistency] window-size trusted by default, status pinned against default
**Demoted-from:** BLOCKING
**Section:** Scope § Out ("Any change to `window-size`") vs Technical context § Gotchas
**Issue:** The discussion rules out touching `window-size` because tmux's `latest` **default** is correct, while simultaneously arguing that reed's server loads the operator's `~/.tmux.conf` (no `-f`) so a tmux default must never be assumed — the exact reason `status` and `mouse` are pinned; a conf setting `window-size largest`/`manual` or `aggressive-resize` makes "post-attach window == client rows" false and reintroduces the rescale this task fixes.
**Fix:** Give `window-size` an explicit disposition under the same argument used for `status` — pin it at boot/attach, or state why the residual `~/.tmux.conf` risk is accepted here but not there.

### [BLOCKING:design] Attach-path box source vs live-window box source unreconciled
**Section:** Decisions § live-window-size-is-the-render-box and § attach-chains-select-layout
**Issue:** The first decision makes `planLayout` (`apply.go:68-80`, whose signature today carries no box) derive its `render.Box` from a `display-message` query of the live window — which at argv-build time is still the PRE-attach size — while the second requires the chained layout be planned for the attaching client's size; no seam is decided for injecting that override, so a plan writer could implement either source on the attach path.
**Fix:** State how the client size reaches the planner (e.g. `planLayout` takes an explicit `render.Box`, live query only in the callers that have no told size) and which source wins on the attach path.

### [NIT:consistency] Chain separator and missing `-t` on the chained select-layout
**Section:** Decisions § attach-chains-select-layout vs Testing (Tier 1, argv shape)
**Issue:** The decision writes the separator as `\;` (a shell escape) while Testing pins `;`; via `exec.Command` the argv element must be a literal `;`. The chained `select-layout` is also shown with no `-t`, unlike every other reed call site, which uses `exactSessionWindowTarget` per `doc.go:79-80`'s target discipline.
**Fix:** Fix the separator spelling to `;` in the decision and state whether the chained `select-layout` carries `-t =<session>:` or deliberately relies on the client's current window.

### [NIT:scope] Deleted-artefact inventory misses reedcli's own attachArgv test
**Section:** Scope § In / Testing (Tier 1)
**Issue:** Only `internal/loomcli/bootstrap_test.go`'s `attachArgv` case is named for deletion, but `internal/reedcli/cli_test.go:86-95` tests the `attachArgv` this task also deletes; likewise the template comment edit touches two files (`template_posix.yaml`, `template_windows.yaml`), not one `reed.yaml`.
**Fix:** Name both attachArgv test sites and both template files in the inventory.

## Verdict

REQUEST_CHANGES
Two false premises and one unreconciled box-source seam need resolving before plan writing.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
