package main

import (
	"fmt"
	"strings"
	"flag"
	"os"
	"os/user"
	"os/exec"
	"path/filepath"
	"bytes"
	"encoding/gob"
	"text/template"
	re "regexp"
	"github.com/vaughan0/go-logging"
	"github.com/kylelemons/go-gypsy/yaml"
	"codetag/log_setup"
	tgrs "codetag/taggers"
)


// Path with a few extra convenience methods.
type path_t string

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

// CLI
// Config file that is used.
var config_path string
// Don't actually run tmsu
var dry_run bool


// Clone context object using gob serialization
func ctx_clone(src, dst interface{}) {
	var err error
	buff := new(bytes.Buffer)
	enc := gob.NewEncoder(buff)
	dec := gob.NewDecoder(buff)
	err = enc.Encode(src)
	if err != nil {
		panic(err)
	}
	err = dec.Decode(dst)
	if err != nil {
		panic(err)
	}
}


// Parsed filters.
type path_filter struct {
	verdict bool
	pattern *re.Regexp
}
type path_filters []path_filter

// Match path against a list of regexp-filters and return whether it
//  should be processed or not.
func (filters *path_filters) match(path string) bool {
	for _, filter := range *filters {
		if filter.pattern.Match([]byte(path)) {
			return filter.verdict
		}
	}
	return true
}


// Writer which line-buffers data, passing each line to log_func
type log_pipe struct {
	log_func func(string)
	buff string ""
}

func (pipe *log_pipe) Log(line string) {
	pipe.log_func(strings.TrimSpace(line))
}

func (pipe *log_pipe) Write(p []byte) (n int, err error) {
	pipe.buff += string(p)
	for strings.Contains(pipe.buff, "\n") {
		lines := strings.SplitN(pipe.buff, "\n", 2)
		pipe.Log(lines[0])
		pipe.buff = lines[1]
	}
	return len(p), nil
}

func (pipe *log_pipe) Flush() {
	if len(pipe.buff) > 0 {
		pipe.Log(pipe.buff)
	}
	pipe.buff = ""
}


func init() {
	gob.Register(tgrs.CtxTagset{})
}


