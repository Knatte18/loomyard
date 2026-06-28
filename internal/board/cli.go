// cli.go exposes the cobra command tree for the board module.
//
// Command() returns the root "board" command with 11 subcommands, each
// wrapping the existing handler bodies from the legacy switch. Configuration
// resolution happens once in a PersistentPreRunE so that "lyx board" with no
// subcommand lists available subcommands without requiring a git repo or board
// config. The hidden --board-path persistent flag bypasses cwd resolution for
// the detached sync child process launched by spawn.go.

package board

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/paths"
	"github.com/spf13/cobra"
)

// Command returns the cobra command tree for the board module.
//
// The parent "board" command carries a PersistentPreRunE that resolves config
// and constructs the Board instance once, before any subcommand runs. The
// resolved Board is shared via a closure variable closed over by all 11 RunEs.
// When the parent is invoked with no subcommand, cobra lists available
// subcommands without invoking the PreRunE, so no board config is needed.
func Command() *cobra.Command {
	// b is populated by PersistentPreRunE and closed over by each subcommand RunE.
	var b *Board

	cmd := &cobra.Command{
		Use:   "board",
		Short: "task-tracker board",
		Long: `board manages the task-tracker wiki board for the current lyx worktree.

Configuration is resolved from the current working directory via _lyx/config/board.yaml.
The --board-path flag (hidden; injected by the detached sync child) bypasses this resolution.
Running "lyx board" with no subcommand lists available subcommands without requiring a git repo.`,
	}

	// --board-path is an internal persistent flag injected by spawnSync so that
	// the detached sync child can bypass cwd resolution and use the absolute board
	// path directly. It must be hidden so it does not appear in help output.
	boardPathFlag := cmd.PersistentFlags().String("board-path", "", "internal: injected absolute board dir for the detached sync child")
	if err := cmd.PersistentFlags().MarkHidden("board-path"); err != nil {
		// MarkHidden only errors when the flag name does not exist, which cannot
		// happen here since we just registered it above.
		panic(fmt.Sprintf("board: MarkHidden board-path: %v", err))
	}

	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		var cfg Config

		// If --board-path is set, use it directly (internal use for detached sync child).
		if *boardPathFlag != "" {
			cfg = Config{Path: *boardPathFlag}
		} else {
			// Resolve configuration from the current working directory.
			cwd, err := paths.Getwd()
			if err != nil {
				output.Err(cmd.OutOrStdout(), fmt.Sprintf("failed to get working directory: %v", err))
				clihelp.Abort(ctx, 1)
				return nil
			}

			cfg, err = LoadConfig(cwd, "board")
			if err != nil {
				output.Err(cmd.OutOrStdout(), err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}
		}

		// Fold BOARD_SKIP_* env into cfg at the single production entry point.
		cfg = applySkipEnv(cfg)
		b = New(cfg)
		return nil
	}

	// upsert subcommand: create or update a single task.
	upsertCmd := &cobra.Command{
		Use:   "upsert [json-payload]",
		Short: "Create or update a single task",
		Long: `Create or update a task identified by its slug. Unknown keys are rejected.

Required field:
  "slug"       string — unique task identifier

Optional fields:
  "title"      string — human-readable title
  "brief"      string — one-line summary shown in board listings
  "body"       string — full markdown body (proposal / background)
  "depends_on" array  — list of slug strings this task depends on
  "isolated"   bool   — true if the task has no dependencies by design
  "deferred"   bool   — true if the task is deferred
  "status"     string — lifecycle status (e.g. "active", "done")

Example:
  lyx board upsert '{"slug":"my-task","title":"My Task","brief":"Short summary"}'`,
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			// cobra strips the "upsert" token; json payload is now args[0].
			if len(args) == 0 {
				return outputError(out, "json payload required")
			}
			var fields map[string]any
			if err := json.Unmarshal([]byte(args[0]), &fields); err != nil {
				return outputError(out, fmt.Sprintf("invalid json: %v", err))
			}
			task, err := b.UpsertTask(fields)
			if err != nil {
				return outputError(out, err.Error())
			}
			return outputSuccessWithTask(out, task)
		}),
	}

	// upsert-batch subcommand: create or update multiple tasks atomically.
	// Allowed wrapper key: {tasks}. A typo'd wrapper (e.g. "taks") errors;
	// an absent or empty tasks array also errors (nothing to upsert is a mistake).
	upsertBatchCmd := &cobra.Command{
		Use:   "upsert-batch [json-payload]",
		Short: "Create or update multiple tasks atomically",
		Long: `Create or update multiple tasks in one atomic write. Unknown wrapper keys are rejected.
An absent or empty "tasks" array is an error. Each task element uses the same fields as
"lyx board upsert" ("slug" required per element); unknown element keys are also rejected.

Required wrapper field:
  "tasks" array — one or more task objects (each with "slug" required)

Example:
  lyx board upsert-batch '{"tasks":[{"slug":"t1","title":"One"},{"slug":"t2","title":"Two"}]}'`,
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if len(args) == 0 {
				return outputError(out, "json payload required")
			}

			// Decode into a map to detect unknown wrapper keys.
			var raw map[string]any
			if err := json.Unmarshal([]byte(args[0]), &raw); err != nil {
				return outputError(out, fmt.Sprintf("invalid json: %v", err))
			}

			// Only "tasks" is permitted at the wrapper level; a typo'd key would
			// decode silently to count:0 with the old typed-struct approach.
			for k := range raw {
				if k != "tasks" {
					return outputError(out, fmt.Sprintf("unknown field: %q", k))
				}
			}

			// tasks is required and must be a non-empty array.
			tasksVal, hasTasksKey := raw["tasks"]
			if !hasTasksKey || tasksVal == nil {
				return outputError(out, "missing required field: tasks")
			}
			tasksArr, ok := tasksVal.([]any)
			if !ok {
				return outputError(out, "tasks must be an array")
			}
			if len(tasksArr) == 0 {
				return outputError(out, "tasks array must not be empty")
			}

			// Convert []any to []map[string]any; the store validates each element's fields.
			tasks := make([]map[string]any, len(tasksArr))
			for i, v := range tasksArr {
				m, ok := v.(map[string]any)
				if !ok {
					return outputError(out, fmt.Sprintf("tasks[%d] must be an object", i))
				}
				tasks[i] = m
			}

			if err := b.UpsertTasksBatch(tasks); err != nil {
				return outputError(out, err.Error())
			}
			return outputSuccessWithCount(out, len(tasks))
		}),
	}

	// set-status subcommand: set or clear the status field of a task identified by
	// slug or numeric id. Allowed keys: {slug, id, status}.
	setStatusCmd := &cobra.Command{
		Use:   "set-status [json-payload]",
		Short: "Set or clear the status of a task",
		Long: `Set or clear the lifecycle status of a task. Unknown keys are rejected.
Exactly one of "slug" or "id" is required. "status" is always required; use null to clear.

Fields:
  "slug"   string      — task slug (mutually exclusive with "id")
  "id"     integer     — numeric task ID (mutually exclusive with "slug")
  "status" string|null — new status value; null clears the current status

Examples:
  lyx board set-status '{"slug":"my-task","status":"active"}'
  lyx board set-status '{"id":96,"status":null}'`,
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if len(args) == 0 {
				return outputError(out, "json payload required")
			}
			// resolveLookup enforces {slug, id, status} allowed keys and exactly-one-of slug/id.
			selector, m, err := resolveLookup([]byte(args[0]), "status")
			if err != nil {
				return outputError(out, err.Error())
			}

			// status key is required: an absent key is an error; an explicit null clears
			// the status. This distinguishes a deliberate clear from a typo that would
			// otherwise silently clear the status value.
			sv, hasStatus := m["status"]
			if !hasStatus {
				return outputError(out, "missing required field: status")
			}
			var status *string
			if sv != nil {
				s, ok := sv.(string)
				if !ok {
					return outputError(out, "status must be a string or null")
				}
				status = &s
			}

			if err := b.SetStatus(selector, status); err != nil {
				return outputError(out, err.Error())
			}
			return outputSuccess(out)
		}),
	}

	// remove subcommand: remove a task by slug or numeric id.
	removeCmd := &cobra.Command{
		Use:   "remove [json-payload]",
		Short: "Remove a task",
		Long: `Remove a task by slug or numeric ID. Unknown keys are rejected.
Exactly one of "slug" or "id" is required. Errors if the task is not found.

Fields:
  "slug" string  — task slug (mutually exclusive with "id")
  "id"   integer — numeric task ID (mutually exclusive with "slug")

Example:
  lyx board remove '{"slug":"my-task"}'`,
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if len(args) == 0 {
				return outputError(out, "json payload required")
			}
			// resolveLookup enforces {slug, id} allowed keys and exactly-one-of.
			selector, _, err := resolveLookup([]byte(args[0]))
			if err != nil {
				return outputError(out, err.Error())
			}
			if err := b.RemoveTask(selector); err != nil {
				return outputError(out, err.Error())
			}
			return outputSuccess(out)
		}),
	}

	// get subcommand: fetch a single task by slug or numeric id; returns task:null
	// for a valid-but-absent target (not an error). Malformed payloads error.
	getCmd := &cobra.Command{
		Use:   "get [json-payload]",
		Short: "Fetch a single task",
		Long: `Fetch a single task by slug or numeric ID. Unknown keys are rejected.
Exactly one of "slug" or "id" is required. Returns {"task":null} if not found (not an error).
Malformed payloads (no identifier key, unknown key) are errors.

Fields:
  "slug" string  — task slug (mutually exclusive with "id")
  "id"   integer — numeric task ID (mutually exclusive with "slug")

Example:
  lyx board get '{"id":96}'`,
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if len(args) == 0 {
				return outputError(out, "json payload required")
			}
			// resolveLookup enforces {slug, id} allowed keys and exactly-one-of.
			selector, _, err := resolveLookup([]byte(args[0]))
			if err != nil {
				return outputError(out, err.Error())
			}
			task, found, err := b.GetTask(selector)
			if err != nil {
				return outputError(out, err.Error())
			}
			if found {
				return outputGetTask(out, &task)
			}
			return outputGetTask(out, nil) // task: null in JSON output
		}),
	}

	// list subcommand: list all tasks with computed fields (layer, has_proposal).
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all tasks with computed fields",
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			tasks, err := b.ListTasksBrief()
			if err != nil {
				return outputError(out, err.Error())
			}
			return outputListBrief(out, tasks)
		}),
	}

	// list-full subcommand: list all tasks as stored in tasks.json.
	listFullCmd := &cobra.Command{
		Use:   "list-full",
		Short: "List all tasks as stored in tasks.json",
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			tasks, err := b.ListTasksFull()
			if err != nil {
				return outputError(out, err.Error())
			}
			return outputListFull(out, tasks)
		}),
	}

	// merge subcommand: remove slugs, upsert one task, and optionally set status — atomically.
	// Allowed top-level keys: {remove_slugs, upsert, set_status}. The inner set_status
	// object is validated identically to the set-status command.
	mergeCmd := &cobra.Command{
		Use:   "merge [json-payload]",
		Short: "Atomically remove, upsert, and set-status",
		Long: `Remove tasks, upsert a task, and optionally set status in one atomic write.
Unknown top-level keys are rejected. The inner "set_status" object is validated
identically to the standalone "set-status" command ({slug|id, status}, exactly-one-of).

Fields:
  "remove_slugs" array  — slug strings to remove (optional; omit to skip)
  "upsert"       object — task to create or update (required; same fields as "lyx board upsert")
  "set_status"   object — status to set after upsert (optional):
    "slug"   string      — task slug (mutually exclusive with "id")
    "id"     integer     — numeric task ID (mutually exclusive with "slug")
    "status" string|null — new status; null clears

Example:
  lyx board merge '{"remove_slugs":["old"],"upsert":{"slug":"new","title":"New"},"set_status":{"slug":"new","status":"active"}}'`,
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if len(args) == 0 {
				return outputError(out, "json payload required")
			}

			// Decode into a map first to detect unknown top-level keys.
			var raw map[string]any
			if err := json.Unmarshal([]byte(args[0]), &raw); err != nil {
				return outputError(out, fmt.Sprintf("invalid json: %v", err))
			}

			// Enforce strict top-level key set; a stale set_phase errors rather than
			// being silently dropped (which would skip the status step with no feedback).
			for k := range raw {
				if k != "remove_slugs" && k != "upsert" && k != "set_status" {
					return outputError(out, fmt.Sprintf("unknown field: %q", k))
				}
			}

			// Parse remove_slugs (optional, default empty).
			var removeSlugs []string
			if rsVal, ok := raw["remove_slugs"]; ok && rsVal != nil {
				rsArr, ok := rsVal.([]any)
				if !ok {
					return outputError(out, "remove_slugs must be an array")
				}
				for _, v := range rsArr {
					s, ok := v.(string)
					if !ok {
						return outputError(out, "remove_slugs elements must be strings")
					}
					removeSlugs = append(removeSlugs, s)
				}
			}

			// Parse upsert (required: contains the task fields to create or update).
			upsertVal, hasUpsert := raw["upsert"]
			if !hasUpsert || upsertVal == nil {
				return outputError(out, "missing required field: upsert")
			}
			upsertFields, ok := upsertVal.(map[string]any)
			if !ok {
				return outputError(out, "upsert must be an object")
			}

			// Parse set_status (optional): validate using the same resolveLookup
			// logic as the standalone set-status command — {slug,id,status} allowed,
			// exactly-one-of slug/id, and status key required.
			var setStatusPtr *MergeStatusUpdate
			if ssVal, ok := raw["set_status"]; ok && ssVal != nil {
				ssBytes, err := json.Marshal(ssVal)
				if err != nil {
					return outputError(out, fmt.Sprintf("set_status: marshal error: %v", err))
				}
				selector, ssMap, err := resolveLookup(ssBytes, "status")
				if err != nil {
					return outputError(out, "set_status: "+err.Error())
				}
				// status key is required inside set_status, mirroring the standalone command.
				sv, hasStatusKey := ssMap["status"]
				if !hasStatusKey {
					return outputError(out, "set_status: missing required field: status")
				}
				var status *string
				if sv != nil {
					s, ok := sv.(string)
					if !ok {
						return outputError(out, "set_status.status must be a string or null")
					}
					status = &s
				}
				setStatusPtr = &MergeStatusUpdate{Selector: selector, Status: status}
			}

			task, err := b.MergeTasks(removeSlugs, upsertFields, setStatusPtr)
			if err != nil {
				return outputError(out, err.Error())
			}
			return outputSuccessWithTask(out, task)
		}),
	}

	// set-deps subcommand: replace the depends_on list for a task.
	// Allowed keys: {slug, depends_on}. depends_on is required (absent errors;
	// explicit [] clears the list, distinguishing intentional clear from a typo
	// that would otherwise silently wipe the task's dependency list).
	setDepsCmd := &cobra.Command{
		Use:   "set-deps [json-payload]",
		Short: "Replace the depends_on list for a task",
		Long: `Replace the full depends_on list for a task wholesale. Unknown keys are rejected.
Both fields are required. An absent "depends_on" is an error; an explicit [] clears the list.

Fields:
  "slug"       string — task slug to update (required)
  "depends_on" array  — complete list of dependency slug strings; replaces existing list (required)

Example:
  lyx board set-deps '{"slug":"my-task","depends_on":["dep-a","dep-b"]}'`,
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if len(args) == 0 {
				return outputError(out, "json payload required")
			}

			// Decode into a map to detect unknown keys and key presence.
			var m map[string]any
			if err := json.Unmarshal([]byte(args[0]), &m); err != nil {
				return outputError(out, fmt.Sprintf("invalid json: %v", err))
			}

			// Reject unknown keys so a typo ("depends") errors instead of silently
			// clearing the dependency list.
			for k := range m {
				if k != "slug" && k != "depends_on" {
					return outputError(out, fmt.Sprintf("unknown field: %q", k))
				}
			}

			// slug is required to identify the target task.
			slug, ok := m["slug"].(string)
			if !ok || slug == "" {
				return outputError(out, "missing required field: slug")
			}

			// depends_on is required: absent key errors; explicit [] clears the list.
			depsVal, hasDeps := m["depends_on"]
			if !hasDeps {
				return outputError(out, "missing required field: depends_on")
			}

			var dependsOn []string
			if depsVal != nil {
				arr, ok := depsVal.([]any)
				if !ok {
					return outputError(out, "depends_on must be an array")
				}
				dependsOn = make([]string, 0, len(arr))
				for _, v := range arr {
					s, ok := v.(string)
					if !ok {
						return outputError(out, "depends_on elements must be strings")
					}
					dependsOn = append(dependsOn, s)
				}
			} else {
				// Explicit null — treat as empty (clear the list).
				dependsOn = []string{}
			}

			if err := b.SetDeps(slug, dependsOn); err != nil {
				return outputError(out, err.Error())
			}
			return outputSuccess(out)
		}),
	}

	// rerender subcommand: rebuild Home.md and _Sidebar.md from the current tasks.json.
	rerenderCmd := &cobra.Command{
		Use:   "rerender",
		Short: "Rebuild Home.md and _Sidebar.md from tasks.json",
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if err := b.Rerender(); err != nil {
				return outputError(out, err.Error())
			}
			return outputSuccess(out)
		}),
	}

	// sync subcommand: commit and push pending changes to the remote.
	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Commit and push pending board changes to the remote",
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if err := b.Sync(); err != nil {
				return outputError(out, err.Error())
			}
			return outputSuccess(out)
		}),
	}

	cmd.AddCommand(
		upsertCmd,
		upsertBatchCmd,
		setStatusCmd,
		removeCmd,
		getCmd,
		listCmd,
		listFullCmd,
		mergeCmd,
		setDepsCmd,
		rerenderCmd,
		syncCmd,
	)

	return cmd
}

