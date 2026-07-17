# Batch: measurement-and-writeup

```yaml
task: Extend codeintel lookup to non-Go languages via LSP
batch: measurement-and-writeup
number: 4
cards: 1
verify: go test -tags integration ./internal/codeintelengine/...
depends-on: [3]
```

## Batch Scope

Runs #008's references precision/cost measurement through the new `lyx codeintel refs` verb across the
agreed matrix — Go/gopls (parity), Python/pyright, Python/pylsp, C#/csharp-ls — and writes the honest
findings to `docs/research/codeintel-multilang.md`. This batch produces no new Go code; its deliverable
is the research write-up plus the raw measurement JSON under `.scratch/codeintel/` (gitignored, not
committed, exactly as #008 did). Because Card 17 installs `gopls` (no sudo), this batch is also where
the shipped live-server integration test (batch 2 Card 12) first becomes runnable — so
`verify: go test -tags integration ./internal/codeintelengine/...` runs it here (it `t.Skip`s cleanly
if `gopls` is somehow still absent). The measurement's own precision numbers are verified by
hand-established ground truth recorded in the write-up, not by a Go assertion.

**Toolchain reality (this dev machine).** Only `go` and `python3` are installed; Ubuntu 26.04 strips
`ensurepip`. The operator has approved sudo installs on request. Server installs:
- `gopls` — `go install golang.org/x/tools/gopls@latest` (no sudo).
- `pyright` — needs node: `sudo apt install -y nodejs npm && sudo npm install -g pyright`.
- `pylsp` — needs pip: `sudo apt install -y python3-pip && pip install --user python-lsp-server`
  (or the no-sudo `curl https://bootstrap.pypa.io/get-pip.py | python3 - --user` bootstrap, then
  `pip install --user python-lsp-server`).
- `csharp-ls` — needs a .NET SDK: `sudo apt install -y dotnet-sdk-8.0 && dotnet tool install --global csharp-ls`.

**Graceful degradation (batch-local decision).** The implementer installs `gopls` unconditionally
(no sudo) and runs the Go parity arm. For each of pyright / pylsp / csharp-ls: if the server binary is
present on `$PATH` (operator pre-installed, or a no-sudo bootstrap succeeded), run and tabulate that
arm; if absent, record that arm in the write-up as **"pending operator install"** with the exact
install command, rather than blocking the task. The write-up is structured to be honest about which
arms actually ran versus which are pending — a partial-but-honest measurement is the correct outcome
when a toolchain the operator has not installed is unavailable, mirroring #008's handling of the
CC-native LSP arm as "characterized, not measured live."

## Cards

### Card 17: run the multi-language measurement and write it up

- **Context:**
  - `docs/research/codeintel-spike.md`
  - `internal/codeintelcli/cli.go`
  - `internal/codeintelengine/registry.go`
  - `.gitignore`
- **Edits:** none
- **Creates:**
  - `docs/research/codeintel-multilang.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Run the measurement, then record it in one committed file
  (`docs/research/codeintel-multilang.md`); every raw artifact stays under `.scratch/codeintel/`
  (gitignored — confirm `.gitignore` covers `**/.scratch/`, it does). **Run:** (1) install `gopls` via
  `go install`; for pyright/pylsp/csharp-ls, check binary presence (`command -v`) and install where a
  no-sudo path exists or the operator has provisioned it — otherwise mark that arm pending. (2) Clone
  one mid-size, real, partially-typed target repo per non-Go language into
  `.scratch/codeintel/targets/python/` and `.scratch/codeintel/targets/csharp/` (permissive licence,
  enough fan-in for interesting reference counts; record the exact repo + commit). Go is measured
  against this repo (loomyard) as #008 did. (3) For each available arm, hand-pick 4–5 benchmark symbols
  stressing static-analysis edge cases (mirroring #008's category table), establish ground truth by
  grep + manual false-match exclusion, and run `lyx codeintel refs <file:line:col> --target-dir
  <target> --lang <lang>` (the `file:line:col` form isolates references precision from
  `workspace/symbol` resolution noise); record warm-up vs steady-state cost separately and the reported
  position list; save raw per-run JSON under `.scratch/codeintel/`. **Write up:** author
  `docs/research/codeintel-multilang.md` in #008's house style — verdict up front (does the generalized
  LSP client reproduce gopls parity; how do pyright/pylsp/csharp-ls compare on references
  precision/cost); a cost table (warm-up once + steady-state per query, per server); a precision table
  per benchmark symbol vs ground truth (false-neg / false-pos columns); per-language honesty notes
  (especially the pyright-vs-pylsp precision spread within Python, and the mature-Roslyn/C# vs
  fuzzy-Python contrast); the exact target repos + commits; a caveats section (single machine,
  hand-picked symbols, order-of-magnitude numbers). Any arm whose server was not installed is recorded
  as **"pending operator install"** with its exact install command and the benchmark plan ready to run,
  not silently omitted. Cross-link the in-tree `docs/modules/codeintel.md` and
  `docs/research/codeintel-spike.md`; refer to `docs/modules/websterv2_extension.md` (the origin doc,
  which lives on `main`, not in this worktree) by name in prose rather than as a relative link that
  would dangle at this branch's HEAD. The single commit carries only the write-up (the `.scratch`
  artifacts are gitignored).
- **Commit:** `docs(codeintel): multi-language references measurement + write-up`

## Batch Tests

`verify: go test -tags integration ./internal/codeintelengine/...` — Card 17 installs `gopls`, so
this batch is the first point at which batch 2 Card 12's live-gopls integration test is runnable; the
integration verify exercises it end-to-end (and `t.Skip`s cleanly if `gopls` is absent). The
measurement's own precision numbers are not a Go assertion — they are hand-established ground truth
(grep + manual false-match exclusion) recorded in the write-up's precision table, exactly as #008
verified its own measurement. The correctness of the shipped code is otherwise covered by batches 1–3's
untagged tests.
