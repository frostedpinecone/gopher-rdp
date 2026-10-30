//go:build windows

package rdpdr

import (
	"io/fs"
	"syscall"
)

// isHidden reports whether the file should be marked with FILE_ATTRIBUTE_HIDDEN.
// On Windows, this checks the native hidden attribute via GetFileAttributes.
func isHidden(name string, fi fs.FileInfo) bool {
	sys := fi.Sys()
	if attr, ok := sys.(*syscall.Win32FileAttributeData); ok {
		return attr.FileAttributes&syscall.FILE_ATTRIBUTE_HIDDEN != 0
	}
	// Fallback: dot-prefix convention
	return len(name) > 0 && name[0] == '.'
}
