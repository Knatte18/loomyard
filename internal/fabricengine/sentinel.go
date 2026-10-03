// sentinel.go holds the one error type behind this package's exported refusal sentinels.

package fabricengine

// sentinelError renders msg byte-for-byte as Error() while matching its sentinel under errors.Is.
// A refusal carries its own operator-facing text, so the sentinel cannot be wrapped on with %w
// without changing that text.
type sentinelError struct {
	sentinel error
	msg      string
}

func (e *sentinelError) Error() string { return e.msg }

func (e *sentinelError) Is(target error) bool { return target == e.sentinel }
