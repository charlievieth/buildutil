// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package buildutil

import (
	"fmt"
	"go/build"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func testPreferredList(t testing.TB, list []string, want map[string]bool) {
	got := make(map[string]bool, len(list))
	for _, s := range list {
		if got[s] {
			t.Errorf("duplicate %q", s)
		}
		if !want[s] {
			t.Errorf("unknown %q", s)
		}
		got[s] = true
	}
	for s := range want {
		if !got[s] {
			t.Errorf("missing %q", s)
		}
	}
}

// TODO: how do want to handle third-class platforms where we don't
// know the valid OS/Arch combos?
func TestPreferredOSList(t *testing.T) {
	oses := make(map[string]bool)
	for _, p := range DefaultGoPlatforms {
		oses[p.GOOS] = true
	}
	testPreferredList(t, PreferredOSList, oses)
}

// TODO: how do want to handle third-class platforms where we don't
// know the valid OS/Arch combos?
func TestPreferredArchList(t *testing.T) {
	arches := make(map[string]bool)
	for _, p := range DefaultGoPlatforms {
		arches[p.GOARCH] = true
	}
	testPreferredList(t, PreferredArchList, arches)
}

var shouldBuildTests = []struct {
	name        string
	content     string
	tags        map[string]bool
	binaryOnly  bool
	shouldBuild bool
	err         error
}{
	{
		name: "Yes",
		content: "// +build yes\n\n" +
			"package main\n",
		tags:        map[string]bool{"yes": true},
		shouldBuild: true,
	},
	{
		name: "Yes2",
		content: "//go:build yes\n" +
			"package main\n",
		tags:        map[string]bool{"yes": true},
		shouldBuild: true,
	},
	{
		name: "Or",
		content: "// +build no yes\n\n" +
			"package main\n",
		tags:        map[string]bool{"yes": true, "no": true},
		shouldBuild: true,
	},
	{
		name: "Or2",
		content: "//go:build no || yes\n" +
			"package main\n",
		tags:        map[string]bool{"yes": true, "no": true},
		shouldBuild: true,
	},
	{
		name: "And",
		content: "// +build no,yes\n\n" +
			"package main\n",
		tags:        map[string]bool{"yes": true, "no": true},
		shouldBuild: false,
	},
	{
		name: "And2",
		content: "//go:build no && yes\n" +
			"package main\n",
		tags:        map[string]bool{"yes": true, "no": true},
		shouldBuild: false,
	},
	{
		name: "Cgo",
		content: "// +build cgo\n\n" +
			"// Copyright The Go Authors.\n\n" +
			"// This package implements parsing of tags like\n" +
			"// +build tag1\n" +
			"package build",
		tags:        map[string]bool{"cgo": true},
		shouldBuild: false,
	},
	{
		name: "Cgo2",
		content: "//go:build cgo\n" +
			"// Copyright The Go Authors.\n\n" +
			"// This package implements parsing of tags like\n" +
			"// +build tag1\n" +
			"package build",
		tags:        map[string]bool{"cgo": true},
		shouldBuild: false,
	},
	{
		name: "AfterPackage",
		content: "// Copyright The Go Authors.\n\n" +
			"package build\n\n" +
			"// shouldBuild checks tags given by lines of the form\n" +
			"// +build tag\n" +
			"//go:build tag\n" +
			"func shouldBuild(content []byte)\n",
		tags:        map[string]bool{},
		shouldBuild: true,
	},
	{
		name: "TooClose",
		content: "// +build yes\n" +
			"package main\n",
		tags:        map[string]bool{},
		shouldBuild: true,
	},
	{
		name: "TooClose2",
		content: "//go:build yes\n" +
			"package main\n",
		tags:        map[string]bool{"yes": true},
		shouldBuild: true,
	},
	{
		name: "TooCloseNo",
		content: "// +build no\n" +
			"package main\n",
		tags:        map[string]bool{},
		shouldBuild: true,
	},
	{
		name: "TooCloseNo2",
		content: "//go:build no\n" +
			"package main\n",
		tags:        map[string]bool{"no": true},
		shouldBuild: false,
	},
	{
		name: "BinaryOnly",
		content: "//go:binary-only-package\n" +
			"// +build yes\n" +
			"package main\n",
		tags:        map[string]bool{},
		binaryOnly:  true,
		shouldBuild: true,
	},
	{
		name: "BinaryOnly2",
		content: "//go:binary-only-package\n" +
			"//go:build no\n" +
			"package main\n",
		tags:        map[string]bool{"no": true},
		binaryOnly:  true,
		shouldBuild: false,
	},
	{
		name: "ValidGoBuild",
		content: "// +build yes\n\n" +
			"//go:build no\n" +
			"package main\n",
		tags:        map[string]bool{"no": true},
		shouldBuild: false,
	},
	{
		name: "MissingBuild2",
		content: "/* */\n" +
			"// +build yes\n\n" +
			"//go:build no\n" +
			"package main\n",
		tags:        map[string]bool{"no": true},
		shouldBuild: false,
	},
	{
		name: "Comment1",
		content: "/*\n" +
			"//go:build no\n" +
			"*/\n\n" +
			"package main\n",
		tags:        map[string]bool{},
		shouldBuild: true,
	},
	{
		name: "Comment2",
		content: "/*\n" +
			"text\n" +
			"*/\n\n" +
			"//go:build no\n" +
			"package main\n",
		tags:        map[string]bool{"no": true},
		shouldBuild: false,
	},
	{
		name: "Comment3",
		content: "/*/*/ /* hi *//* \n" +
			"text\n" +
			"*/\n\n" +
			"//go:build no\n" +
			"package main\n",
		tags:        map[string]bool{"no": true},
		shouldBuild: false,
	},
	{
		name: "Comment4",
		content: "/**///go:build no\n" +
			"package main\n",
		tags:        map[string]bool{},
		shouldBuild: true,
	},
	{
		name: "Comment5",
		content: "/**/\n" +
			"//go:build no\n" +
			"package main\n",
		tags:        map[string]bool{"no": true},
		shouldBuild: false,
	},
}

func TestShouldBuild(t *testing.T) {
	for _, tt := range shouldBuildTests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &build.Context{BuildTags: []string{"yes"}}
			tags := map[string]bool{}
			shouldBuild, binaryOnly, err := shouldBuild(ctx, []byte(tt.content), tags)
			if shouldBuild != tt.shouldBuild || binaryOnly != tt.binaryOnly || !reflect.DeepEqual(tags, tt.tags) || err != tt.err {
				t.Errorf("mismatch:\n"+
					"have shouldBuild=%v, binaryOnly=%v, tags=%v, err=%v\n"+
					"want shouldBuild=%v, binaryOnly=%v, tags=%v, err=%v",
					shouldBuild, binaryOnly, tags, err,
					tt.shouldBuild, tt.binaryOnly, tt.tags, tt.err)
			}
		})
	}
}

