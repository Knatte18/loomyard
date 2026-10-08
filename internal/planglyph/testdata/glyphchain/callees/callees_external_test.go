// callees_external_test.go calls Target from the external callees_test package.

package callees_test

import (
	"testing"

	"example.com/glyphchain/callees"
)

func TestTarget(t *testing.T) { _ = callees.Target() }
