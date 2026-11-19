// rotate.go expands a per-round list cyclically, so a round past the list's length never clamps to its last entry.

package hardenshed

// Rotate expands list cyclically to rounds entries, entry i being list[i % len(list)].
// An empty list stays empty, and a count of rounds below 1 gives no entry.
func Rotate[T any](list []T, rounds int) []T {
	if len(list) == 0 || rounds < 1 {
		return []T{}
	}
	rotated := make([]T, rounds)
	for i := range rotated {
		rotated[i] = list[i%len(list)]
	}
	return rotated
}
