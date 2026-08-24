---
name: golang-build
description: Build and test commands for Go. Use after completing a task.
---

# Go Build Skill

Build and test configuration for Go projects.

---

## Build commands

Run these after completing a task, to verify correctness:

```bash
goimports -w <changed-files>
go vet ./...
go build ./...
go test ./...
golangci-lint run
```

**Convention:** the writing formatter (`goimports -w`) runs on changed files only, never the whole project.
Build, test, and the read-only lint stay whole-project.

## Failure handling

- **Build fails** — analyze the error, fix the issue, retry.
- **Tests fail** — analyze the failure, fix the code or the test, retry.
- **A fix needs changes beyond the current task's scope** — stop and report it.
- Never skip or disable a failing test.

---

## Tool installation

Required before running the build workflow:

- **goimports** — organizes and formats imports.
  Install: `go install golang.org/x/tools/cmd/goimports@latest`
- **golangci-lint** — linter aggregator.
  Install: `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`

If either is missing, report which one and its install command, then stop — don't skip the step silently.

---

## Project configuration

> Customize per project: test discovery and build behavior.

### Test discovery

Before running tests, confirm the project is testable:

1. Look for `*_test.go` files.
   If none exist, report "No test files found" rather than running `go test` on an empty package.
2. Test files live in the same directory as the code they test;
   a package with at least one `*_test.go` file is testable.

### Defaults

- Build all packages in the current working directory and subdirectories.
- Run all tests found in the project.

### Per-project overrides

Specify these when the defaults don't apply:

- Specific package paths to build or test.
- Build flags (`-tags`, `-ldflags`).
- Test flags (`-race`, `-cover`).

<!-- Project-specific build configuration goes here -->
