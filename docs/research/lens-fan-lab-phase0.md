# Fan-lab phase 0: what the forks read, and variants of the master's instructions

Measurement report from the `fan-lab` session (2026-10-10); `.scratch/` paths below are in that worktree and were not kept.

Sources: `.scratch/fixture/transcripts/` (D1 `5107a622`, D2 `084ea0eb`, master and five forks each), `.scratch/fixture/reviews/`, and the round-2 inputs as the D2 master read them, saved from its transcript to `.scratch/fixture/target-r2/` (target) and `.scratch/fixture/r2-inputs/` (prompt, both instruction files, focus, stencil).
File references are the repo paths named in a tool call's input (Read path, or a path inside a Bash/Grep command), deduplicated per actor; a directory-wide grep counts as a search, not a file.
Fresh tokens are input + cache-write + output, taken from the final usage of each message.

## 1. Timelines

### Master before the fork point

| Round | Order | Read before forking | Code files | Judged before forking? |
|---|---|---|---|---|
| D1 | prompt, instruction 1, focus, target, stencil, then code, then instruction 2, then spawn | 6 pattern files, `git show --stat`, 13 Bash calls over 19 code files (regions via `grep`/`sed -n`), ~35k tokens of tool results | 19 | Yes: its pre-fork thinking says "I've identified four findings so far (D9, D8, D7, and the Goal count)", and the forks inherit that context |
| D2 | prompt, instruction 1, **instruction 2 at once**, focus, target, stencil, then spawn | nothing beyond the inputs | 0 | No |

The cause of the split is the stencil layout.
The lens list and "after exploring the target fully, spawn" live in instruction 2 (`burler-step-2-review.md`, `{{.cluster_rules}}`), and the prompt says "never preview a later file's content early".
A compliant master (D1) explores in step 1 as a solo reviewer would, judging as it goes, and learns about the fan only after.
A master that previews step 2 (D2) reads "after exploring the target fully" as "after reading the target" and forks with no code.
Today's text gives either a coloured fan or an empty one; neither is the design.

### Forks after the fork point

| Round | Fork | Tool calls | Turns | Files named | of which master read pre-fork | of which another fork also read |
|---|---|---|---|---|---|---|
| D1 | generic A | 3 | 4 | 4 | 3 | 3 |
| D1 | generic B | 7 | 8 | 9 | 4 | 8 |
| D1 | correctness | 15 | 14 | 11 | 4 | 9 |
| D1 | error-handling | 7 | 8 | 9 | 3 | 5 |
| D1 | test-gaps | 1 | 3 | 4 | 2 | 2 |
| D2 | generic A | 32 | 30 | 27 | 0 | 21 |
| D2 | generic B | 21 | 22 | 18 | 0 | 13 |
| D2 | correctness | 33 | 34 | 24 | 0 | 19 |
| D2 | error-handling | 33 | 28 | 25 | 0 | 16 |
| D2 | test-gaps | 16 | 16 | 11 | 0 | 4 |

Round totals from the usage records: D1 forks 151.6k fresh, 3.3M cache read; D2 forks 485.4k fresh, 12.2M cache read.
D2 forks named 105 file references over 57 distinct files: 48 references (46%) repeat a file another fork read.
25 files were read by two or more D2 forks; 7 were read by four or five of them (`fabriccli/weft_verbs.go`, `webster-template-master.md`, `websterengine/recoverbatch.go`, `rebaseline.go`, `shuttleengine/wait.go`, `fabricengine/spawn.go`, `fabriccli/fabric.go`).

### Master after the fork point

D1: 12 tool calls over 11 files, 7 of them new since its pre-fork reading.
D2: 25 tool calls over 27 files; 13 of the 25 files read by two or more forks were also read by the master in parallel with the forks.
So in D2 the same code picture was built six times.

### The fork prompts the master wrote

In both rounds the master rewrote each lens into its own fork prompt.
D2's prompts all say "Verify the Decisions' factual premises against the actual code in the worktree" or "Read the real code the Decision modifies"; the code lenses were recast as "applied to a design artifact".
These sentences override the boilerplate's "prefer your inherited context", and with nothing inherited the forks had to read.
D1's prompts said "ground every claim in code you actually read" over a context that held the code, and the forks read little.

## 2. Did the BLOCKING finding need the duplication?

Round 2 kept one BLOCKING finding, F1: D7's guard refuses `push` under a webster strand name, which also refuses the detached push child `Fabric.Commit` spawns.
It is tagged `origin: handler`, but four of the five forks (correctness, generic A, generic B, test-gaps) found it independently, each reading `fabricengine/spawn.go`, `fabricengine/commit.go` and `fabriccli/weft_verbs.go`; the master read the same files a fifth time.
One read of those files before the fork point would have served all five.

