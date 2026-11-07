// span.go implements explicit-parent diagnostic spans: a Span carries a dotted path.
// StartSpan opens a root span, and Child opens a child whose path extends its parent's by one name.
// The Debug, Info and Warn methods stamp the span path and the process trace ID from trace.go on each record.
// There is no ambient current span: callers hold their own *Span and pass it explicitly.

package logger

// Span is one node of a span tree, identified by a dotted path that StartSpan begins and Child extends.
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

// End records err on s and logs that the span ended.
// It logs at Debug when err is nil and at Warn otherwise.
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
