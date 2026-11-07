// settings.go renders the JSON-object file targets, such as a worktree's local Claude Code settings.
// It folds a target's entries into one object, merges that object into the file, and records what it set.

package heddleengine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// recordSuffix ends the name of the record of the object a JSON-object target's last render set.
// The record sits in the scratch directory's heddle directory, beside the render lock.
const recordSuffix = ".rendered.json"

// validateObjectEntry refuses an entry that names a JSON-object target unless every target it names is one.
// Such an entry's text must also be one JSON object.
// An entry naming no JSON-object target passes.
func validateObjectEntry(slug string, targets []string, text string) error {
	objects := objectTargetCount(targets)
	switch {
	case objects == 0:
		return nil
	case objects != len(targets):
		return fmt.Errorf("entry %q names a JSON-object target beside a text target, and one text cannot serve both; way forward: aim it at %s alone, and give the text targets an entry of their own", slug, strings.Join(objectTargetNames(), ", "))
	}
	if _, ok := decodeObject(text); !ok {
		return fmt.Errorf("entry %q is aimed at %s, so its text must be one JSON object; way forward: give a --text-file holding one object, such as {\"permissions\": {\"deny\": [\"Workflow\"]}}", slug, strings.Join(targets, ", "))
	}
	return nil
}

// decodeObject decodes text as exactly one JSON object, keeping its numbers exact.
// It reports false for any other text.
func decodeObject(text string) (map[string]any, bool) {
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil || object == nil {
		return nil, false
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, false
	}
	return object, true
}

// encodeObject encodes object indented by two spaces and ending in a newline, with no HTML escaping.
func encodeObject(object map[string]any) (string, error) {
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(object); err != nil {
		return "", err
	}
	return out.String(), nil
}

// foldObjects folds the objects of the layered entries that name target, in render order, and reports whether any does.
// An entry whose text holds no JSON object is left out with a warning, and invalid reports whether any was;
// only a hand-edited store or override file can hold one, since every write validates the text.
func foldObjects(target Target, entries []LayeredEntry) (folded map[string]any, held, invalid bool, warnings []string) {
	folded = map[string]any{}
	for _, entry := range entries {
		if !slices.Contains(entry.Targets, target.Name) {
			continue
		}
		held = true
		object, ok := decodeObject(entry.Text)
		if !ok {
			invalid = true
			warnings = append(warnings, fmt.Sprintf("the text of entry %q holds no JSON object, so %s leaves it out; way forward: give it one with `lym heddle set %s --text-file <path>`, adding `--layer %s` for an override", entry.Slug, target.File, entry.Slug, OverrideLayer))
			continue
		}
		folded = mergeJSON(folded, object).(map[string]any)
	}
	return folded, held, invalid, warnings
}

// mergeJSON returns over merged onto base.
// Two objects merge key by key at every depth, and two lists join, the items of over that base lacks appended.
// Any other pair yields over, so a scalar of over replaces base's.
// Neither input is changed.
func mergeJSON(base, over any) any {
	switch overValue := over.(type) {
	case map[string]any:
		baseObject, ok := base.(map[string]any)
		if !ok {
			return overValue
		}
		merged := make(map[string]any, len(baseObject)+len(overValue))
		maps.Copy(merged, baseObject)
		for key, value := range overValue {
			if held, found := merged[key]; found {
				merged[key] = mergeJSON(held, value)
			} else {
				merged[key] = value
			}
		}
		return merged
	case []any:
		baseList, ok := base.([]any)
		if !ok {
			return overValue
		}
		merged := slices.Clone(baseList)
		for _, item := range overValue {
			if !containsJSON(merged, item) {
				merged = append(merged, item)
			}
		}
		return merged
	}
	return over
}

// dropStale returns current with what previous set and next no longer sets taken out.
// It also reports whether anything of current is left to keep.
// An object loses the members, and a list the items, that previous holds and next lacks, at every depth.
// A scalar previous set is taken out only while current still holds it, so a value changed since stays.
// A container the taking out empties is not kept; one that was empty already is.
// inNext reports whether next is present at all.
// Neither input is changed.
func dropStale(current, previous, next any, inNext bool) (any, bool) {
	switch previousValue := previous.(type) {
	case map[string]any:
		currentObject, ok := current.(map[string]any)
		if !ok {
			return current, true
		}
		nextObject, _ := next.(map[string]any)
		kept := maps.Clone(currentObject)
		for key, previousMember := range previousValue {
			currentMember, found := kept[key]
			if !found {
				continue
			}
			nextMember, memberInNext := nextObject[key]
			if remaining, keep := dropStale(currentMember, previousMember, nextMember, memberInNext); keep {
				kept[key] = remaining
			} else {
				delete(kept, key)
			}
		}
		return kept, len(kept) > 0 || len(currentObject) == 0
	case []any:
		currentList, ok := current.([]any)
		if !ok {
			return current, true
		}
		nextList, _ := next.([]any)
		kept := []any{}
		for _, item := range currentList {
			if !containsJSON(previousValue, item) || containsJSON(nextList, item) {
				kept = append(kept, item)
			}
		}
		return kept, len(kept) > 0 || len(currentList) == 0
	}
	return current, inNext || !reflect.DeepEqual(current, previous)
}

