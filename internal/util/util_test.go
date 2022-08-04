package util

import (
	"fmt"
	"go/build"
	"math/rand"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

type stringsTest struct {
	in, want []string
	val      string
}

var appendTests = []stringsTest{
	{nil, []string{"a"}, "a"},
	{[]string{"a"}, []string{"a"}, "a"},
	{[]string{"a"}, []string{"a", "b"}, "b"},
}

func TestStringsAppend(t *testing.T) {
	for _, x := range appendTests {
		got := StringsAppend(x.in, x.val)
		if !reflect.DeepEqual(got, x.want) {
			t.Errorf("StringsAppend(%q, %q) = %q; want: %q", x.in, x.val, got, x.want)
		}
	}
}

var removeAllTests = []stringsTest{
	{nil, nil, "a"},
	{[]string{"a"}, []string{}, "a"},
	{[]string{"a", "b", "a"}, []string{"b"}, "a"},
}

func TestRemoveAllTests(t *testing.T) {
	for _, x := range removeAllTests {
		got := StringsRemoveAll(x.in, x.val)
		if !reflect.DeepEqual(got, x.want) {
			t.Errorf("StringsRemoveAll(%q, %q) = %q; want: %q", x.in, x.val, got, x.want)
		}
	}
}

// func TestDuplicateStringsInPlace(t *testing.T) {
// 	tests := [][][]string{
// 		nil,
// 		{nil, nil},
// 		{{"a"}, {}},
// 		{{"a"}, nil},
// 		{{"a"}, {"b"}, {"c"}},
// 		{{"a", "a", "a"}, {"b", "b", "b"}, {"c", "c", "c"}},
// 	}
// 	for _, strs := range tests {
// 		orig := append([][]string(nil), strs...)
// 		var want [][]string
// 		var in []*[]string
// 		for i, a := range strs {
// 			want = append(want, append([]string(nil), a...))
// 			in = append(in, &strs[i])
// 		}
// 		DuplicateStringsInPlace(in...)
// 		var got [][]string
// 		for _, a := range in {
// 			if a != nil {
// 				got = append(got, *a)
// 			} else {
// 				got = append(got, nil)
// 			}
// 		}
// 		if !reflect.DeepEqual(got, want) {
// 			t.Errorf("DuplicateStringsInPlace(%q) = %q; want: %q", strs, got, want)
// 		}
//
// 		// Test that we made a copy by changing the original
// 		for i := range orig {
// 			for j := range orig[i] {
// 				orig[i][j] = strings.ToUpper(orig[i][j])
// 			}
// 		}
// 		if !reflect.DeepEqual(got, want) {
// 			t.Errorf("DuplicateStringsInPlace(%q) = %q; want: %q", strs, got, want)
// 		}
//
// 		// Make sure cap == len so that append() does not overwrite
// 		// the next []string
// 		for i, a := range got {
// 			if cap(a) != len(a) {
// 				t.Errorf("%d: %q: cap(%q) = %d; want: %d", i, got, a, cap(a), len(a))
// 			}
// 		}
// 	}
// }

type sameStringsTest struct {
	a1, a2 []string
	want   bool
}

var sameTests = []sameStringsTest{
	{nil, nil, true},
	{nil, []string{}, true},
	{[]string{"1", "2", "3"}, []string{"3", "1", "2"}, true},
	{[]string{"1", "2"}, []string{"1"}, false},
	{[]string{"1", "2", "3"}, []string{"3", "1", "3"}, false},
}

func TestStringsSame(t *testing.T) {
	for _, x := range sameTests {
		got := StringsSame(x.a1, x.a2)
		if got != x.want {
			t.Errorf("StringsSame(%q, %q) = %t; want: %t", x.a1, x.a2, got, x.want)
		}
	}
}

func TestEnviron(t *testing.T) {
	env := []string{
		"AAA1=1",
		"AAA2=2",
		"AAA3=3",
		"AAA4=4",
	}
	rand.Shuffle(len(env), func(i, j int) {
		env[i], env[j] = env[j], env[i]
	})
	e := &Environ{env: env}

	for i := 1; i <= len(env); i++ {
		for n := 1; n <= 2; n++ {
			id := strconv.Itoa(i)
			key := "AAA" + id
			want := strings.Repeat(id, n)
			if v, ok := e.Lookup(key); !ok || v != want {
				t.Errorf("Lookup(%q) = %q, %t; want: %q, %t", key, v, ok, want, true)
			}
			e.Set(key, strings.Repeat(strconv.Itoa(i), n+1))
		}
	}
	e.Set("key", "val")
	if v, ok := e.Lookup("key"); !ok || v != "val" {
		t.Errorf("Lookup(%q) = %q, %t; want: %q, %t", "key", v, ok, "val", true)
	}
}

func TestCopyContext(t *testing.T) {
	orig := build.Default
	orig.BuildTags = []string{"test"}
	orig.ToolTags = []string{
		"goexperiment.regabiwrappers",
		"goexperiment.regabireflect",
		"goexperiment.regabiargs",
		"goexperiment.pacerredesign",
	}
	orig.ReleaseTags = []string{
		"go1.1", "go1.2", "go1.3", "go1.4", "go1.5", "go1.6", "go1.7",
		"go1.8", "go1.9", "go1.10", "go1.11", "go1.12", "go1.13", "go1.14",
		"go1.15", "go1.16", "go1.17", "go1.18",
	}
	ctxt := CopyContext(&orig)
	type valuePair struct {
		got, want interface{}
	}
	fields := map[string]valuePair{
		"GOARCH":      {ctxt.GOARCH, orig.GOARCH},
		"GOOS":        {ctxt.GOOS, orig.GOOS},
		"GOROOT":      {ctxt.GOROOT, orig.GOROOT},
		"GOPATH":      {ctxt.GOPATH, orig.GOPATH},
		"Dir":         {ctxt.Dir, orig.Dir},
		"CgoEnabled":  {ctxt.CgoEnabled, orig.CgoEnabled},
		"UseAllFiles": {ctxt.UseAllFiles, orig.UseAllFiles},
		"Compiler":    {ctxt.Compiler, orig.Compiler},
		"BuildTags":   {ctxt.BuildTags, orig.BuildTags},
		"ToolTags":    {ctxt.ToolTags, orig.ToolTags},
		"ReleaseTags": {ctxt.ReleaseTags, orig.ReleaseTags},
	}
	for name, val := range fields {
		if !reflect.DeepEqual(val.got, val.want) {
			t.Errorf("%s: got: %v want: %v", name, val.got, val.want)
		}
	}

	// Make sure append does not overwrite the neighboring slice
	ctxt.BuildTags = append(ctxt.BuildTags, "a")
	ctxt.BuildTags = ctxt.BuildTags[:len(ctxt.BuildTags)-1]
	ctxt.ToolTags = append(ctxt.ToolTags, "a")
	ctxt.ToolTags = ctxt.ToolTags[:len(ctxt.ToolTags)-1]
	ctxt.ReleaseTags = append(ctxt.ReleaseTags, "a")
	ctxt.ReleaseTags = ctxt.ReleaseTags[:len(ctxt.ReleaseTags)-1]
	for name, val := range fields {
		if !reflect.DeepEqual(val.got, val.want) {
			t.Errorf("%s: got: %v want: %v", name, val.got, val.want)
		}
	}

	// Test that we made a copy by modifying the original
	for i, s := range orig.BuildTags {
		orig.BuildTags[i] = strings.ToUpper(s)
	}
	for i, s := range orig.ToolTags {
		orig.ToolTags[i] = strings.ToUpper(s)
	}
	for i, s := range orig.ReleaseTags {
		orig.ReleaseTags[i] = strings.ToUpper(s)
	}
	if reflect.DeepEqual(ctxt.BuildTags, orig.BuildTags) {
		t.Errorf("failed to copy BuildTags %q", ctxt.BuildTags)
	}
	if reflect.DeepEqual(ctxt.ToolTags, orig.ToolTags) {
		t.Errorf("failed to copy ToolTags %q", ctxt.ToolTags)
	}
	if reflect.DeepEqual(ctxt.ReleaseTags, orig.ReleaseTags) {
		t.Errorf("failed to copy ReleaseTags %q", ctxt.ReleaseTags)
	}
}

func BenchmarkStringsSame_Long(b *testing.B) {
	a := make([]string, 0, 26)
	for i := rune('a'); i <= 'z'; i++ {
		a = append(a, strings.Repeat(string(i), 4))
	}
	a1 := make([]string, 0, len(a)*4)
	for i := 0; i < 4; i++ {
		a1 = append(a1, a...)
	}
	a2 := make([]string, len(a1))
	copy(a2, a1)
	rand.Seed(1234)
	rand.Shuffle(len(a1), func(i, j int) {
		a1[i], a1[j] = a1[j], a1[i]
	})
	rand.Shuffle(len(a2), func(i, j int) {
		a2[i], a2[j] = a2[j], a2[i]
	})
	b.ResetTimer()

	for n := 10; n <= 100; n += 10 {
		b.Run(fmt.Sprintf("%d", n), func(b *testing.B) {
			a1 := a1[:n]
			a2 := a2[:n]
			for i := 0; i < b.N; i++ {
				StringsSame(a1, a2)
			}
		})
	}
}

func BenchmarkCopyContext(b *testing.B) {
	ctxt := build.Default
	ctxt.BuildTags = []string{"test"}
	ctxt.ToolTags = []string{
		"goexperiment.regabiwrappers",
		"goexperiment.regabireflect",
		"goexperiment.regabiargs",
		"goexperiment.pacerredesign",
	}
	ctxt.ReleaseTags = []string{
		"go1.1", "go1.2", "go1.3", "go1.4", "go1.5", "go1.6", "go1.7",
		"go1.8", "go1.9", "go1.10", "go1.11", "go1.12", "go1.13", "go1.14",
		"go1.15", "go1.16", "go1.17", "go1.18",
	}
	for i := 0; i < b.N; i++ {
		CopyContext(&ctxt)
	}
}
