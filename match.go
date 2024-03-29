package buildutil

import (
	"errors"
	"fmt"
	"go/build"
	"go/build/constraint"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/charlievieth/buildutil/internal/util"
	"github.com/charlievieth/reonce"
)

// PreferredArchList is used to pick an OS (GOOS) when matching a build.Context
// to a file.
var PreferredOSList = createPreferredList([]string{
	runtime.GOOS, // deduped in init()
	"darwin",
	"linux",
	"windows",
	"openbsd",
	"freebsd",
	"netbsd",
}, func(p *GoPlatform) string { return p.GOOS })

// PreferredArchList is used to pick an Arch (GOARCH) when matching a build.Context
// to a file.
var PreferredArchList = createPreferredList([]string{
	runtime.GOARCH,
	"amd64",
	"arm64",
	"arm",
	"386",
	"ppc64",
}, func(p *GoPlatform) string { return p.GOARCH })

func createPreferredList(orig []string, fn func(p *GoPlatform) string) []string {
	seen := make(map[string]bool)
	var a []string
	for _, s := range orig {
		if !seen[s] {
			seen[s] = true
			a = append(a, s)
		}
	}
	for _, p := range DefaultGoPlatforms {
		s := fn(&p)
		if !seen[s] {
			seen[s] = true
			a = append(a, s)
		}
	}
	return a
}

var (
	ErrImpossibleGoVersion = errors.New("cannot satisfy go version")
	ErrMatchContext        = errors.New("cannot match context to file")

	// declared here to make testing easier
	errCompilerMismatchGc    = errors.New("compiler mismatch: gc")
	errCompilerMismatchGccGo = errors.New("compiler mismatch: gccgo")
	errCompilerNegatedGc     = errors.New("compiler negated: gc")
	errCompilerNegatedGccGo  = errors.New("compiler negated: gccgo")
)

// A MatchError describes an error matching a build.Context to a file.
type MatchError struct {
	Path      string
	Permanent bool // Error cannot be resolved (e.g. compiler mismatch)
	Err       error
}

func (e *MatchError) Error() string {
	return "buildutil: cannot match " + e.Path + ": " + e.Err.Error()
}

func (e *MatchError) Unwrap() error { return e.Err }

// NB: will need to be updated for go2
var goVersionTagRe = reonce.New(`^go1\.\d+$`)

func isGoReleaseTag(s string) bool {
	return knownReleaseTag[s] ||
		strings.HasPrefix(s, "go1.") && goVersionTagRe.MatchString(s)
}

func isGoExperimentTag(name string) bool {
	return strings.HasPrefix(name, "goexperiment.")
}