// containsJSON reports whether list holds a value equal to item.
func containsJSON(list []any, item any) bool {
	return slices.ContainsFunc(list, func(held any) bool { return reflect.DeepEqual(held, item) })
}

// readObjectFile reads the JSON object in the file at path and reports whether the file exists.
// An absent or blank file gives an empty object, and a file holding anything but one JSON object gives nil.
func readObjectFile(path string) (map[string]any, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read %s: %w", path, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, true, nil
	}
	object, ok := decodeObject(string(data))
	if !ok {
		return nil, true, nil
	}
	return object, true, nil
}

// readRecord reads the object a JSON-object target's last render set.
// It gives nil when no render recorded one, or when the record holds no object.
func readRecord(path string) (map[string]any, error) {
	object, exists, err := readObjectFile(path)
	if err != nil || !exists {
		return nil, err
	}
	return object, nil
}

// removeRecord removes the record at path, and an absent record is no error.
func removeRecord(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

// objectRefusal returns the warning that keeps a JSON-object target's file at path unwritten, and whether one does.
// current is the file's object, nil when the file holds no JSON object.
// A blocked staging directory refuses with no warning of its own, since the render already warned of it.
func objectRefusal(req RenderRequest, target Target, path string, current map[string]any, blocked bool) (string, bool) {
	switch {
	case blocked:
		return "", true
	case !slices.Contains(req.Excluded, target.File):
		return fabricengine.ExcludeWayForward(target.File), true
	case !slices.Contains(req.Excluded, tempDirName):
		return fabricengine.ExcludeWayForward(tempDirName), true
	case current == nil:
		return fmt.Sprintf("%s holds no JSON object, so it is left untouched; way forward: fix or remove it, then run `%s`", path, renderVerb), true
	}
	return "", false
}

// renderObjectTarget merges the entries aimed at the JSON-object target into its file at req's anchor.
// It notes the outcome in report.
// It takes out of the file what the last render set and no entry sets now, then merges the folded entries in.
// A key the file holds that heddle never set stays, and a scalar heddle sets replaces the file's.
// It records the folded object for the next render, and drops the record once the file no longer holds what it names.
// With no entry and no record it touches nothing; with no entry it removes a file the taking out empties.
// While an entry holds no JSON object nothing is taken out, and the record keeps naming what the last render set.
func renderObjectTarget(report *Report, req RenderRequest, target Target, entries []LayeredEntry, blocked bool) error {
	folded, held, invalid, warnings := foldObjects(target, entries)
	for _, warning := range warnings {
		report.warn(warning)
	}
	recordPath := filepath.Join(req.ScratchDir, lymdirs.HeddleDirName, target.Name+recordSuffix)
	previous, err := readRecord(recordPath)
	if err != nil {
		return err
	}
	if !held && previous == nil {
		return nil
	}
	path := filepath.Join(req.Anchor, filepath.FromSlash(target.File))
	current, exists, err := readObjectFile(path)
	if err != nil {
		return err
	}
	if warning, refused := objectRefusal(req, target, path, current, blocked); refused {
		if warning != "" {
			report.warn(warning)
		}
		if held {
			report.Unwritten = append(report.Unwritten, target.File)
		}
		return nil
	}

	var remaining any = current
	recorded := folded
	if invalid {
		recorded = mergeJSON(previous, folded).(map[string]any)
	} else {
		remaining, _ = dropStale(current, previous, folded, true)
	}
	merged := mergeJSON(remaining, folded).(map[string]any)
	if !held && len(merged) == 0 {
		if exists {
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("remove %s: %w", path, err)
			}
			report.Removed = append(report.Removed, target.File)
		}
		return removeRecord(recordPath)
	}
	text, err := encodeObject(merged)
	if err != nil {
		return fmt.Errorf("encode %s: %w", target.File, err)
	}
	if err := writeRendered(filepath.Join(req.Anchor, tempDirName), target.Name, path, text); err != nil {
		return err
	}
	report.Written = append(report.Written, target.File)
	if !held {
		return removeRecord(recordPath)
	}
	record, err := encodeObject(recorded)
	if err != nil {
		return fmt.Errorf("encode the record %s: %w", recordPath, err)
	}
	if err := fsx.AtomicWriteBytes(recordPath, []byte(record)); err != nil {
		return fmt.Errorf("write %s: %w", recordPath, err)
	}
	return nil
}
