//go:build !windows

package rdpdr

import (
	"io/fs"
)

// isHidden reports whether the file should be marked with FILE_ATTRIBUTE_HIDDEN.
// On Unix, files starting with '.' are conventionally hidden.
func isHidden(name string, _ fs.FileInfo) bool {
	return len(name) > 0 && name[0] == '.'
}
