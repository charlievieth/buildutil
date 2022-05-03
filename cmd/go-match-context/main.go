package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/build"
	"log"
	"os"
	"path/filepath"

	"github.com/charlievieth/buildutil"
)

func init() {
	log.SetFlags(log.Lshortfile)
}

type Context struct {
	GOARCH        string
	GOOS          string
	GOROOT        string
	GOPATH        string
	Dir           string
	CgoEnabled    bool
	UseAllFiles   bool
	Compiler      string
	BuildTags     []string
	ToolTags      []string
	ReleaseTags   []string
	InstallSuffix string
}

func main() {
	flag.Usage = func() {
		const usage = "Usage: %s [OPTION] FILE\n" +
			"MatchContext for FILE and print the new build.Context\n"
		fmt.Fprintf(os.Stdout, usage, filepath.Base(os.Args[0]))
		flag.PrintDefaults()
	}
	printJSON := flag.Bool("json", false, "Print output as JSON")
	flag.Parse()
	if flag.NArg() != 1 {
		log.Panicln("error: expect one FILE argument")
		flag.Usage()
		os.Exit(1)
	}
	filename := flag.Arg(0)

	ctxt, err := buildutil.MatchContext(&build.Default, filename, nil)
	if err != nil {
		log.Fatal("error:", err)
	}

	if *printJSON {
		c := Context{
			GOARCH:        ctxt.GOARCH,
			GOOS:          ctxt.GOOS,
			GOROOT:        ctxt.GOROOT,
			GOPATH:        ctxt.GOPATH,
			Dir:           ctxt.Dir,
			CgoEnabled:    ctxt.CgoEnabled,
			UseAllFiles:   ctxt.UseAllFiles,
			Compiler:      ctxt.Compiler,
			BuildTags:     ctxt.BuildTags,
			ToolTags:      ctxt.ToolTags,
			ReleaseTags:   ctxt.ReleaseTags,
			InstallSuffix: ctxt.InstallSuffix,
		}
		data, err := json.MarshalIndent(&c, "", "    ")
		if err != nil {
			log.Fatal("error:", err)
		}
		if _, err := os.Stdout.Write(data); err != nil {
			log.Fatal("error:", err)
		}
	} else {
		fmt.Printf("GOARCH=%q\n", ctxt.GOARCH)
		fmt.Printf("GOOS=%q\n", ctxt.GOOS)
		fmt.Printf("GOROOT=%q\n", ctxt.GOROOT)
		fmt.Printf("GOPATH=%q\n", ctxt.GOPATH)
		fmt.Printf("Dir=%q\n", ctxt.Dir)
		fmt.Printf("CgoEnabled=%t\n", ctxt.CgoEnabled)
		fmt.Printf("UseAllFiles=%t\n", ctxt.UseAllFiles)
		fmt.Printf("Compiler=%q\n", ctxt.Compiler)
		fmt.Printf("BuildTags=%q\n", ctxt.BuildTags)
		fmt.Printf("ToolTags=%q\n", ctxt.ToolTags)
		fmt.Printf("ReleaseTags=%q\n", ctxt.ReleaseTags)
		fmt.Printf("InstallSuffix=%q\n", ctxt.InstallSuffix)
	}
}
