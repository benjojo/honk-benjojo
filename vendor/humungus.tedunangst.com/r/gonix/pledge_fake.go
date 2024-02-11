//go:build !openbsd
// +build !openbsd

package gonix

func xPledge(promises string) error {
	return nil
}

func xUnveil(path string, perms string) error {
	return nil
}

func xUnveilEnd() {
}
