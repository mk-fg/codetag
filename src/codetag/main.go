package main

import (
	"fmt"
	"flag"
	"os"
	"github.com/hoisie/mustache"
	"github.com/vaughan0/go-logging"
)
	// "path/filepath"

type path_t string
func (path path_t) Render() string {
	return fmt.Sprint(path)
}

var config_paths = []path_t{"~/.codetag.yaml", "/etc/codetag.yaml"}
var config string


func main() {

	flag.Usage = func() {
		fmt.Println(mustache.Render( `usage: {{{cmd}}} [ <options> ]

Index code files, using parameters specified in the config file.
If not specified exmplicitly, config file is searched within the
following paths (in that order):
{{#paths}}
  - {{{Render}}}
{{/paths}}
Examples:
  % {{{cmd}}}
  % {{{cmd}}} --config config.yaml

Options:`, map[string]string{"cmd": os.Args[0]}, map[string][]path_t{"paths": config_paths} ))
		flag.PrintDefaults()
	}

	flag.StringVar(&config, "config", "", "Configuration file to use.")
	flag.Parse()
	if flag.NArg() > 0 {
		fmt.Printf("Error: no command-line arguments are allowed (provided: %v)\n", flag.Args())
		os.Exit(1)
	}

	logging.DefaultSetup()
	log := logging.Get("codegen.main")

	log.Infof("config: %#v", config)
}

	// func walk_iter(path string, info os.FileInfo, err error) error
	// filepath.Walk(root string, walkFn WalkFunc) error
