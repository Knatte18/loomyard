# Batch: quarry-scaffold

```yaml
task: "Extract scout into its own standalone repo"
batch: "quarry-scaffold"
number: 1
cards: 5
verify: go -C /home/knatte/Code/quarry/wts/quarry test ./internal/...
depends-on: []
```

## Batch Scope

This batch turns an empty clone into a Go module that compiles and has green tests, without any scout code in it yet.
It writes the repo scaffolding (`go.mod`, `.gitignore`, `LICENSE`, `README.md`), copies the three leaf shared packages verbatim (`lock`, `proc`, `output`), and lands the four research/benchmark docs plus the `servers.yaml` example.
It is one batch because every card is a file-level copy or a short new file with no cross-card design coupling, and together they are the precondition for anything else compiling.

The external interface batch 2 consumes is: a module named `github.com/Knatte18/quarry` at Go 1.26, with `github.com/Knatte18/quarry/internal/output` importable.

Batch-local decision: the leaf packages are copied with a plain filesystem copy, not through the port program.
They import nothing from Loomyard (`lock` imports only `github.com/gofrs/flock`;
`proc` and `output` import only stdlib), so there is no import path to rewrite and no reason to make batch 3's port program a dependency of this batch.

The first commit this batch makes in the quarry worktree is quarry's initial-import commit and must name the Loomyard source commit `1fda8a01c13ec3ec7bb4ef056e5ec9d8aaaac5be` in its message, per the `license-apache-2-carried-over` and `history-not-preserved` decisions.

## Cards

### Card 1: quarry module scaffolding — go.mod, .gitignore, LICENSE

- **Context:**
  - `go.mod`
  - `LICENSE`
- **Edits:** none
- **Creates:**
  - `/home/knatte/Code/quarry/wts/quarry/go.mod`
  - `/home/knatte/Code/quarry/wts/quarry/.gitignore`
  - `/home/knatte/Code/quarry/wts/quarry/LICENSE`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Write quarry's `go.mod` declaring `module github.com/Knatte18/quarry` and `go 1.26`, with direct requires for `github.com/spf13/cobra`, `gopkg.in/yaml.v3`, and `github.com/gofrs/flock` at the same versions Loomyard pins (`v1.10.2`, `v3.0.1`, `v0.8.1` respectively — read them from Loomyard's own `go.mod`, do not guess).
  Copy Loomyard's `LICENSE` byte-for-byte to the quarry worktree.
  Write a `.gitignore` covering Go build output and the coverage-profile artifacts the done gate cleans up: `/quarry`, `*.test`, `*.test.exe`, `*.out`, `*.prof`, and `.scratch/`.
  Add no fourth direct external dependency.
  This card's commit is quarry's first commit and its message must name the Loomyard source commit SHA `1fda8a01c13ec3ec7bb4ef056e5ec9d8aaaac5be` as the origin of the imported code.
  Run it as `git -C /home/knatte/Code/quarry/wts/quarry add …` followed by `git -C /home/knatte/Code/quarry/wts/quarry commit -m …`;
  do not run `cd` into that worktree.
- **Commit:** `chore(quarry): initial import scaffolding from loomyard 1fda8a01`

### Card 2: quarry README with the three mandated statements

- **Context:**
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `/home/knatte/Code/quarry/wts/quarry/README.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Write quarry's `README.md`: what the tool is (LSP-backed code intelligence over five languages), the four verbs (`refs`, `definition`, `symbol`, `assert-no-callers`), and how to build and run it.
  It must carry three statements the `supported-platforms` and `toolchain-cache-is-a-third-axis-and-stays-engine-derived` decisions mandate, each as its own clearly-headed paragraph:
  (a) the supported platform set is linux and windows, with darwin explicitly out because `internal/proc` has no darwin implementation;
  (b) on windows the supervised daemon strategy is unavailable — the daemon hard-codes a Unix socket — so windows works via the native `gopls -remote=auto` strategy only, which is the documented fallback, not an identical path;
  (c) an operator upgrading from `lyx scout` gets one `gopls` re-download, because the toolchain cache segment renamed from `lyx` to `quarry` and that re-keys the cache.
  Also document the config precedence chain (`--config` -> `$QUARRY_CONFIG` -> `os.UserConfigDir()/quarry/servers.yaml` -> built-ins) and the state precedence chain (`--state-dir` -> `$QUARRY_STATE_DIR` -> `os.UserCacheDir()/quarry/<workspace-key>/`), and point at the example config file this batch's card 4 lands.
  State the test tiers: `go test ./...` and `go test -tags lsp ./...`.
- **Commit:** `docs(quarry): README with platform set, windows caveat, and cache re-key note`

### Card 3: copy the three leaf shared packages verbatim

- **Context:**
  - `internal/lock/lock.go`
  - `internal/lock/lock_test.go`
  - `internal/output/output.go`
  - `internal/output/output_test.go`
  - `internal/proc/isalive_test.go`
  - `internal/proc/killpid_test.go`
  - `internal/proc/proc_linux.go`
  - `internal/proc/proc_linux_test.go`
  - `internal/proc/proc_windows.go`
  - `internal/proc/proc_windows_test.go`
- **Edits:** none
- **Creates:**
  - `/home/knatte/Code/quarry/wts/quarry/go.sum`
  - `/home/knatte/Code/quarry/wts/quarry/internal/lock/lock.go`
  - `/home/knatte/Code/quarry/wts/quarry/internal/lock/lock_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/internal/output/output.go`
  - `/home/knatte/Code/quarry/wts/quarry/internal/output/output_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/internal/proc/isalive_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/internal/proc/killpid_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/internal/proc/proc_linux.go`
  - `/home/knatte/Code/quarry/wts/quarry/internal/proc/proc_linux_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/internal/proc/proc_windows.go`
  - `/home/knatte/Code/quarry/wts/quarry/internal/proc/proc_windows_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Copy each listed source file to the corresponding quarry path with `cp`, byte for byte.
  These three packages import nothing from Loomyard — `lock.go:12` imports `github.com/gofrs/flock`, and `proc` and `output` are stdlib-only — so no import path and no package clause changes.
  After copying, prove that with `grep -rl 'Knatte18/loomyard' /home/knatte/Code/quarry/wts/quarry/internal/` returning nothing.
  If any file does contain a `loomyard` import, stop and report rather than hand-rewriting it — the leaf classification in the `dependency-strategy-copy-vs-replace` decision would be wrong and the plan needs revisiting.
  Then run `go -C /home/knatte/Code/quarry/wts/quarry mod tidy` to generate `go.sum`, and confirm the resulting `go.mod` still lists exactly the three direct requires from card 1.
  Note that `proc_windows.go` will not compile on this linux machine;
  it is GOOS-guarded by its filename suffix, so `go test ./internal/...` never builds it here, and that is expected.
