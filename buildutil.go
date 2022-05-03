// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package buildutil exposes some useful internal functions of the go/build.
package buildutil

import (
	"bytes"
	"errors"
	"fmt"
	"go/build"
	"go/build/constraint"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// BuildTags adds and build tags found in name or content to allTags.
func BuildTags(name string, content []byte, allTags map[string]bool) {
	ctxt := build.Default
	goodOSArchFile(&ctxt, filepath.Base(name), allTags)
	shouldBuild(&ctxt, content, allTags)
}

// GoodOSArchFile returns false if the name contains a $GOOS or $GOARCH
// suffix which does not match the build Context.
// The recognized name formats are:
//
//     name_$(GOOS).*
//     name_$(GOARCH).*
//     name_$(GOOS)_$(GOARCH).*
//     name_$(GOOS)_test.*
//     name_$(GOARCH)_test.*
//     name_$(GOOS)_$(GOARCH)_test.*
//
// An exception: if GOOS=android, then files with GOOS=linux are also matched.
func GoodOSArchFile(ctxt *build.Context, name string, allTags map[string]bool) bool {
	return goodOSArchFile(ctxt, filepath.Base(name), allTags)
}

// ShouldBuild reports whether it is okay to use this file, and adds any build
// tags to allTags.
//
// Note: only +build tags are checked.  Syntactically incorrect content may be
// marked as build-able if no +build tags are present.
func ShouldBuild(ctxt *build.Context, content []byte, allTags map[string]bool) bool {
	return shouldBuildOnly(ctxt, content, allTags)
}

func Include(ctxt *build.Context, path string) bool {
	if !goodOSArchFile(ctxt, filepath.Base(path), nil) {
		return false
	}
	var f io.ReadCloser
	var err error
	if fn := ctxt.OpenFile; fn != nil {
		f, err = fn(path)
	} else {
		f, err = os.Open(path)
	}
	if err != nil {
		return false
	}
	data, err := readImportsFast(f)
	f.Close()
	if err != nil {
		return false
	}
	return shouldBuildOnly(ctxt, data, nil)
}

func IncludeTags(ctxt *build.Context, path string, tags map[string]bool) (bool, error) {
	if !goodOSArchFile(ctxt, filepath.Base(path), tags) {
		return false, nil
	}
	var f io.ReadCloser
	var err error
	if fn := ctxt.OpenFile; fn != nil {
		f, err = fn(path)
	} else {
		f, err = os.Open(path)
	}
	if err != nil {
		return false, err
	}
	data, err := readImportsFast(f)
	f.Close()
	if err != nil {
		return false, err
	}
	return shouldBuildOnly(ctxt, data, tags), nil
}

// TODO (CEV): rename
func ShortImport(ctxt *build.Context, path string) (string, bool) {
	if !goodOSArchFile(ctxt, filepath.Base(path), nil) {
		return "", false
	}
	var f io.ReadCloser
	var err error
	if fn := ctxt.OpenFile; fn != nil {
		f, err = fn(path)
	} else {
		f, err = os.Open(path)
	}
	if err != nil {
		return "", false
	}
	data, err := readImportsFast(f)
	f.Close()
	if err != nil {
		return "", false
	}
	if !shouldBuildOnly(ctxt, data, nil) {
		return "", false
	}
	name, err := readPackageName(data)
	return name, err == nil
}

func ReadPackageName(path string, src interface{}) (string, error) {
	rc, err := openReader(&build.Default, path, src)
	if err != nil {
		return "", err
	}
	data, err := readImportsFast(rc)
	rc.Close()
	if err != nil {
		return "", err
	}
	return readPackageName(data)
}

// ReadPackageNameTags evaluates the Go source file at path and returns
// the package name, if it can be used with build.Context ctxt, populates
// any build tags (if tags is not nil), and any error that occured.
func ReadPackageNameTags(path string, src interface{}, tags map[string]bool) (string, bool, error) {
	rc, err := openReader(&build.Default, path, src)
	if err != nil {
		return "", false, err
	}
	data, err := readImportsFast(rc)
	if err != nil {
		return "", false, err
	}
	name, err := readPackageName(data)
	if err != nil {
		return "", false, err
	}
	return name, shouldBuildOnly(&build.Default, data, tags), nil
}

func ReadImports(path string, src interface{}) (pkgname string, imports []string, err error) {
	rc, err := openReader(&build.Default, path, src)
	if err != nil {
		return
	}
	imports = make([]string, 0, 8)
	data, err := readImports(rc, true, &imports)
	if err != nil {
		return
	}
	pkgname, err = readPackageName(data)
	return
}

func openReader(ctxt *build.Context, filename string, src interface{}) (io.ReadCloser, error) {
	if ctxt.OpenFile != nil {
		return ctxt.OpenFile(filename)
	}
	if src != nil {
		switch s := src.(type) {
		case string:
			return ioutil.NopCloser(strings.NewReader(s)), nil
		case []byte:
			return ioutil.NopCloser(bytes.NewReader(s)), nil
		case io.ReadCloser:
			return s, nil
		case io.Reader:
			return ioutil.NopCloser(s), nil
		default:
			return nil, errors.New("invalid source")
		}
	}
	return os.Open(filename)
}

var (
	slashSlash = []byte("//")
	slashStar  = []byte("/*")
	starSlash  = []byte("*/")
)

var (
	bSlashSlash = []byte(slashSlash)
	bSlashStar  = []byte(slashStar)
	bPlusBuild  = []byte("+build")

	goBuildComment = []byte("//go:build")

	errMultipleGoBuild = errors.New("multiple //go:build comments")
)

func isGoBuildComment(line []byte) bool {
	if !bytes.HasPrefix(line, goBuildComment) {
		return false
	}
	line = bytes.TrimSpace(line)
	rest := line[len(goBuildComment):]
	return len(rest) == 0 || len(bytes.TrimSpace(rest)) < len(rest)
}

// Special comment denoting a binary-only package.
// See https://golang.org/design/2775-binary-only-packages
// for more about the design of binary-only packages.
var binaryOnlyComment = []byte("//go:binary-only-package")

// shouldBuild reports whether it is okay to use this file,
// The rule is that in the file's leading run of // comments
// and blank lines, which must be followed by a blank line
// (to avoid including a Go package clause doc comment),
// lines beginning with '// +build' are taken as build directives.
//
// The file is accepted only if each such line lists something
// matching the file. For example:
//
//	// +build windows linux
//
// marks the file as applicable only on Windows and Linux.
//
// For each build tag it consults, shouldBuild sets allTags[tag] = true.
//
// shouldBuild reports whether the file should be built
// and whether a //go:binary-only-package comment was found.
func shouldBuild(ctxt *build.Context, content []byte, allTags map[string]bool) (shouldBuild, binaryOnly bool, err error) {
	// Identify leading run of // comments and blank lines,
	// which must be followed by a blank line.
	// Also identify any //go:build comments.
	content, goBuild, sawBinaryOnly, err := parseFileHeader(content)
	if err != nil {
		return false, false, err
	}

	// If //go:build line is present, it controls.
	// Otherwise fall back to +build processing.
	switch {
	case goBuild != nil:
		x, err := constraint.Parse(string(goBuild))
		if err != nil {
			return false, false, fmt.Errorf("parsing //go:build line: %v", err)
		}
		shouldBuild = eval(ctxt, x, allTags)

	default:
		shouldBuild = true
		p := content
		for len(p) > 0 {
			line := p
			if i := bytes.IndexByte(line, '\n'); i >= 0 {
				line, p = line[:i], p[i+1:]
			} else {
				p = p[len(p):]
			}
			line = bytes.TrimSpace(line)
			if !bytes.HasPrefix(line, bSlashSlash) || !bytes.Contains(line, bPlusBuild) {
				continue
			}
			text := string(line)
			if !constraint.IsPlusBuild(text) {
				continue
			}
			if x, err := constraint.Parse(text); err == nil {
				if !eval(ctxt, x, allTags) {
					shouldBuild = false
				}
			}
		}
	}

	return shouldBuild, sawBinaryOnly, nil
}

// TODO: move to minimize diff with go/build.go
func parseBuildConstraint(content []byte) (constraint.Expr, error) {
	// Identify leading run of // comments and blank lines,
	// which must be followed by a blank line.
	// Also identify any //go:build comments.
	content, goBuild, _, err := parseFileHeader(content)
	if err != nil {
		return nil, err
	}

	// If //go:build line is present, it controls.
	// Otherwise fall back to +build processing.
	if goBuild != nil {
		x, err := constraint.Parse(string(goBuild))
		if err != nil {
			return nil, fmt.Errorf("parsing //go:build line: %w", err)
		}
		return x, nil
	}

	// Synthesize //go:build expression from // +build lines.
	var x constraint.Expr
	p := content
	for len(p) > 0 {
		line := p
		if i := bytes.IndexByte(line, '\n'); i >= 0 {
			line, p = line[:i], p[i+1:]
		} else {
			p = p[len(p):]
		}
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, bSlashSlash) || !bytes.Contains(line, bPlusBuild) {
			continue
		}
		text := string(line)
		if !constraint.IsPlusBuild(text) {
			continue
		}
		y, err := constraint.Parse(text)
		if err != nil {
			return nil, err
		}
		if x == nil {
			x = y
		} else {
			x = &constraint.AndExpr{X: x, Y: y}
		}
	}

	// WARN: x may be nil
	return x, nil
}

