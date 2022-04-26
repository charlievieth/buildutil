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
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
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
		if !match(ctxt, tag, m, false) {
			t.Errorf("%s context should match %s, does not", what, tag)
		}
		if !reflect.DeepEqual(m, want) {
			t.Errorf("%s tags = %v, want %v", tag, m, want)
		}
	}
	nomatch := func(tag string, want map[string]bool) {
		m := make(map[string]bool)
		if match(ctxt, tag, m, false) {
			t.Errorf("%s context should NOT match %s, does", what, tag)
		}
		if !reflect.DeepEqual(m, want) {
			t.Errorf("%s tags = %v, want %v", tag, m, want)
		}
	}

	matchFn(runtime.GOOS+","+runtime.GOARCH, map[string]bool{runtime.GOOS: true, runtime.GOARCH: true})
	matchFn(runtime.GOOS+","+runtime.GOARCH+",!foo", map[string]bool{runtime.GOOS: true, runtime.GOARCH: true, "foo": false})
	nomatch(runtime.GOOS+","+runtime.GOARCH+",foo", map[string]bool{runtime.GOOS: true, runtime.GOARCH: true, "foo": true})

	what = "modified"
	ctxt.BuildTags = []string{"foo"}
	defer func() { ctxt.BuildTags = ctxt.BuildTags[:0] }()

	matchFn(runtime.GOOS+","+runtime.GOARCH, map[string]bool{runtime.GOOS: true, runtime.GOARCH: true})
	matchFn(runtime.GOOS+","+runtime.GOARCH+",foo", map[string]bool{runtime.GOOS: true, runtime.GOARCH: true, "foo": true})
	nomatch(runtime.GOOS+","+runtime.GOARCH+",!foo", map[string]bool{runtime.GOOS: true, runtime.GOARCH: true, "foo": false})
	matchFn(runtime.GOOS+","+runtime.GOARCH+",!bar", map[string]bool{runtime.GOOS: true, runtime.GOARCH: true, "bar": false})
	nomatch(runtime.GOOS+","+runtime.GOARCH+",bar", map[string]bool{runtime.GOOS: true, runtime.GOARCH: true, "bar": true})
	nomatch("!", map[string]bool{})
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
			shouldBuild, binaryOnly, err := shouldBuildX(ctx, []byte(tt.content), tags)
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

/*
type shouldBuildTest struct {
	src   string
	name  string
	tags  map[string]bool
	match bool
}

var shouldBuildTests = []shouldBuildTest{
	{
		src: "// +build tag1\n\n" +
			"package main\n",
		name:  "main",
		tags:  map[string]bool{"tag1": true},
		match: true,
	},
	{
		src: "// +build !tag1\n\n" +
			"package main\n",
		name:  "",
		tags:  map[string]bool{"tag1": false},
		match: false,
	},
	{
		src: "// +build !tag2 tag1\n\n" +
			"package main\n",
		name:  "main",
		tags:  map[string]bool{"tag1": true, "tag2": false},
		match: true,
	},
	{
		src: "// +build cgo\n\n" +
			"// This package implements parsing of tags like\n" +
			"// +build tag1\n" +
			"package build",
		name:  "",
		tags:  map[string]bool{"cgo": true},
		match: false,
	},
	{
		src: "// Copyright The Go Authors.\n\n" +
			"package build\n\n" +
			"// shouldBuild checks tags given by lines of the form\n" +
			"// +build tag\n" +
			"func shouldBuild(content []byte)\n",
		name:  "build",
		tags:  map[string]bool{},
		match: true,
	},
	{
		src: "// Copyright The Go Authors.\n\n" +
			"package build\n\n" +
			"// shouldBuild checks tags given by lines of the form\n" +
			"// +build tag\n" +
			"func shouldBuild(content \n", // here
		name:  "build",
		tags:  map[string]bool{},
		match: true,
	},
}

func TestShouldBuild_XX(t *testing.T) {
	ctx := &build.Context{BuildTags: []string{"tag1"}}

	for i, x := range shouldBuildTests {
		m := map[string]bool{}
		filename := fmt.Sprintf("file%d", i+1)

		if ok := shouldBuild(ctx, []byte(x.src), m); ok != x.match {
			t.Errorf("%d: shouldBuild(%s) = %v, want %v", i, filename, ok, x.match)
		}
		// Test exported wrapper around shouldBuild.
		if ok := ShouldBuild(ctx, []byte(x.src), m); ok != x.match {
			t.Errorf("%d: shouldBuild(%s) = %v, want %v", i, filename, ok, x.match)
		}
		if !reflect.DeepEqual(m, x.tags) {
			t.Errorf("%d: shoudBuild(%s) tags = %v, want %v", i, filename, m, x.tags)
		}
	}
}
*/