func TestParseConstraint(t *testing.T) {
	for _, tt := range shouldBuildTests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &build.Context{BuildTags: []string{"yes"}}
			expr, err := ParseConstraint(ctx, "main.go", []byte(tt.content))
			if err != nil {
				t.Fatal(err)
			}
			shouldBuild := expr.Eval(ctx)
			if shouldBuild != tt.shouldBuild || err != tt.err {
				t.Errorf("mismatch:\n"+
					"have shouldBuild=%v, err=%v\n"+
					"want shouldBuild=%v, err=%v",
					shouldBuild, err,
					tt.shouldBuild, tt.err)
			}
		})
	}

	t.Run("Filename", func(t *testing.T) {
		ctxt := build.Default
		ctxt.GOOS = "linux"
		ctxt.GOARCH = "amd64"
		expr, err := ParseConstraint(&ctxt, "x_linux_amd64.go", []byte("package x\n"))
		if err != nil {
			t.Fatal(err)
		}
		if ok := expr.Eval(&ctxt); !ok {
			t.Errorf("Eval: got: %t want: %t", ok, true)
		}
	})

	t.Run("None", func(t *testing.T) {
		ctxt := build.Default
		expr, err := ParseConstraint(&ctxt, "main.go", []byte("package main\n"))
		if err != nil {
			t.Fatal(err)
		}
		if expr.expr != nil {
			t.Errorf("Expr: got: %v want: %v", expr.expr, nil)
		}
		if !expr.Empty() {
			t.Errorf("Empty: got: %t want: %t", expr.Empty(), true)
		}
	})

	t.Run("NilConstraint", func(t *testing.T) {
		// Make sure a nil Constraint can be used
		var expr *Constraint
		if !expr.Empty() {
			t.Errorf("Empty: got: %t want: %t", expr.Empty(), true)
		}
		if expr.Expr() != nil {
			t.Errorf("Expr: got: %v want: %v", expr.Expr(), nil)
		}
		if !expr.Eval(nil) {
			t.Errorf("Eval: got: %v want: %v", expr.Eval(nil), nil)
		}
	})

	t.Run("Allocs", func(t *testing.T) {
		// Creating a no-op/empty Constraint should not allocate.
		// Since most Go files have no build constraints and the
		// Constraint may be cached we should re-use a global
		// empty Constraint.
		allocs := testing.AllocsPerRun(10, func() {
			expr := NewConstraint(nil, nil)
			if expr == nil {
				t.Fatal("Nil Constraint")
			}
		})
		if allocs != 0 {
			t.Errorf("Allocs: got: %f want: %f", allocs, 0.0)
		}

		empty := make(map[string]bool)
		allocs = testing.AllocsPerRun(10, func() {
			expr := NewConstraint(nil, empty)
			if expr == nil {
				t.Fatal("Nil Constraint")
			}
		})
		if allocs != 0 {
			t.Errorf("Allocs: got: %f want: %f", allocs, 0.0)
		}
	})
}

