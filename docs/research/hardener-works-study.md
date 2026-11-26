# Does the hardener find and fix real faults

A study of the two hardener campaigns on fabric's one-repository layout, from their records and commits.

```yaml
date: 2026-11-26
campaigns:
  c1: harden-fabricengine, landed 37649c772, three rounds
  c2: harden-fabric-single-repo-verbs, landed 2722a66ef, four rounds
follow_through: harden-fabric-undriven-cases, landed 540e8c9b5
records: tags archive/<slug>/<sha> of the records repository, under _lym/reviews/harden/
method: records and code read; no test, sabotage or live trial run
reproductions: none ran
```

## Verdict

The hardener does the first half of its job and not the last.
It finds real faults by driving the modules and fixes them with tests that hold.
It does not converge on a sound basis:
both campaigns ended on rounds that could not raise what they saw or had not driven what they claimed,
and several of its own fixes brought new faults that no round caught.

- **Findings:** every numbered finding of both campaigns names a real behaviour; none is a false positive.
  A few are wording or contrived hand-edited state, and several BLOCKING ratings are high.
- **Fixes:** each cures the scenario its finding shows, and each has a regression test that by reading fails with the fix reverted.
  Parts of some fixes have no test.
- **Focus:** every round stayed inside the campaign's focus, and later focus files steered to what was left unverified.
- **Convergence:** sound in c1 by the rule it ran under, though much of the focus was never driven.
  Weak in c2, which converged with a MEDIUM fault open in prose.
- **Misses:** one fault was reachable and missed outright, in a way forward c2 wrote itself.

All judgements of tests are by reading the test and the fix; no sabotage run was repeated.

## Findings

### c1

| Finding | Judgement | How it was shown | Fix and test |
|---|---|---|---|
| F1 board push lands after the rollback | Real, rare; BLOCKING is high | PLAUSIBLE with a plain-git control; round 2 drove it with the pre-fix binary | `5a5f952`; cures; test holds. The fix brought the seed fault below |
| F2 damaged fetch refspec reads as two repositories | Real, needs hand-damaged config | Live | `a628d0f`; cures; test sets the prime key in every row |
| F3 prime key naming a missing branch exposes `main-weft` to the sweep | Real, needs a corrupted key and `--apply` | Dry run only | `8bd71e3`; cures the missing branch; holds |
| F4 declined board blamed on the host | Real, wording | Live, with a pre-fix control | `5a5f952`; cures; holds |
| F5 seeded records branch has no upstream | Real; every one-repository seed had it | Live | `5a5f952`; cures; holds |
| F6 records push rolled back blind | Real, one stray ref; BLOCKING is high | Live, with `lym` held by SIGSTOP | `704c7ae`; cures; holds. Extends the seed fault |
| F7 prime-key refusal wording; board branch accepted | Half wording, half contrived; BLOCKING is high | Dry run | `ba00dc2`; cures; wording untested |
| F8 merge setting not read as a records-store mark | Real, very contrived | Live | `704c7ae`; cures; holds |

### c2

| Finding | Judgement | How it was shown | Fix and test |
|---|---|---|---|
| F1 push path flags pass the sandbox guard | Real, hidden flags only | Live | `f1ee38d`; cures; holds. `internal/fabriccli/doc.go` not updated |
| F2 archive tag lands in plain `refs/tags/` | Real; the one a normal user hits | Live | `e20e711`; cures equal URL spellings only, and caused G1 and G4 |
| F3 rejected first push says there are no local commits | Real | Live | `5c76ebb`; cures; holds |
| F4 raw error on push with no fetch refspec | Real, contrived | Live | `5c76ebb`; cures the push; sibling callers left (G2) |
| F5 pull reports success and moves nothing | Real, contrived | Live, with a control | `5c76ebb`; cures; holds |
| F6 killed teardown child leaves branches nobody names | Real, rare | Live, `kill -9` | `e20e711`; names them; its re-run caused G3 |
| F7 refusal omits the sandbox hub | Taste | Live | `f1ee38d`; the advised verb refuses outside a hardener run |
| G1 respelled origin URL sends the tag to `refs/tags/` | Real; needs a hand `remote set-url` | Live | `bb29278`; cures; holds |
| G2 `reconcile` gives a raw `gitrepo:` error | Real, contrived | Live | `3a5e28e`; cures; holds |
| G3 the named re-run deletes what was never archived | Real, data loss; the best find | Live, `kill -9` and a declining hook | `a1d8b87`; cures both halves; holds; "a failed owed push stops the delete" untested |
| G4 both stores share one archive-tag prefix | Real design fault | Live; the double listing over hand-pushed tags | `bb29278`; cures; holds |

Round 2's prose observations: O1 (stopped seed advises the wrong clone), O2 (missing `--into` reads as missing git)
and O3 (a failed step drops the pending record) were real and fixed in the round.
O6a (`pull` says "diverged" for a branch only ahead) and O6b (an origin-only records branch is named by no verb) are real
and were not fixed in the campaign.
O4 (a count that rises by two) has no established cause, and O5 (a killed push leaves no record) is by design.

