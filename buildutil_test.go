// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package buildutil

import (
	"go/build"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// The following tests were copied from the Go standard library.

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

var (
	CurrentImportPath       string
	CurrentWorkingDirectory string
)

func init() {
	cwd, _ := os.Getwd()
	for _, s := range build.Default.SrcDirs() {
		if strings.HasPrefix(cwd, s) {
			CurrentImportPath = filepath.ToSlash(strings.TrimLeft(strings.TrimPrefix(cwd, s), string(filepath.Separator)))
			break
		}
	}
	if CurrentImportPath == "" {
		panic("Invalid CurrentImportPath")
	}
	CurrentWorkingDirectory = filepath.ToSlash(cwd)
}

// Copied from go/build/build_test.go
func TestMatch(t *testing.T) {
	ctxt := &build.Default
	what := "default"
	matchFn := func(tag string, want map[string]bool) {
		m := make(map[string]bool)
		if !match(ctxt, tag, m) {
			t.Errorf("%s context should match %s, does not", what, tag)
		}
		if !reflect.DeepEqual(m, want) {
			t.Errorf("%s tags = %v, want %v", tag, m, want)
		}
	}
	nomatch := func(tag string, want map[string]bool) {
		m := make(map[string]bool)
		if match(ctxt, tag, m) {
			t.Errorf("%s context should NOT match %s, does", what, tag)
		}
		if !reflect.DeepEqual(m, want) {
			t.Errorf("%s tags = %v, want %v", tag, m, want)
		}
	}

	matchFn(runtime.GOOS+","+runtime.GOARCH, map[string]bool{runtime.GOOS: true, runtime.GOARCH: true})
	matchFn(runtime.GOOS+","+runtime.GOARCH+",!foo", map[string]bool{runtime.GOOS: true, runtime.GOARCH: true, "foo": true})
	nomatch(runtime.GOOS+","+runtime.GOARCH+",foo", map[string]bool{runtime.GOOS: true, runtime.GOARCH: true, "foo": true})

	what = "modified"
	ctxt.BuildTags = []string{"foo"}
	matchFn(runtime.GOOS+","+runtime.GOARCH, map[string]bool{runtime.GOOS: true, runtime.GOARCH: true})
	matchFn(runtime.GOOS+","+runtime.GOARCH+",foo", map[string]bool{runtime.GOOS: true, runtime.GOARCH: true, "foo": true})
	nomatch(runtime.GOOS+","+runtime.GOARCH+",!foo", map[string]bool{runtime.GOOS: true, runtime.GOARCH: true, "foo": true})
	matchFn(runtime.GOOS+","+runtime.GOARCH+",!bar", map[string]bool{runtime.GOOS: true, runtime.GOARCH: true, "bar": true})
	nomatch(runtime.GOOS+","+runtime.GOARCH+",bar", map[string]bool{runtime.GOOS: true, runtime.GOARCH: true, "bar": true})
	nomatch("!", map[string]bool{})
}

// Copied from go/build/build_test.go
func TestShouldBuild(t *testing.T) {
	const file1 = "// +build tag1\n\n" +
		"package main\n"
	want1 := map[string]bool{"tag1": true}

	const file2 = "// +build cgo\n\n" +
		"// This package implements parsing of tags like\n" +
		"// +build tag1\n" +
		"package build"
	want2 := map[string]bool{"cgo": true}

	const file3 = "// Copyright The Go Authors.\n\n" +
		"package build\n\n" +
		"// shouldBuild checks tags given by lines of the form\n" +
		"// +build tag\n" +
		"func shouldBuild(content []byte)\n"
	want3 := map[string]bool{}

	// Syntax error
	const file4 = "// Copyright The Go Authors.\n\n" +
		"package build\n\n" +
		"// shouldBuild checks tags given by lines of the form\n" +
		"// +build tag\n" +
		"func shouldBuild(content \n" // here
	want4 := map[string]bool{}

	ctx := &build.Context{BuildTags: []string{"tag1"}}
	m := map[string]bool{}
	if !shouldBuild(ctx, []byte(file1), m) {
		t.Errorf("shouldBuild(file1) = false, want true")
	}
	// Test exported wrapper around shouldBuild.
	if !ShouldBuild(ctx, []byte(file1), m) {
		t.Errorf("ShouldBuild(file1) = false, want true")
	}
	if !reflect.DeepEqual(m, want1) {
		t.Errorf("shoudBuild(file1) tags = %v, want %v", m, want1)
	}

	m = map[string]bool{}
	if shouldBuild(ctx, []byte(file2), m) {
		t.Errorf("shouldBuild(file2) = true, want false")
	}
	if ShouldBuild(ctx, []byte(file2), m) {
		t.Errorf("ShouldBuild(file2) = true, want false")
	}
	if !reflect.DeepEqual(m, want2) {
		t.Errorf("shoudBuild(file2) tags = %v, want %v", m, want2)
	}

	m = map[string]bool{}
	ctx = &build.Context{BuildTags: nil}
	if !shouldBuild(ctx, []byte(file3), m) {
		t.Errorf("shouldBuild(file3) = false, want true")
	}
	if !ShouldBuild(ctx, []byte(file3), m) {
		t.Errorf("ShouldBuild(file3) = false, want true")
	}
	if !reflect.DeepEqual(m, want3) {
		t.Errorf("shoudBuild(file3) tags = %v, want %v", m, want3)
	}

	// Syntax error
	m = map[string]bool{}
	if !shouldBuild(ctx, []byte(file4), m) {
		t.Errorf("shouldBuild(file4) = false, want true")
	}
	if !ShouldBuild(ctx, []byte(file4), m) {
		t.Errorf("ShouldBuild(file4) = false, want true")
	}
	if !reflect.DeepEqual(m, want4) {
		t.Errorf("shoudBuild(file4) tags = %v, want %v", m, want3)
	}
}

// Identical to TestShouldBuild, but testing ShortImport
func TestShortImport(t *testing.T) {
	const file1 = "// +build tag1\n\n" +
		"package main\n"

	const file2 = "// +build cgo\n\n" +
		"// This package implements parsing of tags like\n" +
		"// +build tag1\n" +
		"package build"

	const file3 = "// Copyright The Go Authors.\n\n" +
		"package build\n\n" +
		"// shouldBuild checks tags given by lines of the form\n" +
		"// +build tag\n" +
		"func shouldBuild(content []byte)\n"

	// Syntax error
	const file4 = "// Copyright The Go Authors.\n\n" +
		"package build\n\n" +
		"// shouldBuild checks tags given by lines of the form\n" +
		"// +build tag\n" +
		"func shouldBuild(content \n" // here

	overlay := map[string]*nopReadCloser{
		"file1": &nopReadCloser{s: []byte(file1)},
		"file2": &nopReadCloser{s: []byte(file2)},
		"file3": &nopReadCloser{s: []byte(file3)},
		"file4": &nopReadCloser{s: []byte(file4)},
	}
	ctx := &build.Context{
		BuildTags: []string{"tag1"},
		OpenFile: func(path string) (io.ReadCloser, error) {
			rc, ok := overlay[path]
			if !ok {
				panic("missing file: " + path)
			}
			return rc, nil
		},
	}

	var testCases = []struct {
		file string
		name string
		ok   bool
	}{
		{"file1", "main", true},
		{"file2", "", false},
		{"file3", "build", true},
		{"file4", "build", true},
	}

	for _, x := range testCases {
		name, ok := ShortImport(ctx, x.file)
		if ok != x.ok {
			t.Errorf("ShortImport(%s) = %v, want %v", x.file, ok, x.ok)
		}
		if name != x.name {
			t.Errorf("ShortImport(%s) = %q, want %q", x.file, name, x.name)
		}
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

// +build tag1
package build

// +build tag1

import "bytes"

// +build tag1

// shouldBuild checks tags given by lines of the form
// +build tag
func shouldBuild(content []byte) bool {

	// +build tag1

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
	if !shouldBuild(ctx, []byte(file1), m) {
		t.Errorf("shouldBuild(file1) = false, want true")
	}
	if !reflect.DeepEqual(m, want1) {
		t.Errorf("shoudBuild(file1) tags = %v, want %v", m, want1)
	}

	m = map[string]bool{}
	if !shouldBuild(ctx, []byte(file2), m) {
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

func TestGoodOSArchFile(t *testing.T) {
	ctxt := &build.Default
	what := "default"
	matchFn := func(tag string, want map[string]bool) {
		m := make(map[string]bool)
		if !GoodOSArchFile(ctxt, tag, m) {
			t.Errorf("%s GoodOSArchFile should match %s, does not", what, tag)
		}
		if !reflect.DeepEqual(m, want) {
			t.Errorf("%s tags = %v, want %v", tag, m, want)
		}
	}
	nomatch := func(tag string, want map[string]bool) {
		m := make(map[string]bool)
		if GoodOSArchFile(ctxt, tag, m) {
			t.Errorf("%s GoodOSArchFile should NOT match %s, does", what, tag)
		}
		if !reflect.DeepEqual(m, want) {
			t.Errorf("%s tags = %v, want %v", tag, m, want)
		}
	}
	badOS := "windows"
	if badOS == runtime.GOOS {
		badOS = "linux"
	}
	badArch := "386"
	if badArch == runtime.GOARCH {
		badArch = "amd64"
	}
	matchFn("x_"+runtime.GOOS+".go", map[string]bool{runtime.GOOS: true})
	matchFn("x_"+runtime.GOARCH+".go", map[string]bool{runtime.GOARCH: true})
	matchFn("x_"+runtime.GOOS+"_"+runtime.GOARCH+".go", map[string]bool{runtime.GOOS: true, runtime.GOARCH: true})

	tempPath := func(s string) string {
		return filepath.Join(os.TempDir(), s)
	}
	matchFn(tempPath("x_"+runtime.GOOS+".go"), map[string]bool{runtime.GOOS: true})
	matchFn(tempPath("x_"+runtime.GOARCH+".go"), map[string]bool{runtime.GOARCH: true})
	matchFn(tempPath("x_"+runtime.GOOS+"_"+runtime.GOARCH+".go"), map[string]bool{runtime.GOOS: true, runtime.GOARCH: true})

	what = "modified"
	nomatch("x_"+badOS+"_"+runtime.GOARCH+".go", map[string]bool{badOS: true, runtime.GOARCH: true})
	nomatch("x_"+runtime.GOOS+"_"+badArch+".go", map[string]bool{runtime.GOOS: true, badArch: true})

	// Test that we only analyze the base path element.
	p := filepath.Join("x_"+badArch+"_"+runtime.GOARCH+".go", "x_"+runtime.GOOS+"_"+runtime.GOARCH+".go")
	matchFn(p, map[string]bool{runtime.GOOS: true, runtime.GOARCH: true})

	what = "invalid tag"
	matchFn(runtime.GOOS+".go", map[string]bool{})
	matchFn(runtime.GOARCH+".go", map[string]bool{})
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
		for _, path := range list {
			ShortImport(ctxt, path)
		}
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
		rc, ok := overlay[path]
		if !ok {
			panic("missing file: " + path)
		}
		return rc, nil
	}

	benchmarkShortImport(b, &ctxt, list)
}
