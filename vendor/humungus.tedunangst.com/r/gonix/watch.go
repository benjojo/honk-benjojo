package gonix

type Watcher struct {
	x xWatcher
}

func NewWatcher() (Watcher, error) {
	x, err := newWatcher()
	return Watcher{x: x}, err
}

func (w *Watcher) WatchDirectory(dirname string) error {
	return w.x.watchDirectory(dirname)
}

func (w *Watcher) WatchFile(filename string) error {
	return w.x.watchFile(filename)
}

func (watcher *Watcher) WaitForChange() error {
	return watcher.x.waitForChange()
}
