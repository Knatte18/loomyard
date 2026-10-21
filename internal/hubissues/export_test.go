// export_test.go exposes the per-request deadline to the external tests.

package hubissues

import "time"

// SetPerRequest sets the deadline of each of r's requests.
func (r *Repo) SetPerRequest(d time.Duration) {
	r.perRequest = d
}
