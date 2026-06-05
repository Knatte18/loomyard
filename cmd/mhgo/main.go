// Command mhgo is the CLI for the mhgo wiki task tracker.
//
// Usage:
//
//	mhgo [--wiki-path <path>] wiki <subcommand> [json-payload]
//
// Wiki path resolution (first match wins):
//  1. --wiki-path flag
//  2. MHGO_WIKI_PATH environment variable
//  3. ".wiki" in the current directory
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
//
// All output is JSON on stdout.
// Success: {"ok":true, ...}
// Error:   {"ok":false,"error":"..."} with exit code 1.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/Knatte18/mhgo/internal/wiki"
)

func main() {
	// flag.NewFlagSet creates a parser for command-line flags.
	// "mhgo" is the name shown in error messages.
	// flag.ExitOnError: exit automatically on invalid flag input.
	fs := flag.NewFlagSet("mhgo", flag.ExitOnError)
	wikiPathFlag := fs.String("wiki-path", "", "Path to wiki directory")
	fs.Parse(os.Args[1:])

	// Wiki path: flag > env var > default ".wiki"
	wikiPath := *wikiPathFlag
	if wikiPath == "" {
		wikiPath = os.Getenv("MHGO_WIKI_PATH")
	}
	if wikiPath == "" {
		wikiPath = ".wiki"
	}

	// fs.Args() returns the arguments remaining after flags are parsed.
	// Expected: ["wiki", "<subcommand>", "<json-payload>"]
	args := fs.Args()

	if len(args) < 2 || args[0] != "wiki" {
		fmt.Fprintf(os.Stderr, "usage: mhgo wiki <subcommand> [json-payload]\n")
		os.Exit(1)
	}

	subcommand := args[1]
	var jsonPayload string
	if len(args) > 2 {
		jsonPayload = args[2]
	}

	w := wiki.New(wikiPath)

	switch subcommand {
	case "upsert":
		// Create or update a single task. Payload: task fields as a JSON object.
		if jsonPayload == "" {
			outputError("json payload required")
			os.Exit(1)
		}
		var fields map[string]interface{}
		if err := json.Unmarshal([]byte(jsonPayload), &fields); err != nil {
			outputError(fmt.Sprintf("invalid json: %v", err))
			os.Exit(1)
		}
		task, err := w.UpsertTask(fields)
		if err != nil {
			outputError(err.Error())
			os.Exit(1)
		}
		outputSuccessWithTask(task)

	case "upsert-batch":
		// Create or update multiple tasks atomically. Payload: {"tasks":[...]}.
		if jsonPayload == "" {
			outputError("json payload required")
			os.Exit(1)
		}
		var payload struct {
			Tasks []map[string]interface{} `json:"tasks"`
		}
		if err := json.Unmarshal([]byte(jsonPayload), &payload); err != nil {
			outputError(fmt.Sprintf("invalid json: %v", err))
			os.Exit(1)
		}
		err := w.UpsertTasksBatch(payload.Tasks)
		if err != nil {
			outputError(err.Error())
			os.Exit(1)
		}
		outputSuccessWithCount(len(payload.Tasks))

	case "set-phase":
		// Set or clear the status field of a task.
		// Payload: {"id_or_slug": "slug-or-id", "phase": "done"}
		// phase null in JSON clears the status field.
		if jsonPayload == "" {
			outputError("json payload required")
			os.Exit(1)
		}
		var payload struct {
			IDOrSlug interface{} `json:"id_or_slug"`
			Phase    *string     `json:"phase"` // pointer: JSON null → Go nil → status cleared
		}
		if err := json.Unmarshal([]byte(jsonPayload), &payload); err != nil {
			outputError(fmt.Sprintf("invalid json: %v", err))
			os.Exit(1)
		}
		err := w.SetPhase(payload.IDOrSlug, payload.Phase)
		if err != nil {
			outputError(err.Error())
			os.Exit(1)
		}
		outputSuccess()

	case "remove":
		// Remove a task. Returns an error if the slug does not exist.
		// Payload: {"id_or_slug": "slug-or-id"}
		if jsonPayload == "" {
			outputError("json payload required")
			os.Exit(1)
		}
		var payload struct {
			IDOrSlug interface{} `json:"id_or_slug"`
		}
		if err := json.Unmarshal([]byte(jsonPayload), &payload); err != nil {
			outputError(fmt.Sprintf("invalid json: %v", err))
			os.Exit(1)
		}
		err := w.RemoveTask(payload.IDOrSlug)
		if err != nil {
			outputError(err.Error())
			os.Exit(1)
		}
		outputSuccess()

	case "get":
		// Fetch a single task. Returns {"ok":true,"task":null} if not found (not an error).
		// Payload: {"id_or_slug": "slug-or-id"}
		if jsonPayload == "" {
			outputError("json payload required")
			os.Exit(1)
		}
		var payload struct {
			IDOrSlug interface{} `json:"id_or_slug"`
		}
		if err := json.Unmarshal([]byte(jsonPayload), &payload); err != nil {
			outputError(fmt.Sprintf("invalid json: %v", err))
			os.Exit(1)
		}
		task, found, err := w.GetTask(payload.IDOrSlug)
		if err != nil {
			outputError(err.Error())
			os.Exit(1)
		}
		if found {
			outputGetTask(&task)
		} else {
			outputGetTask(nil) // task: null in JSON output
		}

	case "list":
		// List all tasks with computed fields (layer, has_proposal). No payload.
		tasks, err := w.ListTasksBrief()
		if err != nil {
			outputError(err.Error())
			os.Exit(1)
		}
		outputListBrief(tasks)

	case "list-full":
		// List all tasks as stored in tasks.json, without enriched fields. No payload.
		tasks, err := w.ListTasksFull()
		if err != nil {
			outputError(err.Error())
			os.Exit(1)
		}
		outputListFull(tasks)

	case "merge":
		// Remove + upsert + set_phase atomically.
		// Payload: {"remove_slugs":["a"],"upsert":{...},"set_phase":["slug","done"]}
		// set_phase is optional: omit the field or set to null to skip.
		if jsonPayload == "" {
			outputError("json payload required")
			os.Exit(1)
		}
		var payload struct {
			RemoveSlugs []string               `json:"remove_slugs"`
			Upsert      map[string]interface{} `json:"upsert"`
			SetPhase    []interface{}          `json:"set_phase"` // two elements: [id_or_slug, phase]
		}
		if err := json.Unmarshal([]byte(jsonPayload), &payload); err != nil {
			outputError(fmt.Sprintf("invalid json: %v", err))
			os.Exit(1)
		}

		// set_phase must be exactly two elements if provided.
		var setPhasePtr *[2]interface{}
		if len(payload.SetPhase) > 0 {
			if len(payload.SetPhase) != 2 {
				outputError("set_phase must be array of length 2")
				os.Exit(1)
			}
			setPhasePtr = &[2]interface{}{payload.SetPhase[0], payload.SetPhase[1]}
		}

		task, err := w.MergeTasks(payload.RemoveSlugs, payload.Upsert, setPhasePtr)
		if err != nil {
			outputError(err.Error())
			os.Exit(1)
		}
		outputSuccessWithTask(task)

	case "set-deps":
		// Replace the depends_on list for a task, with full validation.
		// Payload: {"slug":"my-task","depends_on":["other-task"]}
		if jsonPayload == "" {
			outputError("json payload required")
			os.Exit(1)
		}
		var payload struct {
			Slug      string   `json:"slug"`
			DependsOn []string `json:"depends_on"`
		}
		if err := json.Unmarshal([]byte(jsonPayload), &payload); err != nil {
			outputError(fmt.Sprintf("invalid json: %v", err))
			os.Exit(1)
		}
		err := w.SetDeps(payload.Slug, payload.DependsOn)
		if err != nil {
			outputError(err.Error())
			os.Exit(1)
		}
		outputSuccess()

	case "rerender":
		// Rebuild Home.md and _Sidebar.md from the current tasks.json. No payload.
		// Useful if render files have been corrupted or manually edited.
		err := w.Rerender()
		if err != nil {
			outputError(err.Error())
			os.Exit(1)
		}
		outputSuccess()

	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n", subcommand)
		os.Exit(1)
	}
}

