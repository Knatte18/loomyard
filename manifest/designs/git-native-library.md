# git-native-library — evaluate a native Go git library instead of shelling out via `gitexec`

> **Status: Speculative, not scoped.** Prompted by `internal/gitrepo`'s crucible hardening (four
> review rounds, 2026-07), which found real bugs in the class of "parse git's human-readable
> stderr as a de-facto API." Not yet a plan — this file exists to hold the reasoning so it isn't
> re-litigated from scratch later. Per the
> [documentation lifecycle](../../docs/overview.md#documentation-lifecycle), if this is ever
> picked up the durable parts fold into the owning package's doc when it lands; if abandoned,
> this file is simply deleted.

## The problem this responds to

`internal/gitexec.RunGit` is the sole choke point (~80 call-sites repo-wide) that shells out to
the real `git` binary and returns raw stdout/stderr/exit-code. Every consumer built on it —
`internal/gitrepo` foremost — has to interpret git's *human-readable* output to decide what
happened: substring-matching stderr for known rejection shapes (`"non-fast-forward"`,
`"rejected"`, `"fetch first"`), matching exact English phrases for special states
(`"ambiguous argument 'HEAD'"`, `"no rebase in progress"`), and reading porcelain command output
(`diff --name-only`) that needs `-z`/`--no-renames` flags to avoid C-style path quoting and
folded rename entries.

`gitrepo`'s crucible hardening run surfaced concrete bugs traceable to exactly this: SHA-shaped
arguments that could be interpreted as git options if unvalidated (a caller-controlled string
landing in an option position), non-ASCII filenames coming back C-quoted from a porcelain diff
command, and an explicit, documented assumption that git's messages are never translated (true
today — this machine has no git `.mo` catalogs installed — but not something the code can prove,
only assume). None of these are `gitrepo`-specific design mistakes; they are the structural cost
of using a subprocess-plus-text-parsing interface instead of a typed one.

## What a native Go git library would and would not fix

A pure-Go git implementation (e.g. `go-git`) returns typed Go values and typed errors instead of
text a caller has to parse, which would eliminate the whole "parse stderr as an API" bug class:

- SHA-argument injection — gone, because there is no command-line argument position for a
  caller-controlled string to land in.
- Non-ASCII / special-character path mangling — gone, diff/status results come back as real Go
  strings, not text a porcelain command has to be flag-tuned to keep verbatim.
- The locale/translated-message assumption — gone, there is no human-facing text to depend on.

It would **not** fix the other bug class the same hardening run found, because those bugs are
inherent to the *problem*, not to how the client talks to git:

- `SetSnapshotSHA`'s N-way adopt-on-conflict race (concurrent writers racing a fast-forward-only
  push to the same ref) is a property of git's own server-side ref-update semantics under
  concurrent writers — the same retry/adopt logic would be needed talking to any backend.
- `StageAndCommit`'s pathspec-scoping contract (commit exactly the listed files, never sweep in
  an unrelated pre-staged entry) is a property of the API `gitrepo` chose to expose, not of the
  transport underneath it.

## Known costs of switching

- `go-git`'s rebase support is notably weaker than real git's — a mechanism `gitrepo.Push`'s
  single-retry recovery path currently depends on (`pull --rebase` / `rebase --abort`).
- `gitexec` is not `gitrepo`-only: ~80 call-sites across the codebase depend on it today. A
  switch would be a large, cross-cutting migration, not a contained change inside one module.
- Real git behavior a pure reimplementation can miss or diverge on at the edges: hooks (the
  crucible harness itself exercised a real `pre-receive` hook in one probe), credential helpers,
  includes/gitattributes, and large-repo performance characteristics.

## Recommendation if this is ever picked up

Scope it as its own evaluation — likely starting with a narrow subset of `gitexec`'s surface
(e.g. just the read-only plumbing `gitrepo` uses: `rev-parse`, `diff --name-only`, ref reads) —
rather than a wholesale swap. Do not fold this into any single module's hardening pass; it is a
cross-cutting backend decision, not a bugfix.
