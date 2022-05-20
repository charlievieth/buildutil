package contextutil

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/build"
	"go/format"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/charlievieth/buildutil/internal/readdir"
	"github.com/charlievieth/buildutil/internal/util"
	"golang.org/x/tools/go/buildutil"
)

var benchInfo struct {
	sync.Once
	PkgDir string
	Err    error
}

func initBenchInfo(b *testing.B) string {
	const pkgName = "github.com/charlievieth/buildutil"
	benchInfo.Once.Do(func() {
		var pkg *build.Package
		pkg, benchInfo.Err = build.Import(pkgName, ".", build.FindOnly)
		if benchInfo.Err == nil {
			benchInfo.PkgDir = pkg.Dir
		}
		b.ResetTimer()
	})
	if benchInfo.Err != nil {
		b.Fatal(benchInfo.Err)
	}
	return benchInfo.PkgDir
}

func TestAbsPath(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	test := func(t *testing.T, ctxt *build.Context, dir, want string) {
		got, err := absPath(ctxt, dir)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("absPath(%q) = %q want: %q", dir, got, want)
		}
	}

	t.Run("Abs", func(t *testing.T) {
		ctxt := util.CopyContext(&build.Default)
		test(t, ctxt, wd, wd)
	})

	t.Run("Clean", func(t *testing.T) {
		const sep = string(filepath.Separator)
		ctxt := util.CopyContext(&build.Default)
		dir := strings.ReplaceAll(wd, sep, sep+sep) + sep
		test(t, ctxt, dir, wd)

		ctxt.JoinPath = func(elem ...string) string {
			if !reflect.DeepEqual(elem, []string{dir}) {
				t.Errorf("JoinPath called with %q want: %q", elem, []string{dir})
			}
			return filepath.Join(elem...)
		}
		test(t, ctxt, dir, wd)
	})

	t.Run("Relative", func(t *testing.T) {
		ctxt := util.CopyContext(&build.Default)
		want := filepath.Join(wd, "foo")
		test(t, ctxt, "foo", want)

		want = filepath.Join(wd, "./..")
		test(t, ctxt, "./..", want)
	})

	t.Run("Context.Dir", func(t *testing.T) {
		ctxt := util.CopyContext(&build.Default)
		ctxt.Dir = "../"
		test(t, ctxt, filepath.Base(wd), wd)
	})
}

func TestContainingDirectory(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	ctxt := build.Default
	if _, err := ContainingDirectory(&ctxt, wd, ""); err == nil {
		t.Error("expected error for no tombstones")
	}
	testAbsErr := func(t *testing.T, path string, err error) {
		var target *os.PathError
		if !errors.As(err, &target) {
			t.Fatalf("want: %T got: %#v", target, err)
		}
		if target.Path != path {
			t.Errorf("path mismatch got: %q want: %q", target.Path, path)
		}
		if target.Err != errNotAbsolute {
			t.Errorf("error mismatch got: %v want: %v", target.Err, errNotAbsolute)
		}
	}
	_, err = ContainingDirectory(&ctxt, wd, "../../buildutil", DefaultProjectTombstones...)
	testAbsErr(t, "../../buildutil", err)
	_, err = ContainingDirectory(&ctxt, "../relative", wd, DefaultProjectTombstones...)
	testAbsErr(t, "../relative", err)
}

func TestContainingDirectory_FakeContext(t *testing.T) {
	orig := buildutil.FakeContext(map[string]map[string]string{
		"modpkg": {
			"go.mod":  "module modpkg",
			"main.go": "",
		},
		"modpkg/internal/p": {
			"p.go": "package p\n\nconst P = 1\n",
		},
	})
	for _, child := range []string{
		"/go/src/modpkg/internal/p",
		"/go/src/modpkg/internal/p.go",
	} {
		dir, err := ContainingDirectory(orig, child, "", "go.mod")
		if err != nil {
			t.Fatal(err)
		}
		dir = filepath.ToSlash(dir)
		if dir != "/go/src/modpkg" {
			t.Errorf("Dir want: %q got: %q", "/go/src/modpkg", dir)
		}
	}
}

func TestFindProjectRoot(t *testing.T) {
	touch := func(t *testing.T, name string) {
		t.Helper()
		if err := os.WriteFile(name, []byte(filepath.Base(name)), 0644); err != nil {
			t.Fatal(err)
		}
	}
	tempdir := t.TempDir()
	root := filepath.Join(tempdir, "src")
	base := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatal(err)
	}
	ctxt := build.Default

	trim := func(s string) string {
		return strings.TrimPrefix(s, tempdir)
	}
	testFind := func(t *testing.T, tombstone string, extra ...string) {
		if strings.HasPrefix(tombstone, "DIR:") {
			tombstone = filepath.Join(root, strings.TrimPrefix(tombstone, "DIR:"))
			if err := os.MkdirAll(tombstone, 0755); err != nil {
				t.Fatal(err)
			}
		} else {
			tombstone = filepath.Join(root, tombstone)
			touch(t, tombstone)
		}
		defer func() {
			if err := os.Remove(tombstone); err != nil {
				t.Fatal(err)
			}
		}()
		got, err := FindProjectRoot(&ctxt, base, extra...)
		if err != nil {
			t.Errorf("%q: %v", trim(tombstone), err)
		}
		if got != root {
			t.Errorf("%q: got: %q want: %q", trim(tombstone), trim(got), trim(root))
		}
	}

	for _, goroot := range []string{"", tempdir} {
		if goroot != "" {
			ctxt.GOROOT = goroot
		}
		t.Logf("## GOROOT: %q", ctxt.GOROOT)

		for _, tombstone := range DefaultProjectTombstones {
			testFind(t, tombstone)
		}

		// Non-existent tombstones
		for _, tombstone := range DefaultProjectTombstones {
			testFind(t, tombstone, ".foo", ".bar")
		}

		// Directory
		testFind(t, "DIR:.svn", ".svn")

		// Not found
		got, err := FindProjectRoot(&ctxt, base)
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("error mismatch got: %v want: %v", err, os.ErrNotExist)
		}
		if got != base {
			t.Fatalf("%q: got: %q want: %q", "<NONE>", trim(got), trim(base))
		}
	}

	// Change base to a file
	base = filepath.Join(base, "f.txt")
	touch(t, base)
	for _, tombstone := range DefaultProjectTombstones {
		testFind(t, tombstone)
	}
}

