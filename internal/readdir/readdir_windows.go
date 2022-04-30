//go:build windows
// +build windows

package readdir

import (
	"io/fs"
	"io/ioutil"
)

func ReadDir(dirname string) ([]fs.FileInfo, error) {
	// No performance advantage on Windows since ioutil.ReadDir
	// and os.ReadDir use the same functionality.
	return ioutil.ReadDir(dirname)
}