func TestCompatibleOsMap(t *testing.T) {
	oses := KnownOSList()
	want := make(map[string][]string)
	for _, s1 := range oses {
		ctxt := build.Context{GOOS: s1}
		for _, s2 := range oses {
			if s1 == s2 {
				continue
			}
			if matchTag(&ctxt, s2, nil) {
				want[s1] = append(want[s1], s2)
			}
		}
	}
	for _, v := range want {
		sort.Strings(v)
	}
	if !reflect.DeepEqual(compatibleOSes, want) {
		t.Errorf("compatibleOSes got: %+v want: %+v", compatibleOSes, want)
	}
}

const ShouldBuild_File1 = "// Copyright The Go Authors.\n\n" +
	"// +build tag1\n\n" + // Valid tag
	"// +build tag2\n" + // Bad tag (no following blank line)
	"package build\n\n" +
	"// +build tag3\n\n" + // Bad tag (after "package" statement)
	"import \"bytes\"\n\n" +
	"// shouldBuild checks tags given by lines of the form\n" +
	"func shouldBuild(content []byte) bool {\n" +
	"// +build tag4\n" + // Bad tag (after "package" statement)
	"\treturn bytes.Equal(content, []byte(\"content\")\n" +
	"}\n\n"

const ShouldBuild_File2 = `
// Copyright The Go Authors.

` + "// +build tag1" + `
package build

` + "// +build tag1" + `

import "bytes"

` + "// +build tag1" + `

// shouldBuild checks tags given by lines of the form
` + "// +build tag" + `
func shouldBuild(content []byte) bool {

	` + "// +build tag1" + `

	return bytes.Equal(content, []byte("content")
}
`

// Test that shouldBuild only reads the leading run of comments.
//
// The build package stops reading the file after imports are completed.
// This tests that shouldBuild does not include build tags that follow
// the "package" clause when passed a complete Go source file.
func TestShouldBuild_Full(t *testing.T) {
	const file1 = ShouldBuild_File1
	want1 := map[string]bool{"tag1": true}

	const file2 = ShouldBuild_File2
	want2 := map[string]bool{}

	ctx := &build.Context{BuildTags: []string{"tag1"}}
	m := map[string]bool{}
	if !shouldBuildOnly(ctx, []byte(file1), m) {
		t.Errorf("shouldBuild(file1) = false, want true")
	}
	if !reflect.DeepEqual(m, want1) {
		t.Errorf("shoudBuild(file1) tags = %v, want %v", m, want1)
	}

	m = map[string]bool{}
	if !shouldBuildOnly(ctx, []byte(file2), m) {
		t.Errorf("shouldBuild(file2) = true, want false")
	}
	if !reflect.DeepEqual(m, want2) {
		t.Errorf("shoudBuild(file2) tags = %v, want %v", m, want2)
	}
}