// resolveLookup decodes a raw JSON payload into a map, validates that every key
// is within the allowed set ({slug, id} plus any extraKeys), and returns the task
// selector and decoded map. Presence is detected via map-key membership so id:0
// is a valid distinct-from-absent lookup. Returns a Go string when slug is present
// or a float64 when id is present (JSON numbers decode as float64).
func resolveLookup(raw []byte, extraKeys ...string) (any, map[string]any, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, nil, fmt.Errorf("invalid json: %v", err)
	}

	// Build the full allowed key set from the base {slug, id} plus any extras.
	allowed := map[string]bool{"slug": true, "id": true}
	for _, k := range extraKeys {
		allowed[k] = true
	}

	// Reject any key outside the allowed set before checking presence.
	for k := range m {
		if !allowed[k] {
			return nil, nil, fmt.Errorf("unknown field: %q", k)
		}
	}

	// Detect which identifier is present via map-key membership, not zero-value,
	// so {"id":0} is distinct from an absent "id" key.
	_, hasSlug := m["slug"]
	_, hasID := m["id"]

	if !hasSlug && !hasID {
		return nil, nil, fmt.Errorf("one of slug or id is required")
	}
	if hasSlug && hasID {
		return nil, nil, fmt.Errorf("only one of slug or id may be given")
	}

	if hasSlug {
		slugStr, ok := m["slug"].(string)
		if !ok || slugStr == "" {
			return nil, nil, fmt.Errorf("slug must be a non-empty string")
		}
		return slugStr, m, nil
	}

	// JSON numbers always decode as float64; the store's type-switch handles both
	// float64 and int, so pass the float64 directly.
	switch v := m["id"].(type) {
	case float64:
		return v, m, nil
	default:
		return nil, nil, fmt.Errorf("id must be a number")
	}
}

