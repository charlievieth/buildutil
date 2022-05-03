//go:build gen_platform_list
// +build gen_platform_list

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
)

func init() {
	log.SetFlags(log.Lshortfile)
}

type GoPlatform struct {
	GOOS         string `json:"GOOS"`
	GOARCH       string `json:"GOARCH"`
	CgoSupported bool   `json:"CgoSupported"`
	FirstClass   bool   `json:"FirstClass"`
}

func loadGoPlatforms() []GoPlatform {
	// TODO: do we need to handle the PATH changing or multiple
	// versions of the go cmd?
	cmd := exec.Command("go", "tool", "dist", "list", "-json")
	cmd.Stderr = os.Stderr
	data, err := cmd.Output()
	if err != nil {
		log.Fatal(err)
	}
	var ps []GoPlatform
	if err := json.Unmarshal(data, &ps); err != nil {
		log.Fatal(err)
	}
	return ps
}

// Sort the platforms so that "first class" platforms are first and then
// sort the "first class" platforms so that the "amd64" and "arm64" ones
// are listed first.
func sortPlatforms(platforms []GoPlatform) []GoPlatform {
	ps := make([]GoPlatform, len(platforms))
	copy(ps, platforms)
	sort.SliceStable(ps, func(i, j int) bool {
		p1 := &ps[i]
		p2 := &ps[j]
		if p1.FirstClass {
			if !p2.FirstClass {
				return true
			}
			return p2.GOARCH == "386" || p2.GOARCH == "arm"
		}
		return false
	})
	return ps
}

func main() {
	pkgName := flag.String("pkg", "buildutil", "package name")
	outFile := flag.String("out", "platform_list.go", "output file name")
	flag.Parse()

	fmt.Fprintln(os.Stderr, "WARN: this does not list all supported OSes and Arches (e.g. loong64 and mips64p32le)")

	platforms := loadGoPlatforms()
	w := &bytes.Buffer{}

	fmt.Fprintf(w, "// Code generated by %s; DO NOT EDIT.\n", filepath.Base(os.Args[0]))
	fmt.Fprintf(w, "// go version: %s\n", runtime.Version())
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "package ", *pkgName)
	fmt.Fprintln(w, "")

	firstClass := true
	fmt.Fprintln(w, "// DefaultGoPlatforms are the default supported Go platforms")
	fmt.Fprintln(w, "// and are ordered by preference and \"first class\" support.")
	fmt.Fprintln(w, "var DefaultGoPlatforms = []GoPlatform{")
	fmt.Fprintln(w, "\t// first class platforms")
	// Print in preferred order
	for _, p := range sortPlatforms(platforms) {
		if firstClass && !p.FirstClass {
			fmt.Fprintln(w, "")
			fmt.Fprintln(w, "\t// second class platforms")
			firstClass = false
		}
		fmt.Fprintf(w, "\t{%q, %q, %t, %t},\n", p.GOOS, p.GOARCH, p.CgoSupported, p.FirstClass)
	}
	fmt.Fprintln(w, "}")
	fmt.Fprintln(w, "")

	fmt.Fprintln(w, "var cgoEnabled = map[string]bool{")
	for _, p := range platforms {
		if p.CgoSupported {
			fmt.Fprintf(w, "\t%q: %t,\n", p.GOOS+"/"+p.GOARCH, p.CgoSupported)
		}
	}
	fmt.Fprintln(w, "}")

	var (
		oses            []string
		arches          []string
		supportedOSArch = make(map[string]map[string]bool)
		seenArches      = make(map[string]bool)
	)
	for _, p := range platforms {
		if supportedOSArch[p.GOOS] == nil {
			supportedOSArch[p.GOOS] = make(map[string]bool)
			oses = append(oses, p.GOOS)
		}
		supportedOSArch[p.GOOS][p.GOARCH] = true
		if !seenArches[p.GOARCH] {
			seenArches[p.GOARCH] = true
			arches = append(arches, p.GOARCH)
		}
	}
	sort.Strings(oses)
	sort.Strings(arches)

	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "var supportedPlatformsOsArch = map[string]map[string]bool{")
	for _, os := range oses {
		fmt.Fprintf(w, "\t%q: {\n", os)
		for _, arch := range arches {
			if supportedOSArch[os][arch] {
				fmt.Fprintf(w, "\t\t%q: %t,\n", arch, true)
			}
		}
		fmt.Fprintln(w, "\t},")
	}
	fmt.Fprintln(w, "}")
	fmt.Fprintln(w, "")

	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "var supportedPlatformsArchOs = map[string]map[string]bool{")
	for _, arch := range arches {
		fmt.Fprintf(w, "\t%q: {\n", arch)
		for _, os := range oses {
			if supportedOSArch[os][arch] {
				fmt.Fprintf(w, "\t\t%q: %t,\n", os, true)
			}
		}
		fmt.Fprintln(w, "\t},")
	}
	fmt.Fprintln(w, "}")
	fmt.Fprintln(w, "")

	source, err := format.Source(w.Bytes())
	if err != nil {
		log.Fatal(err)
	}
	if *outFile == "-" {
		if _, err := os.Stdout.Write(source); err != nil {
			log.Fatal(err)
		}
		return
	}
	dir, name := filepath.Split(*outFile)
	f, err := os.CreateTemp(dir, name+".*")
	if err != nil {
		log.Fatal(err)
	}
	tmpname := f.Name()
	exit := func(err error) error {
		f.Close()
		os.Remove(tmpname)
		return err
	}
	if err := f.Chmod(0644); err != nil {
		log.Fatal(exit(err))
	}
	if _, err := f.Write(source); err != nil {
		log.Fatal(exit(err))
	}
	if err := f.Close(); err != nil {
		log.Fatal(exit(err))
	}
	if err := os.Rename(tmpname, *outFile); err != nil {
		log.Fatal(exit(err))
	}
}