func TestShortImport_Full(t *testing.T) {
	const file1 = ShouldBuild_File1
	const file2 = ShouldBuild_File2

	ctx := &build.Context{BuildTags: []string{"tag1"}}
	ctx.OpenFile = func(path string) (io.ReadCloser, error) {
		if path == "file1.go" {
			return ioutil.NopCloser(strings.NewReader(file1)), nil
		}
		if path == "file2.go" {
			return ioutil.NopCloser(strings.NewReader(file2)), nil
		}
		return os.Open(path)
	}
	{
		name, ok := ShortImport(ctx, "file1.go")
		if !ok {
			t.Errorf("ShortImport(file1) = false, want true")
		}
		if name != "build" {
			t.Errorf("ShortImport(file1) = %q, want \"build\"", name)
		}
	}
	{
		name, ok := ShortImport(ctx, "file2.go")
		if !ok {
			t.Errorf("ShortImport(file2) = false, want true")
		}
		if name != "build" {
			t.Errorf("ShortImport(file2) = %q, want \"build\"", name)
		}
	}
	// remove build tags - should exclude file1
	ctx.BuildTags = nil
	{
		name, ok := ShortImport(ctx, "file1.go")
		if ok {
			t.Errorf("ShortImport(file1) = false, want true")
		}
		if name != "" {
			t.Errorf("ShortImport(file1) = %q, want \"\"", name)
		}
	}
}

// The following tests are buildutil specific.

type goodOSArchFileTest struct {
	GOOS, GOARCH string
	filename     string
	tags         []string
	match        bool
}

var goodOSArchFileTests = []goodOSArchFileTest{
	{
		filename: "main.go",
		match:    true,
	},
	{
		GOOS:     "linux",
		filename: "syscall_dup2_linux.go",
		tags:     []string{"linux"},
		match:    true,
	},
	{
		GOOS:     "darwin",
		GOARCH:   "amd64",
		filename: "syscall_darwin_amd64.go",
		tags:     []string{"darwin", "amd64"},
		match:    true,
	},
	{
		GOOS:     "darwin",
		GOARCH:   "arm64",
		filename: "syscall_darwin_arm64.go",
		tags:     []string{"darwin", "arm64"},
		match:    true,
	},
	{
		GOOS:     runtime.GOOS,
		filename: fmt.Sprintf("syscall_%s.go", runtime.GOOS),
		tags:     []string{runtime.GOOS},
		match:    true,
	},
	{
		GOOS:     runtime.GOOS,
		GOARCH:   runtime.GOARCH,
		filename: fmt.Sprintf("syscall_%s_%s.go", runtime.GOOS, runtime.GOARCH),
		tags:     []string{runtime.GOOS, runtime.GOARCH},
		match:    true,
	},
	{
		GOOS:     "darwin",
		filename: "syscall_dup2_linux.go",
		tags:     []string{"linux"},
		match:    false,
	},
	{
		GOOS:     "darwin",
		GOARCH:   "arm64",
		filename: "syscall_darwin_amd64.go",
		tags:     []string{"darwin", "amd64"},
		match:    false,
	},
	{
		GOOS:     "darwin",
		GOARCH:   "amd64",
		filename: "syscall_darwin_arm64.go",
		tags:     []string{"darwin", "arm64"},
		match:    false,
	},
	// TODO: for now the "unix" build constraint is not applied to file names
	{
		GOOS:     "darwin",
		GOARCH:   "arm64",
		filename: "syscall_unix.go",
		match:    true,
	},
	{
		GOOS:     "windows",
		GOARCH:   "amd64",
		filename: "syscall_unix.go",
		match:    true,
	},
}

func init() {
	// Add a "_test.go" variant to the goodOSArchFile() tests
	for _, test := range goodOSArchFileTests {
		x := test
		x.filename = strings.TrimSuffix(x.filename, ".go") + "_test.go"
		x.tags = append([]string(nil), x.tags...)
		goodOSArchFileTests = append(goodOSArchFileTests, x)
	}
}

