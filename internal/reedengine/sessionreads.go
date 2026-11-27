// sessionreads.go builds the two reads a caller keeping one session's strand makes of the strand table:
// whether the strand is live, and the registration of its pane process.

package reedengine

import "fmt"

// SessionLiveness returns a reader of whether the strand guid names is live, built over the strand table status reads.
// The table is read at each call, and a strand is live when it is present, live and not retiring.
func SessionLiveness(status func() (StatusResult, error)) func(guid string) (bool, error) {
	return func(guid string) (bool, error) {
		result, err := status()
		if err != nil {
			return false, err
		}
		for _, strand := range result.Strands {
			if strand.GUID == guid {
				return strand.Live && !strand.Retiring, nil
			}
		}
		return false, nil
	}
}

// SessionRegistrar returns a function that hands register the pane process of the strand guid names.
// It reads that process from the strand table status reads at each call.
// A strand absent from the table, or with no live pane process, has nothing to register and is an error.
func SessionRegistrar(status func() (StatusResult, error), register func(pid int) error) func(guid string) error {
	return func(guid string) error {
		result, err := status()
		if err != nil {
			return err
		}
		for _, strand := range result.Strands {
			if strand.GUID != guid {
				continue
			}
			if strand.PanePID == 0 {
				return fmt.Errorf("reedengine: strand %s has no live pane process to register", guid)
			}
			return register(strand.PanePID)
		}
		return fmt.Errorf("reedengine: strand %s is not in reed's strand table", guid)
	}
}
