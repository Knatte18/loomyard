// span.go implements explicit-parent diagnostic spans layered on top of the
// process-wide trace identity (trace.go): a Span is a plain value carrying a
// dotted path built up via StartSpan/Child, with span-scoped Debug/Info/Warn
// methods that stamp span=<path> alongside the trace= field every
// package-level Debug/Info/Warn call already stamps. Per discussion.md's
// explicit-span-parenting decision, there is no ambient "current span"
// global -- a caller always holds and threads its own *Span explicitly.

package logger

// Span represents one node in an explicit-parent span tree: a dotted path
// (e.g. "burler.round.spawn") built by StartSpan and extended by Child, plus
// the error End was last called with. A Span is a plain value only ever
// touched by the goroutine holding it -- it has no locking of its own, per
// discussion.md's concurrency-contract decision that spans hold no shared
// state. The zero Span is not meaningful; always construct one via StartSpan
// or Child.
type Span struct {
	path string // dotted path, e.g. "burler.round.spawn"
	err  error  // set by End, read by nothing outside End itself; kept for clarity
}

// StartSpan opens a root span under the process trace: path is set to name
// verbatim, with no parent segment. It emits the new span's own open record
// at Debug (via the span-scoped Debug method, so the record itself carries
// span=name), per explicit-span-parenting's "Levels of the span records
// themselves" rule -- on a normal run this open record never reaches the
// durable sink, since Debug never does.
func StartSpan(name string, args ...any) *Span {
	s := &Span{path: name}
	s.Debug("span started", args...)
	return s
}

// Child returns a new *Span whose path is s's path with name appended as a
// further dotted segment (e.g. "a.b".Child("c") -> "a.b.c"). This is the
// only way a span acquires a parent: an explicit handle passed by the
// caller, never an ambient package-level "current span" -- per
// explicit-span-parenting's "There is no ambient 'current span' global"
// rule. Like StartSpan, it emits the new child's own open record at Debug,
// stamped with the child's own path.
func (s *Span) Child(name string, args ...any) *Span {
	child := &Span{path: s.path + "." + name}
	child.Debug("span started", args...)
	return child
}

// End closes s, recording err. Per explicit-span-parenting's level split:
// End(nil) emits its close record at Debug; End with a non-nil err emits its
// close record at Warn, carrying err as a field, so a failing span's close
// is the one open/close record that does reach the durable sink. End does
// not "restore" any global -- there is nothing to restore, since Span holds
// no ambient state. Callers invoke it via `defer sp.End(err)` at the site
// that opened the span.
func (s *Span) End(err error) {
	s.err = err
	if err != nil {
		s.Warn("span ended", "err", err)
		return
	}
	s.Debug("span ended")
}

// Debug logs msg at debug level through the same dual-handler fan-out the
// package-level Debug uses, additionally stamping span=s.path so the line
// carries this span's causal position in the tree.
func (s *Span) Debug(msg string, args ...any) {
	log.With("trace", TraceID(), "span", s.path).Debug(msg, args...)
}

// Info logs msg at info level through the same dual-handler fan-out the
// package-level Info uses, additionally stamping span=s.path -- an Info
// emitted via a span-scoped call reaches the durable sink exactly like a
// package-level Info call does, carrying span= in addition to trace=.
func (s *Span) Info(msg string, args ...any) {
	log.With("trace", TraceID(), "span", s.path).Info(msg, args...)
}

// Warn logs msg at warn level through the same dual-handler fan-out the
// package-level Warn uses, additionally stamping span=s.path.
func (s *Span) Warn(msg string, args ...any) {
	log.With("trace", TraceID(), "span", s.path).Warn(msg, args...)
}
