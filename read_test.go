// Copyright 2012 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package buildutil

import (
	"bytes"
	"go/build"
	"io"
	"runtime"
	"strings"
	"sync"
	"testing"
)

const quote = "`"

type readTest struct {
	// Test input contains ℙ where readImports should stop.
	in  string
	err string
}

var readGoInfoTests = []readTest{
	{
		`package p`,
		"",
	},
	{
		`package p; import "x"`,
		"",
	},
	{
		`package p; import . "x"`,
		"",
	},
	{
		`package p; import "x";ℙvar x = 1`,
		"",
	},
	{
		`package p

		// comment

		import "x"
		import _ "x"
		import a "x"

		/* comment */

		import (
			"x" /* comment */
			_ "x"
			a "x" // comment
			` + quote + `x` + quote + `
			_ /*comment*/ ` + quote + `x` + quote + `
			a ` + quote + `x` + quote + `
		)
		import (
		)
		import ()
		import()import()import()
		import();import();import()

		ℙvar x = 1
		`,
		"",
	},
	{
		"\ufeff𝔻" + `package p; import "x";ℙvar x = 1`,
		"",
	},
}

var readCommentsTests = []readTest{
	{
		`ℙpackage p`,
		"",
	},
	{
		`ℙpackage p; import "x"`,
		"",
	},
	{
		`ℙpackage p; import . "x"`,
		"",
	},
	{
		"\ufeff𝔻" + `ℙpackage p; import . "x"`,
		"",
	},
	{
		`// foo

		/* bar */

		/* quux */ // baz

		/*/ zot */

		// asdf
		ℙHello, world`,
		"",
	},
	{
		"\ufeff𝔻" + `// foo

		/* bar */

		/* quux */ // baz

		/*/ zot */

		// asdf
		ℙHello, world`,
		"",
	},
}

func testRead(t *testing.T, tests []readTest, read func(io.Reader) ([]byte, error)) {
	for i, tt := range tests {
		beforeP, afterP, _ := cut(tt.in, "ℙ")
		in := beforeP + afterP
		testOut := beforeP

		if beforeD, afterD, ok := cut(beforeP, "𝔻"); ok {
			in = beforeD + afterD + afterP
			testOut = afterD
		}

		r := strings.NewReader(in)
		buf, err := read(r)
		if err != nil {
			if tt.err == "" {
				t.Errorf("#%d: err=%q, expected success (%q)", i, err, string(buf))
			} else if !strings.Contains(err.Error(), tt.err) {
				t.Errorf("#%d: err=%q, expected %q", i, err, tt.err)
			}
			continue
		}
		if tt.err != "" {
			t.Errorf("#%d: success, expected %q", i, tt.err)
			continue
		}

		out := string(buf)
		if out != testOut {
			t.Errorf("#%d: wrong output:\nhave %q\nwant %q\n", i, out, testOut)
		}
	}
}

func TestReadGoInfo(t *testing.T) {
	// run in parallel to shake out any race conditions with the reader pool
	t.Parallel()
	testRead(t, readGoInfoTests, func(r io.Reader) ([]byte, error) {
		var info fileInfo
		err := readGoInfo(r, &info)
		return info.header, err
	})
}

func TestReadGoInfo_Parallel(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: short test")
	}
	numCPU := runtime.NumCPU()
	var wg sync.WaitGroup
	for i := 0; i < numCPU; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100 && !t.Failed(); i++ {
				testRead(t, readGoInfoTests, func(r io.Reader) ([]byte, error) {
					var info fileInfo
					err := readGoInfo(r, &info)
					return info.header, err
				})
			}
		}()
	}
	wg.Wait()
}

// TODO: do we need this test?
func TestReadImports(t *testing.T) {
	t.Parallel()
	testRead(t, readGoInfoTests, func(r io.Reader) ([]byte, error) {
		return readImports(r, true, nil)
	})
}

func TestReadComments(t *testing.T) {
	t.Parallel()
	testRead(t, readCommentsTests, readComments)
}

var readFailuresTests = []readTest{
	{
		`package`,
		"syntax error",
	},
	{
		"package p\n\x00\nimport `math`\n",
		"unexpected NUL in input",
	},
	{
		`package p; import`,
		"syntax error",
	},
	{
		`package p; import "`,
		"syntax error",
	},
	{
		"package p; import ` \n\n",
		"syntax error",
	},
	{
		`package p; import "x`,
		"syntax error",
	},
	{
		`package p; import _`,
		"syntax error",
	},
	{
		`package p; import _ "`,
		"syntax error",
	},
	{
		`package p; import _ "x`,
		"syntax error",
	},
	{
		`package p; import .`,
		"syntax error",
	},
	{
		`package p; import . "`,
		"syntax error",
	},
	{
		`package p; import . "x`,
		"syntax error",
	},
	{
		`package p; import (`,
		"syntax error",
	},
	{
		`package p; import ("`,
		"syntax error",
	},
	{
		`package p; import ("x`,
		"syntax error",
	},
	{
		`package p; import ("x"`,
		"syntax error",
	},
}

