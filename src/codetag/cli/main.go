package main

import (
	"fmt"
	"strings"
	"flag"
	"os"
	"os/user"
	"path/filepath"
	"bytes"
	"encoding/gob"
	"sort"
	"github.com/hoisie/mustache"
	"github.com/mk-fg/go-logging"
	"github.com/kylelemons/go-gypsy/yaml"
	"codetag/log_setup"
	tgrs "codetag/taggers"
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


// Clone context object using gob serialization
func ctx_clone(src, dst interface{}) {
	buff := new(bytes.Buffer)
	enc := gob.NewEncoder(buff)
	dec := gob.NewDecoder(buff)
	enc.Encode(src)
	dec.Decode(dst)
}


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
		config_init = false
	)

	defer func() {
		if config_init {
			return
		}
		// Recover only for configuration issues.
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

	taggers := make(map[string][](func(path string, info os.FileInfo, ctx *map[string]interface{}) []string))

	init_tagger := func(ns, name string, config *yaml.Node) {
		tagger, err := tgrs.Get(name, config)
		if err != nil {
			log.Warnf("Failed to init tagger %v (ns: %v): %v", ns, name, err)
		} else {
			taggers[ns] = append(taggers[ns], tagger)
		}
	}

	for ns, node := range config_map {
		if ns == "_none" {
			ns = ""
		}
		if strings.HasPrefix(ns, "_") {
			log.Warnf("Ignoring namespace name, starting with underscore: %v", ns)
			continue
		}

		config_list, ok := node.(yaml.List)
		if !ok {
			// It's also ok to have "ns: tagger" spec, if there's just one for ns
			tagger, ok := node.(yaml.Scalar)
			if !ok {
				log.Warnf("Invalid tagger(-list) specification (ns: %v): %v", ns, node)
				continue
			}
			init_tagger(ns, string(tagger), nil)
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
				init_tagger(ns, string(tagger), nil)
				continue
			}
			if len(tagger_map) != 1 {
				log.Warnf("Invalid tagger specification - "+
					"map must contain only one element (ns: %v): %v", ns, tagger_map)
				continue
			}
			for tagger, node := range tagger_map {
				init_tagger(ns, tagger, &node)
				continue
			}
		}
	}

	log.Debugf("Using taggers: %v", taggers)

	config_init = true

	// Walk the paths
	var (
		context = make(map[string]map[string]interface{})
		ctx map[string]interface{}
		ctx_tags sort.StringSlice
	)

	for _, path := range paths {
		log.Tracef("Processing path: %s", path)

		walk_iter := func (path string, info os.FileInfo, err error) error {
			if err != nil {
				log.Tracef(" - path: %v (info: %v), error: %v", path, info, err)
				return nil
			}

			// Get context for this path or copy it from parent path
			ctx, ok = context[path]
			if !ok {
				ctx = make(map[string]interface{})
				ctx_parent, ok := context[filepath.Dir(path)]
				if ok {
					ctx_clone(ctx_parent, ctx)
				}
				context[path] = ctx
			}

			// Run all taggers
			for _, tagger_list := range taggers {
				// Maybe split context by ns here?
				for _, tagger := range tagger_list {
					tags := tagger(path, info, &ctx)
					if tags == nil {
						continue
					}
					// Push new tags to the context
					ctx_tags_if, ok := ctx["tags"]
					if !ok {
						ctx_tags = sort.StringSlice{}
					} else {
						ctx_tags = ctx_tags_if.(sort.StringSlice)
					}
					for _, tag := range tags {
						if ctx_tags.Search(tag) < 0 {
							ctx_tags = append(ctx_tags, tag)
						}
					}
					ctx_tags.Sort()
					ctx["tags"] = ctx_tags
				}
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
