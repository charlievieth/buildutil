//go:build !windows
// +build !windows

package readdir

import (
	"io/fs"
	"os"
	"sync"
	"time"
)

type fileInfo struct {
	fs.DirEntry
	info fs.FileInfo
	once sync.Once
}

func (f *fileInfo) Mode() fs.FileMode { return f.DirEntry.Type() }

// The Size, ModTime, and Sys are not used by go/build but implement
// them anyway (in case that changes) and make access thread-safe.

func (f *fileInfo) lazyInit() {
	f.once.Do(func() { f.info, _ = f.DirEntry.Info() })
}

func (f *fileInfo) Size() int64 {
	f.lazyInit()
	if f.info != nil {
		return f.info.Size()
	}
	return 0
}

func (f *fileInfo) ModTime() time.Time {
	f.lazyInit()
	if f.info != nil {
		return f.info.ModTime()
	}
	return time.Time{}
}

func (f *fileInfo) Sys() interface{} {
	f.lazyInit()
	if f.info != nil {
		return f.info.Sys()
	}
	return nil
}

// ReadDir is a faster version of ioutil.ReadDir that uses os.ReadDir
// and returns a wrapper around fs.FileInfo.
//
// This is roughly 3.5-4x faster than ioutil.ReadDir and is used heavily
// by the build.Context when importing packages.
func ReadDir(dirname string) ([]fs.FileInfo, error) {
	des, err := os.ReadDir(dirname)
	if err != nil {
		return nil, err
	}
	fis := make([]fs.FileInfo, len(des))
	for i, d := range des {
		fis[i] = &fileInfo{DirEntry: d}
	}
	return fis, nil
}