// RunCLI is the public seam for the board module CLI.
//
// It delegates to clihelp.Execute with the cobra command tree, passing out as
// the capture writer for all output (including cobra's error text). This
// preserves the existing call contract so that callers and tests are unchanged.
func RunCLI(out io.Writer, args []string) int {
	return clihelp.Execute(Command(), out, args)
}

// outputError writes {"ok":false,"error":"..."} and returns exit code 1.
func outputError(out io.Writer, message string) int {
	return output.Err(out, message)
}

// outputSuccess writes {"ok":true} and returns exit code 0.
func outputSuccess(out io.Writer) int {
	return output.Ok(out, map[string]any{})
}

// outputSuccessWithCount writes {"ok":true,"count":N} and returns exit code 0.
func outputSuccessWithCount(out io.Writer, count int) int {
	return output.Ok(out, map[string]any{"count": count})
}

// outputSuccessWithTask writes {"ok":true,"task":{...}} and returns exit code 0.
func outputSuccessWithTask(out io.Writer, task Task) int {
	return output.Ok(out, map[string]any{"task": task})
}

// outputGetTask writes {"ok":true,"task":{...}} or {"ok":true,"task":null} and returns exit code 0.
// task is a pointer: nil produces task:null in JSON (task not found, but not an error).
func outputGetTask(out io.Writer, task *Task) int {
	return output.Ok(out, map[string]any{"task": task})
}

// outputListBrief writes {"ok":true,"tasks":[...]} with BriefTask objects and returns exit code 0.
func outputListBrief(out io.Writer, tasks []BriefTask) int {
	return output.Ok(out, map[string]any{"tasks": tasks})
}

// outputListFull writes {"ok":true,"tasks":[...]} with full Task objects and returns exit code 0.
func outputListFull(out io.Writer, tasks []Task) int {
	return output.Ok(out, map[string]any{"tasks": tasks})
}

// applySkipEnv folds BOARD_SKIP_GIT and BOARD_SKIP_PUSH environment variables into cfg.
// This is the single production env read; all other consumption sites use the config fields.
func applySkipEnv(cfg Config) Config {
	if os.Getenv("BOARD_SKIP_GIT") == "1" {
		cfg.SkipGit = true
	}
	if os.Getenv("BOARD_SKIP_PUSH") == "1" {
		cfg.SkipPush = true
	}
	return cfg
}