// Make sure this works for packages under GOROOT
func TestFindProjectRoot_GOROOT(t *testing.T) {
	ctxt := build.Default
	if !buildutil.FileExists(&ctxt, ctxt.GOROOT) {
		t.Errorf("GOROOT does not exist: %q", ctxt.GOROOT)
	}
	want := filepath.Join(ctxt.GOROOT, "src")

	path := filepath.Join(ctxt.GOROOT, "src", "net", "http")

	got, err := FindProjectRoot(&ctxt, path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("net/http: got: %q want: %q", got, want)
	}
}

func TestFindProjectRoot_FakeContext(t *testing.T) {
	ctxt := buildutil.FakeContext(map[string]map[string]string{
		"modpkg": {
			"go.mod":  "module modpkg",
			"main.go": "",
		},
		"modpkg/internal/p": {
			"p.go": "package p\n\nconst P = 1\n",
		},
	})
	for _, child := range []string{
		"/go/src/modpkg/internal/p",
		"/go/src/modpkg/internal/p.go",
	} {
		dir, err := FindProjectRoot(ctxt, child)
		if err != nil {
			t.Fatal(err)
		}
		dir = filepath.ToSlash(dir)
		if dir != "/go/src/modpkg" {
			t.Errorf("Dir want: %q got: %q", "/go/src/modpkg", dir)
		}
	}
}

func testReadDir(t *testing.T, ctxt *build.Context, dirname string, expected ...string) {
	t.Helper()
	if len(expected) == 0 {
		fis, err := ioutil.ReadDir(dirname)
		if err != nil {
			t.Fatal(err)
		}
		for _, fi := range fis {
			expected = append(expected, fi.Name())
		}
	}
	if !sort.StringsAreSorted(expected) {
		sort.Strings(expected)
	}

	fis, err := ctxt.ReadDir(dirname)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(fis))
	for i, fi := range fis {
		got[i] = fi.Name()
	}
	if len(fis) != len(expected) {
		t.Errorf("ReadDir(%q) = %q; want %q", dirname, got, expected)
		return
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("ReadDir(%q) = %q; want %q", dirname, got, expected)
			return
		}
	}
}

