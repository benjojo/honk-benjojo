package gonix

/*
Kills the process when you break your promise.
*/
func Pledge(promises string) error {
	return xPledge(promises)
}

/*
Only let me see what's been unveiled.
*/
func Unveil(path string, perms string) error {
	return xUnveil(path, perms)
}

/*
Commit and seal Unveil changes.
*/
func UnveilEnd() {
	xUnveilEnd()
}