func isInternalTag(ctxt *build.Context, name string) bool {
	if name == "gc" || name == "gccgo" || knownOS[name] || knownArch[name] ||
		isGoExperimentTag(name) || isGoReleaseTag(name) {
		return true
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

func lookupTag(x constraint.Expr, tag string) (found, negated bool) {
	switch v := x.(type) {
	case *constraint.TagExpr:
		if v.Tag == tag {
			return true, false
		}
	case *constraint.NotExpr:
		ok, neg := lookupTag(v.X, tag)
		if !ok {
			return false, false
		}
		return true, !neg // ! for double negatives
	case *constraint.AndExpr:
		// WARN: a tag can occur on both sides of the expression
		if found, negated = lookupTag(v.X, tag); found {
			return found, negated
		}
		if found, negated = lookupTag(v.Y, tag); found {
			return found, negated
		}
	case *constraint.OrExpr:
		// WARN: a tag can occur on both sides of the expression
		if found, negated = lookupTag(v.X, tag); found {
			return found, negated
		}
		if found, negated = lookupTag(v.Y, tag); found {
			return found, negated
		}
	default:
		panic(fmt.Sprintf("invalid type: %T", x))
	}
	return false, false
}

func checkCompiler(ctxt *build.Context, x constraint.Expr) error {
	switch ctxt.Compiler {
	case "gc":
		// if 'gccgo' is specified 'gc' cannot be used
		if ok, negated := lookupTag(x, "gccgo"); ok && !negated {
			return errCompilerMismatchGccGo
		}
		if ok, negated := lookupTag(x, "gc"); ok && negated {
			return errCompilerNegatedGc
		}
	case "gccgo":
		// if 'gc' is specified 'gccgo' cannot be used
		if ok, negated := lookupTag(x, "gc"); ok && !negated {
			return errCompilerMismatchGc
		}
		if ok, negated := lookupTag(x, "gccgo"); ok && negated {
			return errCompilerNegatedGccGo
		}
	default:
		// ignore
	}
	return nil
}

// findSupportedArch returns an Arch that is valid for the
// Context's GOOS, if any.
func findSupportedArch(ctxt *build.Context) (string, bool) {
	arches, ok := supportedPlatformsOsArch[ctxt.GOOS]
	if !ok || arches[ctxt.GOARCH] {
		// No mapping for the OS or the OS/Arch combo is valid
		return ctxt.GOARCH, true
	}
	// Try preferred list first
	for _, arch := range PreferredArchList {
		if arches[arch] {
			return arch, true
		}
	}
	// Use the first Arch
	for arch := range arches {
		return arch, true
	}
	return "", false
}

// findSupportedOS returns an OS that is valid for the
// Context's GOARCH, if any.
func findSupportedOS(ctxt *build.Context) (string, bool) {
	oses, ok := supportedPlatformsArchOs[ctxt.GOARCH]
	if !ok || oses[ctxt.GOOS] {
		// No mapping for the Arch or the OS/Arch combo is valid
		return ctxt.GOOS, true
	}
	// Try preferred list first
	for _, os := range PreferredOSList {
		if oses[os] {
			return os, true
		}
	}
	// Use the first OS, if any
	for os := range oses {
		return os, true
	}
	return "", false
}

// matchGOARCH attempts to find an Arch that is valid for the Context's OS and
// satisfies the build constraint expr.
func matchGOARCH(ctxt *build.Context, expr constraint.Expr) bool {
	arches, ok := supportedPlatformsOsArch[ctxt.GOOS]
	if !ok || arches[ctxt.GOARCH] {
		return eval(ctxt, expr, nil)
	}
	origArch := ctxt.GOARCH
	// Try the preferred list first
	for _, arch := range PreferredArchList {
		if arches[arch] {
			ctxt.GOARCH = arch
			if eval(ctxt, expr, nil) {
				return true
			}
		}
	}
	// Try all supported arches
	for arch := range arches {
		ctxt.GOARCH = arch
		if eval(ctxt, expr, nil) {
			return true
		}
	}
	ctxt.GOARCH = origArch
	return false
}

// matchGOOS attempts to find an OS that is valid for the Context's Arch and
// satisfies the build constraint expr.
func matchGOOS(ctxt *build.Context, expr constraint.Expr) bool {
	oses, ok := supportedPlatformsArchOs[ctxt.GOARCH]
	if !ok || oses[ctxt.GOOS] {
		return eval(ctxt, expr, nil)
	}
	origOs := ctxt.GOOS
	// Try the preferred list first
	for _, os := range PreferredOSList {
		if oses[os] {
			ctxt.GOOS = os
			if eval(ctxt, expr, nil) {
				return true
			}
		}
	}
	// Try all supported OSes
	for os := range oses {
		ctxt.GOOS = os
		if eval(ctxt, expr, nil) {
			return true
		}
	}
	ctxt.GOOS = origOs
	return false
}

// TODO: make sure CGO support is correct for the selected platform.
//
// MatchContext returns a build.Context that would include filename in a build.
func MatchContext(orig *build.Context, filename string, src interface{}) (*build.Context, error) {
	if orig == nil {
		orig = &build.Default
	}
	rc, err := openReader(orig, filename, src)
	if err != nil {
		return nil, err
	}
	data, err := readImportsFast(rc)
	rc.Close()
	if err != nil {
		return nil, err
	}

	// copy
	ctxt := util.CopyContext(orig)

	// init
	if ctxt.GOARCH == "" {
		ctxt.GOARCH = runtime.GOARCH
	}
	if ctxt.GOOS == "" {
		ctxt.GOOS = runtime.GOOS
	}
	// WARN: we might not want to set this
	if ctxt.GOROOT == "" {
		ctxt.GOROOT = runtime.GOROOT()
	}
	if ctxt.Compiler == "" {
		ctxt.Compiler = runtime.Compiler
	}

	// We ignore the error here since it's too hard to determine
	// if it matters.
	if gopath, ok := fixGOPATH(ctxt, filename); ok {
		ctxt.GOPATH = gopath
	}

	// Any os/arch specified in the filename *must* be respected.
	var (
		// Some OSes are compatible with each other so we use a map.
		requiredOS   map[string]bool
		requiredArch string
	)
	tags := make(map[string]bool)
	if !goodOSArchFile(ctxt, filepath.Base(filename), tags) {
		for tag := range tags {
			switch {
			case knownOS[tag]:
				ctxt.GOOS = tag
				requiredOS = map[string]bool{tag: true}
				// WARN WARN WARN
				// WARN: we might want to keep these because it's used below
				delete(tags, tag) // remove so that we don't attempt to match it again
			case knownArch[tag]:
				ctxt.GOARCH = tag
				requiredArch = tag
				// WARN WARN WARN
				// WARN: we might want to keep these because it's used below
				delete(tags, tag) // remove so that we don't attempt to match it again
			}
		}
	}

	// Update the requiredOS map with any compatible OSes.
	if requiredOS != nil {
		for _, os := range compatibleOSes[ctxt.GOOS] {
			requiredOS[os] = true
		}
	}

	// If the filename specifies either an OS or Arch (but not both) make sure
	// the OS/Arch is valid.
	switch {
	case requiredOS != nil && requiredArch == "":
		if arch, ok := findSupportedArch(ctxt); ok {
			ctxt.GOARCH = arch
		}
	case requiredArch != "" && requiredOS == nil:
		if os, ok := findSupportedOS(ctxt); ok {
			ctxt.GOOS = os
		}
	}

	ok, _, err := shouldBuild(ctxt, data, tags)
	if err != nil {
		return nil, &MatchError{Path: filename, Err: err}
	}
	if ok {
		// Updating the OS/Arch from the filename fixed the Context
		return ctxt, nil
	}

	expr, err := parseBuildConstraint(data)
	if err != nil {
		return nil, &MatchError{Path: filename, Err: err}
	}

	// CEV: Is this possible and if so how?
	if expr == nil {
		return nil, &MatchError{Path: filename, Err: errors.New("nil build constraint")}
	}
	if len(tags) == 0 {
		return nil, &MatchError{Path: filename, Err: errors.New("no build tags")}
	}

	// GOEXPERIMENT tags
	for name := range tags {
		if isGoExperimentTag(name) {
			ok, negated := lookupTag(expr, name)
			if !ok {
				continue
			}
			if negated {
				ctxt.ToolTags = util.StringsRemoveAll(ctxt.ToolTags, name)
			} else {
				ctxt.ToolTags = util.StringsAppend(ctxt.ToolTags, name)
			}
		}
	}
	if eval(ctxt, expr, nil) {
		return ctxt, nil
	}

	// Quickly try to find a build tag that works
	var buildTags []string
	for name := range tags {
		if !isInternalTag(ctxt, name) {
			buildTags = append(buildTags, name)
		}
	}
	if len(buildTags) != 0 {
		origBuildTags := util.DuplicateStrings(ctxt.BuildTags)
		orig := ctxt.BuildTags
		for _, tag := range buildTags {
			ok, negated := lookupTag(expr, tag)
			if !ok {
				continue // this should not happen
			}
			if negated {
				ctxt.BuildTags = util.StringsRemoveAll(ctxt.BuildTags, tag)
			} else {
				ctxt.BuildTags = util.StringsAppend(ctxt.BuildTags, tag)
			}
			if eval(ctxt, expr, nil) {
				return ctxt, nil
			}
			ctxt.BuildTags = orig
		}

		// Apply all build tags
		// NB: there are likely situations where this will not work
		ctxt.BuildTags = origBuildTags
		for _, tag := range buildTags {
			if ok, negated := lookupTag(expr, tag); ok {
				if negated {
					ctxt.BuildTags = util.StringsRemoveAll(ctxt.BuildTags, tag)
				} else {
					ctxt.BuildTags = util.StringsAppend(ctxt.BuildTags, tag)
				}
			}
		}
		if eval(ctxt, expr, nil) {
			return ctxt, nil
		}
	}

	// Check for release tag constraints since there is nothing we
	// can do to resolve them.
	for name := range tags {
		if isGoReleaseTag(name) {
			ok, negated := lookupTag(expr, name)
			if !ok {
				continue
			}
			hasRelease := util.StringsContains(ctxt.ReleaseTags, name)
			if negated && hasRelease || !negated && !hasRelease {
				return nil, &MatchError{Path: filename, Permanent: true,
					Err: ErrImpossibleGoVersion}
			}
		}
	}

	// Delay checking for the compiler and go version until after trying
	// build and tool tags since some things like the "purego" tag get
	// around this.

	// Check for a compiler mismatch since we cannot adapt the Context
	// to handle that.
	if tags["gc"] || tags["gccgo"] {
		if err := checkCompiler(ctxt, expr); err != nil {
			return nil, &MatchError{Path: filename, Permanent: true, Err: err}
		}
	}

	// Try toggling cgo
	if tags["cgo"] {
		if ctxt.CgoEnabled || cgoEnabled[ctxt.GOOS+"/"+ctxt.GOARCH] {
			ctxt.CgoEnabled = !ctxt.CgoEnabled
			if eval(ctxt, expr, nil) {
				return ctxt, nil
			}
			ctxt.CgoEnabled = !ctxt.CgoEnabled // undo our change
		}
	}

	// Try differet OS/Arch combinations
	hasOS := util.TagsIntersect(tags, knownOS)
	hasArch := util.TagsIntersect(tags, knownArch)
	switch {
	case hasOS && hasArch:
		oldOS := ctxt.GOOS
		oldArch := ctxt.GOARCH
		oldCgo := ctxt.CgoEnabled
		for _, p := range DefaultGoPlatforms {
			if p.GOOS == oldOS && p.GOARCH == oldArch {
				continue
			}
			if requiredArch != "" && p.GOARCH != requiredArch {
				continue
			}
			if requiredOS != nil && !requiredOS[p.GOOS] {
				continue
			}
			ctxt.GOOS = p.GOOS
			ctxt.GOARCH = p.GOARCH
			ctxt.CgoEnabled = p.CgoSupported
			if eval(ctxt, expr, nil) {
				return ctxt, nil
			}
			// Try again without cgo
			if ctxt.CgoEnabled {
				ctxt.CgoEnabled = false
				if eval(ctxt, expr, nil) {
					return ctxt, nil
				}
			}
		}
		ctxt.GOOS = oldOS
		ctxt.GOARCH = oldArch
		ctxt.CgoEnabled = oldCgo
	case hasOS:
		oldOS := ctxt.GOOS
		for _, os := range PreferredOSList {
			if os == oldOS {
				continue
			}
			if requiredOS != nil && !requiredOS[os] {
				continue
			}
			ctxt.GOOS = os
			// Change GOARCH to one that is supported
			if matchGOARCH(ctxt, expr) {
				return ctxt, nil
			}
		}
		ctxt.GOOS = oldOS
	case hasArch:
		oldArch := ctxt.GOARCH
		for _, arch := range PreferredArchList {
			if arch == oldArch {
				continue
			}
			if requiredArch != "" && arch != requiredArch {
				continue
			}
			ctxt.GOARCH = arch
			if matchGOOS(ctxt, expr) {
				return ctxt, nil
			}
		}
		ctxt.GOARCH = oldArch
	}

	// TODO: add additional context to the error (such as
	// the "//go:build" directive).
	return nil, &MatchError{Path: filename, Err: ErrMatchContext}
}

func pathContainsSrcDir(s string) bool {
	if filepath.Separator == '/' {
		return strings.Contains(s, "/src")
	}
	return strings.Contains(s, "/src") || strings.Contains(s, "\\src")
}

func resolveGOPATH(dir string) (string, bool) {
	origDir := dir

	if !pathContainsSrcDir(dir) {
		dir, _ = filepath.EvalSymlinks(dir)
		if !pathContainsSrcDir(dir) {
			return origDir, false
		}
	}

	for {
		parent, name := filepath.Split(dir)
		if parent != "" {
			parent = parent[:len(parent)-1]
		}
		if name == "src" {
			return parent, true
		}
		if parent == "" || parent == dir {
			break
		}
		dir = parent
	}

	return origDir, false
}

// TODO: use or remove
func fixGOPATH(ctxt *build.Context, filename string) (string, bool) {
	dir := filepath.Dir(filename)

	// fast check for GOROOT/GOPATH
	gopath := ctxt.GOPATH
	if gopath == "" {
		gopath = build.Default.GOPATH
	}
	for _, root := range splitPathList(ctxt, gopath) {
		if _, ok := hasSubdir(root, dir); ok {
			return gopath, true
		}
	}
	if ctxt.GOROOT != "" {
		if _, ok := hasSubdir(ctxt.GOROOT, dir); ok {
			return gopath, true
		}
	}

	if path, ok := resolveGOPATH(dir); ok {
		if ctxt.SplitPathList == nil && ctxt.GOPATH != "" {
			path = path + string(filepath.ListSeparator) + ctxt.GOPATH
		}
		return path, true
	}
	return "", false
}