func writeFile(t testing.TB, name string, data interface{}) {
	var b []byte
	switch v := data.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		t.Fatalf("invalid type: %T", data)
	}
	if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
		t.Fatal(err)
	}
	if filepath.Ext(name) == ".go" {
		var err error
		b, err = format.Source(b)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(name, b, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestScopedContext(t *testing.T) {
	const pkgName = "github.com/charlievieth/buildutil"

	orig := util.CopyContext(&build.Default)

	pkg, err := orig.Import(pkgName, ".", build.FindOnly)
	if err != nil {
		t.Fatal(err)
	}
	if !inGopath(orig, pkg.Dir) {
		t.Skipf("Package %q must be in the GOPATH (%q) for this test, found in: %q",
			pkgName, orig.GOPATH, pkg.Dir)
	}

	ctxt, err := ScopedContext(orig, pkg.Dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("Init", func(t *testing.T) {
		if ctxt.ReadDir == nil {
			t.Fatal("ReadDir is nil")
		}
		if ctxt.JoinPath != nil {
			t.Error("JoinPath should be nil")
		}
		if ctxt.SplitPathList != nil {
			t.Error("SplitPathList should be nil")
		}
		if ctxt.IsAbsPath != nil {
			t.Error("IsAbsPath should be nil")
		}
		if ctxt.IsDir != nil {
			t.Error("IsDir should be nil")
		}
		if ctxt.HasSubdir != nil {
			t.Error("HasSubdir should be nil")
		}
		if ctxt.OpenFile != nil {
			t.Error("OpenFile should be nil")
		}
	})

	t.Run("GOPATH", func(t *testing.T) {
		testReadDir(t, ctxt, pkg.Dir)
		testReadDir(t, ctxt, filepath.Join(pkg.Dir, "contextutil"))

		exp := append([]string{"src"}, strings.Split(pkgName, "/")...)
		dir := filepath.Dir(pkg.Dir)
		for i := len(exp) - 1; i >= 0; i-- {
			testReadDir(t, ctxt, dir, exp[i])
			dir = filepath.Dir(dir)
		}
	})

	t.Run("NotDir", func(t *testing.T) {
		tmpdir := t.TempDir()
		name := filepath.Join(tmpdir, "file.txt")
		if err := os.WriteFile(name, []byte("file.txt"), 0644); err != nil {
			t.Fatal(err)
		}
		_, err := ctxt.ReadDir(name)
		if !errors.Is(err, syscall.ENOTDIR) {
			t.Errorf("Expected error %v got: %+v", syscall.ENOTDIR, err)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		tmp := t.TempDir()
		for i := rune('A'); i <= 'C'; i++ {
			// Create some subdirectories so ioutil.ReadDir will return
			// something if called
			if err := os.MkdirAll(filepath.Join(tmp, string(i)), 0755); err != nil {
				t.Fatal(err)
			}
		}
		fis, err := ctxt.ReadDir(tmp)
		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("Expected error %v got: %+v", os.ErrNotExist, err)
		}
		if len(fis) != 0 {
			t.Error("No FileInfos should be returned on error")
		}
	})

	// TODO: should we error instead ??
	// Calling ReadDir with a non-existent directory should not error
	t.Run("StatNotFound", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "nope")
		_, err := ctxt.ReadDir(dir)
		var want *os.PathError
		if !errors.As(err, &want) {
			t.Fatalf("error mismatch got: %#v want: %T", err, want)
		}
		if want.Path != dir {
			t.Errorf("PathError.Path = %q want: %q", want.Path, dir)
		}
	})

	t.Run("GOROOT", func(t *testing.T) {
		testReadDir(t, ctxt, ctxt.GOROOT)
		testReadDir(t, ctxt, filepath.Join(ctxt.GOROOT, "src"))
		testReadDir(t, ctxt, filepath.Join(ctxt.GOROOT, "src", "fmt"))
		testReadDir(t, ctxt, filepath.Join(ctxt.GOROOT, "src", "net", "http"))
	})

	t.Run("SymlinkProject", func(t *testing.T) {
		linkDir := filepath.Join(t.TempDir(), "links")
		if err := os.MkdirAll(linkDir, 0755); err != nil {
			t.Fatal(err)
		}

		// package directory: return all children
		newname := filepath.Join(linkDir, filepath.Base(pkg.Dir))
		if err := os.Symlink(pkg.Dir, newname); err != nil {
			t.Fatal(err)
		}
		testReadDir(t, ctxt, newname)

		// parent directories should only return one child
		exp := append([]string{"src"}, strings.Split(pkgName, "/")...)
		dir := filepath.Dir(pkg.Dir)
		for i := len(exp) - 1; i >= 0; i-- {
			newname := filepath.Join(linkDir, filepath.Base(dir))
			if err := os.Symlink(dir, newname); err != nil {
				t.Fatal(err)
			}
			testReadDir(t, ctxt, newname, exp[i])
			dir = filepath.Dir(dir)
		}
	})

	// Test GOROOT, GOPATH, and project dir symlinks
	t.Run("SymlinkAll", func(t *testing.T) {
		ctxt := util.CopyContext(&build.Default)

		goroot := filepath.Join(t.TempDir(), filepath.Base(ctxt.GOROOT))
		if err := os.Symlink(ctxt.GOROOT, goroot); err != nil {
			t.Fatal(err)
		}
		ctxt.GOROOT = goroot

		gopath := filepath.Join(t.TempDir(), "go")
		if err := os.Symlink(ctxt.GOPATH, gopath); err != nil {
			t.Fatal(err)
		}
		ctxt.GOPATH = gopath

		pkgDir := filepath.Join(gopath, "src", pkgName)
		ctxt, err = ScopedContext(ctxt, pkgDir)
		if err != nil {
			t.Fatal(err)
		}

		// actual path
		testReadDir(t, ctxt, pkg.Dir)
		testReadDir(t, ctxt, filepath.Join(pkg.Dir, "contextutil"))

		// symlink
		testReadDir(t, ctxt, pkgDir)
		testReadDir(t, ctxt, filepath.Join(pkgDir, "contextutil"))

		// actual path
		testReadDir(t, ctxt, ctxt.GOROOT)
		testReadDir(t, ctxt, filepath.Join(ctxt.GOROOT, "src"))
		testReadDir(t, ctxt, filepath.Join(ctxt.GOROOT, "src", "fmt"))
		testReadDir(t, ctxt, filepath.Join(ctxt.GOROOT, "src", "net", "http"))

		// symlink
		testReadDir(t, ctxt, goroot)
		testReadDir(t, ctxt, filepath.Join(goroot, "src"))
		testReadDir(t, ctxt, filepath.Join(goroot, "src", "fmt"))
		testReadDir(t, ctxt, filepath.Join(goroot, "src", "net", "http"))
	})

	t.Run("Modules", func(t *testing.T) {
		tempdir, err := os.MkdirTemp("", "contextutil-test-*")
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if t.Failed() {
				t.Logf("TMPDIR: %s", tempdir)
			} else {
				os.RemoveAll(tempdir)
			}
		}()
		root := filepath.Join(tempdir, "src", "modpkg")
		pkgDir := filepath.Join(root, "pkg", "v")

		writeFile(t, filepath.Join(root, "go.mod"), "module p\n\ngo 1.17\n")
		writeFile(t, filepath.Join(root, "main.go"), "package main\n"+
			"import _ \"p/pkg/v\"\n"+
			"func main() {}\n")

		writeFile(t, filepath.Join(pkgDir, "v.go"), "package v; const V = 1;")

		ctxt := util.CopyContext(&build.Default)
		ctxt, err = ScopedContext(ctxt, pkgDir)
		if err != nil {
			t.Fatal(err)
		}

		testReadDir(t, ctxt, pkgDir)
		testReadDir(t, ctxt, root)
	})

	// If build.Context.ReadDir is non-nil we should fall back to it
	// if we the dir the
	t.Run("Wrapped", func(t *testing.T) {
		ctxt = util.CopyContext(ctxt)
		called := false
		ctxt.ReadDir = func(dir string) ([]fs.FileInfo, error) {
			called = true
			return readdir.ReadDir(dir)
		}
		ctxt, err = ScopedContext(ctxt, pkg.Dir)
		if err != nil {
			t.Fatal(err)
		}

		testReadDir(t, ctxt, filepath.Join(pkg.Dir, "contextutil"))
		if !called {
			t.Error("ScopedContext should use the existing ReadDir function")
		}
		called = false

		tempdir := t.TempDir()
		testReadDir(t, ctxt, tempdir)
		if !called {
			t.Error("ScopedContext failed to use wrapped ReadDir function")
		}
	})

	t.Run("NoPkgDirArgument", func(t *testing.T) {
		_, err := ScopedContext(util.CopyContext(ctxt))
		if err == nil {
			t.Fatal("Expected error when missing pkgdir argument")
		}
	})
}

