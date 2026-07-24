# Multi-language references measurement — `lyx codeintel refs` over LSP

**Task:** `codeintel-multilang`. **Date:** 2026-07-17. **Machine:** Linux x86_64, 12 cores,
30GiB RAM (same host as the [codeintel spike](codeintel-spike.md), #008). **Toolchain:**
`go version go1.26.0 linux/amd64`, `golang.org/x/tools/gopls v0.23.0` (already on `$PATH`
from an earlier batch's `go install`; re-run here as Card 17 requires — no-op, confirms the
pin), `pylsp v1.14.0` / Python 3.14.4 (installed into a throwaway venv — see **Toolchain
notes** below, no sudo used). Measured against `internal/codeintelcli`'s shipped `lyx
codeintel refs <file:line:col> --target-dir <dir> --lang <lang>` verb, built from this
branch at commit `cb12deb472652308f79145baa65452c070c9b2bc`.

## Verdict up front

- **Go/gopls: full parity with gopls's own view — ADOPT, with one honest, quantified gap.**
  Every one of 5 hand-picked benchmark symbols (mirroring #008's category table) returned
  **zero false negatives and zero false positives** against gopls's own default-build-tag
  view. But this repo leans heavily on `//go:build integration`/`//go:build smoke` tags for
  its Test Tier Purity Invariant, and the shipped engine has **no way to pass `-tags`
  through to the language server** (`lspclient.go`'s `initialize` sends no
  `initializationOptions`/`buildFlags`) — so every reference inside a tagged test file is
  invisible to `lyx codeintel refs` on this repo today, by design of gopls's own default
  scope, not a precision bug. Quantified below: **40% of this repo's true call sites** to
  the two most heavily-tested benchmark symbols live behind that tag boundary. This is a
  real, load-bearing limitation for exactly the kind of repo this tool ships in, and is
  worth a follow-up card (thread a `--build-tags` flag through to the server's
  `initializationOptions`) even though it is out of this batch's scope.
- **Python/pylsp: severe, uneven recall — CHARACTERIZE AS RISKY, not a recommended default.**
  Across the same 5-symbol-shaped benchmark on a real, mid-size, partially-typed target
  (`psf/requests`), pylsp found **27 of 73** true references (37% recall) with **zero false
  positives** — it is conservative, never fabricating a wrong reference, but it silently
  drops the majority of real call sites, and the drop rate is wildly uneven per symbol (0%
  on one symbol, 100% on another) rather than following a clean, explainable rule the way
  the Go arm's build-tag gap does. A caller trusting `lyx codeintel refs` output on a
  Python target today would systematically undercount impact, often severely.
- **Python/pyright: pending operator install — recorded, not measured.** No sudo path
  exists for `pyright` in this sandbox (needs `nodejs`/`npm`, both requiring `apt`); ground
  truth and benchmark positions are already established against the same target/commit
  above, so running the pyright arm the moment an operator installs it is a single command,
  not a fresh investigation.
- **C#/csharp-ls: pending operator install — recorded, not measured.** No sudo path exists
  for a .NET SDK in this sandbox either. A target repo is already cloned and 3 benchmark
  positions picked (interface, high-fan-in async method, fluent builder method), ready to
  run.
- **Cost: every arm comfortably inside the "cheap relative to an LLM turn" bar, gopls
  faster than pylsp by roughly 3-6x.** Both are one LSP round trip inside one spawn-per-
  invocation process (the shipped CLI has no held-open server across queries — see **Cost
  table** below for what "warm-up vs steady-state" means in that shape), landing in the
  **hundreds of milliseconds**, not seconds.

## Relationship to the spike and the module doc

This doc is the trade-off record the `internal/codeintelengine` package documentation points
to when it says the LSP-generalized design "trad[es] the spike's sub-millisecond in-process
query cost for one LSP round trip per query" — see that package documentation for the shipped
design (engine/CLI split, typed errors, registry) and [`codeintel-spike.md`](codeintel-spike.md) (#008) for the
Go-only `go/packages`/`go/types` numbers this doc's Go/gopls arm is being compared against.
The original design reasoning that motivated generalizing lookup beyond Go in the first
place lives in `manifest/modules/websterv2.md` — named here in prose rather than as a
link, because that doc lives on `main`, not this task branch, and a relative link here
would dangle at this branch's HEAD.

## Method

Every arm was driven through the shipped verb itself — `lyx codeintel refs
<file:line:col> --target-dir <target> --lang <lang> --timeout 60s` — never a bespoke
harness, so these numbers are exactly what an operator gets. The `file:line:col` form was
used throughout (never a bare symbol name), per Card 17's requirement: it isolates
references precision from `workspace/symbol` resolution noise entirely. Each symbol was
queried **3 times** in a row (fresh process each time — the CLI spawns, initializes,
queries, and tears the server down within a single invocation, with no cross-invocation
server reuse); run 1 is reported as **warm-up**, runs 2–3 as **steady-state**. Raw
per-run JSON (every invocation's full envelope + wall-clock duration) is under
`.scratch/codeintel/runs/{go-gopls,python-pylsp}.jsonl`; the hand-established ground-truth
breakdown per symbol (including the exact false-negative file:line list) is under
`.scratch/codeintel/ground-truth.json` — both gitignored (`**/.scratch/`), not part of this
commit, exactly as #008 kept its raw JSON out of the doc commit.

**Ground truth** was established by hand per symbol: `grep`ing every occurrence of the
symbol's name across the target (both production and test code, this time — unlike #008's
Go-only, `Tests: false` in-process harness, gopls and pylsp both default to indexing test
files, so "ground truth" here means the true reference set a user would expect, tests
included), then manually excluding false grep matches (comments, docstring examples,
string literals, and — critically for the Python arm — same-named-but-distinct methods on
different types, e.g. `Request.prepare` vs `PreparedRequest.prepare`, or `Session.request`
vs the module-level `api.request`). Every exclusion is recorded inline below and in
`ground-truth.json`'s per-symbol `note` field.

## Target repos and commits

| Language | Repo | Commit | Licence | Size (loaded `.py`/`.go`/`.cs` lines) |
|---|---|---|---|---:|
| Go | this repo, `github.com/Knatte18/loomyard` (`codeintel-multilang` branch) | `cb12deb472652308f79145baa65452c070c9b2bc` | — (in-tree) | ~47 `internal/` packages, same corpus #008 used |
| Python | [`psf/requests`](https://github.com/psf/requests) | `f361ead047be5cb873174218582f7d8b9fcd9f49` (2026-07-09) | Apache-2.0 | 6,874 (`src/`, excl. tests) |
| C# (pending) | [`restsharp/RestSharp`](https://github.com/restsharp/RestSharp) | `6a5082169257438cd085f822f050d93256a8e499` (2026-06-02) | Apache-2.0 | 22,378 |

Both non-Go targets are cloned (shallow, `--depth 1`) under `.scratch/codeintel/targets/`
(gitignored, not part of this commit) — real, permissively-licensed, partially-typed
projects with enough fan-in for interesting reference counts, per Card 17's requirements.
`requests` uses type hints sparingly (a handful of modern annotations layered onto a
historically-untyped codebase — genuinely "partially typed"); `RestSharp` is a mainstream,
actively-maintained .NET HTTP client library.

## Benchmark symbols

Chosen to mirror #008's 5-category table, adapted per language (Python has no
Go-style static interface satisfaction, so that category becomes duck-typed dynamic
dispatch; "generics" becomes a thin **kwargs-forwarding wrapper chain, the closest
Python-native analog to a generic-instantiation static-analysis stress case).

### Go (`lyx codeintel refs`, `--lang go`, gopls)

| # | Category | Symbol | Position |
|---|---|---|---|
| 1 | High fan-in plain function | `hubgeometry.Resolve` | `internal/hubgeometry/hubgeometry.go:101:6` |
| 2 | Method with many call sites | `hubgeometry.Layout.WeftWorktree` | `internal/hubgeometry/hubgeometry.go:481:18` |
| 3 | Generics | `state.ReadJSON[T]` | `internal/state/state.go:49:6` |
| 4 | Reflection-adjacent / negative case | `output.Err` | `internal/output/output.go:32:6` |
| 5 | Interface satisfaction | `shuttleengine.Engine.Prepare` | `internal/shuttleengine/engine.go:148:2` |

Same symbols #008 used (1–5 above map onto #008's identical categories, #3 and #5 are the
exact same functions), so the Go arm's numbers are a direct apples-to-apples comparison to
the spike's own `gopls-refs` arm, not just a new measurement.

### Python (`lyx codeintel refs`, `--lang python-pylsp`, pylsp, target: `requests`)

| # | Category (Python analog) | Symbol | Position |
|---|---|---|---|
| 1 | High fan-in plain function | `_internal_utils.to_native_string` | `src/requests/_internal_utils.py:26:5` |
| 2 | Method with many call sites | `PreparedRequest.prepare` | `src/requests/models.py:424:9` |
| 3 | kwargs-forwarding wrapper chain (generics analog) | `api.request` (wrapped by `get`/`post`/…) | `src/requests/api.py:24:5` |
| 4 | Reflection-adjacent / negative case | `Response.json` | `src/requests/models.py:1091:9` |
| 5 | Duck-typed dynamic dispatch (interface analog) | `Session.send` (dispatches to `HTTPAdapter.send` via `self.get_adapter(url)`) | `src/requests/sessions.py:752:9` |

**`--lang python-pylsp` note:** the shipped `builtins()` registry only defines a `python`
alias pointed at `pyright-langserver` (unavailable in this sandbox). Running the pylsp arm
used the registry's own designed extension point — a `_lyx/config/servers.yaml` overlay
adding a second alias, `python-pylsp`, whole-replacing nothing (it is an *additional* key,
not an override) and pointing `command` at the venv-installed `pylsp` binary — created for
the duration of this measurement and removed afterward (not part of this commit; the
registry mechanism itself, `LoadRegistry`'s whole-entry overlay, already shipped in batch
1). This is the exact "operator adds a server without a recompile" use case
the `internal/codeintelengine` package documentation describes, not a side channel.

## Cost table (warm-up = run 1, steady-state = median of runs 2–3, ms; n=3 per symbol)

| arm | warm-up (range across symbols) | steady-state (range across symbols) |
|---|---:|---:|
| `lyx codeintel refs --lang go` (gopls, this repo) | 186–260 | 177–283 |
| `lyx codeintel refs --lang python-pylsp` (pylsp, `requests`) | 566–1443 | 374–1113 |

**Warm-up and steady-state are close, not because either server is fast to "warm" but
because the shipped CLI has no held-open state to warm** — every invocation spawns a fresh
subprocess, does `initialize`/`initialized`, issues one `textDocument/references`, then
runs the graceful `shutdown`/`exit` handshake and exits. This is architecturally identical
to #008's `gopls-cli-refs` arm ("every call pays the load … 'warm-up' and 'steady-state'
converge"), generalized to every language this engine supports — not a regression from
that spike's `gopls-refs` held-open arm (175–392ms warm-up, 3.29–19ms steady-state), which
this shipped CLI deliberately does not implement (no daemon, no long-lived server, per this
task's scope). **Both arms still land in the hundreds of milliseconds**, comfortably inside
the "a few seconds" rubric ceiling and "effectively free relative to an LLM turn," but
pylsp is consistently the slower server — roughly 3–6x gopls's wall clock per call, almost
entirely Python/jedi's own interpreter and analysis startup cost, not this engine's
overhead (the LSP framing and process-spawn machinery is identical between arms).

## Precision table (per benchmark symbol vs. hand-verified ground truth)

### Go/gopls (ground truth scoped to gopls's own default build-tag view — see verdict)

| symbol | ground truth (default view) | found | false neg | false pos |
|---|---:|---:|---:|---:|
| `hubgeometry.Resolve` | 26 | 26 | 0 | 0 |
| `hubgeometry.Layout.WeftWorktree` | 21 | 21 | 0 | 0 |
| `state.ReadJSON` | 14 | 14 | 0 | 0 |
| `output.Err` | 181 | 181 | 0 | 0 |
| `shuttleengine.Engine.Prepare` | 11 | 11 | 0 | 0 |

**The build-tag gap, quantified separately** (not counted as false negatives above, since
it is gopls's own default scope, not a resolution failure): `hubgeometry.Resolve` has 42
additional real call sites (68 total vs the 26 above) and `hubgeometry.Layout.WeftWorktree`
has 17 more (38 total vs 21) living inside `//go:build integration`/`//go:build smoke`
test files — this repo's Test Tier Purity Invariant tags almost every `warpengine`,
`hubgeometry`, and `initengine` test file this way. `state.ReadJSON` and `output.Err` have
**zero** tagged call sites, which is exactly why their found-vs-ground-truth numbers need
no asterisk. Confirmed mechanistically, not just by count match: every one of the 42/17
"missing" call sites' files carry the build tag; every found call site's file does not.
Ruled out as a race/timing artifact first (see **Go per-language honesty notes**).

### Python/pylsp (target: `requests`)

| symbol | ground truth | found | false neg | false pos |
|---|---:|---:|---:|---:|
| `_internal_utils.to_native_string` | 16 | 14 | 2 | 0 |
| `api.request` | 8 | 8 | 0 | 0 |
| `PreparedRequest.prepare` | 7 | 2 | 5 | 0 |
| `Response.json` | 21 | 1 | 20 | 0 |
| `Session.send` | 21 | 2 | 19 | 0 |
| **total** | **73** | **27** | **46 (63%)** | **0** |

## Go per-language honesty notes

**Ruling out a workspace-load race before attributing the gap to build tags.** The first
hypothesis for `hubgeometry.Resolve`'s low count (26 found vs. 68 naive grep) was that the
shipped client fires `textDocument/references` immediately after `initialize`/`initialized`
with no wait for gopls's asynchronous workspace metadata load to finish on a ~47-package
module — a real architectural risk of the spawn-once-query-once-shutdown shape. This was
tested directly with a throwaway probe script (held the gopls connection open, inserted an
explicit 0s/3s/5s/10s/15s delay between `initialized` and the `references` call): **the
count was 26 at every delay, including 0s**, ruling out a race. The actual cause —
confirmed by cross-referencing every "missing" call site's file against its `//go:build`
line — is that all 42 missing `Resolve` call sites and all 17 missing `WeftWorktree` call
sites live in `integration`/`smoke`-tagged files gopls's default (no extra `-tags`) view
never loads, the same way `go build ./...` without `-tags integration` wouldn't either.
**This is a real, user-facing limitation of the shipped engine** (no `-tags` passthrough
in `lspclient.go`'s `initialize` call), not a gopls precision defect — worth a follow-up
card, out of this batch's scope.

**Interface unification reproduces #008's own finding exactly.** Querying
`shuttleengine.Engine.Prepare` (the interface method) returned all 11 entries the spike
predicted: the interface declaration, the one real production dispatch call
(`internal/shuttleengine/run.go:117`, through the interface), and all 9 test-file calls
that dispatch the concrete `claudeengine.Claude.Prepare` directly by name
(`prepare_test.go`, `settings_test.go`) — gopls unifies interface-method references with
every known implementer's references into one set, exactly as #008 documented. The
concrete method's own definition (`claudeengine.go:67`) is correctly excluded (a distinct
`types.Object`, not a reference to the interface method).

## Python per-language honesty note — the pyright-vs-pylsp spread is pending, but pylsp's own spread is already damning

Card 17 asked specifically for "the pyright-vs-pylsp precision spread within Python."
**Only pylsp's side of that comparison could be measured** in this sandbox (pyright needs
`nodejs`/`npm` via `apt`, unavailable without sudo). What *is* measured is stark on its
own: pylsp's recall on the same 5-symbol-shaped benchmark ranges from **0% missed**
(`api.request`, 8/8) to **100% missed** (`Response.json`, 0/20 real call sites found,
declaration only) — and the pattern does not reduce to one clean rule the way the Go arm's
build-tag gap does:

- Every one of `Response.json`'s 20 real call sites lives in `tests/`, and pylsp missed all
  20 — consistent with "pylsp under-indexes the test directory."
- But `to_native_string` also has 2 of its 16 references in `tests/test_utils.py`, and
  pylsp missed exactly those 2 and found everything else (14/14 non-test references) —
  also consistent with that same theory.
- `PreparedRequest.prepare`, though, was missed even for a **same-package, non-test,
  production** call site (`sessions.py:541`) that has the identical
  `p = PreparedRequest(); p.prepare(...)` shape as the one call pylsp *did* find
  (`models.py:363`, inside `Request.prepare`'s own body) — ruling out "just a test-file
  problem" as the whole story.
- `Session.send` was missed even for a call in the **very same file** as its own
  declaration (`sessions.py:292`, inside `Session.request`'s body) while a different
  same-file call three lines of logic away (`sessions.py:651`) *was* found.

**Honest conclusion: pylsp/jedi's reference search has a real, load-bearing scope limit on
a real project this size, but it is not a limit this measurement can fully characterize as
one clean rule** the way gopls's build-tag gap is — it looks like a bounded or lazily-built
project-wide index that inconsistently covers cross-file and even some same-file usages,
not a documented, single-cause scoping rule. **Zero false positives across every symbol**
is the one clean, positive finding: pylsp never fabricated a reference, including two
genuine same-name-collision traps (`Request.prepare` vs `PreparedRequest.prepare`,
`Session.request` vs the module-level `api.request`) it navigated correctly on the symbols
it did find. A caller can trust what pylsp reports; it cannot trust what pylsp omits.

**Mature-Roslyn/C# vs fuzzy-Python contrast — characterized, not measured.** `csharp-ls`
wraps Roslyn, the same static-typed, fully-resolved compiler front end Visual Studio uses;
prior public precision comparisons of Roslyn-backed tooling against Python's
duck-typed/dynamically-resolved static analysis (jedi, and to a lesser extent pyright's own
type-inference engine) consistently favor Roslyn for the reason this measurement's
Python numbers illustrate directly: Python's references resolution depends on data-flow
inference through untyped or partially-typed code (e.g. "what type does `self.get_adapter(url)`
return, three calls deep, with no annotation") in a way C#'s nominal, statically-checked
type system does not need to guess at. This measurement cannot put a number on that
contrast without a working `csharp-ls` install — flagged as **pending operator install**,
not silently assumed.

## Pending arms — install commands and ready benchmark plans

| Arm | Status | Install command | Target (already cloned + commit-pinned) | Benchmark positions (ready) |
|---|---|---|---|---|
| Python/pyright | pending | `sudo apt install -y nodejs npm && sudo npm install -g pyright` | `psf/requests @ f361ead0…` (same as pylsp above) | reuse the 5 positions in the **Python** benchmark table above verbatim — same file:line:col, same target, so the two servers are directly comparable |
| C#/csharp-ls | pending | `sudo apt install -y dotnet-sdk-8.0 && dotnet tool install --global csharp-ls` | `restsharp/RestSharp @ 6a508216…` | `IRestClient` interface decl (`src/RestSharp/IRestClient.cs:20`, interface-satisfaction category); `RestClient.ExecuteAsync` (`src/RestSharp/RestClient.Async.cs:26`, high-fan-in async method); `RestRequest.AddParameter` (`src/RestSharp/Request/RestRequest.cs:254`, fluent-builder method with many call sites) |

Both install commands need `sudo`, which this sandbox could not run interactively; the
pylsp arm above found a genuine no-sudo path (see **Toolchain notes**) but neither of these
two does — `nodejs`/`npm` and a .NET SDK are both `apt`-packaged with no viable no-sudo
tarball-bootstrap attempted here, matching this card's graceful-degradation instruction to
record a real install command and a ready plan rather than block the task or silently omit
the arm.

## Toolchain notes — how each server got installed in this sandbox

- **gopls**: `go install golang.org/x/tools/gopls@latest` — no sudo, exactly the built-in
  registry's `InstallHint`. Already present on this machine from an earlier batch; re-run
  here per Card 17, a no-op confirming the same `v0.23.0` pin.
- **pylsp**: this sandbox's Python 3.14 has no `ensurepip` (Ubuntu 26.04 strips it) and
  `python3 -m pip` / `python3 -m venv` both fail without it. The no-sudo path that worked:
  `python3 -m venv --without-pip <venv>` (creates an interpreter + venv layout with no pip
  bootstrap attempt, which is what fails without `ensurepip`), then
  `<venv>/bin/python3 get-pip.py` (the `get-pip.py` bootstrap script bundles its own pip
  wheel and does not need `ensurepip`), then `<venv>/bin/pip install python-lsp-server`.
  Zero `sudo` calls. This is the same no-sudo bootstrap the batch scope named as a fallback
  to the normal `sudo apt install -y python3-pip && pip install --user python-lsp-server`
  path.
- **pyright / csharp-ls**: not installed — see **Pending arms** above.

## Caveats

One machine, one run of 3 repeats per symbol (enough to distinguish warm-up from
steady-state for a spawn-per-invocation architecture, not a statistically rigorous
benchmark — numbers are order-of-magnitude, matching `docs/benchmarks/` house style and
#008's own caveat). 5 benchmark symbols per language, hand-picked to stress specific
static-analysis edge cases (mirroring #008's category table, adapted per language) rather
than sampled — a different symbol set could surface different edge cases. Ground truth was
hand-verified via `grep` + manual exclusion (documented per-symbol in
`.scratch/codeintel/ground-truth.json`'s `note` fields), not cross-checked against a third
independent tool; the Python arm's disambiguation of same-named methods on different
classes (`Request.prepare`/`PreparedRequest.prepare`, `Session.request`/`api.request`) is
inherently more error-prone by hand than Go's compiler-checked ground truth, though every
exclusion is traceable to a specific line and a stated reason. Only one target repo per
language (this repo for Go, one clone each for Python/C#) — a different-shaped codebase
(different build-tag density, different typing discipline) could shift these numbers
meaningfully, especially for the Go build-tag gap (a repo with no build-tag-gated tests
would show none of it) and the Python recall spread (a fully-typed codebase might behave
differently under pyright's type-inference-driven resolution, still unmeasured here). No
wall-clock comparison against an LLM-driven grep/Read baseline is claimed, matching #008's
own scope boundary.
