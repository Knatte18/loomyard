// callees.go declares the members the caller-coverage tests delete and re-sign, and the one caller inside its own file.

package callees

import "example.com/glyphchain/other"

// Target is called from other files of this package, its external test package and another package.
func Target() int { return 1 }

// Helper is called by UsesHelper in this file.
func Helper() int { return 2 }

// UsesHelper calls Helper.
func UsesHelper() int { return Helper() }

// Thing has a method another package calls.
type Thing struct{}

// Method is called from another package.
func (t Thing) Method() int { return 3 }

// NotACall names Target only in this comment, in a string literal and as a field selector.
func NotACall(h other.Holder) string { return "Target" + string(rune(h.Target)) }
