// scripts.go fills the KWin scripts the package loads with their input.

package kwinplace

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed query.js
var queryScriptSource string

//go:embed apply.js
var applyScriptSource string

// scriptInputPlaceholder is what each script holds where its JSON input object goes.
const scriptInputPlaceholder = "__LYM_KWINPLACE_INPUT__"

// QueryScript returns the script that reports KWin's state.
// The script tags every line with runID and reads the tiles of the output named tilingOutput.
func QueryScript(runID, tilingOutput string) string {
	return fillScript(queryScriptSource, struct {
		RunID        string `json:"runId"`
		TilingOutput string `json:"tilingOutput"`
	}{runID, tilingOutput})
}

// ApplyScript returns the script that carries out order and reports the result, tagging every line with runID.
func ApplyScript(runID string, order ApplyOrder) string {
	return fillScript(applyScriptSource, struct {
		RunID string `json:"runId"`
		ApplyOrder
	}{runID, order})
}

func fillScript(source string, input any) string {
	data, err := json.Marshal(input)
	if err != nil {
		panic(fmt.Sprintf("kwinplace: marshal script input: %v", err))
	}
	return strings.Replace(source, scriptInputPlaceholder, string(data), 1)
}
