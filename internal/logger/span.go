// span.go implements explicit-parent diagnostic spans layered on top of the
// process-wide trace identity (trace.go): a Span is a plain value carrying a
// dotted path built up via StartSpan/Child, with span-scoped Debug/Info/Warn
// methods that stamp span=<path> alongside the trace= field every
// package-level Debug/Info/Warn call already stamps. Per discussion.md's
// explicit-span-parenting decision, there is no ambient "current span"
// global -- a caller always holds and threads its own *Span explicitly.

package logger

// Span represents one node in an explicit-parent span tree: a dotted path built by StartSpan and extended by Child.
type Span struct {
	path string
	err  error
}

// StartSpan opens a root span under the process trace.
func StartSpan(name string, args ...any) *Span {
	s := &Span{path: name}
	s.Debug("span started", args...)
	return s
}

// Child returns a new span whose path is s's path with name appended.
func (s *Span) Child(name string, args ...any) *Span {
	child := &Span{path: s.path + "." + name}
	child.Debug("span started", args...)
	return child
}

// End closes s, recording err. End(nil) emits at Debug; End with non-nil err emits at Warn.
func (s *Span) End(err error) {
	s.err = err
	if err != nil {
		s.Warn("span ended", "err", err)
		return
	}
	s.Debug("span ended")
}

// Debug logs msg at debug level, stamping span=s.path.
func (s *Span) Debug(msg string, args ...any) {
	log.With("trace", TraceID(), "span", s.path).Debug(msg, args...)
}

// Info logs msg at info level, stamping span=s.path.
func (s *Span) Info(msg string, args ...any) {
	log.With("trace", TraceID(), "span", s.path).Info(msg, args...)
}

// Warn logs msg at warn level, stamping span=s.path.
func (s *Span) Warn(msg string, args ...any) {
	log.With("trace", TraceID(), "span", s.path).Warn(msg, args...)
}