func TestGoodOSArchFile(t *testing.T) {
	for _, x := range goodOSArchFileTests {
		ctxt := build.Default
		if x.GOOS != "" {
			ctxt.GOOS = x.GOOS
		}
		if x.GOARCH != "" {
			ctxt.GOARCH = x.GOARCH
		}
		allTags := make(map[string]bool)
		got := goodOSArchFile(&ctxt, x.filename, allTags)
		var tags []string
		for name := range allTags {
			tags = append(tags, name)
		}
		sort.Strings(tags)
		sort.Strings(x.tags)
		if got != x.match || !reflect.DeepEqual(tags, x.tags) {
			t.Errorf("goodOSArchFile(%q)", x.filename)
			t.Logf("    Match: got: %t want: %t", got, x.match)
			t.Logf("    Tags:  got: %q want: %q", tags, x.tags)
			t.Logf("    Test:  %+v", x)
		}
	}
}

func TestImportPath(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	importPathTests := []string{
		".",
		"os",
		"net/http",
		"text/template/parse",
		wd,
		filepath.Join(wd, "vendor", "buildutil_vendor_test", "hello"),
		filepath.Join(wd, "testdata"),
		filepath.Join(wd, "vendor", "does_not_exit"),
		"package-does-not-exist-123ABC",
	}
	for i, s := range importPathTests {
		importPathTests[i] = filepath.ToSlash(s)
	}

	ctxt := build.Default
	ctxt.GOPATH = ""
	for i, dir := range importPathTests {
		t.Run("", func(t *testing.T) {
			dir = filepath.ToSlash(dir)
			pkg, buildErr := ctxt.ImportDir(dir, build.FindOnly)

			path, err := ImportPath(&ctxt, dir)
			if err != nil && buildErr == nil {
				t.Fatalf("%d: failed to import directory %q: %v", i, dir, err)
			}
			if buildErr != nil && err == nil {
				t.Fatalf("%d: expected error for directory %q found: %v, want: %v", i, dir, err, buildErr)
			}
			if err != nil && buildErr != nil && buildErr.Error() != err.Error() {
				t.Fatalf("%d: error mismatch for directory %q, found: %q, want: %q", i, dir, err.Error(), buildErr.Error())
			}
			if path != pkg.ImportPath {
				t.Fatalf("%d: Import succeeded but found: %q, want: %q", i, path, pkg.ImportPath)
			}
		})
	}
}

var parseBuildConstraintTests = []struct {
	buildComment string
	plusBuild    string
}{
	{
		buildComment: ``,
		plusBuild:    ``,
	},
	{
		buildComment: `//go:build linux`,
		plusBuild:    `// +build linux`,
	},
	{
		buildComment: `//go:build !linux`,
		plusBuild:    `// +build !linux`,
	},
	{
		buildComment: `//go:build (linux || freebsd || openbsd || netbsd) && !appengine`,
		plusBuild:    `// +build linux freebsd openbsd netbsd` + "\n" + `// +build !appengine`,
	},
	{
		buildComment: `//go:build foo && bar && baz`,
		plusBuild:    `// +build foo` + "\n" + `// +build bar` + "\n" + `// +build baz`,
	},
}

func TestParseBuildConstraint(t *testing.T) {
	for i, x := range parseBuildConstraintTests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			pkgSrc := fmt.Sprintf("\n\npackage pkg%d\n", i)
			want, err := parseBuildConstraint([]byte(x.buildComment + pkgSrc))
			if err != nil {
				t.Fatal(err)
			}
			got, err := parseBuildConstraint([]byte(x.plusBuild + pkgSrc))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(want, got) {
				t.Errorf("parseBuildConstraint: want: %v got: %v", want, got)
				t.Logf("Build Comment: %q", x.buildComment)
				t.Logf("Plus Build:    %q", x.plusBuild)
				return
			}
		})
	}
}

