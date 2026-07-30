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
    verify: cd plugins/prowler && go test ./...
  - number: 2
    name: plugin-packaging
    file: 02-plugin-packaging.md
    depends-on: [1]
    verify: bash plugins/prowler/scripts/selftest.sh
```

## Shared Decisions

_Cross-cutting decisions every batch inherits. Full rationale lives in `_mill/discussion.md`; this section restates only what binds implementation._

### Decision: authoritative spec is discussion.md

- **Decision:** `_mill/discussion.md` is the complete behavioral specification. Every card lists it in `Context:`. The Millhouse `weblens` skill is the reference implementation prowler ports, but it lives in a separate repo (`/home/knatte/Code/millhouse/...`) that is a black box to this task — do NOT read or depend on it; every behavioral detail needed (headers, Reddit format, cascade thresholds, chromedp launch args, Chrome candidate paths, timeouts) is restated inline in card `Requirements:` and in the decisions below.
- **Rationale:** the plan must be executable cold, with no cross-repo reads.
- **Applies to:** all batches

### Decision: guard-cleanliness by construction (binding on every prowler `.go` file)

- **Decision:** the parent LoomYard module's three disk-walking grep guards (`cmd/lyx/tierpurity_test.go`, `cmd/lyx/hermeticenv_test.go`, `cmd/lyx/ghguard_test.go`) walk into `plugins/prowler/` even though it is a separate nested module. Therefore: (a) **no prowler `*_test.go` file may contain the raw substrings** `exec.Command`, `exec.CommandContext`, `gitexec.RunGit`, or any `lyxtest.` token — not in code, comments, or string literals (raw-substring match); (b) **no prowler production `.go` file may contain** `LookPath("gh")` or a same-line `exec.Command(…, "gh")`/`exec.CommandContext(…, "gh")`. Any shell-out / subprocess-spawning behavior (the build-lock concurrency check) lives in a **non-`*_test.go` shell script** (`scripts/selftest.sh`), never a Go test. Do NOT edit any guard's skip set.
- **Rationale:** the parent `go test ./...` (the configured done-gate) runs these guards against prowler's committed files; a banned substring fails the done-gate. chromedp spawns Chrome from inside the library, not from prowler test source, so an `//go:build integration` test that drives chromedp is clean (it contains no banned substring).
- **Applies to:** all batches (Go source in batch 1; the shell harness in batch 2)

### Decision: nested Go module boundary

- **Decision:** all Go code is one nested module rooted at `plugins/prowler/` — `module github.com/Knatte18/loomyard/plugins/prowler`, `go 1.26`, `package main` in the module-root directory. It is NOT part of lyx's module; its deps (chromedp, go-readability, goquery) never enter lyx's `go.mod`/`go.sum`. The nested `go.mod`/`go.sum` ARE committed (they are source); the built `bin/` is gitignored.
- **Rationale:** isolates the browser-automation dependency stack from lyx and from `go build/test ./...`.
- **Applies to:** all batches

### Decision: flat plugin layout `plugins/prowler/` (supersedes the discussion's versioned default)

- **Decision:** the plugin source lives at `plugins/prowler/` (not `plugins/prowler/1.0.0/`), and the marketplace entry uses `source: ./plugins/prowler`. The Go nested module, `.claude-plugin/plugin.json`, `skills/`, `scripts/`, `settings.json`, and `README.md` all sit directly under `plugins/prowler/`.
- **Rationale:** the discussion chose a versioned subdir (`plugins/prowler/1.0.0/`) as a deliberate divergence, but made it **conditional**: "do not ship the versioned form unverified," with flat named as the sanctioned fallback. Plan-review round 4 verified against real data that the versioned-subdir local `source` has **zero precedent** — 0 of 276 official-marketplace plugins use a version segment in a local `source`, and weblens itself is `source: ./plugins/weblens` with `version: 1.0.0` (version and source-path are decoupled). Because the autonomous mill-go pipeline cannot run the interactive `/plugin install` verification the discussion required, shipping versioned would mean shipping unverified — exactly what the discussion forbade. Flat is therefore the discussion-compliant choice, loses no versioning affordance (the `version` field carries `1.0.0`), and matches the only proven shape. The Go module path (`github.com/Knatte18/loomyard/plugins/prowler`) already had no version segment, so it is unchanged.
- **Applies to:** all batches (every plan path uses `plugins/prowler/…`)

### Decision: settings.json grants three entries incl. `Bash(go *)` (overrides discussion.md's "exactly two")

- **Decision:** `plugins/prowler/settings.json` `permissions.allow` is `["Skill(prowler:*)", "Bash(bash *)", "Bash(go *)"]` — three entries, not the two the discussion's skill-contract decision pinned.
- **Rationale:** the discussion pinned "exactly `Skill(prowler:*)` + `Bash(bash *)`" on the stated basis of "mirroring weblens' shape" and an assumption that the child `go build` is covered transitively as a grandchild of the permitted `bash` call. Plan-review round 7 verified the actual weblens `settings.json` — it grants **three** entries (`Skill(weblens:*)`, `Bash(bash *)`, `Bash(node *)`), explicitly gating the `node` child that its `run.sh` spawns via `exec node`, the exact relationship prowler's `go build` child has to its wrapper. So the discussion mischaracterized weblens' shape; faithfully mirroring it means adding the child-spawn grant `Bash(go *)`. Shipping the two-entry set would gamble on an untested permission-model assumption that the one real precedent contradicts.
- **Applies to:** batch 2 (Card 11 `settings.json`)

### Decision: build-lock timeout ~300s (overrides discussion.md's ~120s)

- **Decision:** `run.sh`'s lock-acquire deadline and lock-age staleness threshold are `~300s`, not the `~120s` stated in `_mill/discussion.md`'s build-on-first-run decision.
- **Rationale:** the staleness reclaim removes a lock purely by age, so the threshold must sit safely above a *legitimate* slow build to avoid stealing a live builder's lock. A cold first build fetches and compiles chromedp/goquery/go-readability, and this repo's own benchmarks record ~4× AV-scanning overhead on the operator's Windows box — plausibly exceeding 120s. Combined with the owner-token + atomic-rename safety (Card 8), ~300s makes a false-orphan reclaim both unlikely and harmless. This is a deliberate override of the discussion's placeholder value, recorded here per the same treatment as the layout override.
- **Applies to:** batch 2 (Card 8 `run.sh`, Card 9 `selftest.sh`)

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
- `plugins/prowler/.claude-plugin/plugin.json`
- `plugins/prowler/browser.go`
- `plugins/prowler/browser_integration_test.go`
- `plugins/prowler/chrome.go`
- `plugins/prowler/chrome_test.go`
- `plugins/prowler/fetch.go`
- `plugins/prowler/fetch_test.go`
- `plugins/prowler/fetcher.go`
- `plugins/prowler/go.mod`
- `plugins/prowler/go.sum`
- `plugins/prowler/headers.go`
- `plugins/prowler/htmltext.go`
- `plugins/prowler/htmltext_test.go`
- `plugins/prowler/main.go`
- `plugins/prowler/main_test.go`
- `plugins/prowler/outfile.go`
- `plugins/prowler/outfile_test.go`
- `plugins/prowler/reddit.go`
- `plugins/prowler/reddit_test.go`
- `plugins/prowler/scripts/run.sh`
- `plugins/prowler/scripts/selftest.sh`
- `plugins/prowler/settings.json`
- `plugins/prowler/skills/INDEX.md`
- `plugins/prowler/skills/prowler/SKILL.md`
- `plugins/prowler/README.md`
