MILL_REVIEW_BEGIN
# Review: Rename hub container suffix from -HUB to -LYXHUB

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-09-13
```

## Findings

### [BLOCKING:decision] Stale `-HUB` hub after the documented re-clone
**Section:** Decisions → *No physical rename* / *Clean break*
**Issue:** `CloneHub`'s collision guard and its `--reset` teardown both key on the *derived* path (`internal/fabricengine/clone.go:165,215` — `os.Stat(hubPath)` → "hub already exists"; `hubPath = HubPath(parent, DeriveWarpName(url))`), so after the value swap a re-clone in a parent holding `<name>-HUB` is neither refused nor reset: it silently creates a second, parallel hub for the same warp (own weft clone, own `_board`, own portals/launchers, own `reedengine.ServerName` socket) while the old one keeps running.
**Fix:** State the disposition of the pre-existing `<name>-HUB` directory on the documented re-create path — whether the operator deletes it manually, and whether `docs/sandbox-hub.md` / the `lyx fabric clone` help text gain a note that `--reset` will not remove an old-suffix hub.

### [NIT:consistency] "costs one re-clone and nothing else" understates the sandbox case
**Section:** Decisions → *Sandbox fixture hub is renamed*, Rationale
**Issue:** `build.cmd -reset` reaches `lyx-test-LYXHUB` only, so the old `lyx-test-HUB` (plus any live reed server on its socket) survives the reset it is claimed to be disposed of by — same mechanism as the finding above.
**Fix:** Qualify the sentence, or point it at whatever disposition the blocking finding settles.

### [NIT:consistency] `tools/sandbox` is stated to have no Go tests
**Section:** Testing → Per-module notes, last bullet
**Issue:** `tools/sandbox/{main,suite,report}_test.go` exist and build hub paths from the `hubName` constant (e.g. `main_test.go:21,398`); they self-update and carry no `-HUB` literal, but the stated premise is false and could justify skipping the package in verification.
**Fix:** Replace with "tests exist and reference `hubName`, so they self-update; no literal sweep owed there".

### [NIT:decision] CONSTRAINTS.md note has no settled home
**Section:** Scope → In, `CONSTRAINTS.md` bullet
**Issue:** "add a short migration note under **Hub Containment Invariant** (or a new line)" leaves placement — and whether this is a bullet on an unrelated invariant (that one is about junctioning, not naming) or its own entry — to the plan writer.
**Fix:** Pick one placement and the exact claim being recorded.

### [NIT:consistency] Stale round-1 instruction block
**Section:** *Open — scope extension*
**Issue:** The section directs polish items to be raised "in the orchestrator-authored discussion-review round 1"; that round has passed, so the section reads as an open item that is in fact closed by its own last line.
**Fix:** Collapse it to the conclusion ("this file covers the rename and nothing beyond it") or mark it resolved.

## Verdict

REQUEST_CHANGES
Re-clone path leaves an undisposed old-suffix hub; everything else verified accurate.
MILL_REVIEW_END
