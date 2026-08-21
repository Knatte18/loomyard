// env.go implements the two unexported helpers every entry uses to validate the Env fields it
// reads: requireAbsRoot for a told absolute-path root, and requireSeam for a told injected seam.

package shedrecipe

import (
	"fmt"
	"path/filepath"
	"reflect"
)

// requireAbsRoot errors when value is empty or is not an absolute path per filepath.IsAbs, naming
// entry and field in both messages.
//
// An entry validates exactly the fields it consumes and never a field it does not read, so a
// caller filling only what its recipe needs is legal.
func requireAbsRoot(entry, field, value string) error {
	if value == "" {
		return fmt.Errorf("shedrecipe: %s: Env.%s must not be empty", entry, field)
	}
	if !filepath.IsAbs(value) {
		return fmt.Errorf("shedrecipe: %s: Env.%s %q is not absolute", entry, field, value)
	}
	return nil
}

// requireSeam errors when seam is nil, naming entry and field.
//
// It detects a typed-nil interface value as nil too, since Env.Shuttle and Env.Burler are
// interfaces a caller can fill with a nil concrete pointer: reflect.ValueOf(seam) is inspected, and
// a reflect.Ptr, reflect.Interface, reflect.Map, reflect.Slice, or reflect.Func kind whose IsNil()
// reports true is treated as nil.
//
// An entry validates exactly the fields it consumes and never a field it does not read, so a
// caller filling only what its recipe needs is legal.
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
