# Plan: prowler: installable Claude Code plugin (Go), hosted in LoomYard

```yaml
task: 'prowler: installable Claude Code plugin (Go), hosted in LoomYard'
slug: prowler
approved: false
started: '20260729-193053'
parent: 'main'
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches. Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: fetch-binary
    file: 01-fetch-binary.md
    depends-on: []
    verify: cd plugins/prowler/1.0.0 && go test ./...
  - number: 2
    name: plugin-packaging
    file: 02-plugin-packaging.md
    depends-on: [1]
    verify: bash plugins/prowler/1.0.0/scripts/selftest.sh
```

## Shared Decisions

_Cross-cutting decisions every batch inherits. Full rationale lives in `_mill/discussion.md`; this section restates only what binds implementation._

### Decision: authoritative spec is discussion.md

- **Decision:** `_mill/discussion.md` is the complete behavioral specification. Every card lists it in `Context:`. The Millhouse `weblens` skill is the reference implementation prowler ports, but it lives in a separate repo (`/home/knatte/Code/millhouse/...`) that is a black box to this task — do NOT read or depend on it; every behavioral detail needed (headers, Reddit format, cascade thresholds, chromedp launch args, Chrome candidate paths, timeouts) is restated inline in card `Requirements:` and in the decisions below.
- **Rationale:** the plan must be executable cold, with no cross-repo reads.
- **Applies to:** all batches

### Decision: guard-cleanliness by construction (binding on every prowler `.go` file)

- **Decision:** the parent LoomYard module's three disk-walking grep guards (`cmd/lyx/tierpurity_test.go`, `cmd/lyx/hermeticenv_test.go`, `cmd/lyx/ghguard_test.go`) walk into `plugins/prowler/1.0.0/` even though it is a separate nested module. Therefore: (a) **no prowler `*_test.go` file may contain the raw substrings** `exec.Command`, `exec.CommandContext`, `gitexec.RunGit`, or any `lyxtest.` token — not in code, comments, or string literals (raw-substring match); (b) **no prowler production `.go` file may contain** `LookPath("gh")` or a same-line `exec.Command(…, "gh")`/`exec.CommandContext(…, "gh")`. Any shell-out / subprocess-spawning behavior (the build-lock concurrency check) lives in a **non-`*_test.go` shell script** (`scripts/selftest.sh`), never a Go test. Do NOT edit any guard's skip set.
- **Rationale:** the parent `go test ./...` (the configured done-gate) runs these guards against prowler's committed files; a banned substring fails the done-gate. chromedp spawns Chrome from inside the library, not from prowler test source, so an `//go:build integration` test that drives chromedp is clean (it contains no banned substring).
- **Applies to:** all batches (Go source in batch 1; the shell harness in batch 2)

### Decision: nested Go module boundary

- **Decision:** all Go code is one nested module rooted at `plugins/prowler/1.0.0/` — `module github.com/Knatte18/loomyard/plugins/prowler`, `go 1.26`, `package main` in the module-root directory. It is NOT part of lyx's module; its deps (chromedp, go-readability, goquery) never enter lyx's `go.mod`/`go.sum`. The nested `go.mod`/`go.sum` ARE committed (they are source); the built `bin/` is gitignored.
- **Rationale:** isolates the browser-automation dependency stack from lyx and from `go build/test ./...`.
- **Applies to:** all batches

### Decision: stdout/stderr discipline

- **Decision:** the Go binary prints **only** the single output-file absolute path to stdout; every diagnostic (build progress, errors, lock notices, "Chrome not found", "Go toolchain not found") goes to stderr. `run.sh` preserves this: it forwards the binary's stdout unchanged and routes all its own diagnostics and the child `go build` output to stderr. The skill checks the wrapper exit code before reading a path.
- **Rationale:** `path=$(bash run.sh …)` must capture exactly one clean path line.
- **Applies to:** all batches

### Decision: Apache-2.0 via plugin.json metadata only

- **Decision:** license is declared as the `"license": "Apache-2.0"` field in `plugin.json` (mirroring weblens), plus a note in `plugins/prowler/README.md`. No separate `LICENSE` file is duplicated into the plugin tree — the repo-root `LICENSE` already covers the repository.
- **Applies to:** batch 2

## All Files Touched

- `.claude-plugin/marketplace.json`
- `.gitattributes`
- `.gitignore`
- `plugins/prowler/1.0.0/.claude-plugin/plugin.json`
- `plugins/prowler/1.0.0/browser.go`
- `plugins/prowler/1.0.0/browser_integration_test.go`
- `plugins/prowler/1.0.0/chrome.go`
- `plugins/prowler/1.0.0/chrome_test.go`
- `plugins/prowler/1.0.0/fetch.go`
- `plugins/prowler/1.0.0/fetch_test.go`
- `plugins/prowler/1.0.0/fetcher.go`
- `plugins/prowler/1.0.0/go.mod`
- `plugins/prowler/1.0.0/go.sum`
- `plugins/prowler/1.0.0/headers.go`
- `plugins/prowler/1.0.0/htmltext.go`
- `plugins/prowler/1.0.0/htmltext_test.go`
- `plugins/prowler/1.0.0/main.go`
- `plugins/prowler/1.0.0/main_test.go`
- `plugins/prowler/1.0.0/outfile.go`
- `plugins/prowler/1.0.0/outfile_test.go`
- `plugins/prowler/1.0.0/reddit.go`
- `plugins/prowler/1.0.0/reddit_test.go`
- `plugins/prowler/1.0.0/scripts/run.sh`
- `plugins/prowler/1.0.0/scripts/selftest.sh`
- `plugins/prowler/1.0.0/settings.json`
- `plugins/prowler/1.0.0/skills/INDEX.md`
- `plugins/prowler/1.0.0/skills/prowler/SKILL.md`
- `plugins/prowler/README.md`