Round 4's O1 to O5, fixed by `540e8c9b5`:
the killed clone with no way forward is real and the most serious;
the unread records layout in `merge`, the unnamed plain archive tags and the discard's untypable way forward are real and minor;
the push of tags kept local is a policy choice whose doc was false.
The follow-through cures all five.
Its `fastForwardWeft` layout read and its `Merge` path have no test.

### Package docs

Updated for most behaviour changes.
Exceptions: c1 F3 and F7 changed function comments only, and c2 F1 left the guard section of `internal/fabriccli/doc.go` stale.
`pattern/PATTERN-single-repo-hub.md` still says a failed seed leaves nothing behind, which the kept-hub paths contradict.

## No reproduction ran

The study allowed up to three reproductions against the commit before each fix, and none ran.
Each needs a one-repository sandbox hub and a build of the older commit.
The task session could make neither: `lym harden sandbox new` refuses outside a hardener run.
The parent runs no trials itself and dropped them.
The three chosen findings are judged from records and commits alone:

| Finding | What the records show | What stays unshown |
|---|---|---|
| c1 F3 | Round 1's dry run listed the prime records branch; round 3 saw the fixed build refuse a missing branch | No round ran `--apply`, so the delete itself rests on reading the code |
| c2 F2 | Round 1 saw the plain tag on origin in two hubs; later rounds saw it under the records namespace | Nothing of weight |
| c2 G3 | Round 2 drove both halves with `kill -9` and a declining hook, with a no-kill control; rounds 3 and 4 re-drove the fix | A real crash: the declining hook stands in for it |

The commits before these fixes were in no ref of the code repository when the study ran,
so a later reproduction may find them collected.

## Faults found by this study

All are from code reading; none was driven.

- **A refused credential keeps the seed's hub with a false message.**
  `seedPushOutcomeUnknown` in `internal/fabricengine/destroy.go` treats any exit without a rejection line as unknown.
  A 403 or `Permission to … denied` on the records push then keeps the hub and says origin "may still take it".
  c1's F1 and F6 fixes brought this, and probably it is the most common way a seed push fails.
- **An empty teardown job clears an earlier teardown's pending record.**
  `RunTeardownChild` calls `clearPendingTeardown` on empty input, and the record it clears is never its own.
- **G3's class is still open in the cleanup sweep.**
  `checkUnlandedRemoteTip` counts local tags as landed, and the remote code-branch sweep of `cleanup` never pushes them.
  No sandbox can drive it, since the sweep lists pull requests through GitHub.
- **`--reset` is advised without regard to unpushed work.**
  The "hub already exists" error names it for a healthy hub too, and `--reset` checks no dirtiness.
- **One records fetch reads no layout.**
  `fetchRunForCatchUp` in `internal/battencli/catchup.go` calls `Fetch` directly when no pair stands.
- **A `remove` record can outlive its pair.**
  A record written by `lym fabric remove` still names it after nothing is left, where it answers `pair not found`.
  `cd8090733` fixed the cancel's records only, by choice.

## Focus

Both campaigns' rounds stayed on the one-repository layout.
Later focus files were built from the residue: re-drive each fix as new text, then the regions not driven.

The limit was reach, not steering.
c1's task session was refused the mutating `lym fabric` verbs, so joining, remote delete, pair verbs and the archive path
went undriven, and seven of its twelve round-1 entries were not driven.
c2 could drive them after `4037bdf36`.
It never drove the real owner-archive push, the hand-over and code-tip reset paths, a lost lease or the code-branch sweep.

An earlier c1 run was cancelled after one review that raised nothing:
its sandbox built only a two-repository hub, and the fabric verbs were refused. It is not judged here.

## Convergence and the stencils

**c1** converged on a round with no finding, a green live gate and a changed model.
That was sound by the rule.
Its undriven regions could not be driven by any further round, and they were carried to c2.

**c2 round 3** (sonnet, medium, 4 minutes) approved with half its focus undriven.
It marked G3 resolved with the discard half undriven and G4 resolved without running the reader that had shown it,
and it reports typing "exactly" a command that holds a `<shortname>` placeholder.
The remaining focus should have blocked approval, since it held the other half of a data-loss fix.
It did block: the Bouncer ruled CONTINUE on the orch's ruling and a red live gate, and round 4 ran.
Round 4 drove the killed discard, the kept-local tag and the failed step with no kill,
and read hand-pushed owner tags through `adopt --archives`.

The study's brief and `docs/research/single-repo-undriven-cases.md` both treat those cases as driven by no round,
and the brief says the orch could order no further round.
The round files show otherwise.
Of the removal kills, the one no round made is a `batten cancel` whose `remove` job is killed.

**c2 round 4** (opus) found five faults and could raise none, because an incremental round admits only BLOCKING findings on changed hunks.
The Bouncer then converged with the MEDIUM killed-clone fault open.
That followed its key-first-raised rule, while the rubric says CONVERGED needs no BLOCKING or MEDIUM finding.
All five went to the board and landed, so nothing was lost, but only because the orch filed them by hand.

