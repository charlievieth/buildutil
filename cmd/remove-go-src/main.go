//go:build never

package main

import (
	"bufio"
	"flag"
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charlievieth/buildutil"
)

var goBuildRe = regexp.MustCompile(`(?m)^//(go:build|\s+\+build)\s+[[:print:]]+`)
var osArchRe *regexp.Regexp

func init() {
	log.SetFlags(log.Lshortfile)

	join := func(a []string) string {
		for i := range a {
			a[i] = regexp.QuoteMeta(a[i])
		}
		return strings.Join(a, "|")
	}
	pattern := fmt.Sprintf("_(%s)(_(%s))?\\.go$",
		join(buildutil.KnownOSList()),
		join(buildutil.KnownArchList()))
	osArchRe = regexp.MustCompile(pattern)
}

func hasBuildDirective(g *ast.CommentGroup) bool {
	if g == nil {
		return false
	}
	for _, c := range g.List {
		if constraint.IsGoBuild(c.Text) || constraint.IsPlusBuild(c.Text) {
			return true
		}
	}
	return false
}

func copyFile(from, to string) error {
	if err := os.MkdirAll(filepath.Dir(to), 0755); err != nil {
		return err
	}
	fo, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	exit := func(err error) error {
		fo.Close()
		os.Remove(to)
		return err
	}

	fset := token.NewFileSet()
	af, err := parser.ParseFile(fset, from, nil, parser.PackageClauseOnly|parser.ParseComments)
	if err != nil {
		return exit(err)
	}
	// Remove non-build directive comments
	if len(af.Comments) != 0 {
		a := af.Comments[:0]
		for _, g := range af.Comments {
			if hasBuildDirective(g) {
				a = append(a, g)
			}
		}
		af.Comments = a
	}
	if err := format.Node(fo, fset, af); err != nil {
		return exit(err)
	}
	if err := fo.Close(); err != nil {
		return exit(err)
	}
	return nil
}

func includeFile(name string) bool {
	if filepath.Ext(name) != ".go" {
		return false
	}
	if osArchRe.MatchString(filepath.Base(name)) {
		return true
	}
	f, err := os.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()
	return goBuildRe.MatchReader(bufio.NewReader(f))
}

func main() {
	fromFlag := flag.String("from", "", "copy Go files from this directory")
	toFlag := flag.String("to", "", "copy Go files to this directory")
	verbose := flag.Bool("v", false, "verbose output")
	flag.Parse()

	if *fromFlag == "" {
		log.Fatal("missing required argument: from")
	}
	if *toFlag == "" {
		log.Fatal("missing required argument: to")
	}
	from, err := filepath.Abs(*fromFlag)
	if err != nil {
		log.Fatal(err)
	}
	to, err := filepath.Abs(*toFlag)
	if err != nil {
		log.Fatal(err)
	}
	if _, err := os.Stat(to); err == nil {
		log.Fatal("refusing to overwrite destination directory: " + to)
	}

	err = filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() && filepath.Ext(path) == ".go" {
			rel, err := filepath.Rel(from, path)
			if err != nil {
				return err
			}
			if includeFile(path) {
				if *verbose {
					fmt.Fprintf(os.Stderr, "copying:  %s\n", rel)
				}
				if err := copyFile(path, filepath.Join(to, rel)); err != nil {
					return err
				}
			} else if *verbose {
				fmt.Fprintf(os.Stderr, "ignoring: %s\n", rel)
			}
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
}

/*
func archiveFile(w *tar.Writer, from, to string) error {
	f, err := os.Open(from)
	if err != nil {
		return err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return err
	}
	hdr, err := tar.FileInfoHeader(fi, "")
	if err != nil {
		return err
	}

	fset := token.NewFileSet()
	af, err := parser.ParseFile(fset, from, f, parser.PackageClauseOnly|parser.ParseComments)
	if err != nil {
		return err
	}
	// Remove non-build directive comments
	if len(af.Comments) != 0 {
		a := af.Comments[:0]
		for _, g := range af.Comments {
			if hasBuildDirective(g) {
				a = append(a, g)
			}
		}
		af.Comments = a
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, af); err != nil {
		return err
	}

	hdr.Size = int64(buf.Len())
	if err := w.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := buf.WriteTo(w); err != nil {
		return err
	}
	return nil
}
*/
