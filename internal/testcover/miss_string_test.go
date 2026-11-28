// miss_string_test.go renders a Miss as text for the failure messages of this package's tests.

package testcover

// String renders the miss as its reason and detail.
func (m Miss) String() string {
	return string(m.Reason) + ": " + m.Detail
}