/*
func TestShortImport(t *testing.T) {
	ctx := &build.Context{BuildTags: []string{"tag1"}}

	for i, x := range shouldBuildTests {
		filename := fmt.Sprintf("file%d", i+1)

		ctx.OpenFile = func(path string) (io.ReadCloser, error) {
			if path != filename {
				t.Errorf("OpenFile: filename = %s want %s", path, filename)
			}
			return ioutil.NopCloser(strings.NewReader(x.src)), nil
		}

		name, ok := ShortImport(ctx, filename)
		if ok != x.match {
			t.Errorf("ShortImport(%s) = %v, want %v", filename, ok, x.match)
		}
		if name != x.name {
			t.Errorf("ShortImport(%s) = %q, want %q", filename, name, x.name)
		}
	}
}
*/

func TestMatchContext_BuildTags(t *testing.T) {

	// Remove tag
	{
		src := "//go:build !tag1\n\n" +
			"package main\n"
		orig := &build.Context{BuildTags: []string{"tag1"}}

		ctx, err := MatchContext(orig, "file1", src)
		if err != nil {
			t.Fatal(err)
		}
		if len(ctx.BuildTags) != 0 {
			t.Errorf("MatchContext - BuildTags: want [] got: %s", ctx.BuildTags)
		}
	}

	// Add tags
	{
		src := "//go:build tag1 && tag2\n\n" +
			"package main\n"

		expTags := []string{"tag1", "tag2"}
		orig := &build.Context{}

		ctx, err := MatchContext(orig, "file1", src)
		if err != nil {
			t.Fatal(err)
		}

		sort.Strings(ctx.BuildTags)
		if !reflect.DeepEqual(ctx.BuildTags, expTags) {
			t.Errorf("MatchContext - BuildTags: want %s got: %s", expTags, ctx.BuildTags)
		}
	}

	// Add + Remove tags
	{
		src := "//go:build tag1,tag2,!tag3,tag4\n\n" +
			"package main\n"

		expTags := []string{"tag1", "tag2", "tag4"}
		orig := &build.Context{BuildTags: []string{"tag3", "tag4"}}

		ctx, err := MatchContext(orig, "file1", src)
		if err != nil {
			t.Fatal(err)
		}

		sort.Strings(ctx.BuildTags)
		if !reflect.DeepEqual(ctx.BuildTags, expTags) {
			t.Errorf("MatchContext - BuildTags: want %s got: %s", expTags, ctx.BuildTags)
		}
	}

	// Handle 'ignore'
	{
		src := "//go:build ignore\n\n" +
			"package main\n"

		expTags := []string{"ignore"}
		orig := &build.Context{}

		ctx, err := MatchContext(orig, "file1", src)
		if err != nil {
			t.Fatal(err)
		}

		sort.Strings(ctx.BuildTags)
		if !reflect.DeepEqual(ctx.BuildTags, expTags) {
			t.Errorf("MatchContext - BuildTags: want %s got: %s", expTags, ctx.BuildTags)
		}
	}
}

