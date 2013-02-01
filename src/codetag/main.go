package main

import (
	"fmt"
	"strings"
	"flag"
	"os"
	"os/user"
	"path/filepath"
	"github.com/hoisie/mustache"
	"github.com/vaughan0/go-logging"
	"github.com/kylelemons/go-gypsy/yaml"
	"codetag/log_setup"
)

type path_t string
func (path path_t) Render() string {
	return fmt.Sprint(path)
}

var config_search = []path_t{"", "~/.codetag.yaml", "/etc/codetag.yaml"}
var config_path string


func configure_loggers(config interface{})


func main() {
	config_search[0] = path_t(os.Args[0] + ".yaml")

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

Options:`, map[string]string{"cmd": os.Args[0]}, map[string][]path_t{"paths": config_search} ))
		flag.PrintDefaults()
	}

	flag.StringVar(&config_path, "config", "", "Configuration file to use.")
	flag.Parse()
	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "Error: no command-line arguments are allowed (provided: %v)\n", flag.Args())
		os.Exit(1)
	}

	if len(config_path) == 0 {
		for _, path := range config_search {
			config_path = filepath.Clean(string(path))
			if strings.HasPrefix(config_path, "~/") {
				parts := strings.SplitN(config_path, "/", 2)
				parts[0] = os.Getenv("HOME")
				if len(parts[0]) == 0 {
					user, err := user.Current()
					if err != nil {
						parts = nil
					} else {
						parts[0] = user.HomeDir
					}
				}
				if parts != nil {
					config_path = strings.Join(parts, "/")
				} else {
					config_path = ""
				}
			}
			_, err := os.Stat(config_path)
			if err == nil {
				break
			}
			config_path = ""
		}
		if len(config_path) == 0 {
			fmt.Fprintf(os.Stderr, "Failed to find any suitable configuration file")
			os.Exit(1)
		}
	}

	// fmt.Fprintf(os.Stderr, "Using config path: %#v", config_path)

	config, err := yaml.ReadFile(config_path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to process configuration file (%q): %#v", config_path, err)
		os.Exit(1)
	}


	defer func() {
		if err := recover(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to process configuration file (%q): %v", config_path, err)
			os.Exit(1)
		}
	}()

	log := logging.Get("codegen.main")

	conf_logging, err := yaml.Child(config.Root, ".logging")
	if err != nil {
		logging.DefaultSetup()
		log.Warnf("Failed to setup logging: %#v", err)
	} else {
		func() {
			conf_logging_map, ok := conf_logging.(yaml.Map)
			if !ok {
				logging.DefaultSetup()
				log.Error("'logging' config section is not a map, ignoring")
				return
			}
			err = log_setup.SetupYAML(&conf_logging_map)
			if err != nil {
				logging.DefaultSetup()
				log.Errorf("Failed to configure logging: %v", err)
			}
		}()

	}
	log.Debug("Done!")
}

	// for key, section := range conf_logging
	// 	fmt.Println(key, section)

	// func walk_iter(path string, info os.FileInfo, err error) error
	// filepath.Walk(root string, walkFn WalkFunc) error
