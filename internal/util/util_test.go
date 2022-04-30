package util

import (
	"fmt"
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
