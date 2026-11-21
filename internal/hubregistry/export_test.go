// export_test.go exposes the repository lookup by shortname to the external tests.

package hubregistry

// LookupRepository answers the repository name of the one hub in dir's record that recorded shortname.
// It refuses as Lookup does when the shortname names no hub or several, and as RepositoryName does after that.
func LookupRepository(dir, shortname string) (string, error) {
	hubPath, err := Lookup(dir, shortname)
	if err != nil {
		return "", err
	}
	return RepositoryName(hubPath)
}
