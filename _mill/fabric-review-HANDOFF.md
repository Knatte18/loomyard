# fabric crucible campaign — HANDOFF

Orchestrator's own state file. Refreshed after every round's verification. Never read by a round
agent (clean-room constraint).

## Right now
Round 1 (`opus-medium-r1`, spawned via `subagent_type: crucible-reviewer-medium`, `model: opus`)
is RUNNING in the background. Seeded from `.scratch/fabric-review-prompt.md`. No verification has
happened yet — do not act on its self-report; wait for it to finish, then run the independent
verification protocol from a cold state, THEN re-seed round 2.

Base commit for this campaign segment: `08520a1b` on branch `fabric-crucible-hardening`.

## Operator correction (2026-08-14) — carry this into every future re-seed
I (the orchestrator) initially characterized the destruction chokepoint (`internal/fabricengine/destroy.go`)
as CLOSED-AND-VERIFIED background from the prior crucible campaign. **That was wrong, and the
operator corrected it.** Checked the git log directly:

```
79a72a38  fabric: crucible hardening pass on V2 (slice 11)                        <- the 6-round adversarial campaign itself (81 findings, 9 BLOCKING, 8 data-loss)
3184cd5a  fabric: one ownership-and-dirtiness gate for all destruction (slice 12) <- destroy.go is BUILT here
1bf8e847  fabric: live-state integration harness (slice 13)
d56b57f7  fabric: accumulate the result envelope from mutations, not control flow (slice 14)
1e605025  fabric: close the corrindex two-phase read-modify-write race (slice 15)
```

`destroy.go` — the chokepoint consolidating ~28 destructive call sites behind one gate
(containment → ownership → dirtiness → force) — was built in slice 12, **after** the adversarial
review+fix rounds had already finished. It was direct implementation work in response to the
rounds' findings, not itself the subject of an independent clean-room review+fix round. **The
chokepoint itself has never been through crucible.** Slices 13-15 are hardening/follow-ups on top
of it, also never independently reviewed as their own target (though round 1's seed did ask for a
re-verification of destroy.go's properties, since the gitexec migration touched it — that's a
narrower ask than a dedicated adversarial round targeting the chokepoint itself).

**Action for round 2: make the destruction chokepoint the PRIMARY target**, not a re-verification
side-note. Full adversarial treatment: try to construct a scenario that gets `destroy.go` to
perform a destructive primitive it shouldn't — a containment bypass, an ownership predicate that
accepts something it shouldn't, a dirtiness probe that reports clean when it isn't, a call site
elsewhere in the package that reaches a destructive primitive without going through one of the
eight executors, `--force` satisfying something other than dirtiness. This is exactly the defect
shape (data-loss, one shape not eight mistakes) the prior campaign spent 5 rounds chasing — the
chokepoint is the thing built to close that shape, and nobody has tried to break it since.

## CLOSED-AND-VERIFIED
Nothing yet this campaign segment — round 1 has not been independently verified.

## RESIDUAL currently seeded
N/A — round 1 is round 1, seeded as a fresh full review (see `.scratch/fabric-review-prompt.md`'s
"Round context seeded" section), not a residual-closing round.

## DEFERRED list
Empty so far.

## Operator instruction (2026-08-14) — deliverables move to `_mill/`, committed continuously
The operator asked for two durable changes to the method itself (applied to
`crucible/README.md`/`orchestrator-prompt.md`/`review-prompt-template.md`, NOT yet committed —
see incident below): (1) crucible deliverables move from gitignored `.scratch/` to committed
`_mill/`, and (2) they get committed as soon as written/updated, not batched. **When round 1
finishes**, move every `.scratch/fabric-*` file (this handoff note, the review prompt, the
precount file, round 1's review + fixer report) to `_mill/` and commit them there. Every path
below still says `.scratch/` because that's where round 1 was seeded to write — treat that as the
last campaign segment to use the old location, not a mistake to fix retroactively.

## Incident: orchestrator `git add` collided with the round's own commit (2026-08-14)
While round 1 was live, I (the orchestrator) staged the `crucible/*.md` doc edits above. The round
agent's own next `git add`/commit (commit-per-fix, same shared working tree, no isolation) swept
those staged files into ITS commit (`1bea8e09 "fabric: fix F6 — ..."`), which now also carries
unrelated crucible-doc changes under a misleading message. Not destructive — the content is fine,
just mis-attributed — and not undone (no rebase/amend on a tree a live round is still writing to).
Fixed the root cause in `orchestrator-prompt.md`'s hard rule 3: the orchestrator must not
`git add`/`git commit` ANYTHING — module-related or not — while a round is live; queue it and
commit once the round completes/pauses. That rule-3 edit, and the earlier `crucible/*.md` edits
generally, are themselves queued uncommitted right now for exactly this reason — commit them once
round 1 finishes, in one clean commit, before touching anything else.

## Exact next action
1. Wait for round 1 (`opus-medium-r1`) to finish.
2. In a clean tree (round 1 fully stopped, nothing pending): commit the queued orchestrator-only
   changes first — `crucible/orchestrator-prompt.md`'s rule-3 strengthening (git-add-during-a-live-round
   ban) — as its own commit, separate from anything fabric-specific.
3. Move every `.scratch/fabric-*` file to `_mill/` (this handoff note included) and commit that
   move as its own commit.
4. Read `_mill/fabric-review-opus-medium-r1.md` and `-fixer-report.md` (paths after the move).
5. Run the independent verification protocol from a cold state on the committed tree (build/vet/
   test, live integration, sabotage-prove every new test, re-drive every BLOCKING fix live in its
   strongest mode — `--force` for destructive verbs). Give the destruction-chokepoint findings the
   sharpest scrutiny given the operator correction above — and note that `fix F6`'s commit also
   carries unrelated crucible-doc changes when reading `git log`/`git show` for it, so it isn't
   mistaken for a bigger or stranger fabric change than it is.
6. Rewrite `_mill/fabric-review-prompt.md`'s "Round context seeded" section for round 2:
   residual(s) verification found, PLUS the destruction-chokepoint emphasis above as the primary
   High-yield focus item (promote it above the gitexec-migration items, which become secondary/
   re-verify-only).
7. Spawn round 2: `subagent_type: crucible-reviewer-high`, `model: opus`, tag `opus-high-r2`.