func TestMatchContext_GOOS(t *testing.T) {

	// find preferred OS that is not the current OS.
	var prefGOOS string
	for _, s := range preferredOSList {
		if s != runtime.GOOS {
			prefGOOS = s
			break
		}
	}
	if prefGOOS == "" || prefGOOS == runtime.GOOS {
		t.Fatal("failed to find GOOS from preferred list!")
	}

	orig := build.Default

	// use only valid GOOS
	{
		src := fmt.Sprintf("// +build %s\n\npackage main\n", prefGOOS)
		ctx, err := MatchContext(&orig, "file1", src)
		if err != nil {
			t.Error(err)
		}
		if ctx.GOOS != prefGOOS {
			t.Errorf("MatchContext: GOOS = %s, want %s", ctx.GOOS, prefGOOS)
		}
	}

	// pick from preferred list
	{
		src := fmt.Sprintf("// +build !%s\n\npackage main\n", runtime.GOOS)
		ctx, err := MatchContext(&orig, "file1", src)
		if err != nil {
			t.Error(err)
		}
		if ctx.GOOS != prefGOOS {
			t.Errorf("MatchContext: GOOS = %s, want %s", ctx.GOOS, prefGOOS)
		}
	}

	// exclude all preferred OSs
	{
		list := make([]string, len(preferredOSList))
		for i, s := range preferredOSList[0:] {
			list[i] = "!" + s
		}
		src := fmt.Sprintf("// +build %s\n\npackage main\n", strings.Join(list, ","))
		ctx, err := MatchContext(&orig, "file1", src)
		if err != nil {
			t.Error(err)
		}
		if ctx.GOOS == runtime.GOOS {
			t.Errorf("MatchContext: GOOS (%s) is negated: %s", ctx.GOOS, src)
		}
		if !knownOS[ctx.GOOS] {
			t.Errorf("MatchContext: GOOS (%s) is not in known OS list: %s", ctx.GOOS, knownOSList)
		}
	}
}

func TestMatchContext_Filename(t *testing.T) {
	const src = "//\n\npackage platform\n\n" +
		"import \"golang.org/x/sys/windows\"\n\n"
	const expGOOS = "windows"
	orig := &build.Context{GOOS: "darwin"}

	ctx, err := MatchContext(orig, "file_"+expGOOS+".go", src)
	if err != nil {
		t.Error(err)
	}
	if ctx.GOOS != expGOOS {
		t.Errorf("MatchContext - Filename: want: %s got: %s", expGOOS, ctx.GOOS)
	}
}

