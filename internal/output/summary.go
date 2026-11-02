// summary.go renders a JSON envelope as a short readable summary for a person at a terminal:
// the outcome, what the verb did, and every error, warning and way forward, one line each, with long lists collapsed to a count.

package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// EnvelopeFlag is the name of the global flag that keeps the full JSON envelope where the readable summary would print.
const EnvelopeFlag = "envelope"

// Envelope keys the summary reads by meaning.
const (
	okKey         = "ok"
	errorKey      = "error"
	wayForwardKey = "way_forward"
)

// Bounds of the summary: a list of plain values longer than summaryListLimit collapses to a count,
// and an object nested deeper than summaryMaxDepth collapses to its field count and the notes inside it.
// A list of objects never collapses, since its one line per item is what a listing verb shows.
// A list of plain values under an error, warning or way-forward key never collapses either.
const (
	summaryListLimit = 5
	summaryMaxDepth  = 2
)

// labelKeys name, in order of preference, the field that identifies an object in a list.
var labelKeys = []string{"slug", "subject", "name", "branch", "id"}

// lineKeys name, in order of preference, the field that says in one line what happened to an object in a list.
var lineKeys = []string{"line", "message", "title", "state", "status", "outcome"}

// summaryFooter closes a summary that left anything out, naming where the full envelope is.
const summaryFooter = "full envelope: rerun with --" + EnvelopeFlag

// Summary renders line as the readable summary when it holds one JSON envelope: an object with a boolean ok key.
// It reports false for any other line, which the caller then prints as it stands.
//
// The summary opens with `ok`, or `failed: ` and the error.
// Every other field follows in key order, an empty or false one left out:
// a plain value on one line;
// a list of plain values joined on one line, or its count when it is longer than the list limit;
// a list of plain values under an error, warning or way-forward key one line per value;
// an object as its fields, indented, or its field count when it lies deeper than the depth limit;
// and a list of objects one line per item, naming the item's label and its line field.
// Whatever a count or a one-line item hides still shows its errors, warnings and ways forward, one line each, by their path.
// A summary that hid anything ends naming the flag that prints the full envelope.
func Summary(line []byte) (string, bool) {
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.UseNumber()
	var envelope map[string]any
	if err := decoder.Decode(&envelope); err != nil || decoder.More() {
		return "", false
	}
	ok, isBool := envelope[okKey].(bool)
	if !isBool {
		return "", false
	}

	r := &summaryRenderer{}
	if ok {
		r.line(0, "ok")
	} else {
		message, _ := envelope[errorKey].(string)
		r.text(0, "failed", message)
	}
	for _, key := range sortedKeys(envelope) {
		if key == okKey || key == errorKey {
			continue
		}
		r.field(0, key, envelope[key])
	}
	if r.hid {
		r.line(0, summaryFooter)
	}
	return r.out.String(), true
}

// summaryRenderer accumulates the summary's lines and whether any part of the envelope was left out.
type summaryRenderer struct {
	out strings.Builder
	hid bool
}

// line writes text as one line at indent levels of two spaces.
func (r *summaryRenderer) line(indent int, text string) {
	r.out.WriteString(strings.Repeat("  ", indent))
	r.out.WriteString(text)
	r.out.WriteString("\n")
}

// text writes `key: value`, a multi-line value's further lines indented one level below the key.
func (r *summaryRenderer) text(indent int, key, value string) {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	r.line(indent, key+": "+lines[0])
	for _, rest := range lines[1:] {
		r.line(indent+1, rest)
	}
}

// field writes key and value at indent and depth, leaving out an empty value.
func (r *summaryRenderer) field(depth int, key string, value any) {
	if isEmpty(value) {
		return
	}
	switch typed := value.(type) {
	case map[string]any:
		if depth >= summaryMaxDepth {
			r.hid = true
			r.line(depth, fmt.Sprintf("%s: %d fields", key, len(typed)))
			r.notes(depth+1, "", typed)
			return
		}
		r.line(depth, key+":")
		for _, child := range sortedKeys(typed) {
			r.field(depth+1, child, typed[child])
		}
	case []any:
		r.list(depth, key, typed)
	default:
		r.text(depth, key, plain(value))
	}
}

// list writes a list of plain values on one line, or its count past the list limit, and a list holding an object one line per item.
// A list of plain values under a note key is never collapsed: each of its non-empty values is one line.
func (r *summaryRenderer) list(depth int, key string, items []any) {
	if !holdsObject(items) {
		if isNoteKey(key) {
			r.noteLines(depth, key, items)
			return
		}
		if len(items) > summaryListLimit {
			r.hid = true
			r.line(depth, fmt.Sprintf("%s: %d items", key, len(items)))
			return
		}
		values := make([]string, len(items))
		for i, item := range items {
			values[i] = plain(item)
		}
		r.line(depth, key+": "+strings.Join(values, ", "))
		return
	}
	r.line(depth, key+":")
	for _, item := range items {
		r.item(depth+1, item)
	}
}

