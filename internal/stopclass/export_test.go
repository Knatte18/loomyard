// export_test.go exposes the settlement table's class set to the external tests.

package stopclass

import (
	"maps"
	"slices"
)

// TableClasses returns the classes the settlement table holds a row for, in no fixed order.
func TableClasses() []Class {
	return slices.Collect(maps.Keys(table))
}
