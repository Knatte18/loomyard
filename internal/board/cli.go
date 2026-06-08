// cli.go — the board module's command router.
//
// RunCLI parses [--wiki-path] <subcommand> [json-payload], resolves the board
// path (flag → MHGO_WIKI_PATH → ../gowiki), dispatches to one Board method, and
// writes the JSON result to the given writer. Owns the board CLI surface so main
// stays a thin module dispatcher.

package board

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
)

// defaultWikiPath is used when neither --wiki-path nor MHGO_WIKI_PATH is set.
// It deliberately points outside the current repo (".wiki" is a junction owned
// by the non-Go millhouse that is still in active use).
const defaultWikiPath = "../gowiki"

// RunCLI parses and executes a "board" subcommand, writing JSON results to out.
// It returns the process exit code (0 on success, 1 on error).
//
// Usage:
//
//	board [--wiki-path <path>] <subcommand> [json-payload]
//
// Board path resolution (first match wins):
//  1. --wiki-path flag
//  2. MHGO_WIKI_PATH environment variable
//  3. "../gowiki"
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
	fs := flag.NewFlagSet("wiki", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	wikiPathFlag := fs.String("wiki-path", "", "Path to wiki directory")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	wikiPath := resolveWikiPath(*wikiPathFlag)

	// fs.Args() returns the arguments remaining after flags are parsed.
	// Expected: ["<subcommand>", "<json-payload>"]
	rest := fs.Args()
	if len(rest) < 1 {
		fmt.Fprintln(os.Stderr, "usage: mhgo board [--wiki-path <path>] <subcommand> [json-payload]")
		return 1
	}

	subcommand := rest[0]
	var jsonPayload string
	if len(rest) > 1 {
		jsonPayload = rest[1]
	}

	w := New(wikiPath)

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
		task, err := w.UpsertTask(fields)
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
		if err := w.UpsertTasksBatch(payload.Tasks); err != nil {
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
		if err := w.SetPhase(payload.IDOrSlug, payload.Phase); err != nil {
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
		if err := w.RemoveTask(payload.IDOrSlug); err != nil {
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
		task, found, err := w.GetTask(payload.IDOrSlug)
		if err != nil {
			return outputError(out, err.Error())
		}
		if found {
			return outputGetTask(out, &task)
		}
		return outputGetTask(out, nil) // task: null in JSON output

	case "list":
		// List all tasks with computed fields (layer, has_proposal). No payload.
		tasks, err := w.ListTasksBrief()
		if err != nil {
			return outputError(out, err.Error())
		}
		return outputListBrief(out, tasks)

	case "list-full":
		// List all tasks as stored in tasks.json, without enriched fields. No payload.
		tasks, err := w.ListTasksFull()
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

		task, err := w.MergeTasks(payload.RemoveSlugs, payload.Upsert, setPhasePtr)
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
		if err := w.SetDeps(payload.Slug, payload.DependsOn); err != nil {
			return outputError(out, err.Error())
		}
		return outputSuccess(out)

	case "rerender":
		// Rebuild Home.md and _Sidebar.md from the current tasks.json. No payload.
		// Useful if render files have been corrupted or manually edited.
		if err := w.Rerender(); err != nil {
			return outputError(out, err.Error())
		}
		return outputSuccess(out)

	case "sync":
		// Commit and push pending changes to the remote. No payload. Normally
		// launched detached by a write; can also be run by hand to force a backup.
		if err := w.Sync(); err != nil {
			return outputError(out, err.Error())
		}
		return outputSuccess(out)

	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n", subcommand)
		return 1
	}
}

// resolveWikiPath applies the flag > env > default precedence for the wiki directory.
func resolveWikiPath(flagVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if env := os.Getenv("MHGO_WIKI_PATH"); env != "" {
		return env
	}
	return defaultWikiPath
}

// writeJSON marshals v and writes it as a single line to out.
func writeJSON(out io.Writer, v any) {
	data, _ := json.Marshal(v)
	fmt.Fprintln(out, string(data))
}

// outputError writes {"ok":false,"error":"..."} and returns exit code 1.
func outputError(out io.Writer, message string) int {
	writeJSON(out, map[string]any{"ok": false, "error": message})
	return 1
}

// outputSuccess writes {"ok":true} and returns exit code 0.
func outputSuccess(out io.Writer) int {
	writeJSON(out, map[string]any{"ok": true})
	return 0
}

// outputSuccessWithCount writes {"ok":true,"count":N} and returns exit code 0.
func outputSuccessWithCount(out io.Writer, count int) int {
	writeJSON(out, map[string]any{"ok": true, "count": count})
	return 0
}

// outputSuccessWithTask writes {"ok":true,"task":{...}} and returns exit code 0.
func outputSuccessWithTask(out io.Writer, task Task) int {
	writeJSON(out, map[string]any{"ok": true, "task": task})
	return 0
}

// outputGetTask writes {"ok":true,"task":{...}} or {"ok":true,"task":null} and returns exit code 0.
// task is a pointer: nil produces task:null in JSON (task not found, but not an error).
func outputGetTask(out io.Writer, task *Task) int {
	writeJSON(out, map[string]any{"ok": true, "task": task})
	return 0
}

// outputListBrief writes {"ok":true,"tasks":[...]} with BriefTask objects and returns exit code 0.
func outputListBrief(out io.Writer, tasks []BriefTask) int {
	writeJSON(out, map[string]any{"ok": true, "tasks": tasks})
	return 0
}

// outputListFull writes {"ok":true,"tasks":[...]} with full Task objects and returns exit code 0.
func outputListFull(out io.Writer, tasks []Task) int {
	writeJSON(out, map[string]any{"ok": true, "tasks": tasks})
	return 0
}
