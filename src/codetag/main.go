package main

import (
	"fmt"
	"strings"
	"flag"
	"os"
	"os/user"
	"path/filepath"
	"github.com/hoisie/mustache"
	"github.com/mk-fg/go-logging"
	"github.com/kylelemons/go-gypsy/yaml"
	"codetag/log_setup"
)


// Path with a few extra convenience methods.
type path_t string

// Render path in human-readable form (for moustache templates).
func (path path_t) Render() string {
	return fmt.Sprint(path)
}

// Expand paths like "~/path" using HOME env var or /etc/passwd.
func (path path_t) ExpandUser() (path_ret path_t, err error) {
	path_str := filepath.Clean(string(path))
	if !strings.HasPrefix(path_str, "~/") {
		return path, nil
	}
	parts := strings.SplitN(path_str, "/", 2)
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
		return path_t(strings.Join(parts, "/")), nil
	}
	return path, err
}


// Default places to look for config file.
// First one is dynamic, depending on argv[0].
var config_search = []path_t{"", "~/.codetag.yaml", "/etc/codetag.yaml"}

// Config file that is used.
var config_path string


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
		fmt.Fprintf(os.Stderr, "Error: no command-line"+
			" arguments are allowed (provided: %v)\n", flag.Args())
		os.Exit(1)
	}

	// Find config path to use
	if len(config_path) == 0 {
		for _, path := range config_search {
			path, err := path.ExpandUser()
			if err != nil {
				continue
			}
			config_path = string(path)
			_, err = os.Stat(config_path)
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

	var (
		log *logging.Logger
		log_init = false
	)

	defer func() {
		if err := recover(); err != nil {
			if log_init {
				log.Fatalf("Failed to process configuration file (%q): %v", config_path, err)
			} else {
				fmt.Fprintf(os.Stderr, "Failed to process configuration file (%q): %v\n", config_path, err)
			}
			os.Exit(1)
		}
	}()

	// Read the config as yaml
	config, err := yaml.ReadFile(config_path)
	if err != nil {
		panic(err)
		os.Exit(1)
	}

	// Configure logging
	log = logging.Get("codegen.main")

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
	log_init = true

	config_map := config.Root.(yaml.Map)

	// Get the list of paths to process
	node, ok := config_map["paths"]
	if !ok {
		log.Fatal("'paths' list must be defined in config")
		os.Exit(1)
	}

	var paths []string
	config_list, ok := node.(yaml.List)
	if !ok {
		path, ok := node.(yaml.Scalar)
		if !ok {
			log.Fatal("'paths' must be a list or (worst-case) scalar")
			os.Exit(1)
		}
		paths = append(paths, string(path))
	} else {
		for _, node := range config_list {
			path, ok := node.(yaml.Scalar)
			if !ok {
				log.Warnf("Skipped invalid path specification: %v", node)
			} else {
				paths = append(paths, string(path))
			}
		}
	}

	// Init taggers
	node, ok = config_map["taggers"]
	if ok {
		config_map, ok = node.(yaml.Map)
	}
	if !ok {
		log.Warn("No 'taggers' defined, nothing to do")
		os.Exit(0)
	}

	taggers := make(map[string][]string)

	for ns, node := range config_map {
		config_list, ok := node.(yaml.List)
		if !ok {
			// It's also ok to have "ns: tagger" spec, if there's just one for ns
			tagger, ok := node.(yaml.Scalar)
			if !ok {
				log.Warnf("Invalid tagger(-list) specification (ns: %v): %v", ns, node)
				continue
			}
			taggers[ns] = append(taggers[ns], string(tagger))
			continue
		}

		for _, node = range config_list {
			tagger_map, ok := node.(yaml.Map)
			if !ok {
				tagger, ok := node.(yaml.Scalar)
				if !ok {
					log.Warnf("Invalid tagger specification - "+
						"must be map or string (ns: %v): %v", ns, node)
					continue
				}
				taggers[ns] = append(taggers[ns], string(tagger))
				continue
			}
			if len(tagger_map) != 1 {
				log.Warnf("Invalid tagger specification - "+
					"map must contain only one element (ns: %v): %v", ns, tagger_map)
				continue
			}
			// TODO: use config value here
			for tagger, _ := range tagger_map {
				taggers[ns] = append(taggers[ns], tagger)
				continue
			}
		}
	}

	log.Debugf("Using taggers: %v", taggers)

	// Walk the paths
	for _, path := range paths {
		log.Tracef("Processing path: %s", path)

		walk_iter := func (path string, info os.FileInfo, err error) error {
			if err != nil {
				log.Tracef(" - path: %v (info: %v), error: %v", path, info, err)
			}
			if info != nil && info.IsDir() {
				log.Tracef(" - dir: %v", path)
			}
			return nil
		}

		path_ext, err := path_t(path).ExpandUser()
		if err == nil {
			path = string(path_ext)
		}

		err = filepath.Walk(path, walk_iter)
		if err != nil {
			log.Errorf("Failed to process path: %s", path)
		}
	}

	log.Debug("Finished")
}
