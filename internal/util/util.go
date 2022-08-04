package util

import (
	"go/build"
	"os"
	"strings"
)

func DuplicateStrings(a []string) []string {
	if len(a) == 0 {
		return nil
	}
	s := make([]string, len(a))
	copy(s, a)
	return s
}

// func DuplicateStringsInPlace(strs ...*[]string) {
// 	if len(strs) == 0 {
// 		return
// 	}
// 	n := 0
// 	for _, a := range strs {
// 		n += len(*a)
// 	}
// 	all := make([]string, n)
// 	n = 0
// 	for i, a := range strs {
// 		n := len(*a)
// 		if n == 0 {
// 			*a = nil
// 			continue
// 		}
// 		dupe := all[0:n:n]
// 		copy(dupe, *a)
// 		*strs[i] = dupe
// 		all = all[n:]
// 	}
// }

func StringsContains(a []string, val string) bool {
	for _, s := range a {
		if s == val {
			return true
		}
	}
	return false
}

func StringsAppend(a []string, val string) []string {
	if !StringsContains(a, val) {
		return append(a, val)
	}
	return a
}

func StringsRemoveAll(a []string, val string) []string {
	v := a[:0]
	for _, s := range a {
		if s != val {
			v = append(v, s)
		}
	}
	return v
}

// StringsSame returns true if the string slices contain the same
// elements ignoring order.
func StringsSame(a1, a2 []string) bool {
	if len(a1) != len(a2) {
		return false
	}
	// brute force search for small string slices
	if len(a1) <= 64 {
	Loop:
		for _, s1 := range a1 {
			for _, s2 := range a2 {
				if s1 == s2 {
					continue Loop
				}
			}
			return false
		}
		return true
	}

	want := make(map[string]int, len(a1))
	for _, s := range a1 {
		want[s]++
	}
	for _, s := range a2 {
		n := want[s]
		if n <= 0 {
			return false
		}
		want[s] = n - 1
	}
	return true
}

func TagsIntersect(m1, m2 map[string]bool) bool {
	for k := range m1 {
		if _, ok := m2[k]; ok {
			return true
		}
	}
	return false
}

type Environ struct {
	env []string
}

func NewEnviron() *Environ { return &Environ{env: os.Environ()} }

func (e *Environ) Environ() []string { return e.env }

func (e *Environ) Index(key string) int {
	n := len(key)
	if n <= 0 {
		return -1
	}
	ch := key[0]
	for i, s := range e.env {
		if len(s) == 0 || s[0] != ch {
			continue
		}
		// Checking len(s) twice is required for bounds-check-elimination
		if len(s) > n && s[0:n] == key && n < len(s) /* BCE */ && s[n] == '=' {
			return i
		}
	}
	return -1
}

func (e *Environ) Lookup(key string) (value string, found bool) {
	if i := e.Index(key); i >= 0 {
		s := e.env[i]
		if j := strings.IndexByte(s, '='); j >= 0 {
			s = s[j+1:]
		}
		return s, true
	}
	return "", false
}

func (e *Environ) Set(key, value string) {
	if i := e.Index(key); i != -1 {
		e.env[i] = key + "=" + value
	} else {
		e.env = append(e.env, key+"="+value)
	}
}

func CopyContext(orig *build.Context) *build.Context {
	tmp := *orig // make a copy
	ctxt := &tmp

	// Use one backing string slice
	n := len(ctxt.BuildTags) + len(ctxt.ToolTags) + len(ctxt.ReleaseTags)
	a := make([]string, n)
	if n := len(ctxt.BuildTags); n != 0 {
		copy(a, ctxt.BuildTags)
		ctxt.BuildTags = a[0:n:n]
		a = a[n:]
	} else {
		ctxt.BuildTags = nil
	}
	if n := len(ctxt.ToolTags); n != 0 {
		copy(a, ctxt.ToolTags)
		ctxt.ToolTags = a[0:n:n]
		a = a[n:]
	} else {
		ctxt.ToolTags = nil
	}
	if n := len(ctxt.ReleaseTags); n != 0 {
		copy(a, ctxt.ReleaseTags)
		ctxt.ReleaseTags = a[0:n:n]
		a = a[n:]
	} else {
		ctxt.ReleaseTags = nil
	}

	return ctxt
}

// type stringSlice struct {
// 	a []string
// }

// func (s *stringSlice) Len() int           { return len(s.a) }
// func (s *stringSlice) Less(i, j int) bool { return s.a[i] < s.a[j] }
// func (s *stringSlice) Swap(i, j int)      { s.a[i], s.a[j] = s.a[j], s.a[i] }

// func isSorted(a []string) bool {
// 	for i := 0; i < len(a)-1; i++ {
// 		if a[i] > a[i+1] {
// 			return false
// 		}
// 	}
// 	return true
// }