The suspected cause is confirmed: the stencils, not the model, kept round 4's faults out of the findings.
Round 3's overstated claims are the model's own.

**`8cf7f4775` is not enough.**
`harden-rubric-review.md` now raises an `origin: focus` finding for an entry left undriven or marked WARN.
It does not name FAIL, the mark round 4 gave the killed clone.
The finding it raises is a `decision` the fixer is never given,
and a defect seen off the changed hunks and outside a focus entry still has no place but prose.

Deferred findings: c2's round-4 observations became a board note, and the record's open risks carry only the MEDIUM one.
No record found shows round 2's O6a or O6b filed.

## Misses

| Fault | Present when the campaign ran | Could it have reached it |
|---|---|---|
| Killed seed's way forward loops (`0e4782373`) | c1 | It saw it three rounds running and could not raise it or drive `--reset` |
| `Fetch` never pruned (`f660ac101`) | c1 | Unlikely: needs a cancel and a re-run of the slug, and the verbs were refused |
| `remove` record names a command that cannot finish (`cd8090733`) | c2, which wrote it | Yes |

c2 had the verbs and ran `lym batten cancel --remove` in its sandbox.
Every kill of a removal went through `lym fabric remove`,
and every hook declined after sleeping so the in-flight push could not land.
An origin branch was therefore always left, and the re-run always found something to do.
A hook that sleeps after receive, then a kill, would have reached the state where the record stands and nothing is left.
c1 had already shown that origin applies a push whose client is dead.
The later hand drive (`docs/research/single-repo-undriven-cases.md`) ran the command after the record was apparently cleared,
so it shows less than it reports; the fault is real in the state above, by reading.

The discard's untypable way forward was caught, late and as a NIT in prose.

## Cost

Review and fix sessions per round.
Tokens are fresh and cache-read from the round usage files.
Dollars are the session records' own, which price no fable session.

| Round | Models | Review / fix minutes | Fresh / cache-read tokens | USD | Raised |
|---|---|---|---|---|---|
| c1 1 | opus medium | 12 / 22 | 0.57 M / 22.6 M | 9.52 | 5 |
| c1 2 | fable high | 42 / 25 | 1.39 M / 20.1 M | unpriced | 3 |
| c1 3 | sonnet medium | 213 / 4 | 0.33 M / 8.5 M | 2.97 | 0 |
| c2 1 | opus medium | 11 / 34 | 0.78 M / 41.7 M | 14.86 | 7 |
| c2 2 | fable high | 41 / 40 | 1.34 M / 58.0 M | unpriced | 4 |
| c2 3 | sonnet medium | 4 / 15 | 0.26 M / 5.0 M | 1.89 | 0 |
| c2 4 | opus medium | 48 / none | 0.61 M / 24.1 M (review) | 9.52 | 0, five in prose |

- c1 ran 5 h 56 min from first row to last, and c2 15 h 12 min, of which 10 h 37 min was one overnight judge row.
- Priced sessions sum to USD 13.85 for c1 and USD 27.99 for c2, each without its fable sessions.
  The follow-through darn run cost USD 15.03.
- c1 round 2 had two interrupted review starts (a hub pause and a machine boot).
  The 213 minutes of c1 round 3 hold a wait the records do not explain.
- The live gate failed for setup reasons in both campaigns: a package list without `./`, a missing gate key,
  a two-tier key refused by `--copies`, and a carried green record that started no test binary (fixed by `592c88c`).

**The rotation.**
Opus and fable rounds produced every finding.
Both sonnet rounds raised none, but each was a medium-effort re-drive after fixes, so model and round position cannot be told apart.
`a66d41345` since made `sonnet[high]` alone the default.
No campaign has run on it, so these records give no evidence that it finds what opus and fable did here.

## Changes to consider

Each is a candidate note.

1. **Let a hardener round raise what it drives.**
   Exempt a hardener round's findings from the changed-file and changed-hunk rules in `burler-review-format.md`
   and `burlerengine.CheckReviewRound`, not only `origin: focus` ones.
2. **Name FAIL beside WARN** in `harden-rubric-review.md`, so a failed focus entry always blocks.
3. **Make the rubric and the Bouncer state one convergence rule** for a MEDIUM first raised in the closing round.
4. **Add a kill rule to the rubric:** kill each job from each caller, once with origin applying the in-flight push and once refusing it,
   then type the printed command in the state left.
5. **Require a whole repro before `resolved`:** a prior finding with an undriven half stays `open`.
6. **Make the fix half drive the neighbours of its fix:** the other failure kinds a new classifier receives,
   and every role that reads a new way forward.
7. **List what no sandbox can drive** (owner takeover, the pull-request sweep, a lost lease) in the verdict as filed follow-ups.
8. **Run the next campaign with one opus round** and compare it with the `sonnet[high]` rounds before keeping the default.
