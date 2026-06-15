// cli.go — the board module's command router.
//
// RunCLI parses <subcommand> [json-payload], resolves the board configuration
// from the current working directory (cwd-authoritative model), dispatches to
// one Board method, and writes the JSON result to the given writer. Owns the
// board CLI surface so main stays a thin module dispatcher.
//
// Configuration resolution (cwd-authoritative):
// RunCLI delegates to LoadConfig, which resolves the board config from the
// current working directory via internal/config. The board module never reads
// config files itself — file layout and overrides are entirely internal/config's
// concern.
//
// When --board-path is set (internal flag for detached sync child), it bypasses
// configuration resolution and uses the provided path directly.

package board

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/Knatte18/mhgo/internal/output"
	"github.com/Knatte18/mhgo/internal/paths"
)

// RunCLI parses and executes a "board" subcommand, writing JSON results to out.
// It returns the process exit code (0 on success, 1 on error).
//
// Usage:
//
//	board <subcommand> [json-payload]
//
// Configuration resolution (cwd-authoritative):
// RunCLI resolves the board configuration cwd-authoritatively via internal/config;
// the module never reads config files or knows their on-disk layout itself.
// The board path is resolved relative to the cwd.
//
// Subcommands and their JSON payloads:
//
//	upsert        '{"slug":"my-task","title":"Do X"}'
//	upsert-batch  '{"tasks":[{"slug":"a"},{"slug":"b"}]}'
//	set-phase     '{"id_or_slug":"my-task","phase":"done"}'   (phase null to clear)
//	remove        '{"id_or_slug":"my-task"}'
//	get           '{"id_or_slug":"my-task"}'
//	list          (no payload — brief view with computed layer)
//	list-full     (no payload — raw task structs)
//	merge         '{"remove_slugs":["old"],"upsert":{...},"set_phase":["new-slug","done"]}'
//	set-deps      '{"slug":"my-task","depends_on":["other"]}'
//	rerender      (no payload — rebuild Home.md and _Sidebar.md from current tasks.json)
//	sync          (no payload — commit + push pending changes to the remote)
//
// All output is JSON on out.
// Success: {"ok":true, ...}
// Error:   {"ok":false,"error":"..."} with exit code 1.
func RunCLI(out io.Writer, args []string) int {
	fs := flag.NewFlagSet("board", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	boardPathFlag := fs.String("board-path", "", "internal: injected absolute board dir for the detached sync child")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	var cfg Config

	// If --board-path is set, use it directly (internal use for detached sync child)
	if *boardPathFlag != "" {
		cfg = DefaultConfig()
		cfg.Path = *boardPathFlag
	} else {
		// Load configuration from cwd
		cwd, err := paths.Getwd()
		if err != nil {
			return outputError(out, err.Error())
		}
		cfg, err = LoadConfig(cwd, "board")
		if err != nil {
			return outputError(out, err.Error())
		}
	}

	// fs.Args() returns the arguments remaining after flags are parsed.
	// Expected: ["<subcommand>", "<json-payload>"]
	rest := fs.Args()
	if len(rest) < 1 {
		fmt.Fprintln(os.Stderr, "usage: mhgo board <subcommand> [json-payload]")
		return 1
	}

	subcommand := rest[0]
	var jsonPayload string
	if len(rest) > 1 {
		jsonPayload = rest[1]
	}

	b := New(cfg)

	switch subcommand {
	case "upsert":
		// Create or update a single task. Payload: task fields as a JSON object.
		if jsonPayload == "" {
			return outputError(out, "json payload required")
		}
		var fields map[string]any
		if err := json.Unmarshal([]byte(jsonPayload), &fields); err != nil {
			return outputError(out, fmt.Sprintf("invalid json: %v", err))
		}
		task, err := b.UpsertTask(fields)
		if err != nil {
			return outputError(out, err.Error())
		}
		return outputSuccessWithTask(out, task)

	case "upsert-batch":
		// Create or update multiple tasks atomically. Payload: {"tasks":[...]}.
		if jsonPayload == "" {
			return outputError(out, "json payload required")
		}
		var payload struct {
			Tasks []map[string]any `json:"tasks"`
		}
		if err := json.Unmarshal([]byte(jsonPayload), &payload); err != nil {
			return outputError(out, fmt.Sprintf("invalid json: %v", err))
		}
		if err := b.UpsertTasksBatch(payload.Tasks); err != nil {
			return outputError(out, err.Error())
		}
		return outputSuccessWithCount(out, len(payload.Tasks))

	case "set-phase":
		// Set or clear the status field of a task.
		// Payload: {"id_or_slug": "slug-or-id", "phase": "done"}
		// phase null in JSON clears the status field.
		if jsonPayload == "" {
			return outputError(out, "json payload required")
		}
		var payload struct {
			IDOrSlug any     `json:"id_or_slug"`
			Phase    *string `json:"phase"` // pointer: JSON null → Go nil → status cleared
		}
		if err := json.Unmarshal([]byte(jsonPayload), &payload); err != nil {
			return outputError(out, fmt.Sprintf("invalid json: %v", err))
		}
		if err := b.SetPhase(payload.IDOrSlug, payload.Phase); err != nil {
			return outputError(out, err.Error())
		}
		return outputSuccess(out)

	case "remove":
		// Remove a task. Returns an error if the slug does not exist.
		// Payload: {"id_or_slug": "slug-or-id"}
		if jsonPayload == "" {
			return outputError(out, "json payload required")
		}
		var payload struct {
			IDOrSlug any `json:"id_or_slug"`
		}
		if err := json.Unmarshal([]byte(jsonPayload), &payload); err != nil {
			return outputError(out, fmt.Sprintf("invalid json: %v", err))
		}
		if err := b.RemoveTask(payload.IDOrSlug); err != nil {
			return outputError(out, err.Error())
		}
		return outputSuccess(out)

	case "get":
		// Fetch a single task. Returns {"ok":true,"task":null} if not found (not an error).
		// Payload: {"id_or_slug": "slug-or-id"}
		if jsonPayload == "" {
			return outputError(out, "json payload required")
		}
		var payload struct {
			IDOrSlug any `json:"id_or_slug"`
		}
		if err := json.Unmarshal([]byte(jsonPayload), &payload); err != nil {
			return outputError(out, fmt.Sprintf("invalid json: %v", err))
		}
		task, found, err := b.GetTask(payload.IDOrSlug)
		if err != nil {
			return outputError(out, err.Error())
		}
		if found {
			return outputGetTask(out, &task)
		}
		return outputGetTask(out, nil) // task: null in JSON output

	case "list":
		// List all tasks with computed fields (layer, has_proposal). No payload.
		tasks, err := b.ListTasksBrief()
		if err != nil {
			return outputError(out, err.Error())
		}
		return outputListBrief(out, tasks)

	case "list-full":
		// List all tasks as stored in tasks.json, without enriched fields. No payload.
		tasks, err := b.ListTasksFull()
		if err != nil {
			return outputError(out, err.Error())
		}
		return outputListFull(out, tasks)

	case "merge":
		// Remove + upsert + set_phase atomically.
		// Payload: {"remove_slugs":["a"],"upsert":{...},"set_phase":["slug","done"]}
		// set_phase is optional: omit the field or set to null to skip.
		if jsonPayload == "" {
			return outputError(out, "json payload required")
		}
		var payload struct {
			RemoveSlugs []string       `json:"remove_slugs"`
			Upsert      map[string]any `json:"upsert"`
			SetPhase    []any          `json:"set_phase"` // two elements: [id_or_slug, phase]
		}
		if err := json.Unmarshal([]byte(jsonPayload), &payload); err != nil {
			return outputError(out, fmt.Sprintf("invalid json: %v", err))
		}

		// set_phase must be exactly two elements if provided.
		var setPhasePtr *[2]any
		if len(payload.SetPhase) > 0 {
			if len(payload.SetPhase) != 2 {
				return outputError(out, "set_phase must be array of length 2")
			}
			setPhasePtr = &[2]any{payload.SetPhase[0], payload.SetPhase[1]}
		}

		task, err := b.MergeTasks(payload.RemoveSlugs, payload.Upsert, setPhasePtr)
		if err != nil {
			return outputError(out, err.Error())
		}
		return outputSuccessWithTask(out, task)

	case "set-deps":
		// Replace the depends_on list for a task, with full validation.
		// Payload: {"slug":"my-task","depends_on":["other-task"]}
		if jsonPayload == "" {
			return outputError(out, "json payload required")
		}
		var payload struct {
			Slug      string   `json:"slug"`
			DependsOn []string `json:"depends_on"`
		}
		if err := json.Unmarshal([]byte(jsonPayload), &payload); err != nil {
			return outputError(out, fmt.Sprintf("invalid json: %v", err))
		}
		if err := b.SetDeps(payload.Slug, payload.DependsOn); err != nil {
			return outputError(out, err.Error())
		}
		return outputSuccess(out)

	case "rerender":
		// Rebuild Home.md and _Sidebar.md from the current tasks.json. No payload.
		// Useful if render files have been corrupted or manually edited.
		if err := b.Rerender(); err != nil {
			return outputError(out, err.Error())
		}
		return outputSuccess(out)

	case "sync":
		// Commit and push pending changes to the remote. No payload. Normally
		// launched detached by a write; can also be run by hand to force a backup.
		if err := b.Sync(); err != nil {
			return outputError(out, err.Error())
		}
		return outputSuccess(out)

	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n", subcommand)
		return 1
	}
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