func main() {
	config_search[0] = path_t(os.Args[0] + ".yaml")

	flag.Usage = func() {
		tpl := template.Must(template.New("test").Parse(""+
			`usage: {{.cmd}} [ <options> ]

Index code files, using parameters specified in the config file.
If not specified exmplicitly, config file is searched within the
following paths (in that order):
{{range .paths}}  - {{.}}
{{end}}
Examples:
  % {{.cmd}}
  % {{.cmd}} --config config.yaml

Options:
`))
		tpl.Execute(os.Stdout, map[string]interface{}{"cmd": os.Args[0], "paths": config_search})
		flag.PrintDefaults()
	}

	flag.StringVar(&config_path, "config", "", "Configuration file to use.")
	flag.BoolVar(&dry_run, "dry-run", false, "Don't actually run tmsu, just process all paths.")
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
	log = logging.Get("codetag")

	// Common processing vars
	var (
		ok bool
		node yaml.Node
		config_map yaml.Map
		config_list yaml.List
	)

	node, err = yaml.Child(config.Root, ".logging")
	if err != nil || node == nil {
		logging.DefaultSetup()
		log.Debugf("No logging config defined (err: %#v), using defaults", err)
	} else {
		config_map, ok = node.(yaml.Map)
		if !ok {
			logging.DefaultSetup()
			log.Error("'logging' config section is not a map, ignoring")
		} else {
			err = log_setup.SetupYAML(config_map)
			if err != nil {
				logging.DefaultSetup()
				log.Errorf("Failed to configure logging: %v", err)
			}
		}
	}

	log_init = true

	// Configure filtering
	filters := path_filters{}
	node, err = yaml.Child(config.Root, ".filter")
	if err != nil || node == nil {
		log.Debug("No path-filters configured")
	} else {
		config_list, ok = node.(yaml.List)
		if !ok {
			log.Fatal("'filters' must be a list of string patterns")
			os.Exit(1)
		}
		for _, node := range config_list {
			pattern, ok := node.(yaml.Scalar)
			if !ok {
				log.Errorf("Pattern must be a string: %v", node)
				continue
			}
			filter, pattern_str := path_filter{}, strings.Trim(string(pattern), "'")
			filter.verdict = strings.HasPrefix(pattern_str, "+")
			if !filter.verdict && !strings.HasPrefix(pattern_str, "-") {
				log.Errorf("Pattern must start with either '+' or '-': %v", pattern_str)
				continue
			}
			pattern_str = pattern_str[1:]
			filter.pattern, err = re.Compile(pattern_str)
			if err != nil {
				log.Errorf("Failed to compile pattern (%v) as regexp: %v", pattern_str, err)
				continue
			}
			filters = append(filters, filter)
		}
	}

	// Get the list of paths to process
	config_map, ok = config.Root.(yaml.Map)
	if !ok {
		log.Fatal("Config must be a map and have 'paths' key")
		os.Exit(1)
	}

	node, ok = config_map["paths"]
	if !ok {
		log.Fatal("'paths' list must be defined in config")
		os.Exit(1)
	}

	var paths []string
	config_list, ok = node.(yaml.List)
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

	taggers := make(map[string][]tgrs.Tagger)

	init_tagger := func(ns, name string, config *yaml.Node) {
		tagger, err := tgrs.Get(name, config, log)
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

	config_init = true

	// Walk the paths
	var (
		context = make(map[string]map[string]map[string]interface{})
		ctx map[string]map[string]interface{}
		ctx_tags tgrs.CtxTagset
	)

	log_tmsu := logging.Get("codetag.tmsu")
	pipe := log_pipe{}
	pipe.log_func = func(line string) {
		log_tmsu.Debug(line)
	}
	tmsu_log_pipe := &pipe

	for _, root := range paths {
		log.Tracef("Processing path: %s", root)

		walk_iter := func (path string, info os.FileInfo, err error) (ret_err error) {
			if err != nil {
				log.Debugf(" - path: %v (info: %v), error: %v", path, info, err)
				return
			}

			if !strings.HasPrefix(path, root) {
				panic(fmt.Errorf("filepath.Walk went outside of root path (%v): %v", root, path))
			}
			path_match := path[len(root):]
			if info.IsDir() {
				path_match += "/"
			}
			if !filters.match(path_match) {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return
			}

			// Get context for this path or copy it from parent path
			ctx, ok = context[path]
			if !ok {
				ctx_parent, ok := context[filepath.Dir(path)]
				if ok {
					ctx = nil
					ctx_clone(ctx_parent, &ctx)
				} else {
					ctx = make(map[string]map[string]interface{}, len(taggers))
				}
				context[path] = ctx
			}

			// Run all taggers
			for ns, tagger_list := range taggers {
				ctx_ns, ok := ctx[ns]
				if !ok {
					ctx[ns] = make(map[string]interface{}, len(taggers) + 1)
					ctx_ns = ctx[ns]
				}
				for _, tagger := range tagger_list {
					tags := tagger(path, info, &ctx_ns)
					if tags == nil {
						continue
					}
					// Push new tags to the context
					ctx_tags_if, ok := ctx_ns["tags"]
					if !ok {
						ctx_tags = make(tgrs.CtxTagset, len(taggers))
					} else {
						ctx_tags = ctx_tags_if.(tgrs.CtxTagset)
					}
					for _, tag := range tags {
						_, ok = ctx_tags[tag]
						if !ok {
							ctx_tags[tag] = true
						}
					}
					ctx_ns["tags"] = ctx_tags
				}
			}

			// Attach tags only to files
			if info.Mode() & os.ModeType != 0 {
				return
			}

			file_tags := []string{}
			for ns, ctx_ns := range ctx {
				ctx_tags_if, ok := ctx_ns["tags"]
				if !ok {
					continue
				}
				ctx_tags = ctx_tags_if.(tgrs.CtxTagset)
				for tag, _ := range ctx_tags {
					file_tags = append(file_tags, ns + ":" + tag)
				}
			}

			log.Tracef(" - file: %v, tags: %v", path, file_tags)
			if !dry_run {
				cmd := exec.Command("tmsu", "tag", path)
				cmd.Args = append(cmd.Args, file_tags...)
				cmd.Stdout, cmd.Stderr = tmsu_log_pipe, tmsu_log_pipe
				err = cmd.Run()
				if err != nil {
					log.Fatalf("Failure running tmsu (file: %v, tags: %v): %v", path, file_tags, err)
				}
				tmsu_log_pipe.Flush()
			}

			return
		}

		path_ext, err := path_t(root).ExpandUser()
		if err == nil {
			root = string(path_ext)
		}

		err = filepath.Walk(root, walk_iter)
		if err != nil {
			log.Errorf("Failed to process path: %s", root)
		}
	}

	log.Debug("Finished")
}
