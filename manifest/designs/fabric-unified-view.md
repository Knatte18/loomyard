# fabric: unified-repo view — one illusion of a single repo, over warp + weft

> **Status: Someday, exploratory.** Raised during a design discussion about `fabric`'s warp/weft split; not committed to, not scoped as a task yet. Several sub-questions below are explicitly open. Does not modify [fabric.md](fabric.md) — that file is deleted at cutover once `internal/fabricengine`'s package doc is the sole remaining source of `fabric`'s rationale; this doc's content belongs in that package doc (or a successor doc) once actually picked up, not in the soon-to-be-deleted file.

## The illusion: one normal repo, from the outside

Junctions (`_lyx`, `_raddle`, later `_pattern`) exist so that, from the host worktree's perspective, weft-backed folders look like ordinary parts of the same repo — even though they're a second git history underneath. The idea explored here: extend that illusion all the way through `fabric`'s own API surface, not just the filesystem layer. Writing to any file in the worktree, whether it physically lands in warp or in weft, should look from the outside — to a human or an LLM — like it just landed in one repo. `fabric` is the only module that knows both repos exist; it should be the only place the distinction is handled, never something a caller has to track.

This is a broader, sibling concern to [host-visibility.md](host-visibility.md) (which hides `CONSTRAINTS.md`/`CLAUDE.local.md` specifically from the host's own git history) — same junction-based mechanism, applied here to `fabric`'s API rather than to one specific file pair.

## `Fabric.Commit` — the idea, and why it may not be needed

The idea as originally raised: a single `Fabric.Commit(paths, msg)` entry point classifies each path against known weft junction mountpoints and dispatches to the warp or weft repo accordingly, instead of requiring a caller to pick `Warp.StageAndCommit`/`SyncWeft` explicitly. Classification itself is a trivial path-prefix check against already-known junction mount points — no new low-level mechanism needed there.

Whether this is worth building at all turns out to be genuinely open — see "Weft-commit stays Go-orchestrator-only" below, which walks through why no confirmed caller has been identified once the warp-side and weft-side questions are both worked through.

## Warp stays ordinary, unrestricted git — for humans and agents alike

**Resolved: plain `git add`/`git commit` (and anything else — rebase, amend, force-push) stays the norm for warp, for both humans and agents. `Fabric.Commit` is not a replacement for warp's git usage.** Reason: warp is a normal project repo other people and tools touch outside any LYX-based workflow — a collaborator who has never heard of `fabric` will just run ordinary git commands, including operations `fabric` cannot see coming (rebase, history rewrite). Any design that assumed *every* warp mutation goes through `Fabric.Commit` would break the moment a real collaborator did the obvious thing. This isn't a new requirement to design for — `fabric.md`'s existing "History-rewrite safety" section already commits to exactly this posture: `fabric` relies on `SHAExists` before trusting any stored SHA reference, doesn't try to be "rebase-aware" any more than `gitrepo` does, and (per fabric.md's resolved open question) surfaces a typed staleness error rather than attempting automatic recovery when an external rewrite invalidates a stored reference.

Consequence for this doc: the earlier open question ("should an agent's own warp commits go through `lyx fabric commit` instead of raw `git commit`, to make classification/safety uniform") is **answered — no.** Forcing it wouldn't achieve uniformity anyway, since nothing can force *other* collaborators through the same path; `fabric`'s job is to stay correct in the face of arbitrary external warp git activity (already designed for), not to become the only door in. This also narrows `Fabric.Commit`'s real value: it matters for giving weft a single, safely-gated entry point, not as a general warp-commit replacement.

## Weft-commit stays Go-orchestrator-only — but does `Fabric.Commit` have a real caller at all?

`CONSTRAINTS.md`'s "Orchestration, not agent" invariant is **not up for revision** here (explicit decision, not left open): every weft commit is Go calling the engine in-process at a round/phase boundary the orchestrator itself controls; an LLM agent never decides *when* a weft commit happens, regardless of how the commit is dispatched internally. That part stands regardless of what happens below.

**Open, and honestly unresolved by this doc's own reasoning: does anything actually call `Fabric.Commit`?** Walking the other conclusions here through to the end: warp-side commits stay on plain git for both humans and agents (see above) — no caller there. Weft-side commits are performed exclusively by Go orchestration code (Finalize, Raddle-regen) that already knows explicitly what it's writing, because it's the code that authored the content in the first place — auto-classification doesn't save that caller anything either, the same counter-argument this idea faced early on (a caller already has to know where a file belongs to decide where to *write* it, so guessing again at commit time buys nothing). **No confirmed caller has been identified.** Whoever picks this up should find a real one before building it, or drop `Fabric.Commit` from scope entirely and keep only the diff/status and enforcement work below.

**If a caller does turn up and this gets built, two things still hold:**

- **Not atomic either way** — a call spanning both warp-side and weft-side paths in one invocation is still two separate underlying git commits; there is no cross-repo transaction. Partial-failure semantics (report both results vs. attempt to roll back whichever side succeeded) remain open.
- **Weft paths must hard-block** (explicit error, not a silent no-op) for any caller that isn't the sanctioned orchestrator context — consistent with the invariant already being an actively *audited* hard rule elsewhere (`internal/websterengine/audit.go` flags a weft-referencing Bash command as a review violation today). An explicit refusal matches that "hard rule, not soft suggestion" posture; since a legitimate caller should never hit this path, an error is the correct signal, not a mystery no-op. **Open question:** the exact mechanism for identifying "the sanctioned orchestrator context" — a caller-identity check, or a separate, more privileged API surface that only orchestration code imports and that never appears on any agent-facing path.
- Restricting an agent-facing surface to only `Fabric.Commit` (never raw `.Warp`/`.Weft` field access, which `fabric.md`'s existing API exposes directly today) would **not** be a reprise of `fabric.md`'s already-rejected "forwarding method per operation" alternative — that alternative was rejected for duplicating the *entire* `gitrepo.Repo` method set (`StageAndCommit`, `ChangedFilesSince`, `CurrentSHA`, `Push`, `Pull`, `SHAExists`, ...) as pass-through methods. This would be *one* method for the one operation needing a safety boundary, not a wholesale duplication. (Checked the shipped CLI: today's `lyx fabric commit` verb is already weft-only — `internal/fabriccli/weft_verbs.go` — there is no existing warp-commit CLI verb, so this would fill a genuine gap rather than duplicate one, *if* a caller exists.) Internal Go orchestration code keeps using `.Warp`/`.Weft` directly regardless — it already knows the distinction.

## `fabric` needs its own unified diff/status — a different, simpler case than Finalize's merge-diff

Two distinct kinds of "diff spanning warp and weft" come up in this project, and only one of them is genuinely special:

- **"What changed in my own worktree since some earlier point"** — the case this section is actually about. **Not special.** An ordinary per-repo diff (`gitrepo.Repo.ChangedFilesSince`), using the correspondence index only to resolve the right starting weft SHA from a warp-SHA reference point, merged into one report. The real gap is narrower than it first looks: raw `git diff`/`git status`, run in the host worktree, is still blind to weft's separate history, so nobody — human or LLM — should need to reach for raw git to get the full local picture. Filling that gap doesn't need anything beyond primitives that already exist.
- **"How my weft content compares to a *different* weft (parent's, at merge-back)"** — genuinely special, and already solved, for a different purpose: [finalize.md](finalize.md)'s document-driven, non-git-conflict mechanism (Go precomputes the diff directly against the real weft worktree via the correspondence index, hands the agent a plain document, never git conflict markers). This doc doesn't need to (and shouldn't) reinvent that — it already exists for the case that actually needs it. Don't conflate the two.

`Topology.Status`/`Fabric.StatusWeft` already exist today, but separately (one per side), and only cover the first case. This proposes an actual **merged** view — a `Fabric.Diff` (and/or an extended `Status`) presenting warp and weft changes together as one report, still only for the "since an earlier point in my own worktree" case above.

**Open question:** whether this becomes a new CLI verb (`lyx fabric diff`) or stays a Go-internal API not exposed as a standalone command — depends on who ends up needing it (a human debugging, an LLM instructed to inspect its own worktree state, or only internal callers like Finalize).

## SHA-bookkeeping — reuse, not a new mechanism

Confirmed feasible without inventing anything: `gitrepo.Repo.SnapshotSHA`/`SetSnapshotSHA` (persisted per-key SHA tracking) and `fabricengine`'s Warp-SHA correspondence index (`RecordCorrespondence`/`WeftSHAForWarpSHA`/`RebuildIndex`) already do exactly this class of bookkeeping today, built for raddle staleness tracking and the weft-sync trailer respectively. A unified status/diff view would generalize reuse of this existing infrastructure, not add a new primitive underneath it.

**What triggers a snapshot?** Nothing new needs inventing here either. `fabric` already leaves cadence to the caller (see `fabric.md`'s resolved "Push timing policy" open question: `fabric` stays unopinionated about cadence, a caller-level policy decision). A unified diff doesn't need `fabric` to autonomously decide "since when" — the caller captures its own reference point (e.g. `CurrentSHA()` at phase-start) and later passes it to `ChangedFilesSince`/`Diff`; this is a pure function of caller-supplied input, not an event `fabric` needs to trigger on its own. The one piece that *is* already auto-triggered today is the correspondence index (`RecordCorrespondence` fires alongside every real `CommitWeft`) — unchanged by anything proposed in this doc.

## How far does "route git through Fabric" go? — tension with an already-deliberate scope boundary

A broader version of this idea was raised and is **not settled**: route *every* git operation through `Fabric`, unconditionally, for full control over warp/weft correctness. This runs directly into `fabric.md`'s own, already-deliberate "Scope boundaries — deliberately not a general-purpose git wrapper" section: `gitrepo`'s scope already excludes rebase, interactive staging, cherry-pick, and conflict resolution, and explicitly preserves "a human always has plain git available in either working tree." That was a conscious choice, not an oversight — and the "warp stays ordinary git" resolution above is a direct instance of it holding.

Two questions need answering before this goes anywhere, not just one:

- **Humans, or only agents?** Preventing a human from running plain `git` in their own worktree is neither technically enforceable (it's their own shell) nor obviously desirable, given warp is meant to be "an ordinary project repo" — reinforced by the collaborator argument above.
- **Does "full control over correspondence" actually require *all* git operations, or only the mutating ones?** The correspondence index only needs to know about commits/pushes/pulls/merges — operations that advance history. Read-only operations (status, diff, log) don't affect correctness at all; routing them through `Fabric` buys a consistent *view* (the unified-diff point above), not control. Wrapping every git verb (rebase, blame, stash, branch, ...) reprises the "forwarding method per operation" pattern `fabric.md` already rejected — at a much larger scale than the single `Commit` method proposed above.

## Open questions (unresolved, for whoever picks this up)

- **Whether `Fabric.Commit` has any real caller at all** — walked through above and came up empty; find one or drop it from scope before building anything.
- Partial-failure semantics for a `Fabric.Commit` call spanning both warp and weft paths in one invocation (only matters if a caller is found).
- The exact mechanism for restricting weft-side dispatch to the sanctioned orchestrator context.
- Whether `Fabric.Diff` becomes a CLI verb or stays Go-internal only.
- Whether "route git through Fabric" should extend beyond commits at all, and if so, to agents only or to humans too — see the section above; leans toward "commits/pushes only, agents only" but this isn't decided.

## Related

- [fabric.md](fabric.md) — the base design this generalizes over. Not itself edited by this doc; deleted at cutover once `internal/fabricengine`'s package doc absorbs the rationale.
- [finalize.md](finalize.md) — the related but distinct, genuinely special cross-worktree diff problem (my weft vs. a different weft), already solved there for the merge-conflict path.
- [host-visibility.md](host-visibility.md) — a related, narrower illusion (hiding `CONSTRAINTS.md`/`CLAUDE.local.md` from host git history) via the same junction mechanism.
- `CONSTRAINTS.md`'s "Orchestration, not agent" section — the invariant this design must not violate, only enforce more consistently.
