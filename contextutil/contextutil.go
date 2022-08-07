package contextutil

import (
	"errors"
	"fmt"
	"go/build"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/charlievieth/buildutil/internal/readdir"
	"golang.org/x/tools/go/buildutil"
)

// DefaultProjectTombstones are the files used by FindProjectRoot to
// determine the root of a project.
var DefaultProjectTombstones = []string{
	".git",
	"go.mod",
	"go.work", // go1.18
	"glide.yaml",
	"Gopkg.toml",
}

var errNotAbsolute = errors.New("path is not absolute")

// ContainingDirectory finds the parent directory of child containing an
// entry named by tombstones. The child directory must be absolute.
//
// The stopAt argument is optional and is used to abort the search early.
// If specified it must be an absolute path and a parent of the child directory.
//
//	// This should return ctxt.GOPATH+"/src/github.com/charlievieth/buildutil"
//	// since it contains a "go.mod" file.
//	ContainingDirectory(
//		ctxt,
//		ctxt.GOPATH+"/src",
//		ctxt.GOPATH+"/src/github.com/charlievieth/buildutil/contextutil",
//		"go.mod",
//	)
func ContainingDirectory(ctxt *build.Context, child, stopAt string, tombstones ...string) (string, error) {
	if len(tombstones) == 0 {
		return "", errors.New("contextutil: no tombstone files specified")
	}

	// TODO: don't require absolute paths
	if stopAt != "" && !buildutil.IsAbsPath(ctxt, stopAt) {
		return "", &fs.PathError{Op: "contextutil: ContainingDirectory",
			Path: stopAt, Err: errNotAbsolute}
	}
	if !buildutil.IsAbsPath(ctxt, child) {
		return "", &fs.PathError{Op: "contextutil: ContainingDirectory",
			Path: child, Err: errNotAbsolute}
	}

	if stopAt != "" {
		stopAt = filepath.Clean(stopAt)
	}
	dir := filepath.Clean(child)
	for {
		for _, name := range tombstones {
			if buildutil.FileExists(ctxt, join2(ctxt, dir, name)) {
				return dir, nil
			}
		}
		if dir == stopAt {
			break
		}
		parent := filepath.Dir(dir)
		if len(parent) >= len(dir) {
			break
		}
		dir = parent
	}
	return child, os.ErrNotExist
}

// join2 joins two paths, which must be clean.
func join2(ctxt *build.Context, p1, p2 string) string {
	if f := ctxt.JoinPath; f != nil {
		return f(p1, p2)
	}
	return p1 + string(os.PathSeparator) + p2
}

