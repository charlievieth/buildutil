package buildutil

import (
	"encoding/json"
	"fmt"
	"go/build"
	"io"
	"io/ioutil"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/charlievieth/buildutil/internal/util"
)

func formatContext(ctxt *build.Context, indent bool) string {
	min := struct {
		GOOS       string
		GOARCH     string
		CgoEnabled bool
		Compiler   string
		BuildTags  []string
		ToolTags   []string
	}{
		GOOS:       ctxt.GOOS,
		GOARCH:     ctxt.GOARCH,
		CgoEnabled: ctxt.CgoEnabled,
		Compiler:   ctxt.Compiler,
		BuildTags:  ctxt.BuildTags,
		ToolTags:   ctxt.ToolTags,
	}
	var data []byte
	var err error
	if indent {
		data, err = json.MarshalIndent(min, "", "    ")
	} else {
		data, err = json.Marshal(min)
	}
	if err != nil {
		panic(err)
	}
	return string(data)
}

type matchContextTest struct {
	filename     string
	build        string
	GOOS, GOARCH string
	BuildTags    []string
	err          string
	want         *build.Context
}

var (
	latestReleaseTag = build.Default.ReleaseTags[len(build.Default.ReleaseTags)-1]
	priorReleaseTag  = build.Default.ReleaseTags[len(build.Default.ReleaseTags)-2]
)

var matchContextTests = []matchContextTest{
	{
		filename: "main.go",
	},
	{
		filename: "main.go",
		build:    "//go:build " + priorReleaseTag,
	},
	{
		filename: "main.go",
		build:    "//go:build !" + priorReleaseTag,
		err:      ErrImpossibleGoVersion.Error(),
	},
	{
		filename: "main.go",
		build:    "//go:build yes && " + latestReleaseTag,
		want:     &build.Context{ReleaseTags: []string{"yes"}},
	},
	{
		filename: "main.go",
		build:    "//go:build !" + latestReleaseTag,
		err:      ErrImpossibleGoVersion.Error(),
	},
	{
		filename: "main.go",
		build:    "//go:build ok || !" + latestReleaseTag,
	},
	{
		filename: "add_tags.go",
		build:    "//go:build tag1 && tag2 && !tag3 && tag4",
		want:     &build.Context{BuildTags: []string{"tag1", "tag2", "tag4"}},
	},
	{
		filename:  "remove_one_tag.go",
		build:     "//go:build !tag1",
		BuildTags: []string{"tag1"},
		want:      &build.Context{BuildTags: []string{}},
	},
	{
		filename:  "remove_tags.go",
		build:     "//go:build tag1 && tag2 && !tag3 && tag4",
		BuildTags: []string{"tag3", "tag4"},
		want:      &build.Context{BuildTags: []string{"tag1", "tag2", "tag4"}},
	},
	{
		filename: "main.go",
		GOOS:     "darwin",
		build:    "//go:build !" + runtime.GOARCH,
		want:     &build.Context{GOOS: "darwin"},
	},
	{
		filename: "sys_linux.go",
		GOOS:     "darwin",
		GOARCH:   runtime.GOARCH,
		want:     &build.Context{GOOS: "linux", GOARCH: runtime.GOARCH},
	},
	{
		filename: "sys_windows.go",
		GOOS:     "darwin",
		GOARCH:   runtime.GOARCH,
		want:     &build.Context{GOOS: "windows", GOARCH: runtime.GOARCH},
	},
	{
		filename: "add_goexperiment.go",
		build:    "//go:build goexperiment.exp1",
		GOOS:     "darwin",
		GOARCH:   "arm64",
		want:     &build.Context{ToolTags: append(build.Default.ToolTags, "goexperiment.exp1")},
	},
	{
		filename: "remove_goexperiment.go",
		build:    "//go:build !goexperiment.fieldtrack",
		GOOS:     "darwin",
		GOARCH:   "arm64",
	},
	{
		filename: "sys_linux_amd64.go",
		GOOS:     "darwin",
		GOARCH:   "arm64",
	},
	{
		filename: "sys_linux_amd64.go",
		build:    `//go:build mytag`,
		GOOS:     "darwin",
		GOARCH:   "arm64",
	},
	{
		// golang.org/x/crypto/chacha20/chacha_noasm.go
		filename: "chacha_noasm.go",
		build:    `//go:build (!arm64 && !s390x && !ppc64le) || (arm64 && !go1.11) || !gc || purego`,
		GOOS:     "darwin",
		GOARCH:   "arm64",
		want:     &build.Context{GOOS: "darwin", GOARCH: "arm64", BuildTags: []string{"purego"}},
	},
	{
		filename: "impossible.go",
		build:    "//go:build ok && !ok",
		err:      ErrMatchContext.Error(),
	},
}

func testMatchContext(t *testing.T, test matchContextTest) {
	orig := build.Default
	if test.GOOS != "" {
		orig.GOOS = test.GOOS
	}
	if test.GOARCH != "" {
		orig.GOARCH = test.GOARCH
	}
	orig.BuildTags = test.BuildTags

	src := "package test\n"
	if test.build != "" {
		src = test.build + "\n\n" + src
	}
	orig.OpenFile = func(name string) (io.ReadCloser, error) {
		if name != test.filename {
			panic("OpenFile called with invalid name: " + name)
		}
		return io.NopCloser(strings.NewReader(src)), nil
	}

	ctxt, err := MatchContext(&orig, test.filename, src)
	if err != nil {
		if !strings.Contains(err.Error(), test.err) {
			t.Fatal(err)
		}
		return
	}
	ok, err := ctxt.MatchFile("", test.filename)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("failed to match context")
		t.Logf("Matched Context:\n%s", formatContext(ctxt, true))
	}

	if want := test.want; want != nil {
		if want.GOOS != "" && !reflect.DeepEqual(ctxt.GOOS, want.GOOS) {
			t.Errorf("GOOS: got: %v want: %v", ctxt.GOOS, want.GOOS)
		}
		if want.GOARCH != "" && !reflect.DeepEqual(ctxt.GOARCH, want.GOARCH) {
			t.Errorf("GOARCH: got: %v want: %v", ctxt.GOARCH, want.GOARCH)
		}
		if want.Compiler != "" && !reflect.DeepEqual(ctxt.Compiler, want.Compiler) {
			t.Errorf("Compiler: got: %v want: %v", ctxt.Compiler, want.Compiler)
		}
		if want.BuildTags != nil && !util.StringsSame(want.BuildTags, ctxt.BuildTags) {
			t.Errorf("BuildTags: got: %v want: %v", ctxt.BuildTags, want.BuildTags)
		}
		if want.ToolTags != nil && !util.StringsSame(want.ToolTags, ctxt.ToolTags) {
			t.Errorf("ToolTags: got: %v want: %v", ctxt.ToolTags, want.ToolTags)
		}
	}
}

func TestMatchContext(t *testing.T) {
	for i, test := range matchContextTests {
		name := fmt.Sprintf("%d_%s", i, test.filename)
		t.Run(name, func(t *testing.T) {
			testMatchContext(t, test)
		})
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
