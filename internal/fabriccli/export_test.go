// export_test.go exposes the add verb's pair-push seam to this package's external tests.

package fabriccli

// SetPairPushForTest makes "lyx fabric add" push a new pair through push instead of a detached child, for the whole test binary.
func SetPairPushForTest(push func(codeWorktree, recordsWorktree string) error) {
	spawnPairPush = push
}