Round 1's three BLOCKING findings came from lens forks (correctness twice, generic once).

## 3. The reading set an ideal master would have read for round 2

The target names 29 repo files by path and about 50 symbols in backticks.
Of the 25 files two or more D2 forks read, 22 are named by path or hold the definition of a named symbol.
The other 3 are one hop from a named one: `fabriccli/weft_verbs.go` holds the verbs of the named `fabriccli/fabric.go`, `websterengine/reset.go` backs the named `webstercli/reset.go`, and `fabricengine/spawn.go` is called from the push path D7 guards (`fabriccli` push → `Fabric.Commit` → `SpawnDetachedPush`); `spawn.go` grounds F1.
So "the files and symbols the target cites, plus the direct callers and callees of each symbol a Decision changes or guards" reproduces the set the forks needed.

Whole files are too large: those 25 files are about 146k tokens.
The read must go by region (definition and call sites), as D1's master did; D2 forks' tool results total about 160k tokens with duplicates, so the distinct regions come to an estimated 60-80k tokens.

### Shared set: per Decision (every lens needs the premises)

| Decision | Regions |
|---|---|
| D1 rebaseline on entry | `websterengine/rebaseline.go` (`Rebaseline`, `ErrRebaselineCardSetChanged`), `fingerprint.go` (`changedPlanFiles`, way-forward text), `beginbatch.go` (`ErrFingerprintMismatch`), `runlevel.go` (run entry, warnings), `shedadapters/webster.go`, `loomshed/webster.go` |
| D2 frozen-plan marker | `shedadapters/rubric.go`, `burlerengine/prompt.go` (`fixScopeRules`), `burler-step-3-fix.md`, `loom-rubric-plan-review.md`, `websterengine/state.go` (`CardHashes`) |
| D3 overview frame | `rebaseline.go` (`recordedOverviewFrame`, `overviewRefusal`) |
| D4 batch check per prefix | `gitwrap.go` (`commitsNamedBy`), `gitexec` `Run`/`runChecked` |
| D5 chain splitter | `planparser/verifymodule.go` (`crossModuleCommands`, `moduleWideCommands`) |
| D6 done-checks | `beginbatch.go` (`DoneCards`), `recordbatch.go` (`postBatchChecks`, `failCardNotDone`) |
| D7 fabric guard | `fabriccli/fabric.go`, `weft_verbs.go`, `fabricengine/commit.go`, `spawn.go` (`SpawnDetachedPush`), `agentname/agentname.go` (`StrandNameEnv`, `MatchesRole`), `configcli/hubwide.go`, `configcli.go`, `hubgeom/parent.go`, `refusal-spec.md` rows naming `lyx fabric` |
| D8 read-only classifier | `fabricengine/refscanner.go` (`IsReadOnlyCommand`), `websterengine/audit.go` (`ClassifyViolation`), `auditledger.go` (`AcceptBatchFabricReference`), `classify.go`, `websterengine/doc.go` import rule |
| D9 shell wait | `shuttleengine/wait.go` (`expiredTurnEnd`, gate re-prompt), `run.go`, `websterengine/strand.go` (`TurnEndedAfter`) |
| D10 `-run` anchoring | `loomshed/publishfailure.go` |
| D11 resume test | `loomcli/smoke_prebootstrap_integration_test.go` |
| D12 recovery cap | `recoverbatch.go` (`recoverSpawn`, `RecoverSpawnOrAttach`), `state.go` (`BatchState`), `beginbatch.go` (rebuild of the record), `reset.go`, `keepreset.go`, `webster-template-master.md` (`card_amended`, dead-batch rungs) |
| D13 entry line cap | `pattern/check.go` (`MaxEntryLineChars`), `planparser/fabricref.go` (`codeSpans`), `loomshed/gates.go` (`ValidatePlan`) |

### Per lens of the `discussion` fan (what each needs beyond the shared set)

