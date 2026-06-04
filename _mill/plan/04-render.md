# Batch: Render

```yaml
task: Port the wiki module to Go
batch: Render
number: 4
cards: 2
verify: PYTHONPATH= go test ./internal/wiki/
depends-on: [3]
```

## Batch Scope

Implements the markdown renderer that turns the task list into `Home.md`, `_Sidebar.md`, and `proposal-<slug>.md` files. After this batch, `render()` produces the full map of output file names to content strings, matching the Python `_render.py` output exactly.

## Cards

### Card 9: internal/wiki/render.go

- **Context:**
  - `internal/wiki/task.go`
  - `internal/wiki/layer.go`
- **Edits:** none
- **Creates:**
  - `internal/wiki/render.go`
- **Deletes:** none
- **Requirements:** Package `wiki`. Implement `render(tasks []Task) (map[string]string, error)`. The function returns a map of relative file paths to their string content. On layer computation error, return the error. Build `Home.md` and `_Sidebar.md` simultaneously while iterating buckets. Bucket iteration order: letter buckets A–Y (sorted), then Z, then `__deferred__`, then `__done__`. Within each bucket, tasks are sorted by ID ascending. Home.md structure: (1) initialise `lines = ["# Tasks", ""]` — for an empty task list this produces `"# Tasks\n"` (the trailing `""` adds only a newline via join); for non-empty lists the first bucket header follows immediately after, producing `"# Tasks\n\n# Layer A\n\n..."`. The blank line between header and first bucket comes from this initial `""` element combined with the bucket header line; (2) for each bucket, a section header: `# Layer <letter>\n\n` for letters, `# Someday\n\n` for `__deferred__`, `# Done\n\n` for `__done__`; (3) for each task in the bucket, a heading `## <display_title>\n` where `display_title = "**#NNN:** " + title + " [layer]"` for letter/Z buckets (pad ID to 3 digits with leading zeros), or `"**#NNN:** " + title` for done/deferred; (4) slug line: `[slug](proposal-slug.md)` if `Body` non-empty, else `[slug]`; append ` [status]` if Status is one of `active`, `done`, `pr-pending`, `ready-to-merge`, `abandoned`; (5) if DependsOn non-empty, a `Depends on: #NNN, #NNN` line using the IDs of the dep slugs (format `#NNN`; if a dep slug is not found in the task map, use `#???: <slug> (missing)`); (6) if Brief non-empty, a blank line followed by the brief text; (7) a trailing blank line after each task block. Sidebar structure: one bullet per task `- <display_title>` or `- [display_title](proposal-slug.md)` if Body non-empty; blank line between bucket groups (no trailing blank line at end). For each task with non-empty Body, add `proposal-<slug>.md` → Body to the result map. Return the map with `Home.md`, `_Sidebar.md`, and all proposal files.
- **Commit:** `feat(wiki): markdown renderer`

### Card 10: internal/wiki/render_test.go

- **Context:**
  - `internal/wiki/task.go`
  - `internal/wiki/layer.go`
  - `internal/wiki/render.go`
- **Edits:** none
- **Creates:**
  - `internal/wiki/render_test.go`
- **Deletes:** none
- **Requirements:** Package `wiki_test`. Test `render`: (a) empty task list → Home.md is exactly `"# Tasks\n"` (single trailing newline), Sidebar is `""`, no proposal files; (b) single task no body → Home.md has correct heading and slug line, no proposal file in result map; (c) single task with body → `proposal-<slug>.md` key present with body content; (d) task with `active` status → slug line ends with ` [active]`; (e) two tasks A depends on B → bucket headers in correct order (B in Layer A section, A in Layer B section); (f) done task → appears under `# Done`, heading has no layer suffix; (g) isolated task → appears under letter `Z` in bucket order after all letter buckets; (h) deferred task → appears under `# Someday`; (i) task with DependsOn → `Depends on: #NNN` line present; (j) multiple tasks in same bucket sorted by ID; (k) Sidebar has blank line between bucket groups; (l) orphan detection — render with body, then render again without body → second call's result map has no proposal file for that slug. Use table-driven tests where multiple variants share the same assertion logic.
- **Commit:** `test(wiki): render tests`

## Batch Tests

`go test ./internal/wiki/` compiles task.go + layer.go + render.go and runs all test files. No filesystem or git access.
