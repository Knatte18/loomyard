// env.go holds the unexported validators every entry runs over the Env fields it reads.
// requireAbsRoot checks a told absolute-path root; requireNonEmpty checks a told plain-name value;
// requireSeam checks a told injected seam.

package shedrecipe

import (
	"fmt"
	"path/filepath"
	"reflect"
)

// requireAbsRoot errors when value is empty or is not an absolute path per filepath.IsAbs.
// The error names entry and field.
func requireAbsRoot(entry, field, value string) error {
	if value == "" {
		return fmt.Errorf("shedrecipe: %s: Env.%s must not be empty", entry, field)
	}
	if !filepath.IsAbs(value) {
		return fmt.Errorf("shedrecipe: %s: Env.%s %q is not absolute", entry, field, value)
	}
	return nil
}

// requireNonEmpty errors when value is empty, naming entry and field.
// It serves plain names rather than paths, so it does not check for an absolute path.
func requireNonEmpty(entry, field, value string) error {
	if value == "" {
		return fmt.Errorf("shedrecipe: %s: Env.%s must not be empty", entry, field)
	}
	return nil
}

// gateFindingsPath returns the gate findings file path for row, or "" when the GateFindingsFile seam is nil.
func (e Env) gateFindingsPath(row string) string {
	if e.GateFindingsFile == nil {
		return ""
	}
	return e.GateFindingsFile(row)
}

// requireSeam errors when seam is nil, naming entry and field.
// A typed nil also counts: a nil pointer, interface, map, slice or func in seam is treated as nil.
// A caller can fill an interface field with such a value.
func requireSeam(entry, field string, seam any) error {
	if seam == nil {
		return fmt.Errorf("shedrecipe: %s: Env.%s must not be nil", entry, field)
	}
	v := reflect.ValueOf(seam)
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Map, reflect.Slice, reflect.Func:
		if v.IsNil() {
			return fmt.Errorf("shedrecipe: %s: Env.%s must not be nil", entry, field)
		}
	}
	return nil
}
