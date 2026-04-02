package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/nijaru/aku/internal/scaffold"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "init":
		if err := runInit(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(2)
	}
}

func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	root := fs.String("dir", ".", "target Go module root")
	command := fs.String("name", "api", "command directory name")
	modulePath := fs.String("module", "", "module path override")
	force := fs.Bool("force", false, "overwrite existing files")

	if err := fs.Parse(args); err != nil {
		return err
	}

	return scaffold.Init(scaffold.Options{
		Root:       *root,
		Command:    *command,
		ModulePath: *modulePath,
		Force:      *force,
	})
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: aku init [--dir path] [--name api] [--module path] [--force]")
}