func testMatchFile(t *testing.T, ctxt *build.Context, dir string) {
	des, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if testing.Short() && len(des) > 64 {
		des = des[:64]
	}

	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("Context: %+v", *ctxt)
		}
	})
	for _, d := range des {
		name := d.Name()
		if !d.Type().IsRegular() || !strings.HasSuffix(name, ".go") {
			continue
		}
		if testing.Short() && strings.HasSuffix(name, "_test.go") {
			continue
		}
		gotName, gotMatch, err := MatchFile(ctxt, dir, name, nil)
		if err != nil {
			t.Fatal(err)
		}
		wantName, err := ReadPackageName(filepath.Join(dir, name), nil)
		if err != nil {
			t.Fatal(err)
		}
		wantMatch, err := ctxt.MatchFile(dir, name)
		if err != nil {
			t.Fatal(err)
		}
		if gotMatch != wantMatch || (wantMatch && gotName != wantName) {
			t.Errorf("MatchFile(%q) = %q, %t; want: %q, %t", name,
				gotName, gotMatch, wantName, wantMatch)
		}
	}
}

func TestMatchFile(t *testing.T) {
	dir := filepath.Join(runtime.GOROOT(), "src", "runtime")
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Skip("skipping: test requires Go source:", err)
	}

	t.Run("darwin/arm64", func(t *testing.T) {
		t.Parallel()
		ctxt := build.Default
		ctxt.GOOS = "darwin"
		ctxt.GOARCH = "arm64"
		testMatchFile(t, &ctxt, dir)
	})

	t.Run("linux/amd64", func(t *testing.T) {
		t.Parallel()
		ctxt := build.Default
		ctxt.GOOS = "linux"
		ctxt.GOARCH = "amd64"
		ctxt.CgoEnabled = true
		testMatchFile(t, &ctxt, dir)
	})
}

func BenchmarkImportPath(b *testing.B) {
	wd, err := os.Getwd()
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < b.N; i++ {
		_, err := ImportPath(&build.Default, wd)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkImportPath_Base(b *testing.B) {
	wd, err := os.Getwd()
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < b.N; i++ {
		_, err := build.ImportDir(wd, build.FindOnly)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func shortImportFiles(b *testing.B) []string {
	list, err := filepath.Glob("testdata/os/*.go")
	if err != nil {
		b.Fatal(err)
	}
	return list
}

func benchmarkShortImport(b *testing.B, ctxt *build.Context, list []string) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ShortImport(ctxt, list[i%len(list)])
	}
}

// Benchmark ShortImport when reading the file being imported.  File reads
// significantly impact the performance of ShortImport.  This benchmark
// helps identify changes that reduce/increase the number of reads.
func BenchmarkShortImport_ReadFile(b *testing.B) {
	list := shortImportFiles(b)
	ctxt := build.Default
	benchmarkShortImport(b, &ctxt, list)
}

// Fast No-Op byte reader/closer
type nopReadCloser struct {
	s []byte
	i int64
}

func (r *nopReadCloser) Read(b []byte) (n int, err error) {
	if r.i >= int64(len(r.s)) {
		return 0, io.EOF
	}
	n = copy(b, r.s[r.i:])
	r.i += int64(n)
	return
}

func (*nopReadCloser) Close() error { return nil }
func (r *nopReadCloser) Reset()     { r.i = 0 }

// Benchmark ShortImport when using an overlay of the files being imported.
// This benchmarks the performance of parsing the 'package' clause by
// eliminating the overhead of reading files.
func BenchmarkShortImport_Overlay(b *testing.B) {
	list := shortImportFiles(b)

	// read the files into memory and create an overlay for the build.Context
	overlay := make(map[string]*nopReadCloser, len(list))
	for _, name := range list {
		src, err := ioutil.ReadFile(name)
		if err != nil {
			b.Fatal(err)
		}
		overlay[name] = &nopReadCloser{s: src}
	}
	ctxt := build.Default
	ctxt.OpenFile = func(path string) (io.ReadCloser, error) {
		rd, ok := overlay[path]
		if !ok {
			panic("missing file: " + path)
		}
		rd.Reset()
		return rd, nil
	}

	benchmarkShortImport(b, &ctxt, list)
}

func BenchmarkMatchFile(b *testing.B) {
	dir := b.TempDir()
	name := filepath.Join(dir, "build.go")
	// if err := os.WriteFile(name, []byte(LongPackageHeader), 0644); err != nil {
	const content = "package foo\n"
	if err := os.WriteFile(name, []byte(content), 0644); err != nil {
		b.Fatal(err)
	}
	ctxt := build.Default

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := MatchFile(&ctxt, dir, name, LongPackageHeader)
		if err != nil {
			b.Fatal(err)
		}
	}
}