// absPath returns an absolute representation of path.
func absPath(ctxt *build.Context, path string) (string, error) {
	if buildutil.IsAbsPath(ctxt, path) {
		if f := ctxt.JoinPath; f != nil {
			return f(path), nil // Use JoinPath to clean path
		}
		return filepath.Clean(path), nil
	}
	if ctxt.Dir != "" {
		dir, err := filepath.Abs(ctxt.Dir)
		if err != nil {
			return "", err
		}
		return buildutil.JoinPath(ctxt, dir, path), nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return buildutil.JoinPath(ctxt, wd, path), nil
}

func isFile(ctxt *build.Context, name string) bool {
	if fn := ctxt.OpenFile; fn != nil {
		f, err := fn(name)
		if err != nil {
			return false
		}
		f.Close()
		return true
	}
	fi, err := os.Stat(name)
	return err == nil && fi.Mode().IsRegular()
}

// FindProjectRoot finds the root directory of the project containing path,
// which can be a file or a directory. The project root is determined to be
// the directory containing any entry with a name in DefaultProjectTombstones.
// The extra argument specifies additional tombstone names (".svn").
//
// The build.Context is used for file system access and to limit the search
// space if path is a child of GOROOT or GOPATH. Otherwise, all parent
// directories of path are searched.
//
// If path is not absolute it is joined with build.Context.Dir (if set) or
// the current working directory.
//
// os.ErrNotExist is returned if the project directory was not found.
func FindProjectRoot(ctxt *build.Context, path string, extra ...string) (string, error) {
	var err error
	path, err = absPath(ctxt, path)
	if err != nil {
		return "", err
	}

	// Allow path to be a file
	if isFile(ctxt, path) {
		path = filepath.Dir(path)
	}

	// Find the GOROOT or GOPATH that is the parent of path, if any.
	var root string
	for _, p := range ctxt.SrcDirs() {
		if isSubdir(p, path) {
			root = p
			break
		}
	}

	tombstones := DefaultProjectTombstones
	if len(extra) != 0 {
		tombstones = make([]string, len(extra)+len(DefaultProjectTombstones))
		copy(tombstones, extra)
		copy(tombstones[len(extra):], DefaultProjectTombstones)
	}
	return ContainingDirectory(ctxt, path, root, tombstones...)
}

// HasSubdir calls ctxt.HasSubdir (if not nil) or else uses the local file
// system to answer the question.
func HasSubdir(ctxt *build.Context, root, dir string) (rel string, ok bool) {
	if f := ctxt.HasSubdir; f != nil {
		return f(root, dir)
	}

	// Clean paths and check again.
	root = filepath.Clean(root)
	dir = filepath.Clean(dir)
	if rel, ok = hasSubdir(root, dir); ok {
		return
	}

	// Check if we're comparing a path that is in the GOROOT and one in
	// the GOPATH. Paths like this are very unlikely to match after
	// expanding symlinks, which is an expensive operation.
	goroot := filepath.Clean(ctxt.GOROOT)
	if (root == goroot || isSubdir(goroot, root)) && inGopath(ctxt, dir) ||
		(dir == goroot || isSubdir(goroot, dir)) && inGopath(ctxt, root) {
		return "", false
	}

	// TODO: improve comment
	//
	// It is unlikely that dir is a subdirectory of root, so optimize
	// for that case using os.Stat and os.SameFile, before attempting
	// the much more expensive call to filepath.EvalSymlinks.
	rootInfo, err := os.Stat(root)
	if err != nil {
		return "", false
	}
	path := dir
	for {
		fi, err := os.Lstat(path)
		if err != nil {
			return "", false
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			break // issue #14054 symlink in dir
		}
		if os.SameFile(rootInfo, fi) {
			break // symlink in root
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", false
		}
		path = parent
	}

	// Try expanding symlinks and comparing
	// expanded against unexpanded and
	// expanded against expanded.
	rootSym, _ := filepath.EvalSymlinks(root)
	if rel, ok = hasSubdir(rootSym, dir); ok {
		return
	}
	dirSym, _ := filepath.EvalSymlinks(dir)
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
func hasSubdir(root, dir string) (rel string, ok bool) {
	if isSubdir(root, dir) {
		return filepath.ToSlash(dir[len(root)+1:]), true
	}
	return "", false
}

// inGopath reports if dir is within the gopath, which may be a list of
// paths joined by os.PathListSeparator.
func inGopath(ctxt *build.Context, dir string) bool {
	if f := ctxt.SplitPathList; f != nil {
		for _, p := range f(ctxt.GOPATH) {
			// Match behavior of go/build.Context.gopath()
			if p == "" || p[0] == '~' || p == ctxt.GOROOT {
				continue
			}
			if p == dir || isSubdir(p, dir) || isSubdir(filepath.Clean(p), dir) {
				return true
			}
		}
		return false
	}
	// iterate through the GOPATH
	s := ctxt.GOPATH
	for {
		n := strings.IndexByte(s, os.PathListSeparator)
		if n < 0 {
			break
		}
		p := s[:n]
		if p == dir || isSubdir(p, dir) {
			return true
		}
		if p != "" {
			p = filepath.Clean(p)
			if p == dir || isSubdir(p, dir) {
				return true
			}
		}
		s = s[n+1:]
	}
	if s != "" {
		s = filepath.Clean(s)
		if s == dir || isSubdir(s, dir) {
			return true
		}
	}
	return false
}

func sameFile(name, base string, baseInfo os.FileInfo) bool {
	if filepath.Base(name) == base {
		if fi, err := os.Stat(name); err == nil {
			return os.SameFile(fi, baseInfo)
		}
	}
	return false
}

func cleanGoPaths(ctxt *build.Context) {
	ctxt.GOROOT = filepath.Clean(ctxt.GOROOT)

	// If there is a custom SplitPathList function we can't reliably
	// rejoin the list after cleaning.
	if ctxt.SplitPathList != nil {
		return
	}

	paths := filepath.SplitList(ctxt.GOPATH)
	a := paths[:0]
	for _, p := range paths {
		// Match behavior of go/build.Context.gopath()
		if p == "" || p[0] == '~' || p == ctxt.GOROOT {
			continue
		}
		a = append(a, filepath.Clean(p))
	}
	gopath := strings.Join(a, string(os.PathListSeparator))
	if gopath != ctxt.GOPATH {
		ctxt.GOPATH = gopath
	}
}

func sortUniqueStrings(list []string) []string {
	if len(list) <= 1 {
		return list
	}
	sort.Strings(list)
	a := list[:1]
	k := a[0]
	for i := 1; i < len(list); i++ {
		if v := list[i]; v != k {
			a = append(a, v)
			k = v
		}
	}
	return a
}

func readSubdirs(ctxt *build.Context, subdirs []string, names map[string]struct{}) ([]os.FileInfo, error) {
	if len(subdirs) == 0 {
		return nil, nil
	}

	if f := ctxt.ReadDir; f != nil {
		fis, err := f(filepath.Dir(subdirs[0]))
		if err != nil {
			return nil, err
		}
		if len(fis) == 0 {
			return nil, nil
		}
		// Filter out any FileInfos that are outside the scope of this Context
		a := fis[:0]
		for _, fi := range fis {
			if _, ok := names[fi.Name()]; ok {
				a = append(a, fi)
			}
		}
		return a, nil
	}

	fis := make([]fs.FileInfo, 0, len(subdirs))
	for _, sub := range subdirs {
		fi, err := os.Lstat(sub)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fis, err
		}
		fis = append(fis, fi)
	}
	return fis, nil
}

// minPackage is a subset of build.Package except that SrcRoot is the src
// directory of the GOPATH/GOROOT the package was found under, if any.
type minPackage struct {
	ImportPath string // import path of package ("" if unknown)
	Root       string // root of Go tree where this package lives
	SrcRoot    string // package source root directory ("" if unknown)
	Goroot     bool   // package found in Go root
	IsModule   bool   // go module package outside of GOPATH
}

// TODO: remove when done testing
func (m minPackage) String() string {
	return fmt.Sprintf("{ImportPath: %q, Root: %q, SrcRoot: %q, Goroot: %t, IsModule: %t}",
		m.ImportPath, m.Root, m.SrcRoot, m.Goroot, m.IsModule,
	)
}

func minImportDir(ctxt *build.Context, dir string) (*minPackage, error) {
	root := join2(ctxt, ctxt.GOROOT, "src")
	if rel, ok := HasSubdir(ctxt, root, dir); ok {
		pkg := &minPackage{
			ImportPath: filepath.ToSlash(rel),
			Root:       filepath.Dir(root),
			SrcRoot:    root,
			Goroot:     true,
		}
		return pkg, nil
	}
	for _, src := range buildutil.SplitPathList(ctxt, ctxt.GOPATH) {
		src = join2(ctxt, src, "src")
		if rel, ok := HasSubdir(ctxt, src, dir); ok {
			pkg := &minPackage{
				ImportPath: filepath.ToSlash(rel),
				Root:       filepath.Dir(src),
				SrcRoot:    src,
				Goroot:     false,
			}
			return pkg, nil
		}
	}

	// Find the module root, if any
	root, err := ContainingDirectory(ctxt, dir, "", "go.mod", "go.work")
	if err != nil {
		return nil, err
	}
	pkg := &minPackage{
		Root:     root,
		IsModule: true,
	}
	return pkg, nil
}

// TODO: export and note that this is faster than buildutil.readDir
//
// readDir behaves like ioutil.readDir, but uses the build context's file
// system interface, if any.
func readDir(ctxt *build.Context, path string) ([]fs.FileInfo, error) {
	if f := ctxt.ReadDir; f != nil {
		return f(path)
	}
	return readdir.ReadDir(path)
}

// ScopedContext returns a build.Context with a ReadDir that is scoped to the
// directories listed by pkgdirs and the GOROOT. That is, ReadDir when called
// with an ancestor of pkgdirs will only return immediate ancestors (that lead
// to the pkgdirs). When called with any of the pkgdirs, GOROOT, or any of their
// children all results are returned (same as ioutil.ReadDir).
//
// A scoped context is designed to limit the search scope of tools that walk the
// entire GOPATH (e.g. "golang.org/x/tools/refactor/rename"), which can greatly
// speed up processing time.
//
//	// In the below example we limit the search path to "/go/src/pkg/buildutil".
//	ctxt, _ := ScopedContext(&build.Default, "/go/src/pkg/buildutil")
//	ctxt.ReadDir("/go")                               // => ["src"]
//	ctxt.ReadDir("/go/src")                           // => ["pkg"]
//	ctxt.ReadDir("/go/src/pkg")                       // => ["buildutil"]
//	ctxt.ReadDir("/go/src/pkg/buildutil")             // => [ALL ENTRIES]
//	ctxt.ReadDir("/go/src/pkg/buildutil/contextutil") // => [ALL ENTRIES]
func ScopedContext(orig *build.Context, pkgdirs ...string) (*build.Context, error) {
	// TODO: allow no pkgdirs to limit Context to GOROOT?
	if len(pkgdirs) == 0 {
		return nil, errors.New("contextutil: no package directories specified")
	}
	for _, dir := range pkgdirs {
		// Require the pkg directory to be absolute. Otherwise, this may not
		// work well with editors (or is being improperly used by editors).
		if !buildutil.IsAbsPath(orig, dir) {
			return nil, &fs.PathError{Op: "contextutil: ScopedContext",
				Path: dir, Err: errNotAbsolute}
		}
		if !buildutil.IsDir(orig, dir) {
			return nil, fmt.Errorf("contextutil: not a directory: %q", dir)
		}
	}

	copy := *orig // make a copy
	ctxt := &copy
	cleanGoPaths(ctxt)

	for i, dir := range pkgdirs {
		pkgdirs[i] = filepath.Clean(dir)
	}

	// TODO: this will not work for all cases of symlinks
	for _, dir := range pkgdirs {
		if p, err := filepath.EvalSymlinks(dir); err == nil && p != dir {
			pkgdirs = append(pkgdirs, p)
		}
	}

	goroots := []string{ctxt.GOROOT}
	if p, err := filepath.EvalSymlinks(ctxt.GOROOT); err == nil && p != ctxt.GOROOT {
		goroots = append(goroots, p)
	}

	// File system map:
	// 	"/go":     ["/go/src"]
	// 	"/go/src": ["/go/src/archive", "/go/src/bufio"]
	dirs := make(map[string][]string)

	for _, root := range pkgdirs {
		pkg, err := minImportDir(ctxt, root)
		if err != nil {
			return nil, err
		}
		if pkg.IsModule {
			// Treat the module directory as a GOROOT since we can assume
			// all of it's children are valid and relevant.
			goroots = append(goroots, pkg.Root)
			continue
		}

		dir := buildutil.JoinPath(ctxt, pkg.SrcRoot, pkg.ImportPath)
		child := filepath.Dir(dir)
		for dir != pkg.SrcRoot && dir != child {
			dirs[child] = append(dirs[child], dir)
			dir = child
			child = filepath.Dir(dir)
		}
		// Include the root GOROOT/GOPATH dir
		dirs[pkg.Root] = append(dirs[pkg.Root], pkg.SrcRoot)
	}

	// The result of ReadDir must be sorted and remove duplicate files
	// due to symlinks.
	for root, subdirs := range dirs {
		if len(subdirs) > 1 {
			dirs[root] = sortUniqueStrings(subdirs)
		}
	}

	// If orig.ReadDir is non-nil create a map of file names to speed up
	// filtering when reading scoped sub-directories.
	var names map[string]map[string]struct{}
	if orig.ReadDir != nil {
		names = make(map[string]map[string]struct{}, len(dirs))
		for dir, subdirs := range dirs {
			m := make(map[string]struct{}, len(subdirs))
			for _, s := range subdirs {
				m[filepath.Base(s)] = struct{}{}
			}
			names[dir] = m
		}
	}

	ctxt.ReadDir = func(dir string) ([]fs.FileInfo, error) {
		if !buildutil.IsAbsPath(ctxt, dir) {
			return nil, &fs.PathError{Op: "contextutil: ReadDir", Path: dir, Err: errNotAbsolute}
		}
		dir = filepath.Clean(dir)

		// Never limit GOROOT
		for _, p := range goroots {
			if p == dir || isSubdir(p, dir) {
				return readDir(orig, dir)
			}
		}

		// Dir is within the package - read normally
		for _, p := range pkgdirs {
			if p == dir || isSubdir(p, dir) {
				return readDir(orig, dir)
			}
		}

		if len(dirs) == 0 {
			return nil, &fs.PathError{Op: "open", Path: dir, Err: os.ErrNotExist}
		}

		if subdirs, ok := dirs[dir]; ok {
			return readSubdirs(orig, subdirs, names[dir])
		}

		// Try comparing file stats
		fi, err := os.Stat(dir)
		if err != nil {
			return nil, err
		}
		if !fi.IsDir() {
			// Replicate the behavior of ioutil.ReadDir
			return nil, &fs.PathError{Op: "readdir", Path: dir, Err: syscall.ENOTDIR}
		}

		base := filepath.Base(dir)
		for _, p := range pkgdirs {
			if sameFile(p, base, fi) {
				return readDir(orig, dir)
			}
		}
		for root, subdirs := range dirs {
			if sameFile(root, base, fi) {
				return readSubdirs(orig, subdirs, names[dir])
			}
		}

		// Fall back to the previous ReadDir, if any.
		if orig.ReadDir != nil {
			return orig.ReadDir(dir)
		}

		// TODO: make sure returning an error here doesn't lead to
		// any issues as the directory *may* actually exist, but is
		// not included in our list of "valid" directories.
		return nil, &fs.PathError{Op: "open", Path: dir, Err: os.ErrNotExist}
	}

	return ctxt, nil
}
