package buildutil

import (
	"reflect"
	"sort"
	"testing"
)

func TestKnownOSList(t *testing.T) {
	if !sort.StringsAreSorted(knownOSList) {
		t.Errorf("knownOSList should be sorted: %q", knownOSList)
	}
	want := make([]string, 0, len(knownOS))
	for s := range knownOS {
		want = append(want, s)
	}
	sort.Strings(want)
	if !reflect.DeepEqual(want, knownOSList) {
		t.Errorf("knownOSList = %q; want: %q", knownOSList, want)
	}
}

func TestKnownArchList(t *testing.T) {
	if !sort.StringsAreSorted(knownArchList) {
		t.Errorf("knownArchList should be sorted: %q", knownArchList)
	}
	want := make([]string, 0, len(knownArch))
	for s := range knownArch {
		want = append(want, s)
	}
	sort.Strings(want)
	if !reflect.DeepEqual(want, knownArchList) {
		t.Errorf("knownArchList = %q; want: %q", knownArchList, want)
	}
}