- **Commit:** `feat(quarry): copy lock, proc, and output leaf packages verbatim`

### Card 4: land the research docs and the servers.yaml example

- **Context:**
  - `docs/research/scout-spike.md`
  - `docs/research/scout-multilang.md`
  - `docs/research/scout-agent-usage-findings.md`
  - `docs/benchmarks/scout-vs-grep.md`
  - `internal/scoutengine/template.yaml`
- **Edits:** none
- **Creates:**
  - `/home/knatte/Code/quarry/wts/quarry/docs/scout-spike.md`
  - `/home/knatte/Code/quarry/wts/quarry/docs/scout-multilang.md`
  - `/home/knatte/Code/quarry/wts/quarry/docs/scout-agent-usage-findings.md`
  - `/home/knatte/Code/quarry/wts/quarry/docs/scout-vs-grep.md`
  - `/home/knatte/Code/quarry/wts/quarry/docs/servers.yaml.example`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Copy the four research/benchmark documents into quarry's `docs/`, keeping their filenames.
  Then fix every relative markdown link inside them that pointed at a Loomyard path — a link to a sibling that also moved becomes a plain relative link within quarry's `docs/`, and a link to a Loomyard file that stayed becomes an absolute `https://github.com/Knatte18/loomyard/blob/main/<path>` URL.
  Keep every recorded measurement and every statement that the benchmark corpus was the Loomyard checkout — that is a true fact about the benchmark target and must not be reworded into a claim about quarry's own tree.
  Copy the content of Loomyard's `internal/scoutengine/template.yaml` to `docs/servers.yaml.example` and reword its operator-visible prose for quarry: its opening comment currently says the file "is never generated or overwritten by lyx itself", which must become a statement about quarry, and the comment must tell the operator that the file's location is the `--config`/`$QUARRY_CONFIG`/`os.UserConfigDir()/quarry/servers.yaml` chain rather than a lyx hub path.
  The example file is plain documentation — write no Go accessor and no `//go:embed` directive for it, since `config-template-is-dropped-not-ported` drops `ConfigTemplate()` entirely.
  After the rewrite, `grep -ric 'lyx' /home/knatte/Code/quarry/wts/quarry/docs/servers.yaml.example` must return 0.
- **Commit:** `docs(quarry): move scout research docs and add servers.yaml example`

### Card 5: open the port log in the task worktree

- **Context:**
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `docs/research/quarry-port-log.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `docs/research/quarry-port-log.md` in **this** worktree — not in quarry — as the running record of the extraction.
  Give it a short header explaining that it is a transient port record deleted by this task's final batch, and a `## Batch 1 — quarry-scaffold` section listing what landed in quarry (the scaffolding files, the three leaf packages, the docs) together with the quarry commit SHAs produced by cards 1 through 4, obtained via `git -C /home/knatte/Code/quarry/wts/quarry log --oneline`.
  Every later batch appends its own `## Batch N` section to this file;
  the file exists so each quarry-side batch also produces a commit in this worktree.
  Commit it here with an ordinary `git add`/`git commit` in the task worktree.
- **Commit:** `docs: open the quarry port log`

## Batch Tests

`verify:` runs `go -C /home/knatte/Code/quarry/wts/quarry test ./internal/...`, which builds and tests exactly the three packages card 3 creates: `internal/lock` (`lock_test.go`), `internal/output` (`output_test.go`), and `internal/proc` (`isalive_test.go`, `killpid_test.go`, `proc_linux_test.go`).
It is scoped to `./internal/...` rather than `./...` because no other package exists yet, and this spelling stays correct at the batch boundary regardless of what a later batch adds.
The three suites are ported unchanged, so a failure here means the copy itself is wrong — a truncated file or a missed dependency — not a behavioural difference.

Cards 1, 2, 4, and 5 have no runnable surface;
they are verified by the greps their `Requirements:` name (no surviving `loomyard` import, no surviving `lyx` token in the example config) plus `go mod tidy` succeeding in card 3.
