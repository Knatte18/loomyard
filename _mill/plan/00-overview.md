# Plan: Rename _other.go platform files to _linux.go

```yaml
task: Rename _other.go platform files to _linux.go
slug: rename-other-to-linux
approved: false
started: 20260709-171900
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: linux-suffix-rename
    file: 01-linux-suffix-rename.md
    depends-on: []
    verify: GOOS=windows go build ./... && GOOS=linux go build ./... && GOOS=linux go vet ./...
```

## Shared Decisions

### Decision: drop-build-tag-no-replacement

- **Decision:** For every renamed `_other.go` → `_linux.go` file, delete the leading
  `//go:build !windows` line **and its immediately-following blank line**. Do **not** add a
  `//go:build linux` tag. The `_linux.go` filename suffix alone supplies the `GOOS=linux`
  build constraint.
- **Rationale:** The suffix already constrains the file to `GOOS=linux`; a redundant tag adds
  nothing, and the honesty goal (unsupported platforms get no impl and fail to build) is met by
  the suffix. The original task premise that the `_windows.go` siblings carry no tag is factually
  wrong — they do carry `//go:build windows` — but dropping the linux tag is still the minimal
  correct change. The `_windows.go` siblings are **not** touched.
- **Applies to:** all batches

### Decision: reword-non-windows-prose-to-linux

- **Decision:** In the renamed files only, reword every "non-Windows" and "POSIX" comment phrase
  to "Linux". lyx supports Windows + Linux only, so "non-Windows" is exactly Linux; the old
  wording implied a broader platform set that does not exist. The exact per-line rewrites are
  enumerated in each card's Requirements.
- **Rationale:** Same build-constraint-honesty theme as the rename; comments should describe the
  one platform the file actually builds for.
- **Applies to:** all batches

### Decision: git-mv-preserves-history

- **Decision:** Every rename is performed with `git mv <old> <new>` FIRST, followed by surgical
  in-place edits (tag drop + comment rewords) only. Never delete-and-recreate; never rewrite a
  moved file from scratch.
- **Rationale:** Preserves git rename history and keeps the review diff to the lines that actually
  change.
- **Applies to:** all batches

## All Files Touched

- `docs/roadmap.md`
- `internal/fslink/fslink_linux.go`
- `internal/muxpoccli/spawnattach_linux.go`
- `internal/proc/proc_linux.go`
- `internal/proc/proc_linux_test.go`
- `internal/vscode/launch_linux.go`