func TestMatchContext_ReleaseTags(t *testing.T) {
	orig := build.Default
	orig.ReleaseTags = []string{"go1.1", "go1.2", "go1.3", "go1.4", "go1.5", "go1.6", "go1.7", "go1.8"}

	src := "// +build !go1.8\n\n" +
		"package main\n"
	ctx, err := MatchContext(&orig, "file1", src)
	if err != nil {
		t.Error(err)
	}
	if len(ctx.BuildTags) != 0 {
		t.Errorf("MatchContext.ReleaseTags: want [] got: %s", ctx.BuildTags)
	}
	if ctx.GOOS != runtime.GOOS {
		t.Errorf("MatchContext.ReleaseTags: GOOS = %s, want %s", ctx.GOOS, runtime.GOOS)
	}
	if ctx.GOARCH != runtime.GOARCH {
		t.Errorf("MatchContext.ReleaseTags: GOARCH = %s, want %s", ctx.GOARCH, runtime.GOARCH)
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

func TestImportPath(t *testing.T) {
	var importPathTests = []string{
		".",
		"os",
		"net/http",
		"text/template/parse",
		CurrentWorkingDirectory,
		filepath.Join(CurrentWorkingDirectory, "vendor", "buildutil_vendor_test", "hello"),
		filepath.Join(CurrentWorkingDirectory, "testdata"),
		filepath.Join(CurrentWorkingDirectory, "vendor", "does_not_exit"),
		"package-does-not-exist-123ABC",
	}

	ctxt := build.Default
	ctxt.GOPATH = ""
	for i, dir := range importPathTests {
		pkg, buildErr := ctxt.ImportDir(dir, build.FindOnly)

		path, err := ImportPath(&ctxt, dir)
		if err != nil && buildErr == nil {
			t.Fatalf("%d: failed to import directory %q: %v", i, dir, err)
		}
		if buildErr != nil && err == nil {
			t.Fatalf("%d: expected error for directory %q found %v, want %v", i, dir, err, buildErr)
		}
		if err != nil && buildErr != nil && buildErr.Error() != err.Error() {
			t.Fatalf("%d: error mismatch for directory %q, found %v, want %v", i, dir, err, buildErr)
		}
		if path != pkg.ImportPath {
			t.Fatalf("%d: Import succeeded but found %q, want %q", i, path, pkg.ImportPath)
		}
	}
}

func TestFixGOPATH(t *testing.T) {
	var tests = []struct {
		In, Exp string
	}{
		{"/Users/foo/go/src/github.com/charlievieth/buildutil/buildutil_test.go", "/Users/foo/go"},
		{"/Users/foo/x/go/src/github.com/charlievieth/buildutil/buildutil_test.go", "/Users/foo/x/go"},
		{"/Users/foo/x/go/buildutil_test.go", build.Default.GOPATH},
	}
	for _, x := range tests {
		ctxt := build.Context{GOROOT: runtime.GOROOT()}
		fixGOPATH(&ctxt, x.In)
		if ctxt.GOPATH != x.Exp {
			t.Errorf("%+v: got: %q want: %q", x, ctxt.GOPATH, x.Exp)
		}
	}
}

func TestEnvMap(t *testing.T) {
	exp := map[string]string{
		"a": "",
		"b": "",
		"c": "v",
	}
	m := envMap([]string{"a", "b=", "c=c", "c=v"})
	if !reflect.DeepEqual(m, exp) {
		t.Errorf("got: %q want: %q", m, exp)
	}
}

func TestMergeTagArgs(t *testing.T) {
	exp := []string{"foo", "race", "bar"}
	tags := mergeTagArgs([]string{"!race", "foo"}, []string{"race", "bar"})
	if !reflect.DeepEqual(tags, exp) {
		t.Errorf("got: %q want: %q", tags, exp)
	}
}

func TestExtractTagArgs(t *testing.T) {
	if a := extractTagArgs([]string{"-v"}); a != nil {
		t.Errorf("got: %v want: %v", a, nil)
	}
	exp := []string{"race", "integration"}
	for _, args := range [][]string{
		{"-c", "-tags=race,integration"},
		{"-c", "-tags", "race,integration"},
		{"-c", "-tags", "race,integration", "--", "-tags=foo"},
	} {
		a := extractTagArgs(args)
		if !reflect.DeepEqual(a, exp) {
			t.Errorf("%q: got: %q want: %q", args, a, exp)
		}
	}
}

func TestReplaceTagArgs(t *testing.T) {
	replace := []string{"foo", "bar"}
	for _, args := range [][]string{
		{"-c", "-tags=race,integration"},
		{"-c", "-tags", "race,integration"},
		{"-c", "-tags", "race,integration", "--", "-tags=foo"},
	} {
		newArgs := replaceTagArgs(args, replace)
		tags := extractTagArgs(newArgs)
		if !reflect.DeepEqual(tags, replace) {
			t.Errorf("%q: got: %q want: %q", args, tags, replace)
		}
	}
}

func BenchmarkImportPath(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := ImportPath(&build.Default, CurrentWorkingDirectory)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkImportPath_Base(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := build.ImportDir(CurrentWorkingDirectory, build.FindOnly)
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
		rd, ok := overlay[path]
		if !ok {
			panic("missing file: " + path)
		}
		rd.Reset()
		return rd, nil
	}

	benchmarkShortImport(b, &ctxt, list)
}

func BenchmarkMatchContext(b *testing.B) {
	data, err := ioutil.ReadFile("buildutil.go")
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		MatchContext(nil, "buildutil.go", data)
	}
}
