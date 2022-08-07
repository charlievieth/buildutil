package buildutil

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"go/build"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

var testGoCommandAll = flag.Bool("gocommand-all", false,
	"Test GoCommand for all supported platforms")

func testGoCommand(t *testing.T, filename string, want []string) {
	dir, err := filepath.Abs("testdata/gocommand")
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(dir, filename)
	if _, err := os.Stat(name); err != nil {
		t.Fatal(err)
	}

	orig := build.Default
	ctxt, err := MatchContext(&orig, name, nil)
	if err != nil {
		t.Fatal(err)
	}

	cmd := GoCommand(ctxt, "go", "list", "-tags", "never", "-json")
	cmd.Dir = dir

	data, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}

	type Package struct {
		GoFiles []string
	}
	var pkg Package
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatal(err)
	}

	sort.Strings(pkg.GoFiles)
	sort.Strings(want)
	if !reflect.DeepEqual(pkg.GoFiles, want) {
		t.Errorf("%s: failed to match files:\nGot:  %q\nWant: %q", filename, pkg.GoFiles, want)
	}
}

func TestGoCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: short test")
	}
	t.Parallel()
	tests := map[string][]string{
		"name_darwin_arm64.go": {
			"main.go",
			"name_arm64.go",
			"name_darwin.go",
			"name_darwin_arm64.go",
		},
		"tag_darwin_arm64_tag.go": {
			"main.go",
			"name_arm64.go",
			"name_darwin.go",
			"name_darwin_arm64.go",
			"tag_darwin_arm64_tag.go",
		},
		"nocgo_darwin_arm64_tag.go": {
			"main.go",
			"name_arm64.go",
			"name_darwin.go",
			"name_darwin_arm64.go",
			"nocgo_darwin_arm64_tag.go",
		},
		"nocgo_windows_amd64_tag.go": {
			"main.go",
			"name_amd64.go",
			"name_windows.go",
			"name_windows_amd64.go",
			"nocgo_windows_amd64_tag.go",
		},
		"name_js.go": {
			"main.go",
			"name_js.go",
			"name_js_wasm.go",
			"name_wasm.go",
		},
		"name_linux_mips64le.go": {
			"main.go",
			"name_linux.go",
			"name_linux_mips64le.go",
		},
		"name_android_arm64.go": {
			"main.go",
			"name_android_arm64.go",
			"name_arm64.go",
			"name_linux.go",
			"name_linux_arm64.go",
		},
	}
	names := make([]string, 0, len(tests))
	for name := range tests {
		names = append(names, name)
	}
	sort.Strings(names)
	for i := range names {
		name := names[i]
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			testGoCommand(t, name, tests[name])
		})
	}
}

func TestGoCommandAll(t *testing.T) {
	t.Parallel()
	if !*testGoCommandAll {
		t.Skip("skipping: test only ran when the `-gocommand-all` flag is provided")
	}
	if testing.Short() {
		t.Skip("skipping: short test")
	}

	dir, gopath := createCommandTestFiles(t)

	names, err := filepath.Glob(dir + "/*.go")
	if err != nil {
		t.Fatal(err)
	}

	for i := range names {
		name := names[i]
		t.Run(filepath.Base(name), func(t *testing.T) {
			t.Parallel()

			orig := build.Default
			orig.GOPATH = gopath
			ctxt, err := MatchContext(&orig, name, nil)
			if err != nil {
				t.Fatal(err)
			}

			cmd := GoCommand(ctxt, "go", "list", "-json")
			cmd.Dir = dir
			cmd.Env = append([]string{"GO111MODULE=auto"}, os.Environ()...)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("%s: %s\n\t%s\n", filepath.Base(name), err,
					bytes.TrimSpace(out))
			}
		})
	}
}

func createCommandTestFiles(t *testing.T) (dir, gopath string) {
	tempdir := t.TempDir()
	dirname := filepath.Join(tempdir, "go", "src", "pkg1")
	if err := os.MkdirAll(dirname, 0755); err != nil {
		t.Fatal(err)
	}

	platforms, err := LoadGoPlatforms()
	if err != nil {
		t.Fatal(err)
	}

	writeFile := func(name, content string) {
		data := []byte(content)
		if filepath.Ext(name) == ".go" {
			b, err := format.Source([]byte(content))
			if err != nil {
				t.Fatal(err)
			}
			data = b
		}
		path := filepath.Join(dirname, name)
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	const buildTag = "somebuildtag"
	const packageName = "\n\npackage main\n"
	{
		const mainSource = packageName + `

		func main() {
			println("Hello!")
		}
		`
		writeFile("main.go", mainSource)
	}

	constraints := make(map[string]struct{})
	for _, p := range platforms {
		for _, name := range []string{p.GOOS, p.GOARCH, p.GOOS + "_" + p.GOARCH} {
			constraints[name] = struct{}{}
		}
	}
	for name := range constraints {
		writeFile("name_"+name+".go", packageName)
		if strings.Contains(name, "_") {
			a := strings.Split(name, "_")
			tag := fmt.Sprintf("//go:build %s && %s && %s\n", buildTag, a[0], a[1])
			writeFile("tag_"+name+"_tag.go", tag+packageName)
		}
	}
	for _, p := range platforms {
		if !p.CgoSupported {
			continue
		}
		name := fmt.Sprintf("nocgo_%s_%s_tag.go", p.GOOS, p.GOARCH)
		src := fmt.Sprintf("//go:build !cgo && %s && %s\n%s",
			p.GOOS, p.GOARCH, packageName)
		writeFile(name, src)
	}
	return dirname, filepath.Join(tempdir, "go")
}

func TestEnvMap(t *testing.T) {
	exp := map[string]string{
		"a": "",
		"b": "",
		"c": "v",
		"d": "v=v",
	}
	m := envMap([]string{"a", "b=", "c=c", "c=v", "d=v=v"})
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

func BenchmarkGoCommand(b *testing.B) {
	orig := build.Default
	ctxt, err := MatchContext(&orig, "testdata/gocommand/name_darwin_arm64.go", nil)
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	exe, err := exec.LookPath("go")
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GoCommandContext(ctx, ctxt, exe, "list", "-json")
	}
}