func TestScopedContext_FakeContext(t *testing.T) {
	orig := buildutil.FakeContext(map[string]map[string]string{
		"modpkg": {
			"go.mod":  "module modpkg",
			"main.go": "package main",
		},
		"other": {
			"go.mod":   "module other",
			"other.go": "package other",
		},
		"modpkg/internal/p": {
			"p.go": "package p\n\nconst P = 1\n",
		},
	})
	orig.GOPATH = orig.GOROOT
	orig.GOROOT = "/xgo"
	ctxt, err := ScopedContext(orig, "/go/src/modpkg")
	if err != nil {
		t.Fatal(err)
	}

	type FileInfo struct {
		Name string
		Mode fs.FileMode
	}
	test := func(t *testing.T, dirname string, want []FileInfo) {
		name := strings.TrimLeft(dirname, "/")
		t.Run(name, func(t *testing.T) {
			fis, err := ctxt.ReadDir(dirname)
			if err != nil {
				t.Fatal(err)
			}
			sort.Slice(want, func(i, j int) bool {
				return want[i].Name < want[j].Name
			})
			for i, fi := range fis {
				x := want[i]
				if fi.Name() != x.Name || fi.Mode() != x.Mode {
					t.Errorf("%d got: {%q, %s} want: {%q, %s}", i, fi.Name(), fi.Mode().Type(),
						x.Name, x.Mode.Type())
				}
			}
			if t.Failed() || len(fis) != len(want) {
				t.Errorf("ReadDir(%q) = %v, want: %v", dirname, fis, want)
			}
		})
	}

	test(t, "/go/src", []FileInfo{{"modpkg", 0755}})
	test(t, "/go/src/modpkg", []FileInfo{{"go.mod", 0644}, {"main.go", 0644}})

	t.Run("NotFound", func(t *testing.T) {
		_, err := ctxt.ReadDir("/go/src/other")
		if err == nil {
			t.Fatalf("ReadDir(%q) should error", "/go/src/other")
		}
		if !os.IsNotExist(err) {
			t.Fatalf("ReadDir(%q) return IsNotExist error got: %v", "/go/src/other", err)
		}
	})
}

func TestScopedContext_Parallel(t *testing.T) {
	if testing.Short() {
		t.Skip("Short test")
	}
	const pkgName = "github.com/charlievieth/buildutil"

	ctxt := util.CopyContext(&build.Default)

	pkg, err := ctxt.Import(pkgName, ".", build.FindOnly)
	if err != nil {
		t.Fatal(err)
	}
	if !inGopath(ctxt, pkg.Dir) {
		t.Skipf("Package %q must be in the GOPATH (%q) for this test, found in: %q",
			pkgName, ctxt.GOPATH, pkg.Dir)
	}

	ctxt, err = ScopedContext(ctxt, pkg.Dir)
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string][]string{
		pkg.Dir:                                          nil,
		filepath.Join(pkg.Dir, "contextutil"):            nil,
		ctxt.GOROOT:                                      nil,
		filepath.Join(ctxt.GOROOT, "src"):                nil,
		filepath.Join(ctxt.GOROOT, "src", "fmt"):         nil,
		filepath.Join(ctxt.GOROOT, "src", "net", "http"): nil,
	}

	exp := append([]string{"src"}, strings.Split(pkgName, "/")...)
	dir := filepath.Dir(pkg.Dir)
	for i := len(exp) - 1; i >= 0; i-- {
		tests[dir] = []string{exp[i]}
		dir = filepath.Dir(dir)
	}
	readdirnames := func(t *testing.T, dirname string) []string {
		f, err := os.Open(dirname)
		if err != nil {
			t.Fatal(err)
		}
		names, err := f.Readdirnames(-1)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		sort.Strings(names)
		return names
	}
	for k, v := range tests {
		if v == nil {
			tests[k] = readdirnames(t, k)
		}
	}

	numCPU := runtime.NumCPU()
	if numCPU < 4 {
		numCPU = 4
	} else if numCPU > 8 {
		numCPU = 8
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < numCPU; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < 100; i++ {
				for dir, exp := range tests {
					if exp == nil {
						testReadDir(t, ctxt, dir)
					} else {
						testReadDir(t, ctxt, dir, exp...)
					}
				}
			}
		}()
	}
	close(start)
	wg.Wait()
}

