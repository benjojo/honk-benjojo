//go:build openbsd || darwin || freebsd || netbsd
// +build openbsd darwin freebsd netbsd

package gonix

import (
	"os"
	"syscall"
)

type xWatcher struct {
	file *os.File
	kq   int
}

func newWatcher() (xWatcher, error) {
	kq, err := syscall.Kqueue()
	return xWatcher{kq: kq}, err
}

func (x *xWatcher) watchDirectory(dirname string) error {
	dir, err := os.Open(dirname)
	if err != nil {
		return err
	}
	x.file = dir
	var kev [1]syscall.Kevent_t
	syscall.SetKevent(&kev[0], int(dir.Fd()), syscall.EVFILT_VNODE, syscall.EV_ADD|syscall.EV_CLEAR)
	kev[0].Fflags = syscall.NOTE_WRITE
	_, err = syscall.Kevent(x.kq, kev[:], nil, nil)
	return err
}

func (x *xWatcher) watchFile(filename string) error {
	if x.file != nil {
		x.file.Close()
		x.file = nil
	}
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	x.file = file
	var kev [1]syscall.Kevent_t
	syscall.SetKevent(&kev[0], int(file.Fd()), syscall.EVFILT_VNODE, syscall.EV_ADD|syscall.EV_CLEAR)
	kev[0].Fflags = syscall.NOTE_WRITE | syscall.NOTE_DELETE
	_, err = syscall.Kevent(x.kq, kev[:], nil, nil)
	return err
}

func (x *xWatcher) waitForChange() error {
	var kev [1]syscall.Kevent_t
	_, err := syscall.Kevent(x.kq, nil, kev[:], nil)
	return err
}