// outputError writes {"ok":false,"error":"..."} to stdout and returns.
// Callers must call os.Exit(1) themselves after this.
func outputError(message string) {
	output := map[string]interface{}{
		"ok":    false,
		"error": message,
	}
	data, _ := json.Marshal(output)
	fmt.Println(string(data))
}

// outputSuccess writes {"ok":true} to stdout.
func outputSuccess() {
	output := map[string]interface{}{
		"ok": true,
	}
	data, _ := json.Marshal(output)
	fmt.Println(string(data))
}

// outputSuccessWithCount writes {"ok":true,"count":N} to stdout.
func outputSuccessWithCount(count int) {
	output := map[string]interface{}{
		"ok":    true,
		"count": count,
	}
	data, _ := json.Marshal(output)
	fmt.Println(string(data))
}

// outputSuccessWithTask writes {"ok":true,"task":{...}} to stdout.
func outputSuccessWithTask(task wiki.Task) {
	output := map[string]interface{}{
		"ok":   true,
		"task": task,
	}
	data, _ := json.Marshal(output)
	fmt.Println(string(data))
}

// outputGetTask writes {"ok":true,"task":{...}} or {"ok":true,"task":null} to stdout.
// task is a pointer: nil produces task:null in JSON (task not found, but not an error).
func outputGetTask(task *wiki.Task) {
	output := map[string]interface{}{
		"ok":   true,
		"task": task,
	}
	data, _ := json.Marshal(output)
	fmt.Println(string(data))
}

// outputListBrief writes {"ok":true,"tasks":[...]} with BriefTask objects to stdout.
func outputListBrief(tasks []wiki.BriefTask) {
	output := map[string]interface{}{
		"ok":    true,
		"tasks": tasks,
	}
	data, _ := json.Marshal(output)
	fmt.Println(string(data))
}

// outputListFull writes {"ok":true,"tasks":[...]} with full Task objects to stdout.
func outputListFull(tasks []wiki.Task) {
	output := map[string]interface{}{
		"ok":    true,
		"tasks": tasks,
	}
	data, _ := json.Marshal(output)
	fmt.Println(string(data))
}