func TestInGopath(t *testing.T) {
	tests := []struct {
		dir    string
		gopath []string
		result bool
	}{
		{"/a/b", []string{"/a"}, true},
		{"/a/b", []string{"/x", "/a"}, true},
		{"/a/b", []string{"//a//", "//x//"}, true},
		{"/a/b", []string{"//x//", "", "//a//"}, true},
		{"/a/b", []string{"", "//a//"}, true},
		{"/a", []string{"/a"}, true}, // Allow exact matches
		{"", []string{""}, false},
		{"/a", []string{"/x", ""}, false},
	}
	for i, tt := range tests {
		tests[i].dir = filepath.Clean(tt.dir)
	}
	ctxt := build.Default
	for _, tt := range tests {
		gopath := strings.Join(tt.gopath, string(os.PathListSeparator))
		ctxt.GOPATH = gopath
		got := inGopath(&ctxt, tt.dir)
		if got != tt.result {
			t.Errorf("inGopath(%q, %q) = %t; want %t", gopath, tt.dir, got, tt.result)
		}
	}
}

func TestSortUniqueStrings(t *testing.T) {
	tests := []struct {
		in, exp []string
	}{
		{nil, nil},
		{[]string{}, []string{}},
		{[]string{"a"}, []string{"a"}},
		{[]string{"a", "a", "z"}, []string{"a", "z"}},
		{[]string{"b", "a", "a", "b"}, []string{"a", "b"}},
	}
	equal := func(s1, s2 []string) bool {
		if len(s1) != len(s2) {
			return false
		}
		for i := range s1 {
			if s1[i] != s2[i] {
				return false
			}
		}
		return true
	}
	for _, tt := range tests {
		got := sortUniqueStrings(append([]string(nil), tt.in...))
		if !equal(got, tt.exp) {
			t.Errorf("sortUniqueStrings(%q) = %q; want %q", tt.in, got, tt.exp)
		}
		if !sort.StringsAreSorted(got) {
			t.Errorf("result %q is not sorted!", got)
		}
	}
}

func TestReadSubdirs(t *testing.T) {
	ctxt := util.CopyContext(&build.Default)

	tmp := t.TempDir()
	for _, r := range "abd" {
		if err := os.Mkdir(filepath.Join(tmp, string(r)), 0755); err != nil {
			t.Fatal(err)
		}
	}
	subdirs := []string{
		filepath.Join(tmp, "a"),
		filepath.Join(tmp, "b"),
		filepath.Join(tmp, "c"), // does not exist
	}
	dirnames := make(map[string]struct{})
	for _, dir := range subdirs {
		dirnames[filepath.Base(dir)] = struct{}{}
	}

	test := func(fis []fs.FileInfo, err error) {
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, fi := range fis {
			names = append(names, fi.Name())
		}
		exp := []string{"a", "b"}
		if !reflect.DeepEqual(names, exp) {
			t.Errorf("readSubdirs(%q) = %q want: %q", subdirs, names, exp)
		}
	}

	ctxt.ReadDir = nil
	test(readSubdirs(ctxt, subdirs, nil))

	ctxt.ReadDir = readdir.ReadDir
	test(readSubdirs(ctxt, subdirs, dirnames))
}

type SubdirTest struct {
	Root, Dir, Rel string
	Ok             bool
}

func (s SubdirTest) String() string {
	return fmt.Sprintf("{Root: %q Dir: %q Rel: %q Ok: %t}",
		s.Root, s.Dir, s.Rel, s.Ok)
}

var subdirTests = []SubdirTest{
	{Root: "/a/b", Dir: "/a/b/c", Rel: "c", Ok: true},
	{Root: "/a/b/", Dir: "/a/b/c", Rel: "c", Ok: true},
	{Root: "/a/b", Dir: "/a/b//c//", Rel: "c", Ok: true},
	{Root: "/a//b//", Dir: "/a/b/c", Rel: "c", Ok: true},
	{Root: "/a/b", Dir: "/a/b//c", Rel: "c", Ok: true},
	{Root: "/a/b", Dir: "/a/b/c/", Rel: "c", Ok: true},
	{Root: "/a/b", Dir: "/a/b/c//", Rel: "c", Ok: true},
	{Root: "/a/b", Dir: "/a/b/"},
	{Root: "/a/b", Dir: "/a/b"},
	{Root: "", Dir: ""},
	{Root: "/", Dir: ""},
	{Root: "", Dir: "/"},
	{Root: "/a", Dir: ""},
	{Root: "", Dir: "/a"},
	{Root: "/", Dir: "/", Ok: true},   // WARN: I think this is a bug
	{Root: "//", Dir: "//", Ok: true}, // WARN: I think this is a bug
}

