// export_test.go exposes the per-request deadline to the external tests.

package hubissues

import "time"

// SetPerRequest sets the deadline each of r's requests runs under, so external tests can change it.
func (r *Repo) SetPerRequest(d time.Duration) {
	r.perRequest = d
}
