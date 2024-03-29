package buildutil

import (
	"context"
	"go/build"
	"os/exec"
	"strings"

	"github.com/charlievieth/buildutil/internal/util"
)

// GoCommandContext returns an exec.Cmd for the provided build.Context and
// context.Context.  The Cmd's env is set to that of the Context. The args
// contains a "-tags" flag it is updated to match the build constraints of
// the Context otherwise the "-tags" are provided via the GOFLAGS env var.
func GoCommandContext(ctx context.Context, ctxt *build.Context, name string, args ...string) *exec.Cmd {
	if ctxt == nil {
		orig := build.Default
		ctxt = &orig
	}

	e := util.NewEnviron()
	e.Set("GOPATH", ctxt.GOPATH)
	if s, _ := e.Lookup("GOROOT"); s != "" && s != ctxt.GOROOT {
		e.Set("GOROOT", ctxt.GOROOT)
	}
	if ctxt.GOOS != "" {
		e.Set("GOOS", ctxt.GOOS)
	}
	if ctxt.GOARCH != "" {
		e.Set("GOARCH", ctxt.GOARCH)
	}
	if ctxt.CgoEnabled {
		e.Set("CGO_ENABLED", "1")
	} else {
		e.Set("CGO_ENABLED", "0")
	}
	if len(ctxt.ToolTags) != 0 {
		a := make([]string, 0, len(ctxt.ToolTags))
		for _, s := range ctxt.ToolTags {
			if strings.HasPrefix(s, "goexperiment.") {
				a = append(a, strings.TrimPrefix(s, "goexperiment."))
			}
		}
		e.Set("GOEXPERIMENT", strings.Join(a, ","))
	}

	if len(ctxt.BuildTags) != 0 {
		// Command line arguments take precedence over the GOFLAGS
		// environment variable so we have to update the "-tags"
		// argument, if provided.
		existingTags := extractTagArgs(args)
		if len(existingTags) != 0 {
			args = replaceTagArgs(args, mergeTagArgs(existingTags, ctxt.BuildTags))
		} else {
			if s, _ := e.Lookup("GOFLAGS"); s != "" {
				// TODO: check if "-tags" is already defined
				e.Set("GOFLAGS", s+" -tags="+strings.Join(ctxt.BuildTags, ","))
			} else {
				e.Set("GOFLAGS", "-tags="+strings.Join(ctxt.BuildTags, ","))
			}
		}
	}

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = e.Environ()

	return cmd

	///////////////////////////////////////////

	// e := env(os.Environ())
	// e = e.Set("GOPATH", ctxt.GOPATH)
	// if s, _ := e.Lookup("GOROOT"); s != "" && s != ctxt.GOROOT {
	// 	e = e.Set("GOROOT", ctxt.GOROOT)
	// }
	// if ctxt.GOOS != "" {
	// 	e = e.Set("GOOS", ctxt.GOOS)
	// }
	// if ctxt.GOARCH != "" {
	// 	e = e.Set("GOARCH", ctxt.GOARCH)
	// }
	// if ctxt.CgoEnabled {
	// 	e = e.Set("CGO_ENABLED", "1")
	// } else {
	// 	e = e.Set("CGO_ENABLED", "0")
	// }
	// if len(ctxt.ToolTags) != 0 {
	// 	a := make([]string, 0, len(ctxt.ToolTags))
	// 	for _, s := range ctxt.ToolTags {
	// 		if strings.HasPrefix(s, "goexperiment.") {
	// 			a = append(a, strings.TrimPrefix(s, "goexperiment."))
	// 		}
	// 	}
	// 	e = e.Set("GOEXPERIMENT", strings.Join(a, ","))
	// }

	// if len(ctxt.BuildTags) != 0 {
	// 	// Command line arguments take precedence over the GOFLAGS
	// 	// environment variable so we have to update the "-tags"
	// 	// argument, if provided.
	// 	existingTags := extractTagArgs(args)
	// 	if len(existingTags) != 0 {
	// 		args = replaceTagArgs(args, mergeTagArgs(existingTags, ctxt.BuildTags))
	// 	} else {
	// 		if s, _ := e.Lookup("GOFLAGS"); s != "" {
	// 			// TODO: check if "-tags" is already defined
	// 			e = e.Set("GOFLAGS", s+" -tags="+strings.Join(ctxt.BuildTags, ","))
	// 		} else {
	// 			e = e.Set("GOFLAGS", "-tags="+strings.Join(ctxt.BuildTags, ","))
	// 		}
	// 	}
	// }

	// cmd := exec.CommandContext(ctx, name, args...)
	// cmd.Env = e.Value()

	// return cmd

	///////////////////////////////////////////

	// m := envMap(os.Environ())
	// m["GOPATH"] = ctxt.GOPATH
	// if s := m["GOROOT"]; s != "" && s != ctxt.GOROOT {
	// 	m["GOROOT"] = ctxt.GOROOT
	// }
	// if ctxt.GOOS != "" {
	// 	m["GOOS"] = ctxt.GOOS
	// }
	// if ctxt.GOARCH != "" {
	// 	m["GOARCH"] = ctxt.GOARCH
	// }
	// if ctxt.CgoEnabled {
	// 	m["CGO_ENABLED"] = "1"
	// } else {
	// 	m["CGO_ENABLED"] = "0"
	// }
	// if len(ctxt.ToolTags) != 0 {
	// 	a := make([]string, 0, len(ctxt.ToolTags))
	// 	for _, s := range ctxt.ToolTags {
	// 		if strings.HasPrefix(s, "goexperiment.") {
	// 			a = append(a, strings.TrimPrefix(s, "goexperiment."))
	// 		}
	// 	}
	// 	m["GOEXPERIMENT"] = strings.Join(a, ",")
	// }
	//
	// if len(ctxt.BuildTags) != 0 {
	// 	// Command line arguments take precedence over the GOFLAGS
	// 	// environment variable so we have to update the "-tags"
	// 	// argument, if provided.
	// 	existingTags := extractTagArgs(args)
	// 	if len(existingTags) != 0 {
	// 		args = replaceTagArgs(args, mergeTagArgs(existingTags, ctxt.BuildTags))
	// 	} else {
	// 		if s := m["GOFLAGS"]; s != "" {
	// 			// TODO: check if "-tags" is already defined
	// 			m["GOFLAGS"] = s + " -tags=" + strings.Join(ctxt.BuildTags, ",")
	// 		} else {
	// 			m["GOFLAGS"] = "-tags=" + strings.Join(ctxt.BuildTags, ",")
	// 		}
	// 	}
	// }
	//
	// env := make([]string, len(m))
	// for k, v := range m {
	// 	env = append(env, k+"="+v)
	// }
	//
	// cmd := exec.CommandContext(ctx, name, args...)
	// cmd.Env = env
	//
	// return cmd
}