// Test that our tests cases are valid for the reference implementation.
func TestHasSubdir_Reference(t *testing.T) {
	ctxt := util.CopyContext(&build.Default)
	ctxt.HasSubdir = nil
	for i, x := range subdirTests {
		rel, ok := buildutil.HasSubdir(ctxt, x.Root, x.Dir)
		if rel != x.Rel || ok != x.Ok {
			t.Errorf("%d: %+v: rel: %q want: %q ok: %t want: %t",
				i, x, rel, x.Rel, ok, x.Ok)
		}
	}
}

func TestHasSubdir(t *testing.T) {
	// TODO: I think there is a bug in the reference implementation
	ignore := map[SubdirTest]bool{
		{Root: "/", Dir: "/", Ok: true}:   true,
		{Root: "//", Dir: "//", Ok: true}: true,
	}
	ctxt := util.CopyContext(&build.Default)
	ctxt.HasSubdir = nil
	for i, x := range subdirTests {
		rel, ok := HasSubdir(ctxt, x.Root, x.Dir)
		if rel != x.Rel || ok != x.Ok {
			if ignore[x] {
				t.Logf("%d: %+v: rel: %q want: %q ok: %t want: %t",
					i, x, rel, x.Rel, ok, x.Ok)
				continue
			}
			t.Errorf("%d: %+v: rel: %q want: %q ok: %t want: %t",
				i, x, rel, x.Rel, ok, x.Ok)
		}
	}
}

func TestMinImportDir(t *testing.T) {
	t.Run("GOROOT", func(t *testing.T) {
		ctxt := util.CopyContext(&build.Default)
		if ctxt.GOROOT == "" || !buildutil.IsDir(ctxt, filepath.Join(ctxt.GOROOT, "src")) {
			t.Fatal("test requires GOROOT")
		}

		dir := filepath.Join(ctxt.GOROOT, "src", "time")
		pkg, err := minImportDir(ctxt, dir)
		if err != nil {
			t.Fatal(err)
		}
		exp := minPackage{
			ImportPath: "time",
			Root:       filepath.Clean(ctxt.GOROOT),
			SrcRoot:    filepath.Join(ctxt.GOROOT, "src"),
			Goroot:     true,
		}
		if *pkg != exp {
			t.Errorf("minImportDir: got: %+v want: %+v", *pkg, exp)
		}
	})

	t.Run("GOPATH", func(t *testing.T) {
		gopath := t.TempDir()
		if err := os.MkdirAll(filepath.Join(gopath, "src/p1/internal/p2"), 0755); err != nil {
			t.Fatal(err)
		}
		const pkgName = "p1/internal/p2"

		for _, dir := range []string{"p1", "p1/internal/p2"} {
			pkg := filepath.Base(dir)
			name := filepath.Join(gopath, "src", dir, pkg+".go")
			if err := os.WriteFile(name, []byte("package "+pkg+"\n"), 0644); err != nil {
				t.Fatal(err)
			}
		}

		ctxt := util.CopyContext(&build.Default)
		ctxt.GOPATH = gopath

		pkg, err := minImportDir(ctxt, filepath.Join(gopath, "src/p1/internal/p2"))
		if err != nil {
			t.Fatal(err)
		}

		exp := minPackage{
			ImportPath: pkgName,
			Root:       ctxt.GOPATH,
			SrcRoot:    filepath.Join(gopath, "src"),
			Goroot:     false,
		}
		if *pkg != exp {
			t.Errorf("minImportDir: got: %+v want: %+v", *pkg, exp)
		}
	})

	t.Run("MOD", func(t *testing.T) {
		tempdir := t.TempDir()
		root := filepath.Join(tempdir, "xpkg")
		if err := os.Mkdir(root, 0755); err != nil {
			t.Fatal(err)
		}

		const modContents = "module example.com/xpkg\n\ngo 1.15\n"
		const goContents = `package main

		import "fmt"

		func main() {
			fmt.Println("vim-go")
		}`
		if err := os.WriteFile(root+"/go.mod", []byte(modContents), 0644); err != nil {
			t.Fatal(err)
		}
		src, err := format.Source([]byte(goContents))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(root+"/main.go", src, 0644); err != nil {
			t.Fatal(err)
		}

		ctxt := build.Default
		pkg, err := minImportDir(&ctxt, root)
		if err != nil {
			t.Fatal(err)
		}

		want := minPackage{
			Root:     root,
			IsModule: true,
		}
		if *pkg != want {
			t.Errorf("minImportDir:\nGot:\n%s\nWant:\n%s\n",
				toJSON(t, *pkg), toJSON(t, want))
		}
	})
}

func toJSON(t testing.TB, v interface{}) string {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "    ")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TODO TODO TODO TODO TODO TODO TODO TODO TODO TODO
//
// Benchmark not found - because symlinks
//
// TODO TODO TODO TODO TODO TODO TODO TODO TODO TODO

// /Users/cvieth/go/src
// /Users/cvieth/go/src/github.com
// /Users/cvieth/go/src/github.com/charlievieth
// /Users/cvieth/go/src/github.com/charlievieth/buildutil
// /Users/cvieth/go/src/github.com/charlievieth/buildutil/contextutil

