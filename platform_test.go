package buildutil

import (
	"reflect"
	"testing"
)

func TestDefaultGoPlatforms(t *testing.T) {
	platforms, err := LoadGoPlatforms()
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[GoPlatform]bool)
	for _, p := range DefaultGoPlatforms {
		got[p] = true
	}
	want := make(map[GoPlatform]bool)
	for _, p := range platforms {
		want[p] = true
		if !got[p] {
			t.Errorf("missing: %+v", p)
		}
	}
	for p := range got {
		if !want[p] {
			t.Errorf("extra: %+v", p)
		}
	}
}

func TestCgoEnabledMap(t *testing.T) {
	want := make(map[string]bool)
	for _, p := range DefaultGoPlatforms {
		if p.CgoSupported {
			want[p.GOOS+"/"+p.GOARCH] = true
		}
	}
	if !reflect.DeepEqual(cgoEnabled, want) {
		t.Errorf("cgoEnabled got: %+v want: %+v", cgoEnabled, want)
	}
}
