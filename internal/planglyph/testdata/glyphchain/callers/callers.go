package callers

import "example.com/glyphchain/callees"

// UseTarget calls callees.Target.
func UseTarget() int { return callees.Target() }

// UseMethod calls the Method of callees.Thing.
func UseMethod(t callees.Thing) int { return t.Method() }
