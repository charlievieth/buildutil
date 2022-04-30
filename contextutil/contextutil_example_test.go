package contextutil_test

import (
	"fmt"
	"go/build"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/charlievieth/buildutil/contextutil"
)

func printReadDir(ctxt *build.Context, path string) {
	// Remove leading temp directory
	name := strings.TrimPrefix(filepath.ToSlash(path),
		filepath.ToSlash(ctxt.GOPATH)+"/")

	fmt.Printf("ReadDir(%q)\n", name)
	fis, err := ctxt.ReadDir(path)
	if err != nil {
		if !os.IsNotExist(err) {
			panic(err)
		}
		fmt.Printf("  open %s: %s\n", name, os.ErrNotExist)
		return
	}
	for _, fi := range fis {
		if fi.IsDir() {
			fmt.Printf("  %s/\n", fi.Name())
		} else {
			fmt.Printf("  %s\n", fi.Name())
		}
	}
}

func ExampleScopedContext() {
	// Create a fake GOPATH and populate it with some files
	//
	// 	src
	// 	└── p
	// 	    ├── p1
	// 	    │   ├── c1
	// 	    │   │    ├── fc1.go
	// 	    │   │    └── fc2.go
	// 	    │   ├── f1.go
	// 	    │   └── f2.go
	// 	    └── p2
	// 	        └── nope.go
	//
	gopath, err := ioutil.TempDir("", "contextutil.*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(gopath)

	pkg1 := filepath.Join(gopath, "src/p/p1")
	sub1 := filepath.Join(gopath, "src/p/p1/c1") // subdirectory of pkg1
	pkg2 := filepath.Join(gopath, "src/p/p2")
	for _, name := range []string{
		filepath.Join(pkg1, "f1.go"),
		filepath.Join(pkg1, "f2.go"),
		filepath.Join(sub1, "fc1.go"),
		filepath.Join(sub1, "fc2.go"),
		filepath.Join(pkg2, "nope.go"),
	} {
		if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
			panic(err)
		}
		err := os.WriteFile(name, []byte(name), 0644)
		if err != nil {
			panic(err)
		}
	}

	// Scope context to path pkg1 ("$GOPATH/src/p/p1")
	orig := build.Default
	orig.GOPATH = gopath
	ctxt, err := contextutil.ScopedContext(&orig, pkg1)
	if err != nil {
		panic(err)
	}

	// Reading $GOPATH/src/p will only return "p1" since
	// that is what we scoped the Context to.
	printReadDir(ctxt, filepath.Dir(pkg1))

	// Reading p1 or any of it's subdirectories returns
	// all results (same as ioutil.ReadDir())
	printReadDir(ctxt, pkg1)
	printReadDir(ctxt, sub1)

	// Reading a directory outside of the scope returns no results.
	printReadDir(ctxt, pkg2)

	// Output:
	// ReadDir("src/p")
	//   p1/
	// ReadDir("src/p/p1")
	//   c1/
	//   f1.go
	//   f2.go
	// ReadDir("src/p/p1/c1")
	//   fc1.go
	//   fc2.go
	// ReadDir("src/p/p2")
	//   open src/p/p2: file does not exist
}
