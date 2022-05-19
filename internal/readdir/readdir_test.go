package readdir

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"
)

func TestReadDir(t *testing.T) {
	compareInfo := func(t *testing.T, fi1, fi2 os.FileInfo) {
		if fi1.Name() != fi2.Name() {
			t.Errorf("Name(%q): got: %v want: %v", fi1.Name(), fi1.Name(), fi2.Name())
		}
		if fi1.Size() != fi2.Size() {
			t.Errorf("Size(%q): got: %v want: %v", fi1.Name(), fi1.Size(), fi2.Size())
		}
		if fi1.Mode().Type() != fi2.Mode().Type() {
			t.Errorf("Mode(%q): got: %v want: %v", fi1.Name(), fi1.Mode().Type(), fi2.Mode().Type())
		}
		if fi1.ModTime() != fi2.ModTime() {
			t.Errorf("ModTime(%q): got: %v want: %v", fi1.Name(), fi1.ModTime(), fi2.ModTime())
		}
		if fi1.IsDir() != fi2.IsDir() {
			t.Errorf("IsDir(%q): got: %v want: %v", fi1.Name(), fi1.IsDir(), fi2.IsDir())
		}
		if !reflect.DeepEqual(fi1.Sys(), fi2.Sys()) {
			t.Errorf("Sys(%q): got: %#v want: %#v", fi1.Name(), fi1.Sys(), fi2.Sys())
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	want, err := ioutil.ReadDir(wd)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ReadDir(wd)
	if err != nil {
		t.Fatal(err)
	}
	if len(want) != len(got) {
		t.Errorf("len want: %d len got: %d", len(want), len(got))
	}
	for i := range got {
		compareInfo(t, want[i], got[i])
		if t.Failed() {
			break
		}
	}
}

func TestReadDirError(t *testing.T) {
	if runtime.GOOS == "windows" {
		// We can't run this test since on Windows the FileInfo is populated
		// in ReadDir - unlike Unix* where it is lazily populated.
		t.Skip("skipping: test not applicable on Windows")
	}
	tempdir := t.TempDir()
	filename := filepath.Join(tempdir, "file.txt")
	if err := os.WriteFile(filename, []byte("file.txt"), 0644); err != nil {
		t.Fatal(err)
	}
	fis, err := ReadDir(tempdir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filename); err != nil {
		t.Fatal(err)
	}
	fi := fis[0]
	if n := fi.Size(); n != 0 {
		t.Errorf("Size(): got: %d want: %d", n, 0)
	}
	if modTime := fi.ModTime(); !modTime.IsZero() {
		t.Errorf("ModTime(): got: %s want: %s", modTime, time.Time{})
	}
	if sys := fi.Sys(); sys != nil {
		t.Errorf("Sys(): got: %v want: %v", sys, nil)
	}
}

func BenchmarkReadDir(b *testing.B) {
	benchdir := filepath.Join(runtime.GOROOT(), "src")
	if _, err := os.Stat(benchdir); err != nil {
		b.Skipf("Skipping: missing GOROOT: %q", benchdir)
	}
	b.Run("ReadDir", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err := ReadDir(benchdir); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("ioutil.ReadDir", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err := ioutil.ReadDir(benchdir); err != nil {
				b.Fatal(err)
			}
		}
	})
}
