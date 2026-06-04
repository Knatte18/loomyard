package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/Knatte18/mhgo/internal/wiki"
)

func main() {
	fs := flag.NewFlagSet("mhgo", flag.ExitOnError)
	wikiPathFlag := fs.String("wiki-path", "", "Path to wiki directory")
	fs.Parse(os.Args[1:])

	// Determine wiki path
	wikiPath := *wikiPathFlag
	if wikiPath == "" {
		wikiPath = os.Getenv("MHGO_WIKI_PATH")
	}
	if wikiPath == "" {
		wikiPath = ".wiki"
	}

	args := fs.Args()

	// Parse command: mhgo wiki <subcommand> [json-payload]
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
		outputSuccess()

	case "set-phase":
		if jsonPayload == "" {
			outputError("json payload required")
			os.Exit(1)
		}
		var payload struct {
			IDOrSlug interface{} `json:"id_or_slug"`
			Phase    *string     `json:"phase"`
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
			outputGetTask(nil)
		}

	case "list":
		tasks, err := w.ListTasksBrief()
		if err != nil {
			outputError(err.Error())
			os.Exit(1)
		}
		outputListBrief(tasks)

	case "list-full":
		tasks, err := w.ListTasksFull()
		if err != nil {
			outputError(err.Error())
			os.Exit(1)
		}
		outputListFull(tasks)

	case "merge":
		if jsonPayload == "" {
			outputError("json payload required")
			os.Exit(1)
		}
		var payload struct {
			RemoveSlugs []string               `json:"remove_slugs"`
			Upsert      map[string]interface{} `json:"upsert"`
			SetPhase    []interface{}          `json:"set_phase"`
		}
		if err := json.Unmarshal([]byte(jsonPayload), &payload); err != nil {
			outputError(fmt.Sprintf("invalid json: %v", err))
			os.Exit(1)
		}

		// Validate set_phase length
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

func outputError(message string) {
	output := map[string]interface{}{
		"ok":    false,
		"error": message,
	}
	data, _ := json.Marshal(output)
	fmt.Println(string(data))
}

func outputSuccess() {
	output := map[string]interface{}{
		"ok": true,
	}
	data, _ := json.Marshal(output)
	fmt.Println(string(data))
}

func outputSuccessWithTask(task wiki.Task) {
	output := map[string]interface{}{
		"ok":   true,
		"task": task,
	}
	data, _ := json.Marshal(output)
	fmt.Println(string(data))
}

func outputGetTask(task *wiki.Task) {
	output := map[string]interface{}{
		"ok":   true,
		"task": task,
	}
	data, _ := json.Marshal(output)
	fmt.Println(string(data))
}

func outputListBrief(tasks []wiki.BriefTask) {
	output := map[string]interface{}{
		"ok":    true,
		"tasks": tasks,
	}
	data, _ := json.Marshal(output)
	fmt.Println(string(data))
}

func outputListFull(tasks []wiki.Task) {
	output := map[string]interface{}{
		"ok":    true,
		"tasks": tasks,
	}
	data, _ := json.Marshal(output)
	fmt.Println(string(data))
}