func TestReadFailuresIgnored(t *testing.T) {
	t.Parallel()
	// Syntax errors should not be reported (false arg to readImports).
	// Instead, entire file should be the output and no error.
	// Convert tests not to return syntax errors.
	tests := make([]readTest, len(readFailuresTests))
	copy(tests, readFailuresTests)
	for i := range tests {
		tt := &tests[i]
		if !strings.Contains(tt.err, "NUL") {
			tt.err = ""
		}
	}
	testRead(t, tests, func(r io.Reader) ([]byte, error) {
		var info fileInfo
		err := readGoInfo(r, &info)
		return info.header, err
	})
}

var packageNameTests = []struct {
	src  string
	name string
	err  error
}{
	{
		src:  "package p",
		name: "p",
	},
	{
		src:  `package p;`,
		name: "p",
	},
	{
		src:  `package main; func main() { println("hello"); };`,
		name: "main",
	},
	{
		src:  "package foo\n",
		name: "foo",
	},
	{
		src:  "// +build !windows\npackage foo\n",
		name: "foo",
	},
	{
		src:  "// +build !windows\npackagee extra_e\n",
		name: "",
		err:  errSyntax,
	},
	{
		src:  "//go:build darwin && go1.12\npackage p\n",
		name: "p",
	},
	{
		src:  "/* foo */ // +build !windows\npackage foo\n",
		name: "foo",
	},
	{
		src: `/*
			   * Long comment
			   *
			   */
			   //
			   package p1 // Ok`,
		name: "p1",
	},
	{
		src: `// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

` + "// +build !go1.5" + `

// For all Go versions other than 1.5 use the Import and ImportDir functions
// declared in go/build.

package buildutil

import "go/build"
`,
		name: "buildutil",
	},
}

func testReadPackageName(t *testing.T, readName func(src []byte) (string, error)) {
	for i, x := range packageNameTests {
		name, err := readName([]byte(x.src))
		if err != x.err {
			t.Errorf("%d error (%v): %v", i, x.err, err)
		}
		if name != x.name {
			t.Errorf("%d name (%s): %s", i, x.name, name)
		}
	}
}

func TestReadPackageName_Internal(t *testing.T) {
	testReadPackageName(t, readPackageName)
}

func TestReadPackageName_External(t *testing.T) {
	testReadPackageName(t, func(b []byte) (string, error) {
		return ReadPackageName("dummy.go", b)
	})
}

func BenchmarkReadPackageName_Short(b *testing.B) {
	src := []byte("package foo\n")
	for i := 0; i < b.N; i++ {
		_, err := readPackageName(src)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReadPackageName_Medium(b *testing.B) {
	src := []byte("//go:build linux\n\npackage foo\n")
	for i := 0; i < b.N; i++ {
		_, err := readPackageName(src)
		if err != nil {
			b.Fatal(err)
		}
	}
}

const LongPackageHeader = `// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

` + "//go:build !go1.5" + `

/*
  For all Go versions other than 1.5 use the Import and ImportDir functions
  declared in go/build.
*/

package buildutil

import "go/build"`

var LongPackageHeaderBytes = []byte(LongPackageHeader)

func BenchmarkReadPackageName_Long(b *testing.B) {
	src := LongPackageHeaderBytes
	for i := 0; i < b.N; i++ {
		_, err := readPackageName(src)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReadImports_Long(b *testing.B) {
	r := bytes.NewReader(LongPackageHeaderBytes)
	for i := 0; i < b.N; i++ {
		readImports(r, true, nil)
		r.Seek(0, 0)
	}
}

func BenchmarkShortImport_Long(b *testing.B) {
	rc := &nopReadCloser{s: LongPackageHeaderBytes}
	ctxt := build.Default
	ctxt.OpenFile = func(path string) (io.ReadCloser, error) {
		if path != "go_darwin_amd64.go" {
			panic("invalid filename: " + path)
		}
		rc.Reset()
		return rc, nil
	}
	for i := 0; i < b.N; i++ {
		ShortImport(&ctxt, "go_darwin_amd64.go")
	}
}

func BenchmarkReadGoInfo(b *testing.B) {
	rc := &nopReadCloser{s: LongPackageHeaderBytes}
	var info fileInfo
	for i := 0; i < b.N; i++ {
		rc.Reset()
		readGoInfo(rc, &info)
	}
}