func BenchmarkNewScopedContext(b *testing.B) {
	wd, err := os.Getwd()
	if err != nil {
		b.Fatal(err)
	}
	ctxt := build.Default
	for i := 0; i < b.N; i++ {
		_, err := ScopedContext(&ctxt, wd)
		if err != nil {
			b.Fatal(err)
		}
		ctxt.ReadDir = nil
	}
}

func BenchmarkScopedContext(b *testing.B) {
	pkgdir := initBenchInfo(b)
	orig := util.CopyContext(&build.Default)
	orig.ReadDir = func(_ string) ([]fs.FileInfo, error) {
		return nil, nil
	}
	ctxt, err := ScopedContext(orig, pkgdir)
	if err != nil {
		b.Fatal(err)
	}

	b.Run("GOROOT", func(b *testing.B) {
		if fi, err := os.Stat(runtime.GOROOT()); err != nil || !fi.IsDir() {
			b.Skipf("benchmark requires valid GOROOT: %q", runtime.GOROOT())
		}
		dir := filepath.Join(runtime.GOROOT(), "src", "time")

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := ctxt.ReadDir(dir)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Pkgdir", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := ctxt.ReadDir(pkgdir)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Subdir", func(b *testing.B) {
		subdir := filepath.Dir(pkgdir)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := ctxt.ReadDir(subdir)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	tmpdir := b.TempDir()

	b.Run("NotFoundInvalidName", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := ctxt.ReadDir(tmpdir)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	// Bench not found when the name matches one of the valid directories.
	b.Run("NotFoundValidName", func(b *testing.B) {
		subdir := filepath.Join(tmpdir, filepath.Base(pkgdir))
		if err := os.MkdirAll(subdir, 0755); err != nil {
			b.Fatal(err)
		}
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			_, err := ctxt.ReadDir(subdir)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkHasSubdirCtxt_Lexical(b *testing.B) {
	const root = "/Users/cvieth/go/src"
	const dir = "/Users/cvieth/go/src/github.com/charlievieth/buildutil"
	ctxt := util.CopyContext(&build.Default)
	ctxt.HasSubdir = nil
	for i := 0; i < b.N; i++ {
		if _, ok := HasSubdir(ctxt, root, dir); !ok {
			b.Fatal("wat")
		}
	}
}

func BenchmarkHasSubdirCtxt_NoMatch(b *testing.B) {
	// const root = "/usr/local/Cellar/go/1.17.2/libexec/src"
	// const dir = "/Users/cvieth/go/src/github.com/charlievieth/buildutil"
	const root = "/usr/local/go/src"
	const dir = "/home/username/go/src/github.com/charlievieth/buildutil"
	ctxt := util.CopyContext(&build.Default)
	ctxt.HasSubdir = nil
	ctxt.GOROOT = "/usr/local/go"
	ctxt.GOPATH = "/home/username/go"
	for i := 0; i < b.N; i++ {
		if _, ok := HasSubdir(ctxt, root, dir); ok {
			b.Fatal("wat")
		}
	}
}

func BenchmarkIsSubdir(b *testing.B) {
	const root = "/usr/local/go"
	for i := 0; i < b.N; i++ {
		isSubdir(root, "/usr/local/go/src/fmt")
	}
}

func BenchmarkInGopath(b *testing.B) {
	ctxt := build.Default
	b.Run("One", func(b *testing.B) {
		ctxt.GOPATH = filepath.FromSlash("/usr/local/go")
		dir := filepath.FromSlash("/usr/local/go/src/fmt")
		for i := 0; i < b.N; i++ {
			inGopath(&ctxt, dir)
		}
	})
	b.Run("Second", func(b *testing.B) {
		ctxt.GOPATH = filepath.FromSlash("/home/username/go" + string(os.PathListSeparator) + "/usr/local/go")
		dir := filepath.FromSlash("/usr/local/go/src/fmt")
		for i := 0; i < b.N; i++ {
			inGopath(&ctxt, dir)
		}
	})
}

func BenchmarkHasSubdir(b *testing.B) {
	// const root = "/usr/local/Cellar/go/1.17.2/libexec"
	// var tests = [...]string{
	// 	"/usr/local/Cellar/go/1.17.2/libexec/src/fmt",
	// 	"/usr/local/Cellar/go/1.17.2/libexec/src/go/build",
	// }
	// for i := 0; i < b.N; i++ {
	// 	hasSubdir(root, tests[i%len(tests)])
	// }
	b.Run("Clean", func(b *testing.B) {
		const root = "/usr/local/Cellar/go/1.17.2/libexec"
		var tests = [...]string{
			"/usr/local/Cellar/go/1.17.2/libexec/src/fmt",
			"/usr/local/Cellar/go/1.17.2/libexec/src/go/build",
		}
		for i := 0; i < b.N; i++ {
			hasSubdir(root, tests[i%len(tests)])
		}
	})
	b.Run("Unclean", func(b *testing.B) {
		const root = "/usr/local/Cellar/go/1.17.2/libexec/"
		var tests = [...]string{
			"/usr/local/Cellar/go/1.17.2/libexec/src/fmt/",
			"/usr/local/Cellar/go/1.17.2/libexec/src//fmt",
			"/usr/local/Cellar/go/1.17.2//libexec/src/go/build/",
			"/usr/local/Cellar/go/1.17.2//libexec/src//go/build",
		}
		for i := 0; i < b.N; i++ {
			hasSubdir(root, tests[i%len(tests)])
		}
	})
	b.Run("Reference", func(b *testing.B) {
		// const root = "/usr/local/Cellar/go/1.17.2/libexec/"
		// var tests = [...]string{
		// 	"/usr/local/Cellar/go/1.17.2/libexec/src//fmt",
		// 	"/usr/local/Cellar/go/1.17.2//libexec/src/go/build/",
		// }
		const root = "/usr/local/Cellar/go/1.17.2/libexec"
		var tests = [...]string{
			"/usr/local/Cellar/go/1.17.2/libexec/src/fmt",
			"/usr/local/Cellar/go/1.17.2/libexec/src/go/build",
		}
		ctxt := util.CopyContext(&build.Default)
		ctxt.HasSubdir = nil
		for i := 0; i < b.N; i++ {
			buildutil.HasSubdir(ctxt, root, tests[i%len(tests)])
		}
	})
	b.Run("Reference/Unclean", func(b *testing.B) {
		const root = "/usr/local/Cellar/go/1.17.2/libexec/"
		var tests = [...]string{
			"/usr/local/Cellar/go/1.17.2/libexec/src/fmt/",
			"/usr/local/Cellar/go/1.17.2/libexec/src//fmt",
			"/usr/local/Cellar/go/1.17.2//libexec/src/go/build/",
			"/usr/local/Cellar/go/1.17.2//libexec/src//go/build",
		}
		ctxt := util.CopyContext(&build.Default)
		ctxt.HasSubdir = nil
		for i := 0; i < b.N; i++ {
			buildutil.HasSubdir(ctxt, root, tests[i%len(tests)])
		}
	})
}

func BenchmarkFindProjectRoot(b *testing.B) {
	const dir = "/Users/cvieth/go/src/github.com/coredns/coredns/plugin/pkg/cache"
	ctxt := build.Default
	for i := 0; i < b.N; i++ {
		root, err := FindProjectRoot(&ctxt, dir)
		if err != nil {
			b.Fatal(err)
		}
		if root == "" {
			b.Fatal("empty root")
		}
	}
}

var stdLibPkgs = map[string]map[string]string{
	"archive":   {"archive.go": "file"},
	"bufio":     {"bufio.go": "file"},
	"builtin":   {"builtin.go": "file"},
	"bytes":     {"bytes.go": "file"},
	"cmd":       {"cmd.go": "file"},
	"compress":  {"compress.go": "file"},
	"container": {"container.go": "file"},
	"context":   {"context.go": "file"},
	"crypto":    {"crypto.go": "file"},
	"database":  {"database.go": "file"},
	"debug":     {"debug.go": "file"},
	"embed":     {"embed.go": "file"},
	"encoding":  {"encoding.go": "file"},
	"errors":    {"errors.go": "file"},
	"expvar":    {"expvar.go": "file"},
	"flag":      {"flag.go": "file"},
	"fmt":       {"fmt.go": "file"},
	"go":        {"go.go": "file"},
	"hash":      {"hash.go": "file"},
	"html":      {"html.go": "file"},
	"image":     {"image.go": "file"},
	"index":     {"index.go": "file"},
	"internal":  {"internal.go": "file"},
	"io":        {"io.go": "file"},
	"log":       {"log.go": "file"},
	"math":      {"math.go": "file"},
	"mime":      {"mime.go": "file"},
	"net":       {"net.go": "file"},
	"os":        {"os.go": "file"},
	"path":      {"path.go": "file"},
	"plugin":    {"plugin.go": "file"},
	"reflect":   {"reflect.go": "file"},
	"regexp":    {"regexp.go": "file"},
	"runtime":   {"runtime.go": "file"},
	"sort":      {"sort.go": "file"},
	"strconv":   {"strconv.go": "file"},
	"strings":   {"strings.go": "file"},
	"sync":      {"sync.go": "file"},
	"syscall":   {"syscall.go": "file"},
	"testdata":  {"testdata.go": "file"},
	"testing":   {"testing.go": "file"},
	"text":      {"text.go": "file"},
	"time":      {"time.go": "file"},
	"unicode":   {"unicode.go": "file"},
	"unsafe":    {"unsafe.go": "file"},
	"vendor":    {"vendor.go": "file"},
}

func BenchmarkReadSubdirs(b *testing.B) {
	subdirs := make([]string, 0, len(stdLibPkgs))
	names := make(map[string]struct{}, len(stdLibPkgs))
	for name := range stdLibPkgs {
		subdirs = append(subdirs, "/go/src/"+name)
		names[name] = struct{}{}
	}
	sort.Strings(subdirs)

	ctxt := buildutil.FakeContext(stdLibPkgs)
	fis, err := readSubdirs(ctxt, subdirs, names)
	if err != nil {
		b.Fatal(err)
	}
	if len(fis) != len(subdirs) {
		b.Fatalf("want %d got %d", len(subdirs), len(fis))
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := readSubdirs(ctxt, subdirs, names)
		if err != nil {
			b.Fatal(err)
		}
	}
}
