//go:build go1.18 && gc
// +build go1.18,gc

package buildutil

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"flag"
	"go/build"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

var testMatchWalkGOROOT = flag.Bool("walk-goroot", false,
	"Run MatchContext on every Go source file in GOPATH")

var testMatchWalkDir = flag.String("walk-path", "",
	"Run MatchContext on every Go source file in the provided "+
		"comma separated list of directoried.")

func TestMatchContextWalkStdLib(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping: short test")
	}

	expectedErrors := map[string]error{
		"cmd/dist/util_gccgo.go": errCompilerMismatchGccGo,
		"go/build/gccgo.go":      errCompilerMismatchGccGo,
		"sort/slice_go14.go":     ErrImpossibleGoVersion,
		"sort/slice_go18.go":     ErrImpossibleGoVersion,
	}
	for _, name := range []string{"go1.17.9", "go1.18.1"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tarball := filepath.Join("./testdata", name+".tgz")
			root := filepath.Join(extractTarball(t, tarball), name, "src")
			testMatchContextWalkDirectory(t, root, expectedErrors)
		})
	}
}

func TestMatchContextWalkGOROOT(t *testing.T) {
	t.Parallel()
	if !*testMatchWalkGOROOT {
		t.Skip("skipping: test only enabled with the `-walk-goroot` flag")
	}
	if testing.Short() {
		t.Skip("skipping: short test")
	}
	orig := build.Default
	root := filepath.Join(orig.GOROOT, "src")
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		t.Skip("skipping: test requires Go source code")
	}
	testMatchContextWalkDirectory(t, root, nil)
}

func TestMatchContextWalkPath(t *testing.T) {
	t.Parallel()
	if *testMatchWalkDir == "" {
		t.Skip("skipping: test only enabled with the `-walk-path` flag")
	}
	if testing.Short() {
		t.Skip("skipping: short test")
	}
	for _, root := range strings.Split(*testMatchWalkDir, ",") {
		testMatchContextWalkDirectory(t, root, nil)
	}
}

func testMatchContextWalkDirectory(t *testing.T, root string, expectedErrors map[string]error) {
	if testing.Short() {
		t.Skip("short test")
	}
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		t.Skip("skipping: cannot open test directory:", root)
		return
	}

	trimRoot := func(path string) string {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			panic(err)
		}
		return rel
	}

	fixupMatchErr := func(err error) error {
		var me *MatchError
		if errors.As(err, &me) {
			me.Path = trimRoot(me.Path)
			return me
		}
		return err
	}

	// create a copy since we modify the expectedErrors map
	if len(expectedErrors) != 0 {
		m := make(map[string]error, len(expectedErrors))
		for name, err := range expectedErrors {
			m[filepath.ToSlash(name)] = err
		}
		expectedErrors = m
	}

	var errsMu sync.Mutex
	checkMatchError := func(t *testing.T, path string, err error) (skip bool) {
		// t.Helper()
		key := filepath.ToSlash(trimRoot(path))
		errsMu.Lock()
		expErr, ok := expectedErrors[key]
		if ok {
			delete(expectedErrors, key)
		}
		errsMu.Unlock()
		switch {
		case ok:
			if !errors.Is(err, expErr) {
				t.Errorf("%s: got error: %v want: %v\n",
					trimRoot(path), fixupMatchErr(err), expErr)
			}
			return true
		case err != nil:
			t.Errorf("%s: %v\n", trimRoot(path), fixupMatchErr(err))
			return true
		default:
			return false
		}
	}

	var (
		failed []string
		mu     sync.Mutex // protect failed
		wg     sync.WaitGroup
	)
	orig := copyContext(&build.Default)
	ch := make(chan string, 64)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range ch {
				ctxt, err := MatchContext(orig, path, nil)
				if checkMatchError(t, path, err) {
					continue
				}
				ok, err := ctxt.MatchFile(filepath.Split(path))
				if err != nil {
					panic(err) // fatal
				}
				if !ok {
					mu.Lock()
					failed = append(failed, trimRoot(path)+"\n    "+formatContext(ctxt, false))
					mu.Unlock()
				}
				if arches, ok := supportedPlatformsOsArch[ctxt.GOOS]; ok && !arches[ctxt.GOARCH] {
					t.Errorf("%s: invalid GOOS: %q GOARCH: %q combination",
						trimRoot(path), ctxt.GOOS, ctxt.GOARCH)
				}
				if ctxt.CgoEnabled {
					if cgo, ok := cgoEnabled[ctxt.GOOS+"/"+ctxt.GOARCH]; ok && !cgo {
						t.Errorf("%s: CGO not supported for GOOS: %q GOARCH: %q combination",
							trimRoot(path), ctxt.GOOS, ctxt.GOARCH)
					}
				}
			}
		}()
	}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() && (name == "" || name[0] == '.' || name[0] == '_' ||
			name == "vendor" || name == "testdata" || name == "internal" ||
			name == "node_modules") {
			return filepath.SkipDir
		}
		if d.Type().IsRegular() && filepath.Ext(name) == ".go" {
			ch <- path
		}
		return nil
	})
	close(ch)
	if err != nil {
		t.Fatal(err)
	}
	wg.Wait()

	if len(failed) > 0 {
		sort.Strings(failed)
		if testing.Verbose() {
			t.Errorf("MatchContext: failed to match %d files:\n%s\n\n",
				len(failed), strings.Join(failed, "\n"))

			t.Logf("\nDefault Context:\n    %s\n", formatContext(&build.Default, false))
		} else {
			t.Errorf("MatchContext: failed to match %d files", len(failed))
		}
	}

	if len(expectedErrors) > 0 {
		t.Errorf("MatchContext: failed to visit the provided invalid files:\n%+v\n", expectedErrors)
	}
}

func extractTarball(t *testing.T, tarball string) string {
	tempdir := t.TempDir()

	fi, err := os.Open(tarball)
	if err != nil {
		t.Fatal(err)
	}
	defer fi.Close()
	gr, err := gzip.NewReader(fi)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gr)

	seen := make(map[string]bool)
	mkdir := func(dir string) error {
		if seen[dir] {
			return nil
		}
		seen[dir] = true
		return os.MkdirAll(dir, 0755)
	}

	buf := make([]byte, 32*1024)
	for {
		hdr, err := tr.Next()
		if err != nil {
			if err != io.EOF {
				t.Fatal(err)
			}
			break
		}
		path := filepath.Join(tempdir, hdr.Name)
		if hdr.Typeflag == tar.TypeDir {
			if err := mkdir(path); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if hdr.Typeflag != tar.TypeReg {
			t.Logf("%s: unsupported type flag: %d", hdr.Name, hdr.Typeflag)
			continue
		}
		if err := mkdir(filepath.Dir(path)); err != nil {
			t.Fatal(err)
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, hdr.FileInfo().Mode())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.CopyBuffer(f, tr, buf); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}

	if err := gr.Close(); err != nil {
		t.Fatal(err)
	}
	return tempdir
}
