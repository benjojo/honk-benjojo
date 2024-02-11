//go:build linux
// +build linux

package gonix

import (
	"syscall"
)

type xWatcher struct {
	id int
}

func newWatcher() (xWatcher, error) {
	id, err := syscall.InotifyInit()
	return xWatcher{id: id}, err
}

func (x *xWatcher) watchDirectory(dirname string) error {
	syscall.InotifyAddWatch(x.id, dirname, syscall.IN_CREATE|syscall.IN_MOVED_TO)
	return nil
}

func (x *xWatcher) watchFile(filename string) error {
	syscall.InotifyAddWatch(x.id, filename, syscall.IN_MODIFY|syscall.IN_DELETE_SELF)
	return nil
}

func (x *xWatcher) waitForChange() error {
	var buf [1024]byte
	syscall.Read(x.id, buf[:])
	return nil
}