| Lens | Extra reading | Seen in the forks |
|---|---|---|
| generic | none | generic A/B read only shared-set files, plus `lyx board get` (generic B) |
| goal-scope | the board entry (`lyx board get '{"slug":"<slug>"}'`) | generic B fetched it on its own |
| pattern-fit | the `pattern/` background files of the PATTERN entries the target names (about 9k tokens for round 2's 20) | error-handling and generic A read 3 of them; D1's master read 6 |
| scenarios | the failure, restart and resume paths of the Decisions: D1 crash between rebaseline and save, D12 re-begin and reset, D9 reattach, D7 child processes | error-handling and correctness read these, all inside the shared set |
| leanness | none: target, rubric and stencil are already in context | none |

The shared set carries nearly all of it; the per-lens extras are the board entry and the pattern background files.

## 4. Variants

Variants B-D need one piece of plumbing that is not text alone.
The lens list must reach the master in step 1, before it explores, so `burler-step-1-explore.md` gets a new marker, `{{.cluster_preread}}`, filled by a new `clusterPrereadBlock(p)` for the reviewer's explore file in a cluster round and empty for the fixer's explore file and for solo rounds.
`clusterRulesBlock` keeps the spawn and consolidation rules in step 2.

### A: today's text (reference)

`clusterRulesBlock` as on `fan-lab` HEAD; no step-1 marker.

### B: the master knows the lenses up front and reads for them, judging nothing

Step 1, `clusterPrereadBlock`:

```
## Reading for the fork reviewers

This round fans out: in step 2 you spawn one fork reviewer per lens below, and each fork inherits your context as it stands at the spawn.
Your reading in this step is the forks' reading, so read here what they will need, once, instead of leaving each fork to read it again.

Lenses for this round:

- <name>: <lens text>
  ...

Before you leave this step, read:

- every file the target names, at the definitions of the symbols it cites;
- the direct callers and callees of each symbol a Decision changes, guards or removes, and the processes it spawns;
- the `pattern/` background file of every PATTERN entry the target names or touches;
- for each lens above, what that lens needs beyond this, such as the board entry for a scope lens or the failure and restart paths for a scenario lens.

Read definitions and call sites, not whole files: a fork re-reads its inherited context on every turn.
Judge nothing while you read: write no finding, verdict or assessment, not even in a status line or a note to yourself.
A judgement in your context before the spawn colours every fork; your own review starts after the forks are spawned.
```

Step 2, `clusterRulesBlock`: the first sentence becomes "This is a cluster round: as soon as step 1's reading is done, spawn ALL of the fork reviewers below in a SINGLE message ..."; the rest is unchanged.

### C: B, plus forks are told their context holds the code, with a budget of 6 tool calls

Step 1 as B.
Step 2, `clusterRulesBlock`, the fork boilerplate paragraph becomes:

```
Each fork's prompt is this boilerplate plus that lens's emphasis text, and nothing else: never add a direction to verify against the code or to read files.
Your inherited context already holds the target, the rules and the code the target rests on; review from it.
You may make at most 6 tool calls, each only for something your lens needs that your context lacks, and never to re-read what your context already shows.
End your final message with one line per tool call you made: what you read and why your context lacked it.
You are READ-ONLY — never Write/Edit/delete any file, never run any git command, never touch the two round output files, and never call the Agent tool yourself (forks cannot nest); return your findings ONLY as your final message, each with a severity, a class, a location, and a one-line summary.
```

and the holistic paragraph gains: "Your holistic review works from the same context; read further only where it or a fork's read list shows a gap."

### D: C, plus a reading manifest before the spawn

Step 1 as B, with this appended:

```
End this step with a reading manifest as a plain message: under each lens name, one line per file and symbol you read for it.
The manifest names what you read and nothing about what you found.
```

Step 2 as C, with the fork boilerplate gaining: "The reading manifest in your context lists what is already read; check it before any tool call."

## 5. Expected effect (to be tested in phase 1)

| Variant | Master pre-fork | Forks | Prediction |
|---|---|---|---|
| A | 0-19 code files, by chance | 30-125 turns | reproduces D2 or D1, depending on whether the master previews step 2 |
| B | ~60-80k tokens of regions | fewer reads, but the master's own fork prompts may still say "verify against code" | fork fresh roughly halves; tells whether pre-reading alone is enough |
| C | as B | ≤6 calls each, about 8 turns | round near 250-300k fresh: master ~120k (pre-read included), forks ~25-35k each, consolidation ~40k |
| D | as B, plus a short manifest | as C | as C; the manifest makes the pre-read measurable and may cut forks' remaining reads further, at a risk that a manifest reads as a judgement |

Thinking caveat: the fork inherits the master's in-turn thinking, so "judge nothing" can be checked only through the visible text and the pre-fork thinking summaries; phase 1 records any pre-fork judgement it finds.

## 6. Decisions for the operator before phase 1

1. Lens count: the brief says the `discussion` fan has 4 lenses, but `template.yaml` gives it 5 (generic, goal-scope, pattern-fit, scenarios, leanness).
   Run with the 5, or drop one (generic is the candidate: the analysis found half of fork findings were cross-lens duplicates)?
2. BLOCKING criterion: round 2's only BLOCKING is F1 (detached push child); a run "finds it" when its consolidated review carries it at BLOCKING, whatever the origin.
3. Plumbing: B-D add the `{{.cluster_preread}}` marker to `burler-step-1-explore.md` and a `clusterPrereadBlock` in `prompt.go`, on the `fan-lab` branch only.
