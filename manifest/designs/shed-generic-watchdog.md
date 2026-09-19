# generalize `ly-drive` and loom's `start`/`run`/`step` CLI verbs into a Shed-generic watchdog

## The idea

`shedengine`/`shedbuild`/`shedrecipe` were already fully generic — the Told-Geometry Invariant — and `internal/loomcli`'s `start.go`/`run.go`/`step.go` were the one place hardcoding loom's own recipe/paths, with no sibling CLI package carrying an equivalent `start`/`run`/`step` trio.

This task generalized `run`/`step`/`status`/`pause` off `loomcli` and onto a leaf package, `internal/shedverbs`, and added `internal/shedcli`, the `lyx shed` subtree that arms either shipped recipe behind one `--recipe` flag.
`ly-drive`, the operator-facing skill that loops the generic `step` verb, now drives any recipe through that subtree rather than loom alone.

## The shipped shape

The generic verb bodies live in `internal/shedverbs`, a leaf package: it owns `run`/`step`/`status`/`pause`'s cobra bodies and nothing else, deriving no path of its own and importing no resolver — no `lyxcwd`, no `os.Getwd`, no `git rev-parse` — and no `<module>cli` package, which keeps a consuming module's own imports acyclic.
Every path this package touches reaches it told, through a `shedverbs.Spec` the arming module fills; every string that differs between the two shipped consumers travels as a told field on that `Spec`, never as a value the generic body derives from another field.
See the `## Shed Verb-Set Invariant` in `CONSTRAINTS.md` for the invariant this package's own tests enforce.

`internal/shedcli` owns the `lyx shed` subtree: a named-recipe arming table (`table.go`) mapping `"loom"`/`"lifecycle"` to the arming function (`loomcli.Arm`/`lifecyclecli.Arm`) that resolves and wires that recipe's whole engine stack, plus the three CLI seams (`Command`, `RunCLI`, `RunCLIIn`) that register the subtree under the lyx root.
The table is a separate declaration from `internal/shedrecipe`'s own engine registry: this one maps a recipe *name* to an *arming function*, the other maps an engine *name* to a `shedengine.ShedProducer` *constructor*, and the two answer different questions for different callers.
`lyx shed <verb> --recipe <name>` (default `loom`) drives whichever recipe the flag names; a verb a recipe's own table entry excludes — `step` for `lifecycle`, which has no analogue — is refused before arming, with a bare error envelope carrying no `kind` field, outside the five-kind vocabulary `step`'s own envelope pins closed.

Both existing subtrees, `internal/loomcli` and `internal/lifecyclecli`, are rearmed onto `shedverbs.Verbs(texts, spec)` with their own surface unchanged byte-for-byte: `lyx loom start|run|step|status|pause|validate-discussion|validate-plan` and `lyx lifecycle run|status|pause` (lifecycle gained `pause` and `status --watch` in this task, the only agreed surface additions).
`internal/shedbuild`'s `ShedPaths`/`NewShed` is the deduplicated assembler both `loomrecipe.New` and `lifecyclerecipe.New` build their `*shedengine.Shed` through, parsing a caller-supplied recipe and building it against a caller-supplied `shedrecipe.Env`.

`start` does not generalize.
There is no `Shed.Start`, because it seeds, commits, spawns the detached driver, and hands the terminal over, all of which sit above the engine — so it stays loom-specific, on `internal/loomcli` alone.
That verdict, settled speculatively before this task, is now closed rather than itself open: nothing in this task's shipped shape gives `start` an engine counterpart, and none is planned.

`ly-drive`'s role in this shape is exactly what it was speculated to be: it loops the generic `step` verb until the driven recipe's own producer list is exhausted or the run reaches a non-`running` state, now over `lyx shed step --recipe <name>` instead of `lyx loom step` alone, with every loom-specific claim in the skill gated on the recipe name.
`loom` is the only recipe shipped today whose table entry supports `step` at all, so the recipe argument exists for the next consumer this loop can drive, not for a second recipe available now.

This task writes no Hardener recipe and no Hardener artefact of any kind.
Hardener is the future consumer `_mill/discussion.md`'s Scope names for both the verb set and the neutralized inner-run engine, but `manifest/designs/hardener.md` still carries a DRAFT banner saying not to implement from it yet, and nothing here changes that.
