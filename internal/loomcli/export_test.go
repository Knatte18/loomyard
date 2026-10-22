// export_test.go re-exports the run owner verdict for the package loomcli_test files that drive it directly over a forged hub.

package loomcli

// OwnerVerdict is the verdict RunOwnerVerdict returns.
type OwnerVerdict = ownerVerdict

// RunOwnerVerdict judges whether another machine owns the run of a slug.
var RunOwnerVerdict = runOwnerVerdict