// TODO: move to minimize diff with go/build.go
func shouldBuildOnly(ctxt *build.Context, content []byte, allTags map[string]bool) bool {
	ok, _, _ := shouldBuild(ctxt, content, allTags)
	return ok
}

func parseFileHeader(content []byte) (trimmed, goBuild []byte, sawBinaryOnly bool, err error) {
	end := 0
	p := content
	ended := false       // found non-blank, non-// line, so stopped accepting // +build lines
	inSlashStar := false // in /* */ comment

Lines:
	for len(p) > 0 {
		line := p
		if i := bytes.IndexByte(line, '\n'); i >= 0 {
			line, p = line[:i], p[i+1:]
		} else {
			p = p[len(p):]
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 && !ended { // Blank line
			// Remember position of most recent blank line.
			// When we find the first non-blank, non-// line,
			// this "end" position marks the latest file position
			// where a // +build line can appear.
			// (It must appear _before_ a blank line before the non-blank, non-// line.
			// Yes, that's confusing, which is part of why we moved to //go:build lines.)
			// Note that ended==false here means that inSlashStar==false,
			// since seeing a /* would have set ended==true.
			end = len(content) - len(p)
			continue Lines
		}
		if !bytes.HasPrefix(line, slashSlash) { // Not comment line
			ended = true
		}

		if !inSlashStar && isGoBuildComment(line) {
			if goBuild != nil {
				return nil, nil, false, errMultipleGoBuild
			}
			goBuild = line
		}
		if !inSlashStar && bytes.Equal(line, binaryOnlyComment) {
			sawBinaryOnly = true
		}

	Comments:
		for len(line) > 0 {
			if inSlashStar {
				if i := bytes.Index(line, starSlash); i >= 0 {
					inSlashStar = false
					line = bytes.TrimSpace(line[i+len(starSlash):])
					continue Comments
				}
				continue Lines
			}
			if bytes.HasPrefix(line, bSlashSlash) {
				continue Lines
			}
			if bytes.HasPrefix(line, bSlashStar) {
				inSlashStar = true
				line = bytes.TrimSpace(line[len(bSlashStar):])
				continue Comments
			}
			// Found non-comment text.
			break Lines
		}
	}

	return content[:end], goBuild, sawBinaryOnly, nil
}

func eval(ctxt *build.Context, x constraint.Expr, allTags map[string]bool) bool {
	return x.Eval(func(tag string) bool { return matchTag(ctxt, tag, allTags) })
}

// Used by MatchContext
var compatibleOSes = map[string][]string{
	"android": {"linux"},
	"illumos": {"solaris"},
	"ios":     {"darwin"},
}

// matchTag reports whether the name is one of:
//
//	cgo (if cgo is enabled)
//	$GOOS
//	$GOARCH
//	ctxt.Compiler
//	linux (if GOOS = android)
//	solaris (if GOOS = illumos)
//	tag (if tag is listed in ctxt.BuildTags or ctxt.ReleaseTags)
//
// It records all consulted tags in allTags.
func matchTag(ctxt *build.Context, name string, allTags map[string]bool) bool {
	if allTags != nil {
		allTags[name] = true
	}

	// special tags
	if ctxt.CgoEnabled && name == "cgo" {
		return true
	}
	if name == ctxt.GOOS || name == ctxt.GOARCH || name == ctxt.Compiler {
		return true
	}
	if ctxt.GOOS == "android" && name == "linux" {
		return true
	}
	if ctxt.GOOS == "illumos" && name == "solaris" {
		return true
	}
	if ctxt.GOOS == "ios" && name == "darwin" {
		return true
	}

	// other tags
	for _, tag := range ctxt.BuildTags {
		if tag == name {
			return true
		}
	}
	for _, tag := range ctxt.ToolTags {
		if tag == name {
			return true
		}
	}
	for _, tag := range ctxt.ReleaseTags {
		if tag == name {
			return true
		}
	}

	return false
}

func inTestdata(sub string) bool {
	return strings.Contains(sub, "/testdata/") || strings.HasSuffix(sub, "/testdata") ||
		strings.HasPrefix(sub, "testdata/") || sub == "testdata"
}

// return ctxt.Import(".", dir, mode)
func ImportPath(ctxt *build.Context, dir string) (string, error) {
	if dir == "" {
		return "", errors.New("empty source dir")
	}
	if !isDir(ctxt, dir) {
		return ".", errors.New("cannot find package \".\" in:\n\t" + dir)
	}
	importPath := "."
	if !strings.HasPrefix(dir, ctxt.GOROOT) {
		all := gopath(ctxt)
		for i, root := range all {
			rootsrc := joinPath(ctxt, root, "src")
			if sub, ok := hasSubdirCtxt(ctxt, rootsrc, dir); ok && !inTestdata(sub) {
				// We found a potential import path for dir,
				// but check that using it wouldn't find something
				// else first.
				if ctxt.GOROOT != "" {
					if dir := joinPath(ctxt, ctxt.GOROOT, "src", sub); isDir(ctxt, dir) {
						// go/build records a conflict here
						goto Found
					}
				}
				for _, earlyRoot := range all[:i] {
					if dir := joinPath(ctxt, earlyRoot, "src", sub); isDir(ctxt, dir) {
						// go/build records a conflict here
						goto Found
					}
				}
				// sub would not name some other directory instead of this one.
				// Record it.
				importPath = sub
				goto Found
			}
		}
	}
	if ctxt.GOROOT != "" {
		root := joinPath(ctxt, ctxt.GOROOT, "src")
		if sub, ok := hasSubdirCtxt(ctxt, root, dir); ok && !inTestdata(sub) {
			importPath = sub
			goto Found
		}
	}

Found:
	return importPath, nil
}

// joinPath calls ctxt.JoinPath (if not nil) or else filepath.Join.
func joinPath(ctxt *build.Context, elem ...string) string {
	if f := ctxt.JoinPath; f != nil {
		return f(elem...)
	}
	return filepath.Join(elem...)
}

// splitPathList calls ctxt.SplitPathList (if not nil) or else filepath.SplitList.
func splitPathList(ctxt *build.Context, s string) []string {
	if f := ctxt.SplitPathList; f != nil {
		return f(s)
	}
	return filepath.SplitList(s)
}

// isDir calls ctxt.IsDir (if not nil) or else uses os.Stat.
func isDir(ctxt *build.Context, path string) bool {
	if f := ctxt.IsDir; f != nil {
		return f(path)
	}
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// hasSubdirCtxt calls ctxt.HasSubdir (if not nil) or else uses
// the local file system to answer the question.
func hasSubdirCtxt(ctxt *build.Context, root, dir string) (rel string, ok bool) {
	if f := ctxt.HasSubdir; f != nil {
		return f(root, dir)
	}

	// Try using paths we received.
	if rel, ok = hasSubdir(root, dir); ok {
		return
	}

	// Try expanding symlinks and comparing
	// expanded against unexpanded and
	// expanded against expanded.
	rootSym, _ := filepath.EvalSymlinks(root)
	dirSym, _ := filepath.EvalSymlinks(dir)

	if rel, ok = hasSubdir(rootSym, dir); ok {
		return
	}
	if rel, ok = hasSubdir(root, dirSym); ok {
		return
	}
	return hasSubdir(rootSym, dirSym)
}

// isSubdir reports if dir is within root by performing lexical analysis only.
func isSubdir(root, dir string) bool {
	n := len(root)
	return 0 < n && n < len(dir) && dir[0:n] == root && os.IsPathSeparator(dir[n])
}

// hasSubdir reports if dir is within root by performing lexical analysis only.
//
// NOTE: this is a faster alloc free version of: go/build.hasSubdir
func hasSubdir(root, dir string) (rel string, ok bool) {
	if isSubdir(root, dir) {
		return filepath.ToSlash(dir[len(root)+1:]), true
	}
	return "", false
}

// gopath returns the list of Go path directories.
func gopath(ctxt *build.Context) []string {
	var all []string
	for _, p := range splitPathList(ctxt, ctxt.GOPATH) {
		if p == "" || p == ctxt.GOROOT {
			// Empty paths are uninteresting.
			// If the path is the GOROOT, ignore it.
			// People sometimes set GOPATH=$GOROOT.
			// Do not get confused by this common mistake.
			continue
		}
		if strings.HasPrefix(p, "~") {
			// Path segments starting with ~ on Unix are almost always
			// users who have incorrectly quoted ~ while setting GOPATH,
			// preventing it from expanding to $HOME.
			// The situation is made more confusing by the fact that
			// bash allows quoted ~ in $PATH (most shells do not).
			// Do not get confused by this, and do not try to use the path.
			// It does not exist, and printing errors about it confuses
			// those users even more, because they think "sure ~ exists!".
			// The go command diagnoses this situation and prints a
			// useful error.
			// On Windows, ~ is used in short names, such as c:\progra~1
			// for c:\program files.
			continue
		}
		all = append(all, p)
	}
	return all
}

// KnownOSList returns the known operating system values, sorted.
func KnownOSList() []string {
	s := make([]string, len(knownOSList))
	copy(s, knownOSList)
	return s
}

// KnownArchList returns the known architecture values, sorted.
func KnownArchList() []string {
	s := make([]string, len(knownArchList))
	copy(s, knownArchList)
	return s
}

func parseFields(fields string) (map[string]bool, []string) {
	a := strings.Fields(fields)
	sort.Strings(a)

	m := make(map[string]bool, len(a))
	for _, s := range a {
		m[s] = true
	}
	return m, a
}

var (
	knownOS, knownOSList     = parseFields(goosList)
	knownArch, knownArchList = parseFields(goarchList)
	knownReleaseTag          = func() map[string]bool {
		m := make(map[string]bool, len(build.Default.ReleaseTags))
		for _, v := range build.Default.ReleaseTags {
			m[v] = true
		}
		return m
	}()
)
