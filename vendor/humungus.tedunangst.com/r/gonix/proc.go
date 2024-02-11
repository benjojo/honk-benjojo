package gonix

// Set the proc name as seen in ps, top, etc.
func SetProcTitle(title string) {
	setproctitle(title)
}
