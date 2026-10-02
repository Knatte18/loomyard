// shortnamebinding.go owns the .lyx-shortname record: a plain single-line file at the board root holding the repo's shortname, recorded once on weft:main beside .lyx-warp.
// Like the warp binding, it is written to disk here but committed onto weft:main by the CLI layer through Bolt;
// this file spawns no git and never calls Bolt itself.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/logger"
)

// ShortnameFileName is the filename of the recorded repo shortname at the board root (<boardDir>/.lyx-shortname).
// It holds only the shortname, plus a trailing newline.
const ShortnameFileName = ".lyx-shortname"

// ReadShortname reads the recorded repo shortname from <boardDir>/.lyx-shortname and reports whether a usable one was found.
// Any read error and an empty-after-trim value report not found, like readWarpBinding;
// a value failing agentname.ValidateShortname also reports not found and logs a named warning,
// so a damaged record reads as a hub with no shortname that `lyx fabric shortname <shortname>` or `clone --shortname` records over.
func ReadShortname(boardDir string) (shortname string, found bool) {
	data, err := os.ReadFile(filepath.Join(boardDir, ShortnameFileName))
	if err != nil {
		return "", false
	}
	return usableShortname(string(data), boardDir)
}

// usableShortname applies the blank-and-grammar rule shared by ReadShortname and the weft probe.
// where names the record's location in the warning.
func usableShortname(raw, where string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false
	}
	if err := agentname.ValidateShortname(trimmed); err != nil {
		logger.Warn("fabricengine: recorded repo shortname is invalid; treating the hub as having no shortname", "file", ShortnameFileName, "where", where, "value", trimmed, "error", err)
		return "", false
	}
	return trimmed, true
}

// WriteShortname writes shortname plus a trailing newline to <boardDir>/.lyx-shortname, replacing any existing content.
// The caller commits the result onto weft:main through Bolt; this function only touches the working tree.
func WriteShortname(boardDir, shortname string) error {
	path := filepath.Join(boardDir, ShortnameFileName)
	if err := os.WriteFile(path, []byte(shortname+"\n"), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// resolveEffectiveShortname encodes the whole shortname-record rule, pure and git-free.
// recorded and found are the record at the probed weft, supplied is the --shortname value,
// and freshBind is true when the weft carries neither the warp binding nor the anchor marker.
// It returns the effective shortname, whether the caller must write a new record, a warning for a bound weft that has no shortname, or a refusal.
func resolveEffectiveShortname(recorded string, found bool, supplied string, freshBind bool) (effective string, writeRecord bool, warning string, err error) {
	if supplied != "" {
		if err := agentname.ValidateShortname(supplied); err != nil {
			return "", false, "", err
		}
	}
	if found {
		if supplied == "" || supplied == recorded {
			return recorded, false, "", nil
		}
		return "", false, "", fmt.Errorf("this repo's recorded shortname is %q, not %q; refusing to change it (`lyx fabric shortname` cannot change a recorded shortname either) — drop --shortname, or pass --shortname %s", recorded, supplied, recorded)
	}
	if supplied != "" {
		return supplied, true, "", nil
	}
	if freshBind {
		return "", false, "", fmt.Errorf("a fresh clone needs the repo's shortname: pass --shortname <shortname>, 2-6 characters matching [a-z][a-z0-9]{1,5}, e.g. lyx fabric clone --shortname lx <weft-url> <warp-url>")
	}
	return "", false, "this weft records no repo shortname, so no strand can be named until one is set; way forward: run `lyx fabric shortname <shortname>` in the hub", nil
}