// item writes one list item on one line, then the notes the line does not show.
// An object's line names its label and its line field, or its plain fields when it has neither.
func (r *summaryRenderer) item(depth int, item any) {
	object, isObject := item.(map[string]any)
	if !isObject {
		r.line(depth, "- "+plain(item))
		return
	}
	label := firstString(object, labelKeys)
	summary := firstString(object, lineKeys)
	shown := map[string]bool{label.key: true, summary.key: true}
	var text string
	switch {
	case label.value != "" && summary.value != "":
		text = label.value + ": " + summary.value
	case label.value != "" || summary.value != "":
		text = label.value + summary.value
	default:
		text, shown = plainFields(object)
	}
	r.line(depth, "- "+firstLine(text))
	for _, key := range sortedKeys(object) {
		if !shown[key] && !isEmpty(object[key]) && !isNoteKey(key) {
			r.hid = true
		}
	}
	r.notes(depth+1, "", object)
}

// notes writes every error, warning and way forward value holds at any depth, one line each, named by its path below the written context.
func (r *summaryRenderer) notes(indent int, path string, value any) {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range sortedKeys(typed) {
			child := typed[key]
			childPath := joinPath(path, key)
			if isNoteKey(key) && !isEmpty(child) {
				r.noteLines(indent, childPath, child)
				continue
			}
			r.notes(indent, childPath, child)
		}
	case []any:
		for _, item := range typed {
			object, isObject := item.(map[string]any)
			if !isObject {
				continue
			}
			itemPath := path
			if label := firstString(object, labelKeys); label.value != "" {
				itemPath = joinPath(path, label.value)
			}
			r.notes(indent, itemPath, object)
		}
	}
}

// noteLines writes a note's value, one line per string it holds.
func (r *summaryRenderer) noteLines(indent int, path string, value any) {
	items, isList := value.([]any)
	if !isList {
		r.text(indent, path, plain(value))
		return
	}
	for _, item := range items {
		if !isEmpty(item) {
			r.text(indent, path, plain(item))
		}
	}
}

// keyValue is one field of an object, both empty when the object has none of the keys asked for.
type keyValue struct {
	key   string
	value string
}

// firstString returns the first of keys whose value in object is a non-empty plain value.
func firstString(object map[string]any, keys []string) keyValue {
	for _, key := range keys {
		value, present := object[key]
		if !present || isEmpty(value) {
			continue
		}
		switch value.(type) {
		case map[string]any, []any:
			continue
		}
		return keyValue{key: key, value: plain(value)}
	}
	return keyValue{}
}

// plainFields joins object's non-empty plain fields that are not notes as `key=value` in key order, and returns the keys it showed.
func plainFields(object map[string]any) (string, map[string]bool) {
	shown := map[string]bool{}
	var parts []string
	for _, key := range sortedKeys(object) {
		value := object[key]
		if isEmpty(value) || isNoteKey(key) {
			continue
		}
		switch value.(type) {
		case map[string]any, []any:
			continue
		}
		shown[key] = true
		parts = append(parts, key+"="+plain(value))
	}
	return strings.Join(parts, ", "), shown
}

// isNoteKey reports whether key names an error, a warning or a way forward.
func isNoteKey(key string) bool {
	return key == errorKey || key == wayForwardKey ||
		strings.HasSuffix(key, "_"+errorKey) ||
		strings.HasSuffix(key, "warning") || strings.HasSuffix(key, "warnings")
}

// isEmpty reports whether value says nothing: null, false, an empty string, an empty list or an empty object.
func isEmpty(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case bool:
		return !typed
	case string:
		return strings.TrimSpace(typed) == ""
	case []any:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	}
	return false
}

// holdsObject reports whether any of items is an object.
func holdsObject(items []any) bool {
	for _, item := range items {
		if _, isObject := item.(map[string]any); isObject {
			return true
		}
	}
	return false
}

// plain formats a plain JSON value as text; a nested value it cannot show on one line reads as its kind and size.
func plain(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case map[string]any:
		return fmt.Sprintf("%d fields", len(typed))
	case []any:
		return fmt.Sprintf("%d items", len(typed))
	}
	return fmt.Sprint(value)
}

// firstLine returns text up to its first line break.
func firstLine(text string) string {
	first, _, _ := strings.Cut(text, "\n")
	return first
}

// joinPath joins a parent path and a child name with a dot, the child alone when the parent is empty.
func joinPath(parent, child string) string {
	if parent == "" {
		return child
	}
	return parent + "." + child
}

// sortedKeys returns object's keys in byte order.
func sortedKeys(object map[string]any) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