// GoCommand returns an exec.Cmd for the provided build.Context. The Cmd's
// env is set to that of the Context. The args contains a "-tags" flag it
// is updated to match the build constraints of the Context otherwise the
// "-tags" are provided via the GOFLAGS env var.
func GoCommand(ctxt *build.Context, name string, args ...string) *exec.Cmd {
	return GoCommandContext(context.Background(), ctxt, name, args...)
}

func envMap(a []string) map[string]string {
	m := make(map[string]string, len(a))
	for _, s := range a {
		k, v, _ := cut(s, "=")
		m[k] = v
	}
	return m
}

func mergeTagArgs(old, new []string) []string {
	if len(old) == 0 {
		return new
	}
	if len(new) == 0 {
		return old
	}
	var args []string
Loop:
	for _, arg := range old {
		s := strings.TrimPrefix(arg, "!")
		for _, x := range new {
			if s == strings.TrimPrefix(x, "!") {
				continue Loop
			}
		}
		args = append(args, arg)
	}
	return append(args, new...)
}

func extractTagArgs(args []string) []string {
	for i := 0; i < len(args); i++ {
		switch s := args[i]; {
		case s == "--":
			// stop parsing args
			return nil
		case s == "-tags":
			if i < len(args)-1 {
				return strings.Split(args[i+1], ",")
			}
			// invalid -tags argument (ignore)
			return nil
		case strings.HasPrefix(s, "-tags="):
			return strings.Split(strings.TrimPrefix(s, "-tags="), ",")
		}
	}
	return nil
}

func replaceTagArgs(args, tags []string) []string {
	a := make([]string, len(args))
	copy(a, args)
	for i := 0; i < len(a); i++ {
		s := a[i]
		if s == "--" {
			break // stop parsing args
		}
		if s == "-tags" {
			if i < len(a)-1 {
				a[i+1] = strings.Join(tags, ",")
			}
			break
		}
		if strings.HasPrefix(s, "-tags=") {
			a[i] = "-tags=" + strings.Join(tags, ",")
			break
		}
	}
	return a
}
